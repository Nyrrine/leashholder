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
	// SGR (Select Graphic Rendition) sequences — colors and text style
	sgrRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
)

func replaceCursorForward(match string) string {
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
}

func stripANSI(s string) string {
	s = cursorForwardRe.ReplaceAllStringFunc(s, replaceCursorForward)
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

// DetectWaiting checks if an idle session is waiting for tool approval
// by looking for approval prompt indicators in the log tail.
func DetectWaiting(logPath string) bool {
	raw := lastNLinesTail(logPath, 30, 4096)
	hasAllow := false
	hasDeny := false
	for i := len(raw) - 1; i >= 0 && i >= len(raw)-15; i-- {
		clean := stripANSI(raw[i])
		lower := strings.ToLower(clean)
		if strings.Contains(lower, "allow") {
			hasAllow = true
		}
		if strings.Contains(lower, "deny") || strings.Contains(lower, "reject") {
			hasDeny = true
		}
	}
	return hasAllow && hasDeny
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

// collapseSpacesRe matches runs of 2+ whitespace characters.
var collapseSpacesRe = regexp.MustCompile(`\s{2,}`)

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
	body = collapseSpacesRe.ReplaceAllString(body, " ")
	if indent > 8 {
		indent = 8
	}
	if indent > 0 {
		return strings.Repeat(" ", indent) + body
	}
	return body
}

// cleanLineColor processes a raw log line, returning both a plain version
// (for filtering/dedup) and a colored version (preserving SGR color codes).
func cleanLineColor(line string) (plain, colored string) {
	plain = cleanLine(line)
	if plain == "" {
		return "", ""
	}

	// Build colored version: handle cursor-forward, then segment by ANSI
	s := cursorForwardRe.ReplaceAllStringFunc(line, replaceCursorForward)

	// Walk through string, processing text segments and keeping SGR sequences
	var b strings.Builder
	remaining := s
	for len(remaining) > 0 {
		loc := ansiRe.FindStringIndex(remaining)
		if loc == nil {
			b.WriteString(cleanTextSegment(remaining))
			break
		}
		if loc[0] > 0 {
			b.WriteString(cleanTextSegment(remaining[:loc[0]]))
		}
		match := remaining[loc[0]:loc[1]]
		if sgrRe.MatchString(match) {
			b.WriteString(match)
		}
		remaining = remaining[loc[1]:]
	}

	result := strings.TrimRight(b.String(), " \t\r\n")

	// Use indent from plain version
	indent := 0
	for _, ch := range plain {
		if ch == ' ' {
			indent++
		} else {
			break
		}
	}

	// Strip leading whitespace from colored, preserving ANSI at the start
	stripped := trimLeftTextSpaces(result)

	if indent > 0 {
		colored = strings.Repeat(" ", indent) + stripped
	} else {
		colored = stripped
	}

	// Append reset if colors present to prevent bleed
	if strings.Contains(colored, "\x1b[") {
		colored += "\x1b[0m"
	}

	return
}

// cleanTextSegment cleans a non-ANSI text segment.
func cleanTextSegment(s string) string {
	s = stripNonPrint(s)
	return collapseSpacesRe.ReplaceAllString(s, " ")
}

// trimLeftTextSpaces trims leading whitespace while preserving ANSI sequences
// that appear before the first non-space character.
func trimLeftTextSpaces(s string) string {
	var prefix strings.Builder
	rest := s
	for len(rest) > 0 {
		loc := sgrRe.FindStringIndex(rest)
		if loc != nil && loc[0] == 0 {
			prefix.WriteString(rest[:loc[1]])
			rest = rest[loc[1]:]
			continue
		}
		if rest[0] == ' ' || rest[0] == '\t' {
			rest = rest[1:]
			continue
		}
		break
	}
	return prefix.String() + rest
}

type linePair struct {
	plain   string
	colored string
}

// dedupPairs removes keystroke echo duplicates and fragment overlaps,
// comparing on plain text but keeping colored versions in sync.
func dedupPairs(pairs []linePair) []linePair {
	deduped := make([]linePair, 0, len(pairs))
	for i, pair := range pairs {
		// Drop if next line starts with this line (keystroke echo)
		if i+1 < len(pairs) && strings.HasPrefix(pairs[i+1].plain, pair.plain) {
			continue
		}
		// Drop exact duplicates
		if len(deduped) > 0 && deduped[len(deduped)-1].plain == pair.plain {
			continue
		}
		// Drop short lines that are substrings of nearby lines (fragment overlaps)
		trimmed := strings.TrimSpace(pair.plain)
		if len(trimmed) < 20 {
			isFragment := false
			for j := max(0, i-5); j < min(len(pairs), i+5); j++ {
				if j == i {
					continue
				}
				if strings.Contains(pairs[j].plain, trimmed) && len(pairs[j].plain) > len(pair.plain) {
					isFragment = true
					break
				}
			}
			if isFragment {
				continue
			}
		}
		deduped = append(deduped, pair)
	}
	return deduped
}

// CleanTail returns the last N lines of actual content from a log file.
// Uses a 2-word minimum to filter out fragments and UI noise.
// Returns colored lines (preserving SGR color codes from the original output).
func CleanTail(path string, n int) []string {
	raw := lastNLinesTail(path, n*20, 16384)
	var pairs []linePair
	for _, line := range raw {
		plain, colored := cleanLineColor(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		if !hasMinWords(plain, 2, 8) {
			continue
		}
		if isUIChrome(plain) {
			continue
		}
		pairs = append(pairs, linePair{plain, colored})
	}
	result := dedupPairs(pairs)
	if len(result) > n {
		result = result[len(result)-n:]
	}
	lines := make([]string, len(result))
	for i, p := range result {
		lines[i] = p.colored
	}
	return lines
}

// CleanLog returns up to maxLines of cleaned log content for the full view.
// Uses a relaxed 1-word minimum to preserve more content.
// Returns colored lines (preserving SGR color codes from the original output).
func CleanLog(path string, maxLines int) []string {
	raw := lastNLinesTail(path, maxLines*20, 65536)
	var pairs []linePair
	for _, line := range raw {
		plain, colored := cleanLineColor(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		if !hasMinWords(plain, 1, 2) {
			continue
		}
		if isUIChrome(plain) {
			continue
		}
		pairs = append(pairs, linePair{plain, colored})
	}
	result := dedupPairs(pairs)
	if len(result) > maxLines {
		result = result[len(result)-maxLines:]
	}
	lines := make([]string, len(result))
	for i, p := range result {
		lines[i] = p.colored
	}
	return lines
}

var noisePatterns = []string{
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

func isUIChrome(s string) bool {
	lower := strings.ToLower(s)
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
