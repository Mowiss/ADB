// Aldebaran — serveur local de partie.
// Sert l'appli aux joueurs du même réseau et relaie les messages entre le MJ et les joueurs
// (même principe que PeerJS, mais sans Internet : tout passe par ce petit programme).
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"rsc.io/qr"
)

const (
	Port    = 4777
	Version = "1.0"
	AppName = "aldebaran-local"
)

// Adresses de l'appli en ligne (la première qui répond sert à mettre à jour la copie locale)
var onlineBases = []string{"https://mowiss.github.io/ADB/", "https://mowiss.github.io/adb/"}

//go:embed site
var siteFS embed.FS

//go:embed icon.ico
var iconICO []byte

//go:embed status.html
var statusHTML []byte

var cacheDir string

/* ---------------- relais (WebSocket) ---------------- */

type client struct {
	id   string
	ws   *websocket.Conn
	out  chan []byte
	once sync.Once
}

func (c *client) close() { c.once.Do(func() { close(c.out) }) }

type hub struct {
	mu sync.Mutex
	m  map[string]*client
}

var H = &hub{m: map[string]*client{}}

func (h *hub) players() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for id := range h.m {
		if !strings.HasPrefix(id, "aldebaran-") {
			n++
		}
	}
	return n
}

// code de la table ouverte par le MJ (vide si aucune)
func (h *hub) mjCode() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.m {
		if strings.HasPrefix(id, "aldebaran-") {
			return strings.ToUpper(strings.TrimPrefix(id, "aldebaran-"))
		}
	}
	return ""
}

func (h *hub) sendTo(c *client, b []byte) {
	defer func() { recover() }() // canal déjà fermé
	select {
	case c.out <- b:
	default: // client saturé : on le coupe, il se reconnectera
		c.ws.Close()
	}
}

func (h *hub) get(id string) *client {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.m[id]
}

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1 << 16, WriteBufferSize: 1 << 16,
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" || len(id) > 80 {
		http.Error(w, "id", 400)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(64 << 20)
	c := &client{id: id, ws: ws, out: make(chan []byte, 512)}

	H.mu.Lock()
	old := H.m[id]
	H.m[id] = c
	H.mu.Unlock()
	if old != nil { // même identifiant : le plus récent gagne, l'ancien est prévenu
		H.sendTo(old, []byte(`{"type":"taken"}`))
		go func() { time.Sleep(300 * time.Millisecond); old.ws.Close() }()
	}
	logf("connexion %s (%s)", id, r.RemoteAddr)
	if a := r.URL.Query().Get("app"); a != "" {
		appSeen(a)
	}

	// écriture
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case b, ok := <-c.out:
				if !ok {
					return
				}
				ws.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if ws.WriteMessage(websocket.TextMessage, b) != nil {
					ws.Close()
					return
				}
			case <-t.C:
				ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if ws.WriteMessage(websocket.PingMessage, nil) != nil {
					ws.Close()
					return
				}
			}
		}
	}()
	H.sendTo(c, []byte(`{"type":"open","id":`+jsonStr(id)+`}`))

	ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		_, b, err := ws.ReadMessage()
		if err != nil {
			break
		}
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		route(c, b)
	}
	ws.Close()
	c.close()
	H.mu.Lock()
	gone := H.m[id] == c
	if gone {
		delete(H.m, id)
	}
	others := make([]*client, 0, len(H.m))
	for _, x := range H.m {
		others = append(others, x)
	}
	H.mu.Unlock()
	if gone {
		msg := []byte(`{"type":"gone","id":` + jsonStr(id) + `}`)
		for _, x := range others {
			H.sendTo(x, msg)
		}
		logf("déconnexion %s", id)
	}
}

func route(c *client, b []byte) {
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return
	}
	var typ, to string
	json.Unmarshal(m["type"], &typ)
	json.Unmarshal(m["to"], &to)
	switch typ {
	case "connect", "accept", "data", "close":
	default:
		return
	}
	dst := H.get(to)
	if dst == nil {
		if typ == "connect" {
			H.sendTo(c, []byte(`{"type":"nopeer","cid":`+string(orNull(m["cid"]))+`,"to":`+jsonStr(to)+`}`))
		}
		return
	}
	m["from"] = json.RawMessage(jsonStr(c.id))
	delete(m, "to")
	out, _ := json.Marshal(m)
	H.sendTo(dst, out)
}

func orNull(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
}
func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

/* ---------------- adresses du PC ---------------- */

func privateIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 || (ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) || (ip4[0] == 192 && ip4[1] == 168)
}

var skipIf = []string{"virtualbox", "vmware", "vethernet", "hyper-v", "wsl", "docker", "loopback", "bluetooth", "vbox", "virtual"}

