# Leash

A terminal dashboard for monitoring multiple Claude Code sessions in parallel. Built in Go — single binary, zero runtime dependencies.

Instead of tab-cycling to check which Claude sessions need your attention, Leash auto-discovers your sessions, shows their status (IDLE / GENERATING / DONE), detects their mode (code / plan / edit), previews their output, and lets you jump to any session with Enter.

## Demo

```
LEASH  1 idle
1 active, 2 total

   #  STATUS      MODE  PROJECT                         AGE
>  1  IDLE        plan  ~/projects/api-server             5m
!  2  GENERATING  code  ~/projects/frontend               12m
   3  DONE        ----  ~/Documents/Lord-of-Claudeyuan    2m

  ──────────────────────────────────────────────────────────
  #1  IDLE        plan  ~/projects/api-server
  ──────────────────────────────────────────────────────────
    You know what's really wonderful? The fact that you're
    here, working on things that matter to you.
  ──────────────────────────────────────────────────────────

  arrows navigate  enter focus  s spawn  c clean  q quit
```

## Install

```bash
cd leash/
go build -o leash .
sudo cp leash /usr/local/bin/
```

Requires Go 1.24+ to build. The output is a single static binary.

## Commands

| Command | Description |
|---------|-------------|
| `leash` | Opens the live-updating TUI dashboard |
| `leash spawn [-- claude-args...]` | Opens a new Windows Terminal window with a Claude session in the current directory |
| `leash worker --session-id <id> --cwd <path> [-- claude-args...]` | Internal — runs inside the spawned window. Users don't call this directly |
| `leash clean` | Removes session files for dead/finished processes |

## Dashboard

- **Arrow keys** — navigate between sessions
- **Enter** — focus the selected session's terminal window
- **s** — spawn a new Claude session
- **c** — clean finished sessions
- **q** — quit

### Status Detection

Status is detected by comparing log file size between 2-second polling intervals:

| Status | Meaning |
|--------|---------|
| **GENERATING** | Log is growing — Claude is outputting text, thinking, or using tools |
| **IDLE** | Log stopped growing — Claude is waiting for your input |
| **DONE** | Process exited |

### Mode Detection

Mode is parsed from the terminal output:

| Mode | Meaning |
|------|---------|
| **code** | Default mode |
| **plan** | Plan mode active (`plan mode on` detected) |
| **edit** | Accept edits mode active (`accept edits on` detected) |

### Preview Pane

The preview shows the last 8 lines of meaningful Claude output for the selected session. The log is captured via `script(1)` which records terminal output while keeping Claude fully interactive. The preview pipeline:

1. Reads the last 16KB of the log file (efficient tail read)
2. Converts cursor-forward ANSI sequences (`ESC[nC`) to spaces
3. Strips all remaining ANSI escape codes
4. Removes non-printable and box-drawing characters
5. Filters out lines with fewer than 3 real words (kills UI chrome)
6. Filters out known noise patterns (shortcuts hints, status bars, etc.)
7. Deduplicates incremental keystroke echoes

## Architecture

### Session Registry

```
~/.leash/
├── sessions/
│   └── <id>.json      # Session metadata (PID, cwd, status, timestamps)
└── logs/
    └── <id>.log        # Terminal output captured by script(1)
```

### Spawn Flow

1. `leash spawn` generates a session ID and writes `~/.leash/sessions/<id>.json`
2. Detects your default Windows Terminal profile (by GUID) and WSL distro name
3. Launches: `wt.exe -w leash-<id> new-tab --profile <guid> -- wsl.exe -e bash -li -c "leash worker ..."`
4. The new window opens with your terminal rice intact (custom profile, colors, font, etc.)
5. `leash worker` updates the session JSON with its PID, then runs `script -qfc "claude <args>" <logfile>`
6. Claude runs fully interactively — the user works with it as normal
7. On exit, the session status is set to `done`

### Focus

Each session gets a named Windows Terminal window (`leash-<id>`). Pressing Enter in the dashboard runs `wt.exe -w leash-<id> focus-tab` to bring it to the foreground.

## Project Structure

```
leash/
├── main.go              # Entry point, command dispatch
├── cmd/
│   ├── dashboard.go     # Runs the TUI (Bubbletea)
│   ├── spawn.go         # Opens new WT window, detects profile/distro
│   ├── worker.go        # Wraps claude via script(1), manages session lifecycle
│   └── clean.go         # Removes dead session files
├── session/
│   ├── types.go         # Session struct, status/mode constants
│   ├── registry.go      # Read/write session JSON, list/remove sessions
│   └── status.go        # Status detection, mode detection, log parsing
└── tui/
    ├── model.go         # Bubbletea model — polling, navigation, keybindings
    └── view.go          # Bubbletea view — table, preview pane, styling
```

## Dependencies

- [bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [lipgloss](https://github.com/charmbracelet/lipgloss) — Terminal styling

Everything else is Go standard library.

## Requirements

- **WSL2** on Windows (tested on Ubuntu 24.04)
- **Windows Terminal** (for spawn/focus features)
- **Claude Code CLI** (`claude`) installed and on PATH
- **Go 1.24+** to build

## License

MIT
