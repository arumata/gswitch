package main

import (
	"os"

	"github.com/arumata/gswitch/internal/gswitch"
)

// version is set at build time via -ldflags
var version = "dev"

func main() {
	gswitch.Execute(os.Args, version)
}
