package session

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

var (
	// Cursor forward: ESC[<n>C — replace with spaces to preserve word gaps
	cursorForwardRe = regexp.MustCompile(`\x1b\[(\d*)C`)
	// All other ANSI sequences
	ansiRe = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\].*?\x07|\[[^@-~]*[@-~])`)
)

func stripANSI(s string) string {
	// Replace cursor-forward (ESC[nC) with spaces to preserve word gaps
	s = cursorForwardRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := cursorForwardRe.FindStringSubmatch(match)
		n := 1
		if len(sub) > 1 && sub[1] != "" {
			if v, err := strconv.Atoi(sub[1]); err == nil {
				n = v
			}
		}
		if n > 10 {
			n = 1 // large jumps are cursor positioning, not word spacing
		}
		return strings.Repeat(" ", n)
	})
	return ansiRe.ReplaceAllString(s, "")
}

// stripNonPrint removes all non-printable characters except space and tab.
func stripNonPrint(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || (unicode.IsPrint(r) && r < 0x2500) {
			// Keep printable ASCII and common unicode, drop box-drawing (U+2500+) and block elements
			return r
		}
		if r >= 0x2500 {
			return -1
		}
		if r < 32 {
			return -1
		}
		return r
	}, s)
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func LogSize(id string) int64 {
	info, err := os.Stat(LogPath(id))
	if err != nil {
		return 0
	}
	return info.Size()
}

func DetectStatus(s *Session, prevLogSize, currLogSize int64) DetectedStatus {
	if s.Status == StatusDone {
		return DetectedDone
	}
	pid := s.PID
	if pid == 0 {
		if time.Since(s.StartedAt) > 30*time.Second {
			return DetectedDone
		}
		return DetectedGenerating
	}
	if !isProcessAlive(pid) {
		return DetectedDone
	}
	if currLogSize != prevLogSize {
		return DetectedGenerating
	}
	return DetectedIdle
}

// DetectMode parses the raw log tail for Claude Code's mode indicator.
func DetectMode(logPath string) Mode {
	raw := lastNLines(logPath, 60)
	// Scan from bottom up for the most recent mode indicator
	for i := len(raw) - 1; i >= 0; i-- {
		clean := stripANSI(raw[i])
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "plan mode on") {
			return ModePlan
		}
		if strings.Contains(lower, "accept edits on") {
			return ModeEdit
		}
		// If we see the prompt area or a mode-off indicator, it's default
		if strings.Contains(lower, "plan mode off") ||
			strings.Contains(lower, "accept edits off") {
			return ModeDefault
		}
	}
	return ModeDefault
}

// wordRe matches sequences of word characters.
var wordRe = regexp.MustCompile(`[a-zA-Z]{2,}`)

// isContentLine returns true if a line looks like actual Claude output text
// (contains multiple real English words, not UI chrome).
func isContentLine(s string) bool {
	if len(s) < 8 {
		return false
	}
	words := wordRe.FindAllString(s, -1)
	return len(words) >= 3
}

// CleanTail returns the last N lines of actual content from a log file.
func CleanTail(path string, n int) []string {
	raw := lastNLines(path, n*20)
	var result []string
	for _, line := range raw {
		clean := stripANSI(line)
		clean = stripNonPrint(clean)
		clean = strings.TrimSpace(clean)
		// Collapse whitespace
		for strings.Contains(clean, "  ") {
			clean = strings.ReplaceAll(clean, "  ", " ")
		}
		if !isContentLine(clean) {
			continue
		}
		// Skip known UI noise patterns
		if isUIChrome(clean) {
			continue
		}
		result = append(result, clean)
	}
	// Dedup: drop line if next line starts with it (keystroke echo)
	deduped := make([]string, 0, len(result))
	for i, line := range result {
		if i+1 < len(result) && strings.HasPrefix(result[i+1], line) {
			continue
		}
		if len(deduped) > 0 && deduped[len(deduped)-1] == line {
			continue
		}
		deduped = append(deduped, line)
	}
	if len(deduped) > n {
		deduped = deduped[len(deduped)-n:]
	}
	return deduped
}

func isUIChrome(s string) bool {
	lower := strings.ToLower(s)
	noisePatterns := []string{
		"shift+tab to cycle",
		"esc to interrupt",
		"esc to cancel",
		"for shortcuts",
		"enter to select",
		"to navigate",
		"? for shortcuts",
		"checking for updates",
		"connector needs auth",
		"connectors need auth",
		"resume this session",
		"script done on",
		"process exited",
		"press ctrl-d",
		"ctrl+d again",
	}
	for _, p := range noisePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func lastNLines(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	const tailSize = 16384
	info, err := f.Stat()
	if err != nil {
		return nil
	}

	offset := info.Size() - tailSize
	if offset < 0 {
		offset = 0
	}
	f.Seek(offset, 0)

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 8*1024), 64*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
