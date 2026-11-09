//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var procShellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

// Ouvre le pare-feu Windows au serveur (réseaux privés ET publics : Windows classe souvent
// le wifi de la maison en « public »). Supprime aussi les règles de blocage créées quand on a
// fermé la fenêtre du pare-feu sans autoriser. Demande l'autorisation administrateur (UAC).
func fixFirewall() error {
	self, _ := os.Executable()
	inst := filepath.Join(installDir(), "Aldebaran-Serveur.exe")
	cmd := fmt.Sprintf(`/c netsh advfirewall firewall delete rule name=all program="%s" & netsh advfirewall firewall delete rule name=all program="%s" & netsh advfirewall firewall delete rule name="Aldebaran Serveur" & netsh advfirewall firewall add rule name="Aldebaran Serveur" dir=in action=allow protocol=TCP localport=%d profile=any & netsh advfirewall firewall add rule name="Aldebaran Serveur" dir=in action=allow program="%s" profile=any`, self, inst, Port, inst)
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString("cmd.exe")
	args, _ := syscall.UTF16PtrFromString(cmd)
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(args)), 0, 0) // 0 = fenêtre cachée
	if r <= 32 {
		return fmt.Errorf("autorisation refusée (%d)", r)
	}
	logf("règles du pare-feu mises à jour")
	return nil
}
