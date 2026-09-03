package runtime

import (
	"errors"
	"fmt"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	xkbExtensionName        = "XKEYBOARD"
	xkbUseExtensionOpcode   = 0
	xkbGetStateOpcode       = 4
	xkbUseCoreKeyboard      = 0x0100
	xkbProtocolMajor        = 1
	layoutSwitchAttempts    = 3
	layoutGroupPollAttempts = 200
	layoutGroupPollDelay    = 10 * time.Millisecond
)

type layoutGroupReader interface {
	CurrentGroup() (uint8, error)
}

type x11LayoutGroupReader struct {
	conn      *xgb.Conn
	xkbOpcode byte
}

func newX11LayoutGroupReader() (*x11LayoutGroupReader, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect to X11: %w", err)
	}

	closeOnError := true
	defer func() {
		if closeOnError {
			conn.Close()
		}
	}()

	extension, err := xproto.QueryExtension(
		conn,
		uint16(len(xkbExtensionName)),
		xkbExtensionName,
	).Reply()
	if err != nil {
		return nil, fmt.Errorf("query XKEYBOARD extension: %w", err)
	}
	if extension == nil || !extension.Present {
		return nil, errors.New("XKEYBOARD extension is unavailable")
	}

	reader := &x11LayoutGroupReader{conn: conn, xkbOpcode: extension.MajorOpcode}
	if err := reader.useXKBExtension(); err != nil {
		return nil, err
	}
	closeOnError = false
	return reader, nil
}

func (reader *x11LayoutGroupReader) Close() {
	reader.conn.Close()
}

func (reader *x11LayoutGroupReader) CurrentGroup() (uint8, error) {
	reply, err := reader.requestReply(xkbGetStateRequest(reader.xkbOpcode))
	if err != nil {
		return 0, fmt.Errorf("get XKB keyboard state: %w", err)
	}
	const groupOffset = 12
	if len(reply) <= groupOffset {
		return 0, fmt.Errorf("get XKB keyboard state: short reply (%d bytes)", len(reply))
	}
	return reply[groupOffset], nil
}

func (reader *x11LayoutGroupReader) useXKBExtension() error {
	reply, err := reader.requestReply(xkbUseExtensionRequest(reader.xkbOpcode))
	if err != nil {
		return fmt.Errorf("initialize XKEYBOARD extension: %w", err)
	}
	if len(reply) < 12 {
		return fmt.Errorf("initialize XKEYBOARD extension: short reply (%d bytes)", len(reply))
	}
	if reply[1] != 1 {
		return errors.New("XKEYBOARD 1.0 is unsupported by the X server")
	}
	return nil
}

func (reader *x11LayoutGroupReader) requestReply(request []byte) ([]byte, error) {
	cookie := reader.conn.NewCookie(true, true)
	reader.conn.NewRequest(request, cookie)
	return cookie.Reply()
}

func xkbUseExtensionRequest(opcode byte) []byte {
	request := make([]byte, 8)
	request[0] = opcode
	request[1] = xkbUseExtensionOpcode
	xgb.Put16(request[2:], 2)
	xgb.Put16(request[4:], xkbProtocolMajor)
	return request
}

func xkbGetStateRequest(opcode byte) []byte {
	request := make([]byte, 8)
	request[0] = opcode
	request[1] = xkbGetStateOpcode
	xgb.Put16(request[2:], 2)
	xgb.Put16(request[4:], xkbUseCoreKeyboard)
	return request
}

func triggerAndConfirmLayoutSwitch(
	reader layoutGroupReader,
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
		return errors.New("layout switch confirmation requires positive attempts")
	}

	groupBefore, err := reader.CurrentGroup()
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
			pause(layoutGroupPollDelay)
			group, groupErr := reader.CurrentGroup()
			if groupErr != nil {
				return groupErr
			}
			if group != groupBefore {
				return nil
			}
		}
	}
	return errors.New("GNOME X11 layout group did not change")
}
