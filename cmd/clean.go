package cmd

import (
	"fmt"
	"leash/session"
	"os"
)

func RunClean() error {
	sessions, err := session.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	removed := 0
	failed := 0
	for _, s := range sessions {
		status := session.DetectStatus(s, 0, 0)
		if status == session.DetectedDone {
			if err := session.RemoveSession(s.ID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove session %s: %v\n", s.ID, err)
				failed++
			} else {
				removed++
			}
		}
	}

	fmt.Printf("Cleaned %d finished session(s)\n", removed)
	if failed > 0 {
		fmt.Printf("Failed to clean %d session(s)\n", failed)
	}
	return nil
}
