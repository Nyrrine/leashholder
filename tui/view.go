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
	if m.fullView {
		return m.viewFullOutput()
	}
	return m.viewDashboard()
}

func (m Model) viewDashboard() string {
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

			visibleLines := m.visiblePreviewLines()
			preview := info.Preview
			total := len(preview)

			if total == 0 {
				b.WriteString("  " + dimStyle.Render("  (no output yet)") + "\n")
			} else {
				// Apply scroll: scroll=0 means show the latest lines (bottom)
				end := total - m.previewScroll
				if end < 0 {
					end = 0
				}
				start := end - visibleLines
				if start < 0 {
					start = 0
				}

				if start > 0 {
					b.WriteString("  " + dimStyle.Render(fmt.Sprintf("  ↑ %d more lines", start)) + "\n")
				}

				for _, line := range preview[start:end] {
					display := line
					if len(display) > previewWidth-4 {
						display = display[:previewWidth-7] + "..."
					}
					b.WriteString("  " + previewTextStyle.Render("  "+display) + "\n")
				}

				if m.previewScroll > 0 {
					b.WriteString("  " + dimStyle.Render(fmt.Sprintf("  ↓ %d more lines", m.previewScroll)) + "\n")
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
		keyStyle.Render("v") + footerStyle.Render(" full view"),
		keyStyle.Render("s") + footerStyle.Render(" spawn"),
		keyStyle.Render("c") + footerStyle.Render(" clean"),
		keyStyle.Render("q") + footerStyle.Render(" quit"),
	}
	b.WriteString("  " + strings.Join(keys, footerStyle.Render("  ")) + "\n")

	return b.String()
}

func (m Model) viewFullOutput() string {
	var b strings.Builder

	w := m.width
	if w == 0 {
		w = 80
	}

	previewWidth := w - 4
	if previewWidth < 40 {
		previewWidth = 40
	}

	border := previewBorderStyle.Render(strings.Repeat("─", previewWidth))

	// Header
	b.WriteString("  " + border + "\n")

	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		info := m.sessions[m.cursor]
		label := fmt.Sprintf("  %s  %s  %s  %s",
			previewLabelStyle.Render(fmt.Sprintf("#%d", m.cursor+1)),
			renderStatus(info.Status),
			renderMode(info.Mode),
			dimStyle.Render(shortenPath(info.Session.CWD)),
		)
		b.WriteString(label + "\n")
	}

	b.WriteString("  " + border + "\n")

	visible := m.visibleFullViewLines()
	total := len(m.fullViewLines)

	if total == 0 {
		b.WriteString("  " + dimStyle.Render("  (no output)") + "\n")
		for i := 1; i < visible; i++ {
			b.WriteString("\n")
		}
	} else {
		start := m.fullViewScroll
		end := start + visible
		if end > total {
			end = total
		}

		for _, line := range m.fullViewLines[start:end] {
			display := line
			if len(display) > previewWidth-4 {
				display = display[:previewWidth-7] + "..."
			}
			b.WriteString("  " + previewTextStyle.Render("  "+display) + "\n")
		}

		// Pad remaining lines so footer stays at the bottom
		shown := end - start
		for i := shown; i < visible; i++ {
			b.WriteString("\n")
		}
	}

	b.WriteString("  " + border + "\n")

	// Scroll position
	if total > visible {
		endLine := min(m.fullViewScroll+visible, total)
		pos := fmt.Sprintf("lines %d–%d of %d", m.fullViewScroll+1, endLine, total)
		b.WriteString("  " + dimStyle.Render(pos) + "\n")
	} else {
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	keys := []string{
		keyStyle.Render("esc") + footerStyle.Render(" back"),
		keyStyle.Render("↑↓/jk") + footerStyle.Render(" scroll"),
		keyStyle.Render("pgup/pgdn") + footerStyle.Render(" page"),
		keyStyle.Render("home/end") + footerStyle.Render(" jump"),
		keyStyle.Render("r") + footerStyle.Render(" refresh"),
	}
	b.WriteString("  " + strings.Join(keys, footerStyle.Render("  ")) + "\n")

	return b.String()
}
