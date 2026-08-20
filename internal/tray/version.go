package tray

// appVersion is the application version shown in the UI.
// Set via SetVersion from the main package (embedded with ldflags).
var appVersion = "dev"

// SetVersion sets the application version for display in the UI.
func SetVersion(v string) {
	if v != "" {
		appVersion = v
	}
}
