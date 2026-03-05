package cmd

import (
	"fmt"
	"leash/session"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func RunWorker(sessionID, cwd string, claudeArgs []string) error {
	if err := session.EnsureDirs(); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}

	s, err := session.ReadSession(sessionID)
	if err != nil {
		return fmt.Errorf("read session: %w", err)
	}

	s.PID = os.Getpid()
	if err := session.WriteSession(s); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	logPath := session.LogPath(sessionID)

	// Build the claude command string
	claudeCmd := "claude"
	if len(claudeArgs) > 0 {
		claudeCmd += " " + strings.Join(claudeArgs, " ")
	}

	// Use `script` to tee output to a log file while preserving the real TTY.
	// This keeps claude fully interactive (it sees a PTY, not a pipe).
	// -q: quiet (no "Script started" banner)
	// -f: flush after each write (so dashboard can read fresh output)
	// -c: command to run
	cmd := exec.Command("script", "-qfc", claudeCmd, logPath)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start script: %w", err)
	}

	s.ClaudePID = cmd.Process.Pid
	session.WriteSession(s)

	go func() {
		for sig := range sigCh {
			if cmd.Process != nil {
				cmd.Process.Signal(sig)
			}
		}
	}()

	err = cmd.Wait()
	signal.Stop(sigCh)

	s.Status = session.StatusDone
	session.WriteSession(s)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("claude exited with error: %w", err)
	}

	return nil
}
