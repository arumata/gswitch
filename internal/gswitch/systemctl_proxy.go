package gswitch

import (
	"fmt"
	"os"
	"os/exec"
)

const systemdUnitName = "gswitch.service"

func runSystemctlProxy(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: --systemctl <start|stop|restart|enable|disable>")
		os.Exit(1)
	}

	switch args[0] {
	case "start", "stop", "restart", "enable", "disable":
	default:
		fmt.Fprintln(os.Stderr, "usage: --systemctl <start|stop|restart|enable|disable>")
		os.Exit(1)
	}

	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "--systemctl must be run as root (use pkexec)")
		os.Exit(1)
	}

	// #nosec G204 -- action is validated against an allowlist above and the unit name is constant.
	cmd := exec.Command("systemctl", args[0], systemdUnitName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
