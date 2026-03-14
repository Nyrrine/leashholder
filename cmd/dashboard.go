package cmd

import (
	"fmt"
	"leash/session"
	"leash/tui"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func RunDashboard() error {
	ptyMgr := session.NewPTYManager()
	defer ptyMgr.CloseAll()

	for {
		model := tui.NewModel(
			func() {
				id := session.GenerateID()
				name := session.PickBranchName()
				cwd, _ := os.Getwd()
				if err := ptyMgr.Spawn(id, name, cwd, nil); err != nil {
					fmt.Fprintf(os.Stderr, "spawn error: %v\n", err)
				}
			},
			func() { RunClean() },
			func(id string) { FocusSession(id) },
		)
		model.IsTabSession = func(id string) bool {
			return ptyMgr.Get(id) != nil
		}

		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("dashboard: %w", err)
		}

		m := finalModel.(tui.Model)
		if m.AttachID != "" {
			ps := ptyMgr.Get(m.AttachID)
			if ps != nil {
				attachSession(ps)
			}
			continue
		}
		break
	}
	return nil
}
