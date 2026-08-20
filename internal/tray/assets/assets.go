// Package assets provides embedded resources for the tray application.
package assets

import "embed"

//go:embed flags/*.png
var FlagsFS embed.FS
