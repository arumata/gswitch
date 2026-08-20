package main

import (
	"fmt"
	"os"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"

	"github.com/arumata/gswitch/internal/tray"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	// Initialize GTK (required before creating any GTK widgets)
	gtk.Init(nil)

	tray.SetVersion(version)
	app := tray.New()

	fmt.Println("Starting gswitch-tray...")

	// The tray icon (fyne.io/systray) runs its own D-Bus StatusNotifier
	// loop, while the settings window and dialogs need the GTK main loop.
	// Run the systray loop in a goroutine and GTK on the main thread.
	errChan := make(chan error, 1)
	go func() {
		err := app.Run()
		errChan <- err
		// Systray loop ended (Quit or error) - stop the GTK main loop
		glib.IdleAdd(gtk.MainQuit)
	}()

	gtk.Main()

	if err := <-errChan; err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
