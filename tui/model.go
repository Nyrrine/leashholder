package tui

import (
	"leash/session"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type tickMsg time.Time

type SessionInfo struct {
	Session *session.Session
	Status  session.DetectedStatus
	Mode    session.Mode
	Preview []string
}

type Model struct {
	sessions    []SessionInfo
	prevLogSize map[string]int64 // id -> log size from previous tick
	cursor      int
	width       int
	height      int
	err         error
	OnSpawn     func()
	OnClean     func()
	OnFocus     func(id string)

	previewScroll int // 0 = bottom (newest), positive = lines scrolled up

	fullView       bool
	fullViewLines  []string
	fullViewScroll int // 0 = top
}

// previewBuffer is how many preview lines to cache for scrolling.
const previewBuffer = 50

func NewModel(onSpawn, onClean func(), onFocus func(string)) Model {
	return Model{
		OnSpawn:     onSpawn,
		OnClean:     onClean,
		OnFocus:     onFocus,
		prevLogSize: make(map[string]int64),
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.makeRefreshCmd(), tickCmd())
}

type sessionsMsg struct {
	infos       []SessionInfo
	newLogSizes map[string]int64
}
type errMsg error

// makeRefreshCmd creates a refresh command that captures current prevLogSize.
func (m Model) makeRefreshCmd() tea.Cmd {
	prev := make(map[string]int64, len(m.prevLogSize))
	for k, v := range m.prevLogSize {
		prev[k] = v
	}
	return func() tea.Msg {
		sessions, err := session.ListSessions()
		if err != nil {
			return errMsg(err)
		}
		newSizes := make(map[string]int64, len(sessions))
		var infos []SessionInfo
		for _, s := range sessions {
			currSize := session.LogSize(s.ID)
			prevSize := prev[s.ID]
			newSizes[s.ID] = currSize

			logPath := session.LogPath(s.ID)
			infos = append(infos, SessionInfo{
				Session: s,
				Status:  session.DetectStatus(s, prevSize, currSize),
				Mode:    session.DetectMode(logPath),
				Preview: session.CleanTail(logPath, previewBuffer),
			})
		}
		return sessionsMsg{infos: infos, newLogSizes: newSizes}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.fullView {
			return m.updateFullView(msg)
		}
		return m.updateDashboard(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tea.Batch(m.makeRefreshCmd(), tickCmd())

	case sessionsMsg:
		m.sessions = msg.infos
		m.prevLogSize = msg.newLogSizes
		if m.cursor >= len(msg.infos) {
			m.cursor = max(0, len(msg.infos)-1)
		}
		m.err = nil

	case errMsg:
		m.err = msg
	}

	return m, nil
}

func (m Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "s":
		if m.OnSpawn != nil {
			go m.OnSpawn()
		}
		return m, nil

	case "c":
		if m.OnClean != nil {
			go m.OnClean()
		}
		return m, tea.Batch(
			func() tea.Msg { time.Sleep(200 * time.Millisecond); return m.makeRefreshCmd()() },
		)

	case "down":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor + 1) % len(m.sessions)
			m.previewScroll = 0
		}

	case "up":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor - 1 + len(m.sessions)) % len(m.sessions)
			m.previewScroll = 0
		}

	case "tab":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor + 1) % len(m.sessions)
			m.previewScroll = 0
		}

	case "enter":
		if m.OnFocus != nil && m.cursor >= 0 && m.cursor < len(m.sessions) {
			id := m.sessions[m.cursor].Session.ID
			go m.OnFocus(id)
		}

	case "v":
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			logPath := session.LogPath(info.Session.ID)
			m.fullView = true
			m.fullViewLines = session.CleanLog(logPath, 500)
			// Start at bottom
			maxScroll := len(m.fullViewLines) - m.visibleFullViewLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.fullViewScroll = maxScroll
		}

	case "pgup":
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			m.previewScroll += m.visiblePreviewLines()
			maxScroll := len(m.sessions[m.cursor].Preview) - m.visiblePreviewLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.previewScroll > maxScroll {
				m.previewScroll = maxScroll
			}
		}

	case "pgdown":
		m.previewScroll -= m.visiblePreviewLines()
		if m.previewScroll < 0 {
			m.previewScroll = 0
		}
	}

	return m, nil
}

func (m Model) updateFullView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxScroll := len(m.fullViewLines) - m.visibleFullViewLines()
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc", "q":
		m.fullView = false
		m.fullViewLines = nil
		m.fullViewScroll = 0
		return m, nil

	case "up", "k":
		if m.fullViewScroll > 0 {
			m.fullViewScroll--
		}

	case "down", "j":
		if m.fullViewScroll < maxScroll {
			m.fullViewScroll++
		}

	case "pgup":
		m.fullViewScroll -= m.visibleFullViewLines()
		if m.fullViewScroll < 0 {
			m.fullViewScroll = 0
		}

	case "pgdown":
		m.fullViewScroll += m.visibleFullViewLines()
		if m.fullViewScroll > maxScroll {
			m.fullViewScroll = maxScroll
		}

	case "home":
		m.fullViewScroll = 0

	case "end":
		m.fullViewScroll = maxScroll

	case "r":
		// Refresh full view content
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			logPath := session.LogPath(info.Session.ID)
			m.fullViewLines = session.CleanLog(logPath, 500)
			if m.fullViewScroll > maxScroll {
				m.fullViewScroll = maxScroll
			}
		}
	}

	return m, nil
}

// visiblePreviewLines returns how many preview lines fit in the dashboard view.
func (m Model) visiblePreviewLines() int {
	if m.height == 0 {
		return 8
	}
	// title(1) + subtitle(1) + blank(1) + header(1) + sessions(N) + blank(1)
	// + border(1) + label(1) + border(1) + content(?) + border(1) + blank(1) + footer(1)
	overhead := 11 + len(m.sessions)
	available := m.height - overhead
	if available < 4 {
		return 4
	}
	if available > 20 {
		return 20
	}
	return available
}

// visibleFullViewLines returns how many content lines fit in the full view.
func (m Model) visibleFullViewLines() int {
	if m.height == 0 {
		return 20
	}
	// border(1) + label(1) + border(1) + content(?) + border(1) + scrollinfo(1) + blank(1) + footer(1)
	overhead := 7
	available := m.height - overhead
	if available < 5 {
		return 5
	}
	return available
}
