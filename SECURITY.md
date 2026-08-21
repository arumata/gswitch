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
- **Root is required only for input access.** Privileges are needed to read
  `/dev/input/*` and write to `/dev/uinput`; there is no other privileged
  behavior.
- **Reproducible artifacts.** Releases are built by CI with GoReleaser
  directly from a git tag; every release ships a `checksums.txt`.
- **Auditable.** The full source of the released code is in this repository.
