//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func dataDir() string {
	b, _ := os.UserCacheDir()
	if d := os.Getenv("ADB_DATA"); d != "" {
		b = d
	}
	return filepath.Join(b, "Aldebaran", "appli")
}
func fatal(text string)    { fmt.Fprintln(os.Stderr, text) }
func openBrowser(u string) { fmt.Println("MJ :", u) }
func keepAwake()           {}
func runUI() {
	for _, ip := range lanIPs() {
		fmt.Println("Joueurs :", playerURL(ip))
	}
	select {}
}
