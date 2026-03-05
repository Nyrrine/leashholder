# Architecture

## Overview

Leash is a session monitor for Claude Code. It has four components:

1. **Dashboard** (`leash`) — a Bubbletea TUI that polls session state every 2 seconds
2. **Spawner** (`leash spawn`) — creates a session and opens a new Windows Terminal window
3. **Worker** (`leash worker`) — runs inside the spawned window, wraps Claude with `script(1)`
4. **Cleaner** (`leash clean`) — removes session files for dead processes

## Data Flow

```
leash spawn
    │
    ├── Creates ~/.leash/sessions/<id>.json
    ├── Detects WSL distro name (wsl.exe -l -q)
    ├── Detects default WT profile GUID (reads settings.json via powershell)
    └── Launches: wt.exe -w leash-<id> new-tab --profile <guid> -- wsl.exe -e bash -li -c "leash worker ..."
                    │
                    └── leash worker
                            │
                            ├── Updates session JSON with PID
                            ├── Runs: script -qfc "claude <args>" ~/.leash/logs/<id>.log
                            │           │
                            │           ├── Claude runs interactively (real PTY via script)
                            │           └── All terminal output tee'd to log file
                            │
                            └── On exit: sets session status to "done"

leash (dashboard)
    │
    ├── Every 2s: reads all ~/.leash/sessions/*.json
    ├── For each session:
    │   ├── kill -0 <pid> → alive?
    │   ├── Compare log size to previous tick → GENERATING or IDLE
    │   ├── Parse raw log tail for "plan mode on" / "accept edits on" → mode
    │   └── CleanTail(log) → preview lines
    └── Renders TUI with Bubbletea + Lip Gloss
```

## Status Detection

Previous iterations tried parsing Claude's terminal output for specific patterns (prompt detection, spinner detection, etc.). This was unreliable because:

- Claude's TUI uses heavy ANSI cursor manipulation
- `script(1)` captures raw terminal bytes including redraws
- Pattern matching on this output is fragile

The current approach is simple and reliable:

1. **Is the process alive?** (`kill -0 <pid>`) — if not, it's DONE
2. **Did the log file grow since last check?** — if yes, GENERATING; if no, IDLE

This works because when Claude is generating output (thinking, streaming text, running tools), the terminal receives data which `script` writes to the log. When Claude is idle (waiting at the prompt), no output is produced.

## Mode Detection

Mode is detected by scanning the raw log (before the aggressive content filtering) for Claude Code's status bar text:

- `plan mode on` → PLAN
- `accept edits on` → EDIT
- Neither / `plan mode off` / `accept edits off` → DEFAULT (code)

The scan goes from the bottom of the log upward and returns the first match, giving us the most recent mode.

## Preview Pipeline

The raw log from `script(1)` is extremely noisy — full of ANSI escape codes, cursor positioning, box-drawing characters, and UI chrome. The cleaning pipeline:

1. **Read tail** — only the last 16KB of the file (seek to end - 16KB, read forward)
2. **Cursor-to-space** — `ESC[nC` (cursor forward n positions) is replaced with n spaces. Claude uses this between words instead of literal space characters
3. **Strip ANSI** — remove all remaining escape sequences
4. **Strip non-printable** — remove control chars, box-drawing (U+2500+), block elements
5. **Collapse whitespace** — multiple spaces → single space
6. **Content filter** — only keep lines with 3+ real English words (2+ chars each)
7. **UI chrome filter** — drop lines matching known noise patterns
8. **Dedup** — if line N is a prefix of line N+1, drop N (keystroke echo from `script`)

## Window Management

Each spawned session gets a named Windows Terminal window: `leash-<id>`.

- Spawn: `wt.exe -w leash-<id> new-tab ...`
- Focus: `wt.exe -w leash-<id> focus-tab -t 0`

The profile GUID is auto-detected from Windows Terminal's `settings.json` so spawned windows use the user's custom terminal theme.

## Why `script(1)` instead of Go io.MultiWriter?

Claude Code detects whether it's running in a real terminal (TTY) or a pipe. If stdout is piped (e.g., through Go's `io.MultiWriter`), Claude refuses to start in interactive mode.

`script(1)` allocates a real PTY, so Claude sees a terminal and runs normally. The `-q` flag suppresses the "Script started" banner, and `-f` flushes output immediately so the dashboard can read fresh data.
