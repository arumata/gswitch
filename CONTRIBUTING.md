# Contributing to gswitch

Thanks for your interest in improving gswitch!

## How this repository works

Development happens in a private working tree; this repository receives each
release as a single squashed commit. This has two practical consequences:

- **Issues are the best way to contribute.** Bug reports (please use the
  template — environment details matter a lot) and feature requests are
  read and acted on.
- **Pull requests are welcome but are not merged directly.** An accepted
  change is integrated into the working tree and lands in the next release
  commit, with credit to the author in the release notes. For anything
  non-trivial, please open an issue first so we can agree on the approach
  before you spend time on code.

## Building and testing

Requirements: Go (version from `go.mod`), and for the tray application the
GTK3 headers (`libgtk-3-dev`, `libxkbcommon-dev`, `libx11-dev` on
Debian/Ubuntu).

```bash
go build -o gswitch ./cmd/gswitch            # daemon
go build -o gswitch-tray ./cmd/gswitch-tray  # tray (needs GTK3 headers)
go vet ./...
go test ./...
```

Linting uses [golangci-lint](https://golangci-lint.run/) v2 with the config
in `.golangci.yml`:

```bash
golangci-lint run ./...
```

To try your build: `sudo ./gswitch --debug` (root is needed for
`/dev/input/*`).

## Guidelines

- Keep code `gofmt`-clean and passing `.golangci.yml`.
- Prefer table-driven tests with deterministic inputs; tests must not require
  root, `/dev/input`, or a running desktop session.
- Don't edit generated files (`*_generated.go`) by hand.
- Treat changes as security-sensitive: gswitch runs privileged and reads raw
  input. No network code, no persisting of keystrokes — see
  [SECURITY.md](SECURITY.md).

## Conduct

Be respectful. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
