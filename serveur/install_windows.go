//go:build windows

package main

import (
	"io"
	"net/http"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func installDir() string { return filepath.Dir(dataDir()) } // %LOCALAPPDATA%\Aldebaran

// Premier lancement depuis les Téléchargements : on propose d'installer (copie + raccourcis).
// Renvoie true si le programme doit s'arrêter (la copie installée a pris le relais).
func maybeInstall() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	self, _ = filepath.EvalSymlinks(self)
	dst := filepath.Join(installDir(), "Aldebaran-Serveur.exe")
	if strings.EqualFold(filepath.Clean(self), filepath.Clean(dst)) {
		return false
	}
	_, had := os.Stat(dst)
	q := "Installer le serveur local Aldebaran sur ce PC ?\n\nUn raccourci « Aldebaran Serveur » sera ajouté sur le bureau et dans le menu Démarrer. Vous pourrez ensuite supprimer ce fichier téléchargé."
	if had == nil {
		q = "Mettre à jour le serveur local Aldebaran installé sur ce PC avec cette version ?"
	}
	if msgBox(q, "Aldebaran — serveur local", mbYesNo|mbIconInfo) != idYes {
		return false
	}
	// une ancienne version tourne encore : on l'arrête, sinon son .exe est verrouillé
	if had == nil {
		if r, e := http.Post("http://127.0.0.1:4777/stop", "text/plain", nil); e == nil {
			r.Body.Close()
			time.Sleep(1200 * time.Millisecond)
		}
	}
	var cerr error
	for i := 0; i < 6; i++ {
		if cerr = copyFile(self, dst); cerr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if cerr != nil {
		fatal("L'installation a échoué : " + cerr.Error() + "\n\nFermez le serveur (icône près de l'horloge › Arrêter le serveur) puis relancez ce fichier.")
		return false
	}
	shortcuts(dst)
	refreshIcons()
	fixFirewall() // une fenêtre Windows demande l'autorisation : sans elle, les téléphones ne voient pas le serveur
	cmd := exec.Command(dst)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if cmd.Start() != nil {
		return false
	}
	return true
}

func copyFile(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0o755)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

func shortcuts(target string) {
	defer func() { recover() }()
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	defer ole.CoUninitialize()
	obj, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		logf("raccourcis : %v", err)
		return
	}
	defer obj.Release()
	sh, err := obj.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return
	}
	defer sh.Release()
	sf, err := oleutil.GetProperty(sh, "SpecialFolders")
	if err != nil {
		return
	}
	folders := sf.ToIDispatch()
	defer folders.Release()
	for _, where := range []string{"Desktop", "Programs"} {
		v, err := oleutil.CallMethod(folders, "Item", where)
		if err != nil || v.ToString() == "" {
			continue
		}
		lnk := filepath.Join(v.ToString(), "Aldebaran Serveur.lnk")
		cs, err := oleutil.CallMethod(sh, "CreateShortcut", lnk)
		if err != nil {
			continue
		}
		d := cs.ToIDispatch()
		oleutil.PutProperty(d, "TargetPath", target)
		oleutil.PutProperty(d, "WorkingDirectory", filepath.Dir(target))
		oleutil.PutProperty(d, "IconLocation", target+",0")
		oleutil.PutProperty(d, "Description", "Aldebaran — serveur de partie locale")
		oleutil.CallMethod(d, "Save")
		d.Release()
		logf("raccourci créé : %s", lnk)
	}
}

// Windows garde les icônes en cache : on lui signale le changement (raccourcis, barre des tâches)
func refreshIcons() {
	syscall.NewLazyDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
}
