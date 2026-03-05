package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"leash/session"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// getWSLDistro returns the default WSL distribution name.
func getWSLDistro() string {
	out, err := exec.Command("wsl.exe", "-l", "-q").Output()
	if err != nil {
		return "Ubuntu"
	}
	cleaned := bytes.ReplaceAll(out, []byte{0}, nil)
	lines := strings.Split(strings.TrimSpace(string(cleaned)), "\n")
	if len(lines) > 0 {
		name := strings.TrimSpace(lines[0])
		if name != "" {
			return name
		}
	}
	return "Ubuntu"
}

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

func RunSpawn(claudeArgs []string) error {
	if err := session.EnsureDirs(); err != nil {
		return fmt.Errorf("ensure dirs: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	id := session.GenerateID()

	s := &session.Session{
		ID:        id,
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

	windowName := "leash-" + id
	shellCmd := fmt.Sprintf(
		`wt.exe -w '%s' new-tab %s--title 'leash: %s' -- wsl.exe -e bash -li -c '%s'`,
		escape(windowName), profileFlag, id, escape(innerCmd),
	)

	cmd := exec.Command("bash", "-c", shellCmd)
	if err := cmd.Start(); err != nil {
		session.RemoveSession(id)
		return fmt.Errorf("start wt.exe: %w", err)
	}

	fmt.Printf("Spawned session %s in %s\n", id, cwd)
	return nil
}

// FocusSession brings the Windows Terminal window for a session to the foreground.
func FocusSession(id string) error {
	windowName := "leash-" + id
	cmd := exec.Command("bash", "-c", fmt.Sprintf("wt.exe -w '%s' focus-tab -t 0", windowName))
	return cmd.Run()
}