func lanIPs() []string {
	primary := ""
	if c, err := net.Dial("udp", "192.168.1.1:9"); err == nil { // aucun paquet n'est envoyé : on lit juste l'interface choisie
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && privateIP(a.IP) {
			primary = a.IP.String()
		}
		c.Close()
	}
	if c, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && privateIP(a.IP) {
			primary = a.IP.String()
		}
		c.Close()
	}
	var list []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := strings.ToLower(i.Name)
		bad := false
		for _, s := range skipIf {
			if strings.Contains(n, s) {
				bad = true
			}
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && privateIP(ipn.IP) {
				s := ipn.IP.String()
				if s == primary || bad {
					continue
				}
				list = append(list, s)
			}
		}
	}
	sort.SliceStable(list, func(a, b int) bool { // 192.168 d'abord (box), puis 10.x, puis 172.x
		rank := func(s string) int {
			switch {
			case strings.HasPrefix(s, "192.168."):
				return 0
			case strings.HasPrefix(s, "10."):
				return 1
			}
			return 2
		}
		return rank(list[a]) < rank(list[b])
	})
	if primary != "" {
		list = append([]string{primary}, list...)
	}
	return list
}

func playerURL(ip string) string { return fmt.Sprintf("http://%s:%d/", ip, Port) }

/* ---------------- fichiers de l'appli ---------------- */

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Private-Network", "true")
	h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "*")
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == http.MethodOptions {
		return
	}
	ips := lanIPs()
	ip := ""
	if len(ips) > 0 {
		ip = ips[0]
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"app": AppName, "v": Version, "port": Port, "ip": ip, "ips": ips, "players": H.players(), "mj": H.mjCode(), "mjurl": mjLink(), "visits": recentVisits()})
}

func qrHandler(w http.ResponseWriter, r *http.Request) {
	cors(w)
	t := r.URL.Query().Get("t")
	if t == "" || len(t) > 600 {
		http.Error(w, "t", 400)
		return
	}
	code, err := qr.Encode(t, qr.M)
	if err != nil {
		http.Error(w, "qr", 500)
		return
	}
	n, q := code.Size, 4
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#2f3340" d="`, n+2*q, n+2*q)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&sb, "M%d %dh1v1h-1z", x+q, y+q)
			}
		}
	}
	sb.WriteString(`"/></svg>`)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	io.WriteString(w, sb.String())
}

var dl = &http.Client{Timeout: 8 * time.Second}

// liveBase : l'adresse en ligne qui a répondu (pour récupérer les fichiers manquants)
var liveBase string
var liveMu sync.Mutex

// à chaque lancement : copie à jour de l'appli si Internet est là, sinon la dernière copie, sinon celle intégrée
func refreshApp() {
	bases := onlineBases
	if b := savedBase(); b != "" {
		bases = append([]string{b}, bases...)
	}
	for _, b := range bases {
		r, err := dl.Get(b + "index.html")
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 40<<20))
		r.Body.Close()
		if r.StatusCode == 200 && strings.Contains(string(body), "Aldebaran") {
			os.MkdirAll(cacheDir, 0o755)
			os.WriteFile(filepath.Join(cacheDir, "index.html"), body, 0o644)
			setBase(b)
			logf("appli mise à jour depuis %s", b)
			return
		}
	}
	logf("hors ligne : appli locale utilisée")
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	p := path.Clean("/" + r.URL.Path)
	if p == "/" {
		p = "/index.html"
	}
	rel := strings.TrimPrefix(p, "/")
	if strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	if rel == "sw.js" { // pas de service worker en local (http) : on évite un cache qui mélangerait les versions
		http.NotFound(w, r)
		return
	}
	ct := mime.TypeByExtension(path.Ext(rel))
	if ct == "" && strings.HasSuffix(rel, ".webmanifest") {
		ct = "application/manifest+json"
	}
	serve := func(b []byte) {
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if rel == "index.html" {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "max-age=3600")
		}
		w.Write(b)
	}
	if b, err := os.ReadFile(filepath.Join(cacheDir, filepath.FromSlash(rel))); err == nil {
		serve(b)
		return
	}
	if b, err := fs.ReadFile(siteFS, "site/"+rel); err == nil {
		serve(b)
		return
	}
	liveMu.Lock()
	base := liveBase
	liveMu.Unlock()
	if base != "" { // fichier ajouté depuis (image de mob…) : on le récupère une fois
		if resp, err := dl.Get(base + rel); err == nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 60<<20))
			resp.Body.Close()
			if resp.StatusCode == 200 {
				dst := filepath.Join(cacheDir, filepath.FromSlash(rel))
				os.MkdirAll(filepath.Dir(dst), 0o755)
				os.WriteFile(dst, b, 0o644)
				serve(b)
				return
			}
		}
	}
	http.NotFound(w, r)
}

