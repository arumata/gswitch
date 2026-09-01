package detect

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

const (
	gnomeWMKeybindingsSchema  = "org.gnome.desktop.wm.keybindings"
	gnomeSwitchInputSourceKey = "switch-input-source"
	gnomeX11Accelerator       = "XF86Launch7"
)

var runGsettingsForLaunch7 = RunGsettings

// EnsureGNOMEX11Launch7Binding appends XF86Launch7 to GNOME's layout-switch
// accelerators. The existing accelerators remain in their original order. The
// setting is intentionally persistent so daemon restarts and XKB map rebuilds
// cannot invalidate the single-key X11 route.
func EnsureGNOMEX11Launch7Binding(env *SessionEnv) (bool, error) {
	if !isGNOMEX11Session(env) {
		return false, nil
	}

	originalOutput, err := runGsettingsForLaunch7(
		env, "get", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey,
	)
	if err != nil {
		return false, fmt.Errorf("read GNOME layout-switch bindings: %w", err)
	}

	original := parseGsettingsArray(strings.TrimSpace(string(originalOutput)))
	if slices.Contains(original, gnomeX11Accelerator) {
		return false, nil
	}

	updated := append(slices.Clone(original), gnomeX11Accelerator)
	if _, err := runGsettingsForLaunch7(
		env,
		"set",
		gnomeWMKeybindingsSchema,
		gnomeSwitchInputSourceKey,
		formatGsettingsStringArray(updated),
	); err != nil {
		return false, fmt.Errorf("add GNOME XF86Launch7 layout-switch binding: %w", err)
	}

	verifiedOutput, verifyErr := runGsettingsForLaunch7(
		env, "get", gnomeWMKeybindingsSchema, gnomeSwitchInputSourceKey,
	)
	if verifyErr == nil && slices.Contains(
		parseGsettingsArray(strings.TrimSpace(string(verifiedOutput))),
		gnomeX11Accelerator,
	) {
		return true, nil
	}

	if _, rollbackErr := runGsettingsForLaunch7(
		env,
		"set",
		gnomeWMKeybindingsSchema,
		gnomeSwitchInputSourceKey,
		formatGsettingsStringArray(original),
	); rollbackErr != nil {
		return false, errors.Join(
			fmt.Errorf("verify XF86Launch7 binding: %w", verificationError(verifyErr)),
			fmt.Errorf("restore original GNOME bindings: %w", rollbackErr),
		)
	}

	return false, fmt.Errorf("verify XF86Launch7 binding: %w", verificationError(verifyErr))
}

func isGNOMEX11Session(env *SessionEnv) bool {
	if !isGNOME(env) {
		return false
	}

	if env != nil {
		if strings.EqualFold(env.SessionType, "wayland") || env.WaylandDisplay != "" {
			return false
		}
		return strings.EqualFold(env.SessionType, "x11") || env.Display != ""
	}

	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") || os.Getenv("WAYLAND_DISPLAY") != "" {
		return false
	}
	return strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "x11") || os.Getenv("DISPLAY") != ""
}

func formatGsettingsStringArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `'`, `\'`)
		quoted = append(quoted, `'`+escaped+`'`)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func verificationError(err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("gsettings value does not contain %s", gnomeX11Accelerator)
}
