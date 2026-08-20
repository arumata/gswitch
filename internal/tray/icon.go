package tray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/arumata/gswitch/internal/tray/assets"
)

const iconSize = 22

// letterPatterns defines pixel patterns for uppercase letters.
// Each pattern is a list of (x, y) coordinates relative to the letter's top-left.
var letterPatterns = map[rune][][2]int{
	'A': {
		{3, 0}, {4, 0}, {2, 1}, {3, 1}, {4, 1}, {5, 1},
		{1, 2}, {2, 2}, {5, 2}, {6, 2}, {0, 3}, {1, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {6, 4}, {7, 4}, {0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5}, {6, 5}, {7, 5},
		{0, 6}, {1, 6}, {2, 6}, {3, 6}, {4, 6}, {5, 6}, {6, 6}, {7, 6},
		{0, 7}, {1, 7}, {6, 7}, {7, 7}, {0, 8}, {1, 8}, {6, 8}, {7, 8},
		{0, 9}, {1, 9}, {6, 9}, {7, 9}, {0, 10}, {1, 10}, {6, 10}, {7, 10},
	},
	'B': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0},
		{0, 1}, {1, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2}, {0, 3}, {1, 3}, {5, 3}, {6, 3},
		{0, 4}, {1, 4}, {2, 4}, {3, 4}, {4, 4}, {5, 4},
		{0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{0, 6}, {1, 6}, {5, 6}, {6, 6}, {0, 7}, {1, 7}, {6, 7}, {7, 7},
		{0, 8}, {1, 8}, {6, 8}, {7, 8}, {0, 9}, {1, 9}, {5, 9}, {6, 9},
		{0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'D': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0},
		{0, 1}, {1, 1}, {4, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2}, {0, 3}, {1, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {6, 4}, {7, 4}, {0, 5}, {1, 5}, {6, 5}, {7, 5},
		{0, 6}, {1, 6}, {6, 6}, {7, 6}, {0, 7}, {1, 7}, {6, 7}, {7, 7},
		{0, 8}, {1, 8}, {6, 8}, {7, 8}, {0, 9}, {1, 9}, {4, 9}, {5, 9}, {6, 9},
		{0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'E': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0}, {7, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}, {7, 1},
		{0, 2}, {1, 2}, {0, 3}, {1, 3}, {0, 4}, {1, 4}, {2, 4}, {3, 4}, {4, 4}, {5, 4},
		{0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{0, 6}, {1, 6}, {0, 7}, {1, 7}, {0, 8}, {1, 8},
		{0, 9}, {1, 9}, {2, 9}, {3, 9}, {4, 9}, {5, 9}, {6, 9}, {7, 9},
		{0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10}, {6, 10}, {7, 10},
	},
	'F': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0}, {7, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}, {7, 1},
		{0, 2}, {1, 2}, {0, 3}, {1, 3}, {0, 4}, {1, 4}, {2, 4}, {3, 4}, {4, 4}, {5, 4},
		{0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{0, 6}, {1, 6}, {0, 7}, {1, 7}, {0, 8}, {1, 8},
		{0, 9}, {1, 9}, {0, 10}, {1, 10},
	},
	'G': {
		{2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0},
		{1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}, {7, 1},
		{0, 2}, {1, 2}, {0, 3}, {1, 3}, {0, 4}, {1, 4},
		{0, 5}, {1, 5}, {4, 5}, {5, 5}, {6, 5}, {7, 5},
		{0, 6}, {1, 6}, {4, 6}, {5, 6}, {6, 6}, {7, 6},
		{0, 7}, {1, 7}, {6, 7}, {7, 7}, {0, 8}, {1, 8}, {6, 8}, {7, 8},
		{1, 9}, {2, 9}, {5, 9}, {6, 9}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'I': {
		{2, 0}, {3, 0}, {4, 0}, {5, 0}, {2, 1}, {3, 1}, {4, 1}, {5, 1},
		{3, 2}, {4, 2}, {3, 3}, {4, 3}, {3, 4}, {4, 4}, {3, 5}, {4, 5},
		{3, 6}, {4, 6}, {3, 7}, {4, 7}, {3, 8}, {4, 8},
		{2, 9}, {3, 9}, {4, 9}, {5, 9}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'K': {
		{0, 0}, {1, 0}, {6, 0}, {7, 0},
		{0, 1}, {1, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {4, 2}, {5, 2},
		{0, 3}, {1, 3}, {3, 3}, {4, 3},
		{0, 4}, {1, 4}, {2, 4}, {3, 4},
		{0, 5}, {1, 5}, {2, 5}, {3, 5},
		{0, 6}, {1, 6}, {3, 6}, {4, 6},
		{0, 7}, {1, 7}, {4, 7}, {5, 7},
		{0, 8}, {1, 8}, {5, 8}, {6, 8},
		{0, 9}, {1, 9}, {6, 9}, {7, 9},
		{0, 10}, {1, 10}, {7, 10}, {8, 10},
	},
	'L': {
		{0, 0}, {1, 0}, {0, 1}, {1, 1}, {0, 2}, {1, 2}, {0, 3}, {1, 3},
		{0, 4}, {1, 4}, {0, 5}, {1, 5}, {0, 6}, {1, 6}, {0, 7}, {1, 7},
		{0, 8}, {1, 8}, {0, 9}, {1, 9}, {2, 9}, {3, 9}, {4, 9}, {5, 9}, {6, 9}, {7, 9},
		{0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10}, {6, 10}, {7, 10},
	},
	'N': {
		{0, 0}, {1, 0}, {6, 0}, {7, 0}, {0, 1}, {1, 1}, {2, 1}, {6, 1}, {7, 1},
		{0, 2}, {1, 2}, {2, 2}, {3, 2}, {6, 2}, {7, 2},
		{0, 3}, {1, 3}, {3, 3}, {4, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {4, 4}, {5, 4}, {6, 4}, {7, 4},
		{0, 5}, {1, 5}, {4, 5}, {5, 5}, {6, 5}, {7, 5},
		{0, 6}, {1, 6}, {5, 6}, {6, 6}, {7, 6},
		{0, 7}, {1, 7}, {6, 7}, {7, 7}, {0, 8}, {1, 8}, {6, 8}, {7, 8},
		{0, 9}, {1, 9}, {6, 9}, {7, 9}, {0, 10}, {1, 10}, {6, 10}, {7, 10},
	},
	'O': {
		{2, 0}, {3, 0}, {4, 0}, {5, 0},
		{1, 1}, {2, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2}, {0, 3}, {1, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {6, 4}, {7, 4}, {0, 5}, {1, 5}, {6, 5}, {7, 5},
		{0, 6}, {1, 6}, {6, 6}, {7, 6}, {0, 7}, {1, 7}, {6, 7}, {7, 7},
		{0, 8}, {1, 8}, {6, 8}, {7, 8}, {1, 9}, {2, 9}, {5, 9}, {6, 9},
		{2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'P': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0},
		{0, 1}, {1, 1}, {4, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2}, {0, 3}, {1, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {5, 4}, {6, 4}, {0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{0, 6}, {1, 6}, {0, 7}, {1, 7}, {0, 8}, {1, 8},
		{0, 9}, {1, 9}, {0, 10}, {1, 10},
	},
	'R': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2}, {0, 3}, {1, 3}, {6, 3}, {7, 3},
		{0, 4}, {1, 4}, {5, 4}, {6, 4}, {0, 5}, {1, 5}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{0, 6}, {1, 6}, {2, 6}, {3, 6}, {4, 6},
		{0, 7}, {1, 7}, {3, 7}, {4, 7}, {0, 8}, {1, 8}, {4, 8}, {5, 8},
		{0, 9}, {1, 9}, {5, 9}, {6, 9}, {0, 10}, {1, 10}, {6, 10}, {7, 10},
	},
	'S': {
		{2, 0}, {3, 0}, {4, 0}, {5, 0}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {2, 2}, {0, 3}, {1, 3},
		{1, 4}, {2, 4}, {3, 4}, {2, 5}, {3, 5}, {4, 5}, {5, 5},
		{4, 6}, {5, 6}, {6, 6}, {5, 7}, {6, 7}, {7, 7},
		{5, 8}, {6, 8}, {7, 8}, {0, 9}, {1, 9}, {5, 9}, {6, 9},
		{1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
	},
	'T': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0}, {7, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}, {7, 1},
		{3, 2}, {4, 2}, {3, 3}, {4, 3}, {3, 4}, {4, 4}, {3, 5}, {4, 5},
		{3, 6}, {4, 6}, {3, 7}, {4, 7}, {3, 8}, {4, 8},
		{3, 9}, {4, 9}, {3, 10}, {4, 10},
	},
	'U': {
		{0, 0}, {1, 0}, {0, 1}, {1, 1}, {0, 2}, {1, 2}, {0, 3}, {1, 3},
		{0, 4}, {1, 4}, {0, 5}, {1, 5}, {0, 6}, {1, 6}, {0, 7}, {1, 7},
		{0, 8}, {1, 8}, {0, 9}, {1, 9}, {2, 9}, {3, 9}, {4, 9}, {5, 9},
		{6, 9}, {7, 9}, {0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10},
		{6, 10}, {7, 10}, {6, 0}, {7, 0}, {6, 1}, {7, 1}, {6, 2}, {7, 2},
		{6, 3}, {7, 3}, {6, 4}, {7, 4}, {6, 5}, {7, 5}, {6, 6}, {7, 6},
		{6, 7}, {7, 7}, {6, 8}, {7, 8},
	},
	'Y': {
		{0, 0}, {1, 0}, {6, 0}, {7, 0}, {0, 1}, {1, 1}, {6, 1}, {7, 1},
		{1, 2}, {2, 2}, {5, 2}, {6, 2}, {2, 3}, {3, 3}, {4, 3}, {5, 3},
		{3, 4}, {4, 4}, {3, 5}, {4, 5}, {3, 6}, {4, 6}, {3, 7}, {4, 7},
		{3, 8}, {4, 8}, {3, 9}, {4, 9}, {3, 10}, {4, 10},
	},
	'Z': {
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}, {5, 0}, {6, 0}, {7, 0},
		{0, 1}, {1, 1}, {2, 1}, {3, 1}, {4, 1}, {5, 1}, {6, 1}, {7, 1},
		{5, 2}, {6, 2}, {4, 3}, {5, 3}, {3, 4}, {4, 4}, {3, 5}, {4, 5},
		{2, 6}, {3, 6}, {1, 7}, {2, 7}, {0, 8}, {1, 8},
		{0, 9}, {1, 9}, {2, 9}, {3, 9}, {4, 9}, {5, 9}, {6, 9}, {7, 9},
		{0, 10}, {1, 10}, {2, 10}, {3, 10}, {4, 10}, {5, 10}, {6, 10}, {7, 10},
	},
	'?': {
		{2, 0}, {3, 0}, {4, 0}, {5, 0},
		{1, 1}, {2, 1}, {5, 1}, {6, 1},
		{0, 2}, {1, 2}, {6, 2}, {7, 2},
		{6, 3}, {7, 3}, {5, 4}, {6, 4},
		{4, 5}, {5, 5}, {3, 6}, {4, 6},
		{3, 7}, {4, 7},
		{3, 9}, {4, 9}, {3, 10}, {4, 10},
	},
}

// GenerateLayoutIcon creates a 22x22 icon with the given 2-letter layout code.
func GenerateLayoutIcon(code string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	// Fill with transparent background
	for y := range iconSize {
		for x := range iconSize {
			img.Set(x, y, color.Transparent)
		}
	}

	white := color.RGBA{255, 255, 255, 255}

	// Get up to 2 characters from code
	runes := []rune(code)
	if len(runes) > 2 {
		runes = runes[:2]
	}

	// Position letters: first at x=2, second at x=12
	positions := []int{2, 12}
	for i, r := range runes {
		if pattern, ok := letterPatterns[r]; ok {
			for _, p := range pattern {
				x := positions[i] + p[0]
				y := 5 + p[1] // vertical offset
				if x < iconSize && y < iconSize {
					img.Set(x, y, white)
				}
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// Pre-generated icons for common layouts (avoids regeneration on each switch)
var (
	kbIcon      = GenerateLayoutIcon("KB")
	warningIcon = generateWarningIcon()
	errorIcon   = generateErrorIcon()
	iconCache   = make(map[string][]byte)
	flagCache   = make(map[string][]byte)
)

// generateWarningIcon creates a 22x22 warning icon (yellow exclamation mark).
func generateWarningIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	// Fill with transparent background
	for y := range iconSize {
		for x := range iconSize {
			img.Set(x, y, color.Transparent)
		}
	}

	yellow := color.RGBA{255, 193, 7, 255} // #FFC107
	black := color.RGBA{0, 0, 0, 255}

	// Draw yellow circle background
	centerX, centerY := iconSize/2, iconSize/2
	radius := iconSize/2 - 1
	for y := range iconSize {
		for x := range iconSize {
			dx := x - centerX
			dy := y - centerY
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, yellow)
			}
		}
	}

	// Draw exclamation mark "!" in black
	// Vertical line (top part)
	for y := 5; y <= 13; y++ {
		img.Set(10, y, black)
		img.Set(11, y, black)
	}
	// Dot (bottom part)
	for y := 15; y <= 17; y++ {
		img.Set(10, y, black)
		img.Set(11, y, black)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// generateErrorIcon creates a 22x22 error icon (red X mark).
func generateErrorIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	// Fill with transparent background
	for y := range iconSize {
		for x := range iconSize {
			img.Set(x, y, color.Transparent)
		}
	}

	red := color.RGBA{244, 67, 54, 255} // #F44336
	white := color.RGBA{255, 255, 255, 255}

	// Draw red circle background
	centerX, centerY := iconSize/2, iconSize/2
	radius := iconSize/2 - 1
	for y := range iconSize {
		for x := range iconSize {
			dx := x - centerX
			dy := y - centerY
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, red)
			}
		}
	}

	// Draw X mark in white
	// Diagonal from top-left to bottom-right
	for i := range 10 {
		x := 6 + i
		y := 6 + i
		img.Set(x, y, white)
		img.Set(x+1, y, white)
		img.Set(x, y+1, white)
	}
	// Diagonal from top-right to bottom-left
	for i := range 10 {
		x := 15 - i
		y := 6 + i
		img.Set(x, y, white)
		img.Set(x-1, y, white)
		img.Set(x, y+1, white)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// GetWarningIcon returns the pre-generated warning icon.
func GetWarningIcon() []byte {
	return warningIcon
}

// GetErrorIcon returns the pre-generated error icon.
func GetErrorIcon() []byte {
	return errorIcon
}

// GetLayoutIcon returns a cached icon for the given layout code.
// It first tries to load a flag icon, falling back to text icon if not found.
func GetLayoutIcon(code string) []byte {
	if icon, ok := iconCache[code]; ok {
		return icon
	}

	// Try to load flag icon first
	icon := loadFlagIcon(strings.ToLower(code))
	if icon != nil {
		iconCache[code] = icon
		return icon
	}

	// Fallback to text icon
	icon = GenerateLayoutIcon(code)
	iconCache[code] = icon
	return icon
}

// loadFlagIcon loads a flag icon from embedded assets.
// Returns nil if the flag is not found.
func loadFlagIcon(code string) []byte {
	if icon, ok := flagCache[code]; ok {
		return icon
	}

	data, err := assets.FlagsFS.ReadFile(fmt.Sprintf("flags/%s.png", code))
	if err != nil {
		return nil
	}

	flagCache[code] = data
	return data
}

// GetFlagIcon returns a flag icon for the layout code, or nil if not found.
func GetFlagIcon(code string) []byte {
	return loadFlagIcon(strings.ToLower(code))
}

// HasFlagIcon returns true if a flag icon exists for the given layout code.
func HasFlagIcon(code string) bool {
	return loadFlagIcon(strings.ToLower(code)) != nil
}

func init() {
	// Pre-cache flag icons for common layouts
	for _, code := range []string{"us", "ru", "de", "fr", "es", "it", "ua", "pl", "gb", "pt"} {
		if data, err := assets.FlagsFS.ReadFile(fmt.Sprintf("flags/%s.png", code)); err == nil {
			flagCache[code] = data
			iconCache[strings.ToUpper(code)] = data
		}
	}
}
