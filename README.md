# gswitch

`gswitch` is a Linux keyboard layout switcher that fixes text typed in the wrong layout.  
It works at the `/dev/input` level and replays corrected keystrokes through `uinput`.

## Key Features

- System-wide correction in any app
- Double-Shift trigger by default (or custom convert key)
- Last-word, whole-phrase, and selected-text conversion
- Automatic keyboard hotplug handling
- Auto-detection of layout switch key from system settings (`xkb`, `gnome`, `kde`)

## Installation

Requirements:

- Linux
- `uinput` support
- Root access to read `/dev/input/*`
- For selection conversion on pure Wayland: `wl-clipboard`

### From packages (recommended)

Download the latest `.deb` or `.rpm` from [Releases](https://github.com/arumata/gswitch/releases):

```bash
sudo dpkg -i gswitch_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo rpm -i gswitch_<version>_linux_amd64.rpm    # Fedora/openSUSE

sudo gswitch --configure
sudo systemctl enable --now gswitch
```

The package installs the daemon, the tray application, a systemd unit,
udev rules, icons, and a polkit policy.

### With Go

```bash
go install github.com/arumata/gswitch/cmd/gswitch@latest
```

Installs the daemon binary only — no systemd unit, udev rules, or tray.
Clipboard-based selection conversion requires a CGO-enabled build.

### From source

```bash
git clone https://github.com/arumata/gswitch.git
cd gswitch
go build -o builds/gswitch ./cmd/gswitch
```

## Quick Start

```bash
# 1) Interactive setup (writes /etc/gswitch/default.conf)
sudo gswitch --configure

# 2) Run in foreground
sudo gswitch --run

# 3) Run in debug mode (verbose logs)
sudo gswitch --debug

# 4) Detect layout-switch keys as JSON
gswitch --detect-layout-switch
```

## CLI Commands

```text
gswitch --configure
gswitch --run
gswitch --debug
gswitch --version
gswitch --help
gswitch --detect-layout-switch [--source=xkb|gnome|kde]
```

Flags:

- `-c`, `--configure`: interactive configuration
- `-r`, `--run`: run in foreground
- `-d`, `--debug`: run in debug mode
- `-v`, `--version`: print version
- `-h`, `--help`: show help
- `--detect-layout-switch`: detect layout switch keys, JSON output
- `--source=...`: provider for detection (`xkb`, `gnome`, `kde`)

## Configuration

Default config file:

```text
/etc/gswitch/default.conf
```

Public parameters:

- `layout-switch`: key scancode(s) for layout switching (`auto`, `125`, `29+42`)
- `convert-key`: correction trigger key (`0` means double-Shift mode)
- `delay`: delay between synthetic key events in milliseconds (`0..1000`)
- `layout-switch-delay`: extra delay after layout switch in milliseconds (`0..2000`)
- `blacklist`: comma-separated device UIDs to ignore
- `layout1`, `layout2`: optional explicit layout pair for conversion (example: `us`, `ru`, `ua(unicode)`)

Minimal example:

```ini
layout-switch=auto
convert-key=0
delay=10
layout-switch-delay=100
# layout1=us
# layout2=ru
```

## License

MIT. See `LICENSE`.
