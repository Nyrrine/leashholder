package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"leash/session"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#c4a7e7"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Bold(true)

	idleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eb6f92")).
			Bold(true)

	generatingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ccfd8")).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Mode styles
	modeDefaultStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))

	modePlanStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4a7e7")).
			Bold(true)

	modeEditStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f6c177")).
			Bold(true)

	indexStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	projectStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	ageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c4a7e7")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	previewBorderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	previewTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250"))

	previewLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#c4a7e7")).
				Bold(true)
)

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Base(p)
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func renderStatus(s session.DetectedStatus) string {
	label := fmt.Sprintf("%-10s", string(s))
	switch s {
	case session.DetectedIdle:
		return idleStyle.Render(label)
	case session.DetectedGenerating:
		return generatingStyle.Render(label)
	case session.DetectedDone:
		return doneStyle.Render(label)
	default:
		return label
	}
}

func renderMode(m session.Mode) string {
	switch m {
	case session.ModePlan:
		return modePlanStyle.Render("plan")
	case session.ModeEdit:
		return modeEditStyle.Render("edit")
	default:
		return modeDefaultStyle.Render("code")
	}
}

func (m Model) View() string {
	var b strings.Builder

	w := m.width
	if w == 0 {
		w = 80
	}

	active := 0
	idle := 0
	for _, info := range m.sessions {
		if info.Status != session.DetectedDone {
			active++
		}
		if info.Status == session.DetectedIdle {
			idle++
		}
	}

	// Title
	title := titleStyle.Render("LEASH")
	if idle > 0 {
		title += idleStyle.Render(fmt.Sprintf("  %d idle", idle))
	}
	b.WriteString(title + "\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("%d active, %d total", active, len(m.sessions))))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(idleStyle.Render(fmt.Sprintf("  error: %v", m.err)))
		b.WriteString("\n\n")
	}

	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("  No sessions. Press s to spawn one."))
		b.WriteString("\n")
	} else {
		// Table header
		b.WriteString(fmt.Sprintf("  %s  %s  %s  %s  %s\n",
			headerStyle.Render(fmt.Sprintf("%2s", "#")),
			headerStyle.Render(fmt.Sprintf("%-10s", "STATUS")),
			headerStyle.Render(fmt.Sprintf("%-4s", "MODE")),
			headerStyle.Render(fmt.Sprintf("%-30s", "PROJECT")),
			headerStyle.Render(fmt.Sprintf("%6s", "AGE")),
		))

		for i, info := range m.sessions {
			isSelected := i == m.cursor
			isDone := info.Status == session.DetectedDone

			project := shortenPath(info.Session.CWD)
			if len(project) > 30 {
				project = "..." + project[len(project)-27:]
			}

			cursor := " "
			if isSelected {
				cursor = titleStyle.Render(">")
			} else if info.Status == session.DetectedIdle {
				cursor = idleStyle.Render("!")
			}

			idx := indexStyle.Render(fmt.Sprintf("%2d", i+1))
			if isSelected {
				idx = selectedStyle.Render(fmt.Sprintf("%2d", i+1))
			}

			status := renderStatus(info.Status)
			mode := renderMode(info.Mode)
			if isDone {
				mode = doneStyle.Render("----")
			}

			projStr := fmt.Sprintf("%-30s", project)
			var proj string
			if isDone {
				proj = doneStyle.Render(projStr)
			} else if isSelected {
				proj = selectedStyle.Render(projStr)
			} else {
				proj = projectStyle.Render(projStr)
			}

			ageStr := fmt.Sprintf("%6s", formatAge(info.Session.StartedAt))
			age := ageStyle.Render(ageStr)
			if isDone {
				age = doneStyle.Render(ageStr)
			}

			b.WriteString(fmt.Sprintf("%s %s  %s  %s  %s  %s\n", cursor, idx, status, mode, proj, age))
		}

		// Preview pane
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			b.WriteString("\n")

			previewWidth := w - 4
			if previewWidth < 40 {
				previewWidth = 40
			}

			border := previewBorderStyle.Render(strings.Repeat("─", previewWidth))
			b.WriteString("  " + border + "\n")

			label := fmt.Sprintf("  %s  %s  %s  %s",
				previewLabelStyle.Render(fmt.Sprintf("#%d", m.cursor+1)),
				renderStatus(info.Status),
				renderMode(info.Mode),
				dimStyle.Render(shortenPath(info.Session.CWD)),
			)
			b.WriteString(label + "\n")
			b.WriteString("  " + border + "\n")

			if len(info.Preview) == 0 {
				b.WriteString("  " + dimStyle.Render("  (no output yet)") + "\n")
			} else {
				for _, line := range info.Preview {
					display := line
					if len(display) > previewWidth-4 {
						display = display[:previewWidth-7] + "..."
					}
					b.WriteString("  " + previewTextStyle.Render("  "+display) + "\n")
				}
			}

			b.WriteString("  " + border + "\n")
		}
	}

	// Footer
	b.WriteString("\n")
	keys := []string{
		keyStyle.Render("arrows") + footerStyle.Render(" navigate"),
		keyStyle.Render("enter") + footerStyle.Render(" focus"),
		keyStyle.Render("s") + footerStyle.Render(" spawn"),
		keyStyle.Render("c") + footerStyle.Render(" clean"),
		keyStyle.Render("q") + footerStyle.Render(" quit"),
	}
	b.WriteString("  " + strings.Join(keys, footerStyle.Render("  ")) + "\n")

	return b.String()
}
