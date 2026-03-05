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
    Sure, I can help with that. Let me take a look at the
    codebase and figure out what needs to change.
  ──────────────────────────────────────────────────────────

  arrows navigate  enter focus  v full view  s spawn  c clean  q quit
```

## Install

```bash
git clone https://github.com/Nyrrine/leashholder.git
cd leashholder/
go build -o leash .
sudo cp leash /usr/local/bin/
```

Requires **Go 1.24+** to build. The output is a single static binary.

## Commands

| Command | Description |
|---------|-------------|
| `leash` | Opens the live-updating TUI dashboard |
| `leash spawn [-- claude-args...]` | Opens a new Windows Terminal window with a Claude session in the current directory |
| `leash worker --session-id <id> --cwd <path> [-- claude-args...]` | Internal — runs inside the spawned window. Users don't call this directly |
| `leash clean` | Removes session files for dead/finished processes |

## Keybindings

### Dashboard

| Key | Action |
|-----|--------|
| `↑` `↓` | Navigate between sessions |
| `Enter` | Focus the selected session's terminal window |
| `v` | Open full output view for the selected session |
| `PgUp` `PgDn` | Scroll the preview pane |
| `s` | Spawn a new Claude session |
| `c` | Clean finished sessions |
| `q` | Quit |

### Full Output View

| Key | Action |
|-----|--------|
| `↑` `↓` / `j` `k` | Scroll one line |
| `PgUp` `PgDn` | Scroll one page |
| `Home` `End` | Jump to top / bottom |
| `r` | Refresh content |
| `Esc` / `q` | Back to dashboard |

## Status Detection

Status is detected by comparing log file size between 2-second polling intervals:

| Status | Meaning |
|--------|---------|
| **GENERATING** | Log is growing — Claude is outputting text, thinking, or using tools |
| **IDLE** | Log stopped growing — Claude is waiting for your input |
| **DONE** | Process exited |

## Mode Detection

Mode is parsed from the terminal output:

| Mode | Meaning |
|------|---------|
| **code** | Default mode |
| **plan** | Plan mode active |
| **edit** | Accept edits mode active |

## How It Works

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

### Preview Pipeline

The log from `script(1)` captures raw terminal bytes including ANSI escape codes, cursor positioning, and full-screen TUI redraws. The cleaning pipeline extracts readable output:

1. Reads the last 16KB of the log file (efficient tail read)
2. Splits lines on carriage returns to separate Claude's response text from UI chrome that shares the same terminal line
3. Converts cursor-forward ANSI sequences (`ESC[nC`) to spaces
4. Strips all remaining ANSI escape codes and non-printable characters
5. Preserves leading indentation (up to 8 spaces) for code blocks
6. Filters out known UI noise (shortcuts hints, status bars, startup screen, thinking indicators)
7. Deduplicates keystroke echoes and partial-render fragments

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
