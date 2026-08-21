package gswitch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	gsruntime "github.com/arumata/gswitch/internal/gswitch/runtime"
)

var version = "dev"

// Execute runs gswitch CLI commands and exits with appropriate status codes.
func Execute(args []string, buildVersion string) {
	version = buildVersion
	gsruntime.SetVersion(buildVersion)

	if len(args) < 2 {
		runHelp(version)
		return
	}

	switch args[1] {
	case "-c", "--configure":
		runConfigure()
	case "-r", "--run":
		run(true)
	case "-d", "--debug":
		run(false)
	case "--write-config":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: --write-config 'key=value,...'")
			os.Exit(1)
		}
		if err := writeConfigFromArgs(args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "-v", "--version":
		fmt.Printf("gswitch version %s\n", version)
	case "--test-session-env":
		runTestSessionEnv()
	case "--detect-layout-switch":
		runDetectLayoutSwitch(args[2:])
	case "-h", "--help":
		runHelp(version)
	default:
		runHelp(version)
	}
}

// runTestSessionEnv tests GetActiveSessionEnv functionality.
func runTestSessionEnv() {
	fmt.Println("Testing GetActiveSessionEnv...")
	fmt.Printf("Running as UID: %d\n", os.Getuid())

	env, err := GetActiveSessionEnv()
	if err != nil {
		switch {
		case errors.Is(err, ErrNoSystemd):
			fmt.Println("Result: ErrNoSystemd - loginctl not available")
			fmt.Println("This is expected on non-systemd systems")
		case errors.Is(err, ErrNoActiveSession):
			fmt.Println("Result: ErrNoActiveSession - no active graphical session")
			fmt.Println("This is expected when no user is logged in")
		default:
			fmt.Printf("Error: %v\n", err)
		}
		return
	}

	fmt.Println("Session found:")
	fmt.Printf("  UID:           %d\n", env.UID)
	fmt.Printf("  GID:           %d\n", env.GID)
	fmt.Printf("  User:          %s\n", env.User)
	fmt.Printf("  Home:          %s\n", env.Home)
	fmt.Printf("  SessionType:   %s\n", env.SessionType)
	fmt.Printf("  Display:       %s\n", env.Display)
	fmt.Printf("  XAuthority:    %s\n", env.XAuthority)
	fmt.Printf("  DBusAddress:   %s\n", env.DBusAddress)
	fmt.Printf("  RuntimeDir:    %s\n", env.RuntimeDir)
	fmt.Printf("  XDGConfigHome: %s\n", env.XDGConfigHome)
	fmt.Printf("  IBusAddress:   %s\n", env.IBusAddress)
}

func runHelp(version string) {
	fmt.Printf("gswitch - keyboard layout switcher %s\n", version)
	fmt.Println()
	fmt.Println("Usage: gswitch [option]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("   -c,   --configure          configure gswitch")
	fmt.Println("   -r,   --run                run")
	fmt.Println("   -d,   --debug              run in a debug mode")
	fmt.Println("         --write-config       write config (intended for pkexec)")
	fmt.Println("   -v,   --version            show version")
	fmt.Println("   -h,   --help               show this help")
	fmt.Println()
	fmt.Println("Detection:")
	fmt.Println("   --detect-layout-switch           detect layout switch keys (JSON output)")
	fmt.Println("         --source=SOURCE            use specific provider (xkb, gnome, kde)")
}

// runDetectLayoutSwitch handles the --detect-layout-switch command.
// Always outputs JSON to stdout (per task spec: "JSON only").
// Parses optional flag: --source=SOURCE.
func runDetectLayoutSwitch(args []string) {
	var source string
	for _, arg := range args {
		if val, ok := strings.CutPrefix(arg, "--source="); ok {
			source = val
			if source == "" {
				outputJSONError("--source requires a value (xkb, gnome, kde)")
			}
			if !isValidDetectionSource(DetectionSource(source)) {
				outputJSONError("unknown source: " + source + " (valid: xkb, gnome, kde)")
			}
		} else {
			outputJSONError("unknown flag: " + arg)
		}
	}

	var opts *DetectionOptions
	if source != "" {
		opts = &DetectionOptions{SourceOverride: DetectionSource(source)}
	}

	result, err := DetectLayoutSwitchKeys(opts)
	runDetectJSONOutput(result, err)
}

// outputJSONError outputs an error in JSON format and exits with code 2.
func outputJSONError(errMsg string) {
	output := &DetectJSONOutput{
		Schema:   1,
		Status:   "error",
		Error:    errMsg,
		Attempts: []DetectJSONAttempt{},
	}
	//nolint:errcheck // Best effort output before exit
	json.NewEncoder(os.Stdout).Encode(output)
	os.Exit(2)
}

// runDetectJSONOutput outputs detection result as JSON to stdout.
func runDetectJSONOutput(result *DetectionResult, err error) {
	output := BuildDetectJSONOutput(result, err)

	if encErr := json.NewEncoder(os.Stdout).Encode(output); encErr != nil {
		fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", encErr)
		os.Exit(2)
	}

	switch output.Status {
	case "found":
		os.Exit(0)
	case "not_found":
		os.Exit(1)
	default:
		os.Exit(2)
	}
}

func isValidDetectionSource(source DetectionSource) bool {
	switch source {
	case SourceXKB, SourceGNOME, SourceKDE:
		return true
	default:
		return false
	}
}
