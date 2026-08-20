package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// X11Selection provides access to X11 PRIMARY selection
type X11Selection struct {
	conn   *xgb.Conn
	window xproto.Window
	atoms  struct {
		primary  xproto.Atom
		utf8     xproto.Atom
		targets  xproto.Atom
		propName xproto.Atom
	}
}

// NewX11Selection creates a new X11Selection instance
func NewX11Selection() (*X11Selection, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11: %w", err)
	}

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Create a window to receive selection events
	window, err := xproto.NewWindowId(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create window ID: %w", err)
	}

	err = xproto.CreateWindowChecked(
		conn,
		screen.RootDepth,
		window,
		screen.Root,
		0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwEventMask,
		[]uint32{xproto.EventMaskPropertyChange},
	).Check()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create window: %w", err)
	}

	x := &X11Selection{
		conn:   conn,
		window: window,
	}

	// Get atoms we need
	x.atoms.primary, err = x.getAtom("PRIMARY")
	if err != nil {
		x.Close()
		return nil, err
	}

	x.atoms.utf8, err = x.getAtom("UTF8_STRING")
	if err != nil {
		x.Close()
		return nil, err
	}

	x.atoms.targets, err = x.getAtom("TARGETS")
	if err != nil {
		x.Close()
		return nil, err
	}

	x.atoms.propName, err = x.getAtom("GSWITCH_SEL")
	if err != nil {
		x.Close()
		return nil, err
	}

	return x, nil
}

func (x *X11Selection) getAtom(name string) (xproto.Atom, error) {
	//nolint:gosec // atom names are always short strings
	reply, err := xproto.InternAtom(x.conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("failed to get atom %s: %w", name, err)
	}
	return reply.Atom, nil
}

// ReadPrimary reads the PRIMARY selection (currently selected text)
func (x *X11Selection) ReadPrimary() (string, error) {
	// Check if there's an owner for PRIMARY selection
	owner, err := xproto.GetSelectionOwner(x.conn, x.atoms.primary).Reply()
	if err != nil {
		return "", fmt.Errorf("failed to get selection owner: %w", err)
	}

	// No owner means nothing is selected
	if owner.Owner == xproto.WindowNone {
		return "", nil
	}

	// Request the selection in UTF8 format
	err = xproto.ConvertSelectionChecked(
		x.conn,
		x.window,
		x.atoms.primary,
		x.atoms.utf8,
		x.atoms.propName,
		xproto.TimeCurrentTime,
	).Check()
	if err != nil {
		return "", fmt.Errorf("failed to request selection: %w", err)
	}

	// Wait for SelectionNotify event
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case <-timeout:
			return "", errors.New("timeout waiting for selection")
		default:
			ev, err := x.conn.PollForEvent()
			if err != nil {
				return "", fmt.Errorf("error polling events: %w", err)
			}
			if ev == nil {
				time.Sleep(10 * time.Millisecond)
				continue
			}

			if e, ok := ev.(xproto.SelectionNotifyEvent); ok {
				if !x.isExpectedSelectionNotify(e) {
					continue
				}
				if e.Property == xproto.AtomNone {
					// Selection owner couldn't provide the data in requested format
					return "", nil
				}
				return x.readProperty()
			}
		}
	}
}

func (x *X11Selection) isExpectedSelectionNotify(e xproto.SelectionNotifyEvent) bool {
	return e.Requestor == x.window &&
		e.Selection == x.atoms.primary &&
		e.Target == x.atoms.utf8 &&
		(e.Property == x.atoms.propName || e.Property == xproto.AtomNone)
}

func (x *X11Selection) readProperty() (string, error) {
	// Read the property containing selection data
	reply, err := xproto.GetProperty(
		x.conn,
		true, // delete after reading
		x.window,
		x.atoms.propName,
		xproto.AtomAny,
		0,
		1<<20, // max 1MB
	).Reply()
	if err != nil {
		return "", fmt.Errorf("failed to read property: %w", err)
	}

	return string(reply.Value), nil
}

// Close closes the X11 connection
func (x *X11Selection) Close() {
	if x.conn != nil {
		x.conn.Close()
	}
}
