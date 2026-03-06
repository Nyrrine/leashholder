# Contributing

## Prerequisites

- **Go 1.24+**
- **WSL2** on Windows (tested on Ubuntu 24.04)
- **Windows Terminal** (required for spawn/focus features)
- **Claude Code CLI** (`claude`) installed and on PATH

## Building from Source

```bash
git clone https://github.com/Nyrrine/leashholder.git
cd leashholder/
go build -o leash .
```

Run it directly:

```bash
./leash
```

Or install system-wide:

```bash
sudo cp leash /usr/local/bin/
```

## Project Structure

```
leashholder/
├── main.go              # Entry point, command dispatch, arg parsing
├── cmd/
│   ├── dashboard.go     # Runs the TUI (Bubbletea)
│   ├── spawn.go         # Opens new WT window, detects profile/distro
│   ├── worker.go        # Wraps claude via script(1), manages session lifecycle
│   ├── clean.go         # Removes dead session files
│   └── rename.go        # Renames a session
├── session/
│   ├── types.go         # Session struct, status/mode constants
│   ├── registry.go      # Read/write session JSON, list/remove sessions
│   ├── names.go         # Session naming
│   └── status.go        # Status detection, mode detection, log parsing
├── tui/
│   ├── model.go         # Bubbletea model — polling, navigation, keybindings
│   └── view.go          # Bubbletea view — table, preview pane, styling
└── docs/
    ├── architecture.md  # Deep dive into internals
    ├── troubleshooting.md
    └── contributing.md  # You are here
```

For a detailed explanation of data flow, status detection, and the preview pipeline, see [architecture.md](architecture.md).

## Adding a Command

Commands follow a consistent pattern:

1. Create `cmd/<name>.go` with a `RunName(args) error` function
2. Add a case to the `switch` in `main.go`
3. Parse any flags in `main.go` (Leash uses manual arg parsing, not a flag library)

Look at `cmd/clean.go` for a minimal example, or `cmd/spawn.go` for a more complex one.

## Submitting PRs

1. Fork the repo and create a feature branch
2. Make sure `go build -o leash .` compiles cleanly
3. Test your changes manually (automated tests are not yet in place)
4. Keep commits focused -- one logical change per commit
5. Open a PR against `main` with a short description of what and why
