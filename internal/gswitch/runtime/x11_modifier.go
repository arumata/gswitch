package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/jezek/xgb/xproto"
)

const (
	modifierReleasePollAttempts = 200
	modifierReleasePollDelay    = 10 * time.Millisecond
)

type modifierStateReader interface {
	ModifierMask() (uint16, error)
}

func (x *X11Selection) ModifierMask() (uint16, error) {
	root := xproto.Setup(x.conn).DefaultScreen(x.conn).Root
	reply, err := xproto.QueryPointer(x.conn, root).Reply()
	if err != nil {
		return 0, fmt.Errorf("query X11 modifier state: %w", err)
	}
	if reply == nil {
		return 0, errors.New("query X11 modifier state returned no reply")
	}
	return reply.Mask, nil
}

func waitForSelectionModifiersReleased(
	reader modifierStateReader,
	attempts int,
	pause func(time.Duration),
) error {
	if attempts <= 0 {
		return errors.New("modifier release wait requires at least one attempt")
	}

	const selectionModifiers = xproto.ModMaskControl | xproto.ModMaskShift
	for attempt := range attempts {
		mask, err := reader.ModifierMask()
		if err != nil {
			return err
		}
		if mask&selectionModifiers == 0 {
			return nil
		}
		if attempt < attempts-1 {
			pause(modifierReleasePollDelay)
		}
	}

	return errors.New("timed out waiting for X11 Ctrl/Shift release")
}
