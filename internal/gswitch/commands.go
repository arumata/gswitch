package gswitch

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	gsruntime "github.com/arumata/gswitch/internal/gswitch/runtime"
)

type keyboardInfo struct {
	path   string
	active atomic.Bool
}

func run(daemon bool) {
	exitCode := gsruntime.Run(daemon, os.Stderr)
	os.Exit(exitCode)
}

//nolint:gocognit,nestif,revive,funlen,maintidx // interactive configuration flow requires complex logic
func runConfigure() {
	fmt.Println("gswitch keyboard configuration started.")

	// Create config directory
	dir := filepath.Dir(gsruntime.ConfigFile)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Printf("Cannot create directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	// Load existing config or use defaults
	cfg := &Config{
		Delay:             10,
		LayoutSwitchDelay: 100,
	}

	fmt.Println("Checking existing config...")
	if existingCfg, err := LoadConfig(); err == nil {
		cfg.Delay = existingCfg.Delay
		cfg.LayoutSwitchDelay = existingCfg.LayoutSwitchDelay
		cfg.Blacklist = existingCfg.Blacklist
		fmt.Println("Done.")
	} else {
		fmt.Println("No existing config found, will create new one.")
	}

	// Find keyboards
	fmt.Println()
	fmt.Println("Scanning keyboards...")

	keyboards := findKeyboards()
	if len(keyboards) == 0 {
		fmt.Println("Error.")
		fmt.Println("No keyboards found. Are you root?")
		os.Exit(1)
	}
	fmt.Printf("Found %d input device(s).\n\n", len(keyboards))

	// Start keyboard detection threads
	kbInfos := make([]keyboardInfo, len(keyboards))
	for i, kb := range keyboards {
		kbInfos[i] = keyboardInfo{path: kb}
	}

	// Channel to stop detection goroutines
	stopCh := make(chan struct{})

	// Start detection goroutines
	for i := range kbInfos {
		go func(idx int) {
			detectKeyboard(&kbInfos[idx], stopCh)
		}(i)
	}

	// Ask about convert key mode
	fmt.Println("Please set the key combination you will use to correct text.")
	fmt.Println("You can use the default combination or define your own.")
	fmt.Println("The default combination is:")
	fmt.Println(" - double SHIFT to correct the last word;")
	fmt.Println(" - double SHIFT while holding the other SHIFT to correct the whole text.")
	fmt.Println()
	fmt.Print("Do you want to use the default combination? (y/n) ")

	reader := bufio.NewReader(os.Stdin)
	var convertKey uint16

	for {
		choice, err := reader.ReadString('\n')
		if err != nil {
			close(stopCh)
			fmt.Printf("Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		choice = strings.TrimSpace(strings.ToLower(choice))

		if choice == "y" {
			convertKey = 0 // double-shift mode
			fmt.Println()
			fmt.Println("gswitch will use the default combination to correct the text - double SHIFT.")
			break
		}
		if choice == "n" {
			fmt.Println()
			fmt.Println("Press the key you want to use to correct text.")
			fmt.Println("Please DO NOT use:")
			fmt.Println("  - Letters and numbers: A-Z, 0-9")
			fmt.Println("  - Special characters: ~ - = { } ; \" , . / * - + etc.")
			fmt.Println("  - Keys that move cursor: arrows, TAB, PAGEUP, PAGEDOWN etc.")
			fmt.Println("  - Special keys: CTRL, ALT, SHIFT, BACKSPACE, DEL etc.")
			fmt.Println()
			fmt.Println("Waiting for your input...")

			// Use first available keyboard for detection
			var kbPath string
			for range gsruntime.WaitForKeyboardIterations / 10 {
				time.Sleep(100 * time.Millisecond)
				for i := range kbInfos {
					if kbInfos[i].active.Load() {
						kbPath = kbInfos[i].path
						break
					}
				}
				if kbPath != "" {
					break
				}
			}

			if kbPath == "" && len(keyboards) > 0 {
				kbPath = keyboards[0]
			}

			kbFD, err := os.OpenFile(kbPath, os.O_RDONLY|syscall.O_SYNC, 0)
			if err != nil {
				close(stopCh)
				fmt.Printf("Error opening keyboard: %v\n", err)
				os.Exit(1)
			}

			convertKey = detectReplaceKey(kbFD)
			kbFD.Close()

			if convertKey == 0 {
				fmt.Println("Timeout reached.")
				os.Exit(1)
			}

			fmt.Printf("Captured key: %s\n", gsruntime.GetKeyName(convertKey))
			break
		}
		fmt.Print("Invalid input. Please enter 'y' or 'n': ")
	}

	cfg.ConvertKey = convertKey
	fmt.Println()

	// Try auto-detection first
	fmt.Println("Detecting layout switch key from system settings...")
	var layoutKeys []uint16

	detectResult, detectErr := DetectLayoutSwitchKeys(nil)

	if detectErr == nil && len(detectResult.Scancodes) > 0 {
		// Auto-detect succeeded
		fmt.Printf("Detected: %s (%s)\n", detectResult.KeyNames, detectResult.Source)
		if detectResult.Warning != "" {
			fmt.Printf("Warning: %s\n", detectResult.Warning)
		}
		fmt.Print("Use this setting? (Y/n) ")

		choice, err := reader.ReadString('\n')
		if err != nil {
			close(stopCh)
			fmt.Printf("Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		choice = strings.TrimSpace(strings.ToLower(choice))

		if choice == "" || choice == "y" {
			layoutKeys = detectResult.Scancodes
			fmt.Println()
		}
	} else {
		// Auto-detect failed - show diagnostic info
		fmt.Println("Auto-detect could not find layout switch setting.")
		if detectResult != nil && len(detectResult.Attempts) > 0 {
			fmt.Println("Detection attempts:")
			for _, attempt := range detectResult.Attempts {
				status := string(attempt.Status)
				if attempt.Error != "" {
					status = "error: " + attempt.Error
				}
				fmt.Printf("  %s: %s\n", attempt.Provider, status)
			}
		}
		fmt.Println()
	}

	// If auto-detect didn't provide keys, use manual capture
	if len(layoutKeys) == 0 {
		fmt.Println("Please specify the key that is currently used to switch the keyboard layout in your system.")
		fmt.Println("Press the key or key combination.")
		fmt.Println("Waiting for your input...")

		// Find first available keyboard
		var kbPath string
		for range gsruntime.WaitForKeyboardIterations / 10 {
			time.Sleep(100 * time.Millisecond)
			for i := range kbInfos {
				if kbInfos[i].active.Load() {
					kbPath = kbInfos[i].path
					break
				}
			}
			if kbPath != "" {
				break
			}
		}

		// Stop detection goroutines
		close(stopCh)

		if kbPath == "" && len(keyboards) > 0 {
			kbPath = keyboards[0]
		}

		kbFD, err := os.OpenFile(kbPath, os.O_RDONLY|syscall.O_SYNC, 0)
		if err != nil {
			fmt.Printf("Error opening keyboard: %v\n", err)
			os.Exit(1)
		}

		layoutKeys = detectLayoutSwitchKey(kbFD)
		kbFD.Close()

		if len(layoutKeys) == 0 {
			fmt.Println("Timeout reached.")
			os.Exit(1)
		}

		if len(layoutKeys) == 1 {
			fmt.Printf("Captured key: %s\n\n", gsruntime.GetKeyName(layoutKeys[0]))
		} else {
			fmt.Printf("Captured key combination: %s+%s\n\n", gsruntime.GetKeyName(layoutKeys[0]), gsruntime.GetKeyName(layoutKeys[1]))
		}
	} else {
		// Close stop channel if auto-detect was used
		close(stopCh)
	}

	cfg.LayoutSwitchKey = layoutKeys

	// Configure layouts for conversion
	cfg.Layouts = configureLayouts(reader)

	// Save config
	fmt.Println("Saving configuration...")
	if err := SaveConfig(cfg); err != nil {
		fmt.Printf("Error writing configuration file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration is successfully saved.")
	fmt.Printf("See %s to edit additional parameters.\n", gsruntime.ConfigFile)
}

// configureLayouts asks user to select or enter layouts for conversion.
// Returns nil if auto-detection should be used (exactly 2 layouts detected).
func configureLayouts(reader *bufio.Reader) []LayoutSpec {
	fmt.Println("Configuring keyboard layouts for text conversion...")
	fmt.Println()

	// Try to detect current layouts
	detected, err := gsruntime.GetCurrentLayouts()
	if err != nil {
		fmt.Printf("Could not auto-detect layouts: %v\n", err)
		fmt.Println("You need to specify layouts manually.")
		return selectLayoutsManually(reader)
	}

	fmt.Printf("Detected %d layout(s): ", len(detected))
	for i, l := range detected {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(gsruntime.FormatLayout(l))
	}
	fmt.Println()
	fmt.Println()

	// If exactly 2 layouts - ask if user wants to use them
	if len(detected) == 2 {
		fmt.Printf("Do you want to use these layouts for conversion? (y/n) ")
		for {
			choice, err := reader.ReadString('\n')
			if err != nil {
				fmt.Printf("Error reading input: %v\n", err)
				os.Exit(1)
			}
			choice = strings.TrimSpace(strings.ToLower(choice))

			if choice == "y" {
				fmt.Println()
				return nil // auto-detect will be used
			}
			if choice == "n" {
				fmt.Println()
				return selectLayoutsManually(reader)
			}
			fmt.Print("Invalid input. Please enter 'y' or 'n': ")
		}
	}

	// More than 2 layouts - must choose
	fmt.Println("gswitch supports exactly 2 layouts for conversion.")
	fmt.Println("Please select which layouts to use:")
	fmt.Println()

	return selectLayoutsFromList(reader, detected)
}

// selectLayoutsFromList lets user pick two layouts from detected list.
func selectLayoutsFromList(reader *bufio.Reader, layouts []LayoutSpec) []LayoutSpec {
	// Show numbered list
	for i, l := range layouts {
		fmt.Printf("  %d. %s\n", i+1, gsruntime.FormatLayout(l))
	}
	fmt.Println("  0. Enter layout manually")
	fmt.Println()

	selected := make([]LayoutSpec, 0, 2)

	for len(selected) < 2 {
		ordinal := "first"
		if len(selected) == 1 {
			ordinal = "second"
		}
		fmt.Printf("Enter number for %s layout (1-%d, or 0 for manual): ", ordinal, len(layouts))

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			os.Exit(1)
		}
		input = strings.TrimSpace(input)

		num, err := strconv.Atoi(input)
		if err != nil || num < 0 || num > len(layouts) {
			fmt.Printf("Invalid number. Please enter 0-%d\n", len(layouts))
			continue
		}

		if num == 0 {
			// Manual entry
			layout := enterLayoutManually(reader, ordinal)
			if layout != nil {
				selected = append(selected, *layout)
			}
			continue
		}

		choice := layouts[num-1]

		// Check for duplicate
		if len(selected) == 1 && selected[0].Name == choice.Name && selected[0].Variant == choice.Variant {
			fmt.Println("You already selected this layout. Please choose a different one.")
			continue
		}

		selected = append(selected, choice)
		fmt.Printf("Selected: %s\n", gsruntime.FormatLayout(choice))
	}

	fmt.Println()
	return selected
}

// selectLayoutsManually asks user to enter two layouts manually.
func selectLayoutsManually(reader *bufio.Reader) []LayoutSpec {
	fmt.Println("Enter layouts manually.")
	fmt.Println("Format: 'us' or 'ua(unicode)' for layout with variant")
	fmt.Println()

	selected := make([]LayoutSpec, 0, 2)

	for len(selected) < 2 {
		ordinal := "first"
		if len(selected) == 1 {
			ordinal = "second"
		}

		layout := enterLayoutManually(reader, ordinal)
		if layout == nil {
			continue
		}

		// Check for duplicate
		if len(selected) == 1 && selected[0].Name == layout.Name && selected[0].Variant == layout.Variant {
			fmt.Println("You already entered this layout. Please enter a different one.")
			continue
		}

		selected = append(selected, *layout)
	}

	fmt.Println()
	return selected
}

// enterLayoutManually prompts for a single layout and validates it.
func enterLayoutManually(reader *bufio.Reader, ordinal string) *LayoutSpec {
	fmt.Printf("Enter %s layout (e.g., 'us' or 'ru(phonetic)'): ", ordinal)

	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Error reading input: %v\n", err)
		return nil
	}
	input = strings.TrimSpace(input)

	if input == "" {
		fmt.Println("Layout cannot be empty.")
		return nil
	}

	layout, variant := splitLayoutVariant(input)

	// Validate by trying to load the layout
	_, err = gsruntime.LoadLayout(layout, variant)
	if err != nil {
		fmt.Printf("Invalid layout '%s': %v\n", input, err)
		fmt.Println("Available layouts can be found in /usr/share/X11/xkb/symbols/")
		return nil
	}

	spec := LayoutSpec{Name: layout, Variant: variant}
	fmt.Printf("Validated: %s\n", gsruntime.FormatLayout(spec))
	return &spec
}

func findKeyboards() []string {
	entries, err := os.ReadDir(gsruntime.InputDevicesDir)
	if err != nil {
		return nil
	}

	keyboards := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "event") {
			continue
		}

		path := filepath.Join(gsruntime.InputDevicesDir, entry.Name())
		fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}

		// Check if device supports EV_KEY (keyboard capability)
		if !gsruntime.HasKeyboardCapability(fd) {
			syscall.Close(fd)
			continue
		}
		syscall.Close(fd)

		keyboards = append(keyboards, path)
	}

	return keyboards
}

func detectKeyboard(kb *keyboardInfo, stopCh <-chan struct{}) {
	fd, err := os.OpenFile(kb.path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer fd.Close()

	eventBuf := make([]byte, gsruntime.InputEventSize)
	for range gsruntime.DetectKeyboardIterations {
		select {
		case <-stopCh:
			return
		default:
		}

		n, err := fd.Read(eventBuf)
		if err != nil || n != gsruntime.InputEventSize {
			time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
			continue
		}

		event := *(*gsruntime.InputEvent)(unsafe.Pointer(&eventBuf[0]))
		if event.Type == gsruntime.EV_KEY &&
			(event.Code == gsruntime.KEY_ENTER || event.Code == gsruntime.KEY_KPENTER) &&
			(event.Value == 0 || event.Value == 1 || event.Value == 2) {
			kb.active.Store(true)
			return
		}
		time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
	}
}

func detectLayoutSwitchKey(fd *os.File) []uint16 {
	eventBuf := make([]byte, gsruntime.InputEventSize)
	var keys []uint16

	for range gsruntime.DetectKeyIterations {
		n, err := fd.Read(eventBuf)
		if err != nil || n != gsruntime.InputEventSize {
			time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
			continue
		}

		event := *(*gsruntime.InputEvent)(unsafe.Pointer(&eventBuf[0]))
		if event.Type == gsruntime.EV_KEY && (event.Value == 0 || event.Value == 1) {
			keys = append(keys, event.Code)
			if len(keys) == 2 {
				if event.Value == 0 {
					// Single key
					return keys[:1]
				}
				// Key combination
				return keys
			}
		}
		time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
	}

	return keys
}

func detectReplaceKey(fd *os.File) uint16 {
	eventBuf := make([]byte, gsruntime.InputEventSize)

	for range gsruntime.DetectKeyIterations {
		n, err := fd.Read(eventBuf)
		if err != nil || n != gsruntime.InputEventSize {
			time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
			continue
		}

		event := *(*gsruntime.InputEvent)(unsafe.Pointer(&eventBuf[0]))
		if event.Type == gsruntime.EV_KEY && event.Value == 1 {
			return event.Code
		}
		time.Sleep(gsruntime.DetectSleepMs * time.Millisecond)
	}

	return 0
}
