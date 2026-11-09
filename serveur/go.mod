module aldebaranserveur

go 1.24

replace golang.org/x/sys => github.com/golang/sys v0.30.0

replace fyne.io/systray => github.com/fyne-io/systray v1.11.0

replace rsc.io/qr => github.com/rsc/qr v0.2.0

require (
	fyne.io/systray v0.0.0-00010101000000-000000000000
	github.com/gorilla/websocket v1.5.3
	rsc.io/qr v0.0.0-00010101000000-000000000000
)

require (
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
)
