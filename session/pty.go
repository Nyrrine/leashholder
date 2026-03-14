package session

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// PTYSession represents a Claude session running in a managed PTY.
type PTYSession struct {
	ID      string
	PTY     *os.File
	Cmd     *exec.Cmd
	LogFile *os.File

	mu       sync.Mutex
	fgWriter io.Writer // when non-nil, output is forwarded to the terminal
	done     chan struct{}
}

// Attach connects this session's output to the given writer (typically os.Stdout).
func (ps *PTYSession) Attach(w io.Writer) {
	ps.mu.Lock()
	ps.fgWriter = w
	ps.mu.Unlock()
}

// Detach disconnects output from the terminal.
func (ps *PTYSession) Detach() {
	ps.mu.Lock()
	ps.fgWriter = nil
	ps.mu.Unlock()
}

// Done returns a channel that closes when the session's process exits.
func (ps *PTYSession) Done() <-chan struct{} {
	return ps.done
}

// outputLoop reads from the PTY and writes to log file + foreground writer.
func (ps *PTYSession) outputLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := ps.PTY.Read(buf)
		if n > 0 {
			ps.LogFile.Write(buf[:n])
			ps.mu.Lock()
			if ps.fgWriter != nil {
				ps.fgWriter.Write(buf[:n])
			}
			ps.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// PTYManager manages all in-process PTY sessions.
type PTYManager struct {
	mu       sync.Mutex
	sessions map[string]*PTYSession
}

// NewPTYManager creates a new PTY manager.
func NewPTYManager() *PTYManager {
	return &PTYManager{
		sessions: make(map[string]*PTYSession),
	}
}

// Spawn creates a new PTY session running Claude.
func (m *PTYManager) Spawn(id, name, cwd string, claudeArgs []string) error {
	if err := EnsureDirs(); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}

	s := &Session{
		ID:        id,
		Name:      name,
		CWD:       cwd,
		StartedAt: time.Now().UTC(),
		Status:    StatusRunning,
	}
	if err := WriteSession(s); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	logFile, err := os.OpenFile(LogPath(id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		RemoveSession(id)
		return fmt.Errorf("open log: %w", err)
	}

	cmd := exec.Command("claude", claudeArgs...)
	cmd.Dir = cwd

	// Get current terminal size for initial PTY size
	size, _ := pty.GetsizeFull(os.Stdin)
	if size == nil {
		size = &pty.Winsize{Rows: 24, Cols: 80}
	}

	ptmx, err := pty.StartWithSize(cmd, size)
	if err != nil {
		logFile.Close()
		RemoveSession(id)
		return fmt.Errorf("start pty: %w", err)
	}

	s.PID = cmd.Process.Pid
	s.ClaudePID = cmd.Process.Pid
	WriteSession(s)

	ps := &PTYSession{
		ID:      id,
		PTY:     ptmx,
		Cmd:     cmd,
		LogFile: logFile,
		done:    make(chan struct{}),
	}

	go ps.outputLoop()
	go func() {
		cmd.Wait()
		close(ps.done)
		if s2, err := ReadSession(id); err == nil {
			s2.Status = StatusDone
			WriteSession(s2)
		}
	}()

	m.mu.Lock()
	m.sessions[id] = ps
	m.mu.Unlock()

	return nil
}

// Get returns the PTYSession for the given ID, or nil.
func (m *PTYManager) Get(id string) *PTYSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// CloseAll terminates all managed sessions.
func (m *PTYManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ps := range m.sessions {
		if ps.Cmd != nil && ps.Cmd.Process != nil {
			ps.Cmd.Process.Signal(syscall.SIGTERM)
		}
		if ps.PTY != nil {
			ps.PTY.Close()
		}
		if ps.LogFile != nil {
			ps.LogFile.Close()
		}
	}
	m.sessions = make(map[string]*PTYSession)
}
