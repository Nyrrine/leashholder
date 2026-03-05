package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LeashDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".leash")
}

func SessionsDir() string {
	return filepath.Join(LeashDir(), "sessions")
}

func LogsDir() string {
	return filepath.Join(LeashDir(), "logs")
}

func EnsureDirs() error {
	if err := os.MkdirAll(SessionsDir(), 0755); err != nil {
		return err
	}
	return os.MkdirAll(LogsDir(), 0755)
}

func GenerateID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func SessionPath(id string) string {
	return filepath.Join(SessionsDir(), id+".json")
}

func LogPath(id string) string {
	return filepath.Join(LogsDir(), id+".log")
}

func WriteSession(s *Session) error {
	if err := EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SessionPath(s.ID), data, 0644)
}

func ReadSession(id string) (*Session, error) {
	data, err := os.ReadFile(SessionPath(id))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func ListSessions() ([]*Session, error) {
	entries, err := os.ReadDir(SessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []*Session
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		s, err := ReadSession(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read session %s: %v\n", id, err)
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func RemoveSession(id string) error {
	os.Remove(SessionPath(id))
	os.Remove(LogPath(id))
	return nil
}
