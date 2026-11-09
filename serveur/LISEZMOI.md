# Aldebaran — serveur local (source)

Compilation (Go 1.24) :

    mkdir site && copier le contenu du site (index.html, icônes, mobs/…) dans site/
    rsrc -arch amd64 -ico icon.ico -manifest app.manifest -o rsrc_windows_amd64.syso
    GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o Aldebaran-Serveur.exe .

Port 4777. /ws relais, /info état, /qr.svg QR code, / l'appli.
