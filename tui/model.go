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
}

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

const previewLines = 8

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
				Preview: session.CleanTail(logPath, previewLines),
			})
		}
		return sessionsMsg{infos: infos, newLogSizes: newSizes}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			}
		case "up":
			if len(m.sessions) > 0 {
				m.cursor = (m.cursor - 1 + len(m.sessions)) % len(m.sessions)
			}
		case "tab":
			if len(m.sessions) > 0 {
				m.cursor = (m.cursor + 1) % len(m.sessions)
			}
		case "enter":
			if m.OnFocus != nil && m.cursor >= 0 && m.cursor < len(m.sessions) {
				id := m.sessions[m.cursor].Session.ID
				go m.OnFocus(id)
			}
		}

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