/* ---------------- journal ---------------- */

var logger *log.Logger

func logf(f string, a ...any) {
	if logger != nil {
		logger.Printf(f, a...)
	}
}

func setupLog() {
	os.MkdirAll(cacheDir, 0o755)
	lp := filepath.Join(filepath.Dir(cacheDir), "serveur.log")
	if st, err := os.Stat(lp); err == nil && st.Size() > 2<<20 {
		os.Remove(lp)
	}
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		logger = log.New(f, "", log.LstdFlags)
	} else {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
}

/* ---------------- démarrage ---------------- */

// déjà lancé ? (le port répond avec notre signature)
func alreadyRunning() bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/info", Port))
	if err != nil {
		return false
	}
	defer r.Body.Close()
	var j map[string]any
	json.NewDecoder(r.Body).Decode(&j)
	return j["app"] == AppName
}

/* visites venues d'autres appareils (diagnostic : si rien n'arrive, c'est le réseau ou le pare-feu qui bloque) */
type visit struct {
	T    string `json:"t"`
	IP   string `json:"ip"`
	Path string `json:"path"`
}

var visits []visit
var visitMu sync.Mutex

func visitLog(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && r.URL.Path != "/info" && r.URL.Path != "/qr.svg" {
			visitMu.Lock()
			visits = append(visits, visit{time.Now().Format("15:04:05"), host, r.URL.Path})
			if len(visits) > 8 {
				visits = visits[len(visits)-8:]
			}
			visitMu.Unlock()
			logf("visite %s %s %s", host, r.URL.Path, r.UserAgent())
		}
		h.ServeHTTP(w, r)
	})
}

func startServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler)
	mux.HandleFunc("/info", infoHandler)
	mux.HandleFunc("/qr.svg", qrHandler)
	mux.HandleFunc("/serveur", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(statusHTML)
	})
	mux.HandleFunc("/firewall", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if r.Method != http.MethodPost || !net.ParseIP(host).IsLoopback() {
			http.Error(w, "non", 403)
			return
		}
		if err := fixFirewall(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if r.Method != http.MethodPost || !net.ParseIP(host).IsLoopback() {
			http.Error(w, "non", 403)
			return
		}
		io.WriteString(w, "ok")
		logf("arrêt demandé depuis la page d'état")
		go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
	})
	mux.HandleFunc("/", fileHandler)
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", Port))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: visitLog(mux), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	return nil
}

func savedBase() string {
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(cacheDir), "adresse.txt"))
	s := strings.TrimSpace(string(b))
	if strings.HasPrefix(s, "https://") {
		return s
	}
	return ""
}
func setBase(b string) {
	liveMu.Lock()
	liveBase = b
	liveMu.Unlock()
	if b != savedBase() {
		os.WriteFile(filepath.Join(filepath.Dir(cacheDir), "adresse.txt"), []byte(b), 0o644)
	}
}

// l'appli du MJ se présente (son adresse en ligne) : on la retient et on met la copie locale à jour
func appSeen(a string) {
	u, err := url.Parse(a)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Host, "github.io") {
		return
	}
	b := u.Scheme + "://" + u.Host + path.Dir(u.Path+"x") + "/"
	b = strings.Replace(b, "//"+u.Host+"//", "//"+u.Host+"/", 1)
	liveMu.Lock()
	same := liveBase == b
	liveMu.Unlock()
	if !same {
		setBase(b)
		go refreshApp()
	}
}

func recentVisits() []visit {
	visitMu.Lock()
	defer visitMu.Unlock()
	return append([]visit{}, visits...)
}

func statusLink() string { return fmt.Sprintf("http://localhost:%d/serveur", Port) }

func mjLink() string {
	liveMu.Lock()
	b := liveBase
	liveMu.Unlock()
	if b == "" {
		b = savedBase()
	}
	if b == "" {
		b = onlineBases[0]
	}
	return b + "?mode=mj"
}

func main() {
	cacheDir = dataDir()
	setupLog()
	if alreadyRunning() {
		openBrowser(statusLink())
		return
	}
	if maybeInstall() {
		return
	}
	if err := startServer(); err != nil {
		fatal(fmt.Sprintf("Le serveur local n'a pas pu démarrer : le port %d est déjà utilisé par un autre programme.\n\n%v", Port, err))
		return
	}
	logf("serveur local démarré sur le port %d", Port)
	done := make(chan struct{})
	go func() { refreshApp(); close(done) }()
	keepAwake()
	select { // on attend la mise à jour (quelques secondes au plus) pour ouvrir la bonne adresse
	case <-done:
	case <-time.After(6 * time.Second):
	}
	runUI()
}
