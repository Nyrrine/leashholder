package tui

import (
	"leash/session"
	"os"
	"slices"
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
	selectedID  string // track selected session by ID across re-sorts
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
	fullViewFollow bool // auto-scroll to bottom on refresh

	prevStatuses map[string]session.DetectedStatus // for bell detection

	renaming    bool   // text input mode for renaming
	renameInput string // current rename text
}

// previewBuffer is how many preview lines to cache for scrolling.
const previewBuffer = 50

func NewModel(onSpawn, onClean func(), onFocus func(string)) Model {
	return Model{
		OnSpawn:      onSpawn,
		OnClean:      onClean,
		OnFocus:      onFocus,
		prevLogSize:  make(map[string]int64),
		prevStatuses: make(map[string]session.DetectedStatus),
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

// statusPriority returns sort priority: WAITING(0) > IDLE(1) > GENERATING(2) > DONE(3).
func statusPriority(s session.DetectedStatus) int {
	switch s {
	case session.DetectedWaiting:
		return 0
	case session.DetectedIdle:
		return 1
	case session.DetectedGenerating:
		return 2
	case session.DetectedDone:
		return 3
	default:
		return 4
	}
}

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
			status := session.DetectStatus(s, prevSize, currSize)
			if status == session.DetectedIdle && session.DetectWaiting(logPath) {
				status = session.DetectedWaiting
			}
			infos = append(infos, SessionInfo{
				Session: s,
				Status:  status,
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
		if m.renaming {
			return m.updateRename(msg)
		}
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
		// Detect GENERATING → IDLE transitions for bell
		shouldBell := false
		for _, info := range msg.infos {
			id := info.Session.ID
			prev, exists := m.prevStatuses[id]
			if exists && prev == session.DetectedGenerating && (info.Status == session.DetectedIdle || info.Status == session.DetectedWaiting) {
				shouldBell = true
			}
		}

		// Update previous statuses
		newStatuses := make(map[string]session.DetectedStatus, len(msg.infos))
		for _, info := range msg.infos {
			newStatuses[info.Session.ID] = info.Status
		}
		m.prevStatuses = newStatuses

		m.sessions = msg.infos
		m.prevLogSize = msg.newLogSizes

		// Sort: IDLE first, then GENERATING, then DONE; within same status by StartedAt
		slices.SortStableFunc(m.sessions, func(a, b SessionInfo) int {
			pa, pb := statusPriority(a.Status), statusPriority(b.Status)
			if pa != pb {
				return pa - pb
			}
			return a.Session.StartedAt.Compare(b.Session.StartedAt)
		})

		// Restore cursor to tracked session
		if m.selectedID != "" {
			for i, info := range m.sessions {
				if info.Session.ID == m.selectedID {
					m.cursor = i
					break
				}
			}
		}
		if m.cursor >= len(m.sessions) {
			m.cursor = max(0, len(m.sessions)-1)
		}
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			m.selectedID = m.sessions[m.cursor].Session.ID
		}

		// Auto-refresh full view
		if m.fullView && m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			logPath := session.LogPath(info.Session.ID)
			m.fullViewLines = session.CleanLog(logPath, 500)
			if m.fullViewFollow {
				maxScroll := len(m.fullViewLines) - m.visibleFullViewLines()
				if maxScroll < 0 {
					maxScroll = 0
				}
				m.fullViewScroll = maxScroll
			}
		}

		m.err = nil

		if shouldBell {
			return m, emitBell
		}

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
			return m, func() tea.Msg {
				m.OnClean()
				return m.makeRefreshCmd()()
			}
		}

	case "d":
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			if info.Status == session.DetectedDone {
				session.RemoveSession(info.Session.ID)
				return m, m.makeRefreshCmd()
			}
		}

	case "x":
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			if info.Status != session.DetectedDone {
				session.KillSession(info.Session.ID)
				return m, m.makeRefreshCmd()
			}
		}

	case "down":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor + 1) % len(m.sessions)
			m.previewScroll = 0
			m.selectedID = m.sessions[m.cursor].Session.ID
		}

	case "up":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor - 1 + len(m.sessions)) % len(m.sessions)
			m.previewScroll = 0
			m.selectedID = m.sessions[m.cursor].Session.ID
		}

	case "tab":
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor + 1) % len(m.sessions)
			m.previewScroll = 0
			m.selectedID = m.sessions[m.cursor].Session.ID
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
			m.fullViewFollow = true
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

	case "n":
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			m.renaming = true
			m.renameInput = ""
		}
	}

	return m, nil
}

func (m Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.renaming = false
		m.renameInput = ""
	case tea.KeyEnter:
		if m.renameInput != "" && m.cursor >= 0 && m.cursor < len(m.sessions) {
			id := m.sessions[m.cursor].Session.ID
			session.RenameSession(id, m.renameInput)
		}
		m.renaming = false
		m.renameInput = ""
		return m, m.makeRefreshCmd()
	case tea.KeyBackspace:
		if len(m.renameInput) > 0 {
			m.renameInput = m.renameInput[:len(m.renameInput)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.renameInput += string(msg.Runes)
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
		m.fullViewFollow = false
		return m, nil

	case "up", "k":
		if m.fullViewScroll > 0 {
			m.fullViewScroll--
			m.fullViewFollow = false
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
		m.fullViewFollow = false

	case "pgdown":
		m.fullViewScroll += m.visibleFullViewLines()
		if m.fullViewScroll > maxScroll {
			m.fullViewScroll = maxScroll
		}

	case "home":
		m.fullViewScroll = 0
		m.fullViewFollow = false

	case "end", "G":
		m.fullViewScroll = maxScroll
		m.fullViewFollow = true

	case "r":
		// Refresh full view content
		if m.cursor >= 0 && m.cursor < len(m.sessions) {
			info := m.sessions[m.cursor]
			logPath := session.LogPath(info.Session.ID)
			m.fullViewLines = session.CleanLog(logPath, 500)
			newMax := len(m.fullViewLines) - m.visibleFullViewLines()
			if newMax < 0 {
				newMax = 0
			}
			if m.fullViewFollow || m.fullViewScroll > newMax {
				m.fullViewScroll = newMax
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.fullView {
		return m.viewFullOutput()
	}
	return m.viewDashboard()
}

// emitBell writes a bell character directly to the terminal.
func emitBell() tea.Msg {
	if f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		f.Write([]byte("\a"))
		f.Close()
	}
	return nil
}

// visiblePreviewLines returns how many preview lines fit in the dashboard view.
func (m Model) visiblePreviewLines() int {
	if m.height == 0 {
		return 8
	}
	// banner(6) + version(1) + subtitle(1) + blank(1) + header(1) + sessions(N) + blank(1)
	// + border(1) + label(1) + border(1) + content(?) + border(1) + blank(1) + footer(2)
	overhead := 18 + len(m.sessions)
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
	// border(1) + label(1) + border(1) + content(?) + border(1) + scrollinfo(1) + blank(1) + footer(2)
	overhead := 8
	available := m.height - overhead
	if available < 5 {
		return 5
	}
	return available
}
