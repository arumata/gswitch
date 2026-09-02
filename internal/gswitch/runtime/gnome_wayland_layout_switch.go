package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	gnomeWaylandLayoutSwitchAttempts    = 2
	gnomeWaylandLayoutGroupPollAttempts = 20
	gnomeWaylandLayoutGroupPollDelay    = 50 * time.Millisecond
)

type layoutStateReader interface {
	CurrentState() (string, error)
}

type gnomeWaylandLayoutStateReader struct {
	env *SessionEnv
}

var runGsettingsForGNOMEWayland = RunGsettings

func (reader gnomeWaylandLayoutStateReader) CurrentState() (string, error) {
	output, err := runGsettingsForGNOMEWayland(
		reader.env,
		"get",
		"org.gnome.desktop.input-sources",
		"mru-sources",
	)
	if err != nil {
		return "", fmt.Errorf("read GNOME input source: %w", err)
	}
	state := strings.TrimSpace(string(output))
	if state == "" {
		return "", errors.New("read GNOME input source: empty mru-sources")
	}
	return state, nil
}

func triggerAndConfirmGNOMEWaylandLayoutSwitch(
	reader layoutStateReader,
	keys []uint16,
	emit func(KeyEvent) error,
	attempts int,
	pollAttempts int,
	eventDelay time.Duration,
	pause func(time.Duration),
) error {
	if len(keys) == 0 {
		return errors.New("layout switch has no keys")
	}
	if attempts <= 0 || pollAttempts <= 0 {
		return errors.New("GNOME Wayland layout switch confirmation requires positive attempts")
	}

	stateBefore, err := reader.CurrentState()
	if err != nil {
		return err
	}
	for range attempts {
		for _, key := range keys {
			if err := emit(KeyEvent{Code: key, Value: K_DOWN}); err != nil {
				return err
			}
			pause(eventDelay)
		}
		for index := len(keys) - 1; index >= 0; index-- {
			if err := emit(KeyEvent{Code: keys[index], Value: K_UP}); err != nil {
				return err
			}
			pause(eventDelay)
		}

		for range pollAttempts {
			pause(gnomeWaylandLayoutGroupPollDelay)
			state, stateErr := reader.CurrentState()
			if stateErr != nil {
				return stateErr
			}
			if state != stateBefore {
				return nil
			}
		}
	}
	return errors.New("GNOME Wayland input source did not change")
}
