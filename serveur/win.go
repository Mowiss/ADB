//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/systray"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	user32                      = syscall.NewLazyDLL("user32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procMessageBoxW             = user32.NewProc("MessageBoxW")
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
	mbYesNo          = 0x00000004
	mbIconWarning    = 0x00000030
	mbIconError      = 0x00000010
	mbIconInfo       = 0x00000040
	mbTopmost        = 0x00040000
	idYes            = 6
)

func dataDir() string {
	b := os.Getenv("LOCALAPPDATA")
	if b == "" {
		b, _ = os.UserCacheDir()
	}
	return filepath.Join(b, "Aldebaran", "appli")
}

func msgBox(text, title string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags|mbTopmost)
	return int(r)
}

func fatal(text string) { logf("%s", text); msgBox(text, "Aldebaran — serveur local", mbIconError) }

func openBrowser(u string) {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Start()
}

// le PC ne se met pas en veille tant que le serveur tourne (l'écran, lui, peut s'éteindre)
func keepAwake() {
	go func() {
		runtime.LockOSThread()
		for {
			procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
			time.Sleep(time.Minute)
		}
	}()
}

func runUI() { systray.Run(onReady, func() {}) }

func onReady() {
	systray.SetIcon(iconICO)
	systray.SetTitle("Aldebaran")
	systray.SetTooltip("Aldebaran — serveur local")

	head := systray.AddMenuItem("Aldebaran — serveur local", "")
	head.Disable()
	addr := systray.AddMenuItem("Adresse : recherche…", "Adresse à donner aux joueurs (même wifi)")
	addr.Disable()
	who := systray.AddMenuItem("Aucun joueur connecté", "")
	who.Disable()
	systray.AddSeparator()
	open := systray.AddMenuItem("Ouvrir la table du MJ", "Ouvre l'appli Aldebaran dans le navigateur")
	lg := systray.AddMenuItem("Journal du serveur", "Ouvre le fichier journal")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Arrêter le serveur", "Ferme la partie locale")

	upd := func() {
		ips := lanIPs()
		if len(ips) > 0 {
			addr.SetTitle("Adresse : " + playerURL(ips[0]))
		} else {
			addr.SetTitle("Adresse : aucun réseau détecté")
		}
		n := H.players()
		t := fmt.Sprintf("%d joueurs connectés", n)
		switch n {
		case 0:
			t = "Aucun joueur connecté"
		case 1:
			t = "1 joueur connecté"
		}
		who.SetTitle(t)
		systray.SetTooltip("Aldebaran — serveur local · " + t)
	}
	upd()
	go func() {
		for range time.Tick(2 * time.Second) {
			upd()
		}
	}()

	openBrowser(mjLink())

	go func() {
		for {
			select {
			case <-open.ClickedCh:
				openBrowser(mjLink())
			case <-lg.ClickedCh:
				openBrowser(filepath.Join(filepath.Dir(cacheDir), "serveur.log"))
			case <-quit.ClickedCh:
				n := H.players()
				if n > 0 {
					s := "s"
					if n == 1 {
						s = ""
					}
					if msgBox(fmt.Sprintf("%d joueur%s connecté%s. Arrêter quand même le serveur ?\n\nLa partie locale s'arrêtera pour tout le monde.", n, s, s), "Aldebaran — serveur local", mbYesNo|mbIconWarning) != idYes {
						continue
					}
				}
				systray.Quit()
				os.Exit(0)
			}
		}
	}()
}
