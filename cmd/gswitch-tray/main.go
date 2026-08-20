package main

import (
	"fmt"
	"os"

	"github.com/gotk3/gotk3/gtk"

	"github.com/arumata/gswitch/internal/tray"
)

func main() {
	// Initialize GTK (required before creating any GTK widgets)
	gtk.Init(nil)

	app := tray.New()

	fmt.Println("Starting gswitch-tray...")

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
