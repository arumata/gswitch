# Security Policy

gswitch runs as a privileged daemon and reads raw keyboard input from
`/dev/input/*`. Security reports are taken seriously.

## Supported Versions

Only the latest release receives security fixes.

## Reporting a Vulnerability

Please report vulnerabilities privately via
[GitHub Security Advisories](https://github.com/arumata/gswitch/security/advisories/new)
("Report a vulnerability"). Do not open a public issue for security problems.

You can expect an initial response within a few days. Once a fix is released,
the advisory will be published with credit to the reporter (unless you prefer
to stay anonymous).

## Security Model

- **No network access.** The codebase contains no network code; nothing is
  ever transmitted anywhere.
- **In-memory buffer only.** Buffered keystrokes live only in process memory,
  are never written to disk or logs, and the buffer is cleared whenever focus
  can change (mouse clicks, Tab, arrows, Enter, …).
- **What root is used for.** Reading `/dev/input/*`, emitting keys through
  `/dev/uinput`, and integrating with the active graphical session: the
  daemon discovers the session environment (including the user's
  `Xauthority` for X11 selection/clipboard access), and helper commands
  (`gsettings`, clipboard tools) are spawned with privileges dropped to
  the session user. In addition, two privileged entry points exist for
  the tray application, gated by polkit (`pkexec`): `--write-config`
  (writes `/etc/gswitch/default.conf`) and `--systemctl` (an allowlist of
  start/stop/restart/enable/disable for the fixed unit `gswitch.service`).
- **Reproducible artifacts.** Releases are built by CI with GoReleaser
  directly from a git tag; every release ships a `checksums.txt`.
- **Auditable.** The full source of the released code is in this repository.
