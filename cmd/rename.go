package cmd

import (
	"fmt"
	"leash/session"
)

func RunRename(id, name string) error {
	if err := session.RenameSession(id, name); err != nil {
		return fmt.Errorf("rename session: %w", err)
	}
	fmt.Printf("Renamed session %s to %s\n", id, name)
	return nil
}
