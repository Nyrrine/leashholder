# Troubleshooting

## Spawn fails with "file not found" error

**Symptom:** `leash spawn` prints an error like `error 2147942402 (0x80070002)`.

**Cause:** Usually a quoting issue with paths containing spaces when calling `wt.exe` from WSL.

**Fix:** Leash shells out through `bash -c` to handle quoting. Make sure `wt.exe` and `wsl.exe` are on your PATH. Run `which wt.exe` to verify.

## Spawned terminal doesn't use my theme/rice

**Symptom:** New window opens with default colors/font instead of your customized profile.

**Cause:** Leash auto-detects your default Windows Terminal profile by reading the `defaultProfile` GUID from `settings.json`. If you have multiple profiles with the same name (e.g., two "Ubuntu" entries), it might pick the wrong one.

**Fix:** Ensure your preferred profile is set as the default in Windows Terminal settings (Ctrl+,).

## Claude says "Input must be provided through stdin"

**Symptom:** The spawned terminal shows a Claude error about stdin instead of starting interactively.

**Cause:** This happened in an earlier version where Go's `io.MultiWriter` was used to pipe Claude's output. Claude detected it wasn't a real TTY and refused to start.

**Fix:** This is fixed — Leash now uses `script(1)` which allocates a real PTY. If you still see this, make sure you're running the latest build.

## Sessions show as GENERATING when Claude is idle

**Symptom:** Dashboard shows GENERATING even though Claude is sitting at the prompt.

**Cause:** On the first poll after starting the dashboard, all sessions show as GENERATING because there's no previous log size to compare against. After one tick (2 seconds), it should stabilize.

## `leash clean` doesn't remove sessions

**Symptom:** Stale sessions persist after running `leash clean`.

**Cause:** The session's PID is 0 (worker never started) and it's less than 30 seconds old, so it's not yet considered dead.

**Fix:** Wait 30 seconds or manually remove files: `rm ~/.leash/sessions/*.json`

## Enter doesn't focus the session window

**Symptom:** Pressing Enter in the dashboard does nothing.

**Cause:** The session was spawned before the named-window feature was added (sessions used `-w new` instead of `-w leash-<id>`).

**Fix:** Clean old sessions and spawn fresh ones.

## Preview is empty or shows "(no output yet)"

**Symptom:** The preview pane is blank even though Claude has produced output.

**Cause:** The content filter is aggressive — it requires lines with 3+ English words and drops anything that looks like UI chrome. Very short Claude responses might get filtered.

**Fix:** This is a tradeoff for clean output. The preview is a summary, not a full transcript. Focus the session (Enter) to see everything.
