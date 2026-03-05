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
	raw := lastNLinesTail(logPath, 60, 16384)
	for i := len(raw) - 1; i >= 0; i-- {
		clean := stripANSI(raw[i])
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "plan mode on") {
			return ModePlan
		}
		if strings.Contains(lower, "accept edits on") {
			return ModeEdit
		}
		if strings.Contains(lower, "plan mode off") ||
			strings.Contains(lower, "accept edits off") {
			return ModeDefault
		}
	}
	return ModeDefault
}

// wordRe matches sequences of word characters (2+ letters).
var wordRe = regexp.MustCompile(`[a-zA-Z]{2,}`)

// hasMinWords checks if a line has at least min words and a minimum length.
func hasMinWords(s string, minWords int, minLen int) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < minLen {
		return false
	}
	words := wordRe.FindAllString(trimmed, -1)
	return len(words) >= minWords
}

// cleanLine processes a single raw log line, stripping ANSI and non-printable
// characters while preserving leading indentation.
func cleanLine(line string) string {
	clean := stripANSI(line)
	clean = stripNonPrint(clean)
	clean = strings.TrimRight(clean, " \t\r\n")

	// Preserve leading indentation (capped at 8 spaces)
	indent := 0
	for _, c := range clean {
		if c == ' ' {
			indent++
		} else if c == '\t' {
			indent += 4
		} else {
			break
		}
	}
	body := strings.TrimLeft(clean, " \t")
	// Collapse runs of spaces in the body
	for strings.Contains(body, "  ") {
		body = strings.ReplaceAll(body, "  ", " ")
	}
	if indent > 8 {
		indent = 8
	}
	if indent > 0 {
		return strings.Repeat(" ", indent) + body
	}
	return body
}

// dedupLines removes keystroke echo duplicates and fragment overlaps.
func dedupLines(lines []string) []string {
	deduped := make([]string, 0, len(lines))
	for i, line := range lines {
		// Drop if next line starts with this line (keystroke echo)
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], line) {
			continue
		}
		// Drop exact duplicates
		if len(deduped) > 0 && deduped[len(deduped)-1] == line {
			continue
		}
		// Drop short lines that are substrings of nearby lines (fragment overlaps)
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 20 {
			isFragment := false
			for j := max(0, i-5); j < min(len(lines), i+5); j++ {
				if j == i {
					continue
				}
				if strings.Contains(lines[j], trimmed) && len(lines[j]) > len(line) {
					isFragment = true
					break
				}
			}
			if isFragment {
				continue
			}
		}
		deduped = append(deduped, line)
	}
	return deduped
}

// CleanTail returns the last N lines of actual content from a log file.
// Uses a 2-word minimum to filter out fragments and UI noise.
func CleanTail(path string, n int) []string {
	raw := lastNLinesTail(path, n*20, 16384)
	var cleaned []string
	for _, line := range raw {
		clean := cleanLine(line)
		if strings.TrimSpace(clean) == "" {
			continue
		}
		if !hasMinWords(clean, 2, 8) {
			continue
		}
		if isUIChrome(clean) {
			continue
		}
		cleaned = append(cleaned, clean)
	}
	result := dedupLines(cleaned)
	if len(result) > n {
		result = result[len(result)-n:]
	}
	return result
}

// CleanLog returns up to maxLines of cleaned log content for the full view.
// Uses a relaxed 1-word minimum to preserve more content.
func CleanLog(path string, maxLines int) []string {
	raw := lastNLinesTail(path, maxLines*20, 65536)
	var cleaned []string
	for _, line := range raw {
		clean := cleanLine(line)
		if strings.TrimSpace(clean) == "" {
			continue
		}
		if !hasMinWords(clean, 1, 2) {
			continue
		}
		if isUIChrome(clean) {
			continue
		}
		cleaned = append(cleaned, clean)
	}
	result := dedupLines(cleaned)
	if len(result) > maxLines {
		result = result[len(result)-maxLines:]
	}
	return result
}

func isUIChrome(s string) bool {
	lower := strings.ToLower(s)
	noisePatterns := []string{
		// Shortcut hints
		"shift+tab to cycle",
		"esc to interrupt",
		"esc to cancel",
		"for shortcuts",
		"enter to select",
		"to navigate",
		"? for shortcuts",
		// System messages
		"checking for updates",
		"resume this session",
		"script started on",
		"script done on",
		"process exited",
		"press ctrl-d",
		"ctrl+d again",
		// Claude Code startup screen
		"able to read, edit, and execute",
		"security guide",
		"trust this folder",
		"tips for getting started",
		"welcome back",
		"run /init",
		"recent activity",
		"no recent activity",
		// Status bar / model info
		"claude code v",
		"with high effort",
		"with low effort",
		"with standard effort",
		"claude max",
		"claude pro",
		// Thinking / loading indicators
		"drizzling",
		"tempering",
		// MCP / auth / connection
		"mcp server",
		"needs auth",
		"connectors need",
		"connector needs",
		"claude.ai conn",
		// Status bar fragments
		"organization",
		"install-slack",
	}
	for _, p := range noisePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func lastNLinesTail(path string, n int, tailSize int64) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	offset := info.Size() - tailSize
	if offset < 0 {
		offset = 0
	}
	f.Seek(offset, 0)

	var rawLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 8*1024), 128*1024)
	for scanner.Scan() {
		rawLines = append(rawLines, scanner.Text())
	}

	if offset > 0 && len(rawLines) > 0 {
		rawLines = rawLines[1:]
	}

	// Split each line on \r (carriage return). Claude Code's TUI uses cursor
	// positioning with \r to paint multiple screen rows in a single \n-delimited
	// line. Splitting on \r separates the actual response text from UI chrome
	// (status bar, shortcuts hints, etc.) that would otherwise cause the entire
	// line to be filtered out.
	var lines []string
	for _, line := range rawLines {
		parts := strings.Split(line, "\r")
		lines = append(lines, parts...)
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
