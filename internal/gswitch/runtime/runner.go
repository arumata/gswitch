package runtime

import (
	"fmt"
	"io"
	"os"

	cfg "github.com/arumata/gswitch/internal/gswitch/config"
)

// Run executes daemon/debug mode lifecycle and returns process exit code.
func Run(daemon bool, stderr io.Writer) int {
	config, err := cfg.LoadConfigFrom(cfg.DefaultConfigPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		fmt.Fprintln(stderr, "Run 'gswitch -c' to configure.")
		return 1
	}

	switcher, err := NewSwitcher(config, !daemon)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	defer switcher.Close()

	if err := switcher.Run(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	return 0
}

// RunDefault executes runtime and exits process with the resulting code.
func RunDefault(daemon bool) {
	os.Exit(Run(daemon, os.Stderr))
}
