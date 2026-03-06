package session

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
)

type DetectedStatus string

const (
	DetectedIdle       DetectedStatus = "IDLE"
	DetectedWaiting    DetectedStatus = "WAITING"
	DetectedGenerating DetectedStatus = "GENERATING"
	DetectedDone       DetectedStatus = "DONE"
)

type Mode string

const (
	ModeDefault Mode = "default"
	ModePlan    Mode = "plan"
	ModeEdit    Mode = "edit"
)

type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	ClaudePID int       `json:"claude_pid"`
	CWD       string    `json:"cwd"`
	StartedAt time.Time `json:"started_at"`
	Status    Status    `json:"status"`
}
