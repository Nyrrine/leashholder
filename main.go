package main

import (
	"fmt"
	"leash/cmd"
	"os"
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
		claudeArgs := extractAfterDash(args[1:])
		if err := cmd.RunSpawn(claudeArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "worker":
		sessionID, cwd, claudeArgs := parseWorkerArgs(args[1:])
		if sessionID == "" || cwd == "" {
			fmt.Fprintf(os.Stderr, "Usage: leash worker --session-id <id> --cwd <path> [-- claude-args...]\n")
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
		fmt.Fprintf(os.Stderr, "Usage: leash [spawn|worker|clean]\n")
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

func extractAfterDash(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	return nil
}
