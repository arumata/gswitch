//go:build dev

package tray

import (
	"os"
	"strings"
)

func forcedDetectionStatus() (DetectionInfo, bool) {
	forced := strings.TrimSpace(os.Getenv("GSWITCH_TRAY_FORCE_STATUS"))
	if forced == "" {
		return DetectionInfo{}, false
	}

	switch strings.ToLower(forced) {
	case "needs_config", "needs-config", "not_found", "not-found", "warning":
		return DetectionInfo{Status: TrayStatusNeedsConfig}, true
	case "service_error", "service-error", "service":
		return DetectionInfo{Status: TrayStatusServiceError, Error: strTooltipServiceStopped}, true
	case "detect_error", "detect-error", "error":
		return DetectionInfo{Status: TrayStatusDetectError, Error: "simulated auto-detection error"}, true
	case "ok":
		return DetectionInfo{Status: TrayStatusOK, Source: "sim", KeyNames: "Alt+Shift"}, true
	default:
		return DetectionInfo{}, false
	}
}
