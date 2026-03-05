package cmd

import (
	"fmt"
	"leash/session"
)

func RunClean() error {
	sessions, err := session.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	removed := 0
	for _, s := range sessions {
		status := session.DetectStatus(s, 0, 0)
		if status == session.DetectedDone {
			session.RemoveSession(s.ID)
			removed++
		}
	}

	fmt.Printf("Cleaned %d finished session(s)\n", removed)
	return nil
}
