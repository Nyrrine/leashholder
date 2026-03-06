package main

import (
	"fmt"
	"leash/cmd"
	"os"
	"regexp"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		if err := cmd.RunDashboard(); err != nil {
			os.Exit(1)
		}
		return
	}

	switch args[0] {
	case "spawn":
		dir, name, claudeArgs := parseSpawnArgs(args[1:])
		if err := cmd.RunSpawn(dir, name, claudeArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "rename":
		if len(args) < 3 {
			fmt.Fprintf(os.Stderr, "Usage: leash rename <id> <name>\n")
			os.Exit(1)
		}
		if err := cmd.RunRename(args[1], args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "worker":
		sessionID, cwd, claudeArgs := parseWorkerArgs(args[1:])
		if sessionID == "" || cwd == "" {
			fmt.Fprintf(os.Stderr, "Usage: leash worker --session-id <id> --cwd <path> [-- claude-args...]\n")
			os.Exit(1)
		}
		if !hexIDRe.MatchString(sessionID) {
			fmt.Fprintf(os.Stderr, "Error: invalid session ID %q (expected 6 hex chars)\n", sessionID)
			os.Exit(1)
		}
		if err := cmd.RunWorker(sessionID, cwd, claudeArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "clean":
		if err := cmd.RunClean(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "Usage: leash [spawn|worker|clean|rename]\n")
		os.Exit(1)
	}
}

func parseWorkerArgs(args []string) (sessionID, cwd string, claudeArgs []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session-id":
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			}
		case "--cwd":
			if i+1 < len(args) {
				cwd = args[i+1]
				i++
			}
		case "--":
			claudeArgs = args[i+1:]
			return
		}
	}
	return
}

var hexIDRe = regexp.MustCompile(`^[0-9a-f]{6}$`)

// parseSpawnArgs extracts an optional directory, name, and claude args from spawn arguments.
// Usage: leash spawn [dir] [--name <name>] [-- claude-args...]
func parseSpawnArgs(args []string) (dir, name string, claudeArgs []string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			claudeArgs = args[i+1:]
			return
		case "--name":
			if i+1 < len(args) {
				name = args[i+1]
				i++
			}
		default:
			if dir == "" {
				dir = args[i]
			}
		}
	}
	return
}
