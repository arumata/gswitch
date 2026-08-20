package gswitch

import "github.com/arumata/gswitch/internal/gswitch/detect"

type DetectionSource = detect.DetectionSource
type AttemptStatus = detect.AttemptStatus
type DetectionAttempt = detect.DetectionAttempt
type DetectionContext = detect.DetectionContext
type DetectionResult = detect.DetectionResult
type Provider = detect.Provider
type DetectionOptions = detect.DetectionOptions

type DetectJSONOutput = detect.DetectJSONOutput
type DetectJSONResult = detect.DetectJSONResult
type DetectJSONAttempt = detect.DetectJSONAttempt

type SessionEnv = detect.SessionEnv

const (
	SourceAuto    = detect.SourceAuto
	SourceXKB     = detect.SourceXKB
	SourceGNOME   = detect.SourceGNOME
	SourceKDE     = detect.SourceKDE
	SourceIBus    = detect.SourceIBus
	SourceFcitx5  = detect.SourceFcitx5
	SourceSession = detect.SourceSession
)

const (
	StatusFound       = detect.StatusFound
	StatusNotFound    = detect.StatusNotFound
	StatusInactive    = detect.StatusInactive
	StatusError       = detect.StatusError
	StatusUnsupported = detect.StatusUnsupported
)

var (
	ErrNoLayoutSwitchOption    = detect.ErrNoLayoutSwitchOption
	ErrKDEConfigPathUnknown    = detect.ErrKDEConfigPathUnknown
	ErrKDENoShortcutConfigured = detect.ErrKDENoShortcutConfigured
	ErrNoActiveSession         = detect.ErrNoActiveSession
	ErrNoSystemd               = detect.ErrNoSystemd
	ErrNotRoot                 = detect.ErrNotRoot
)

func ScancodesToKeyNames(scancodes []uint16) string {
	return detect.ScancodesToKeyNames(scancodes)
}

func DetectLayoutSwitchKeys(opts *DetectionOptions) (*DetectionResult, error) {
	return detect.DetectLayoutSwitchKeys(opts)
}

func DetectLayoutSwitchScancodes() ([]uint16, error) {
	return detect.DetectLayoutSwitchScancodes()
}

func GetXKBOptions() ([]string, error) {
	return detect.GetXKBOptions()
}

func DetectKDELayoutSwitchKeys() ([]uint16, string, error) {
	return detect.DetectKDELayoutSwitchKeys()
}

func BuildDetectJSONOutput(result *DetectionResult, err error) *DetectJSONOutput {
	return detect.BuildDetectJSONOutput(result, err)
}

func GetActiveSessionEnv() (*SessionEnv, error) {
	return detect.GetActiveSessionEnv()
}

func ApplySessionEnv(env *SessionEnv) error {
	return detect.ApplySessionEnv(env)
}

func RunAsSessionUser(env *SessionEnv, command string, args ...string) ([]byte, error) {
	return detect.RunAsSessionUser(env, command, args...)
}

func RunCommand(env *SessionEnv, command string, args ...string) ([]byte, error) {
	return detect.RunCommand(env, command, args...)
}

func RunGsettings(env *SessionEnv, args ...string) ([]byte, error) {
	return detect.RunGsettings(env, args...)
}
