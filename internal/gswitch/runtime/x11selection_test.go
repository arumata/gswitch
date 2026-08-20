package runtime

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestX11SelectionIsExpectedSelectionNotify(t *testing.T) {
	x := &X11Selection{window: xproto.Window(10)}
	x.atoms.primary = xproto.Atom(20)
	x.atoms.utf8 = xproto.Atom(30)
	x.atoms.propName = xproto.Atom(40)

	tests := []struct {
		name  string
		event xproto.SelectionNotifyEvent
		want  bool
	}{
		{
			name: "matches expected atoms",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(10),
				Selection: xproto.Atom(20),
				Target:    xproto.Atom(30),
				Property:  xproto.Atom(40),
			},
			want: true,
		},
		{
			name: "none property still matches",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(10),
				Selection: xproto.Atom(20),
				Target:    xproto.Atom(30),
				Property:  xproto.AtomNone,
			},
			want: true,
		},
		{
			name: "different requestor rejected",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(11),
				Selection: xproto.Atom(20),
				Target:    xproto.Atom(30),
				Property:  xproto.Atom(40),
			},
			want: false,
		},
		{
			name: "different selection rejected",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(10),
				Selection: xproto.Atom(21),
				Target:    xproto.Atom(30),
				Property:  xproto.Atom(40),
			},
			want: false,
		},
		{
			name: "different target rejected",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(10),
				Selection: xproto.Atom(20),
				Target:    xproto.Atom(31),
				Property:  xproto.Atom(40),
			},
			want: false,
		},
		{
			name: "unexpected property rejected",
			event: xproto.SelectionNotifyEvent{
				Requestor: xproto.Window(10),
				Selection: xproto.Atom(20),
				Target:    xproto.Atom(30),
				Property:  xproto.Atom(41),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := x.isExpectedSelectionNotify(tt.event)
			if got != tt.want {
				t.Fatalf("isExpectedSelectionNotify() = %v, want %v", got, tt.want)
			}
		})
	}
}
