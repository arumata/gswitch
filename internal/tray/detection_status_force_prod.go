//go:build !dev

package tray

func forcedDetectionStatus() (DetectionInfo, bool) {
	return DetectionInfo{}, false
}
