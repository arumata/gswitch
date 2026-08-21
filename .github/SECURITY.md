# Security Policy

gswitch reads raw keyboard input from `/dev/input/*` and can inject input via
`/dev/uinput`. The daemon runs as the active graphical-session user, not as
root, but those device permissions are security-sensitive. Security reports
are taken seriously.

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
- **No root daemon.** The packaged daemon is a systemd user service. udev and
  systemd-logind grant the active local session ACLs for keyboard event nodes
  and `/dev/uinput`; permanent membership in the global `input` group is not
  required. The kernel interfaces require permission on their device nodes,
  not UID 0.
- **Sensitive device authority.** Access to raw keyboard events is equivalent
  to keylogger capability within the accessible session, and access to uinput
  permits synthetic input. Running as an ordinary user limits the impact of a
  daemon compromise but does not make these interfaces safe for untrusted
  code. ACL removal does not revoke file descriptors that are already open;
  logging out stops the user service, but concurrent-session/fast-user-switch
  setups require particular care.
- **What root is still used for.** Package installation installs binaries,
  the systemd user unit, and udev rules. The tray's only privileged runtime
  entry point is the polkit-gated `--write-config`, which writes the fixed
  system configuration file `/etc/gswitch/default.conf`. Service control uses
  `systemctl --user` and does not cross a privilege boundary.
- **Root compatibility mode.** A manual root foreground launch remains
  supported for troubleshooting. Session-specific helpers are then spawned
  with the graphical user's UID, GID, supplementary groups, and environment;
  this is not the packaged or recommended operating mode.
- **Reproducible artifacts.** Releases are built by CI with GoReleaser
  directly from a git tag; every release ships a `checksums.txt`.
- **Auditable.** The full source of the released code is in this repository.
