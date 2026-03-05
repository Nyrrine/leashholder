package cmd

import (
	"fmt"
	"leash/tui"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func RunDashboard() error {
	model := tui.NewModel(
		func() { RunSpawn(nil) },
		func() { RunClean() },
		func(id string) { FocusSession(id) },
	)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running dashboard: %v\n", err)
		return err
	}
	return nil
}
