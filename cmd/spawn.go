package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"leash/session"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// getDefaultProfileGUID reads the Windows Terminal settings to find the defaultProfile GUID.
func getDefaultProfileGUID() string {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Get-Content "$env:LOCALAPPDATA\Packages\Microsoft.WindowsTerminal_8wekyb3d8bbwe\LocalState\settings.json"`).Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`"defaultProfile"\s*:\s*"(\{[^}]+\})"`)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if m := re.FindStringSubmatch(scanner.Text()); m != nil {
			return m[1]
		}
	}
	return ""
}

const (
	compactCols = 80
	compactRows = 12
	estWindowH  = 310 // approximate pixel height of a 12-row terminal window
)

// getScreenWorkArea returns the primary screen's working area (excludes taskbar).
func getScreenWorkArea() (width, height int) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command",
		`Add-Type -AssemblyName System.Windows.Forms; $a=[System.Windows.Forms.Screen]::PrimaryScreen.WorkingArea; "$($a.Width),$($a.Height)"`).Output()
	if err != nil {
		return 1920, 1040
	}
	parts := strings.Split(strings.TrimSpace(string(bytes.ReplaceAll(out, []byte{0}, nil))), ",")
	if len(parts) != 2 {
		return 1920, 1040
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	if w == 0 {
		w = 1920
	}
	if h == 0 {
		h = 1040
	}
	return w, h
}

// calcWindowSlot returns the position for a new compact window based on
// how many active sessions already exist. Stacks vertically from top-left,
// wrapping to the next column when the screen runs out of vertical space.
func calcWindowSlot(activeCount int) (x, y int) {
	_, screenH := getScreenWorkArea()
	perCol := screenH / estWindowH
	if perCol < 1 {
		perCol = 1
	}
	col := activeCount / perCol
	row := activeCount % perCol
	// Each column is roughly 700px wide (80 cols at ~8.5px/char + chrome)
	x = col * 700
	y = row * estWindowH
	return x, y
}

func RunSpawn(dir string, name string, claudeArgs []string) error {
	if err := session.EnsureDirs(); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}

	var cwd string
	if dir != "" {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("directory %q: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", dir)
		}
		cwd = dir
	} else {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
	}

	id := session.GenerateID()

	if name == "" {
		name = session.PickBranchName()
	}

	s := &session.Session{
		ID:        id,
		Name:      name,
		CWD:       cwd,
		StartedAt: time.Now().UTC(),
		Status:    session.StatusRunning,
	}

	if err := session.WriteSession(s); err != nil {
		return fmt.Errorf("write session: %w", err)
	}

	leashBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}

	// Build the inner command that runs inside the new terminal.
	escape := func(s string) string {
		return strings.ReplaceAll(s, "'", "'\\''")
	}
	innerCmd := fmt.Sprintf("cd '%s' && '%s' worker --session-id '%s' --cwd '%s'",
		escape(cwd), escape(leashBin), id, escape(cwd))
	if len(claudeArgs) > 0 {
		innerCmd += " --"
		for _, arg := range claudeArgs {
			innerCmd += fmt.Sprintf(" '%s'", escape(arg))
		}
	}

	// Use -w new + new-tab (works reliably from WSL, unlike new-window).
	// Use the default profile GUID to get the user's riced terminal.
	// Use bash -li -c for interactive login shell (loads .bashrc/.profile).
	profileFlag := ""
	if guid := getDefaultProfileGUID(); guid != "" {
		profileFlag = fmt.Sprintf("--profile '%s' ", escape(guid))
	}

	// Count active sessions to determine stacking position.
	activeSessions, _ := session.ListSessions()
	activeCount := 0
	for _, as := range activeSessions {
		if as.Status != session.StatusDone && as.ID != id {
			activeCount++
		}
	}
	posX, posY := calcWindowSlot(activeCount)

	windowName := "leash-" + id
	tabTitle := fmt.Sprintf("leash: %s Branch", name)
	shellCmd := fmt.Sprintf(
		`wt.exe -w '%s' --size %d,%d --pos %d,%d new-tab %s--title '%s' -- wsl.exe -e bash -li -c '%s'`,
		escape(windowName), compactCols, compactRows, posX, posY, profileFlag, escape(tabTitle), escape(innerCmd),
	)

	cmd := exec.Command("bash", "-c", shellCmd)
	if err := cmd.Start(); err != nil {
		session.RemoveSession(id)
		return fmt.Errorf("start wt.exe: %w", err)
	}

	fmt.Printf("Spawned %s Branch (%s) in %s\n", name, id, cwd)
	return nil
}

// FocusSession brings the Windows Terminal window for a session to the foreground.
func FocusSession(id string) error {
	windowName := "leash-" + id
	cmd := exec.Command("bash", "-c", fmt.Sprintf("wt.exe -w '%s' focus-tab -t 0", windowName))
	return cmd.Run()
}
