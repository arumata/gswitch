package runtime

import (
	"os/exec"

	cfg "github.com/arumata/gswitch/internal/gswitch/config"
	"github.com/arumata/gswitch/internal/gswitch/detect"
)

type Config = cfg.Config
type LayoutSpec = cfg.LayoutSpec
type DetectionResult = detect.DetectionResult
type DetectionOptions = detect.DetectionOptions
type DetectionSource = detect.DetectionSource
type SessionEnv = detect.SessionEnv

const (
	SourceGNOME = detect.SourceGNOME
	SourceXKB   = detect.SourceXKB
)

var (
	ErrNoActiveSession           = detect.ErrNoActiveSession
	ErrNoSystemd                 = detect.ErrNoSystemd
	ErrNoLayoutSwitchOption      = detect.ErrNoLayoutSwitchOption
	ensureGNOMEX11Launch7Binding = detect.EnsureGNOMEX11Launch7Binding
)

func GetActiveSessionEnv() (*SessionEnv, error) {
	return detect.GetActiveSessionEnv()
}

func ApplySessionEnv(env *SessionEnv) error {
	return detect.ApplySessionEnv(env)
}

func CommandAsSessionUser(env *SessionEnv, name string, args ...string) (*exec.Cmd, error) {
	return detect.CommandAsSessionUser(env, name, args...)
}

func DetectLayoutSwitchKeys(opts *DetectionOptions) (*DetectionResult, error) {
	return detect.DetectLayoutSwitchKeys(opts)
}
