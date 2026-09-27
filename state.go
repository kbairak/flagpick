package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// lastCommand is the persisted state used by --resume: the command flagpick was
// last invoked on and the argv (after the command name) that was composed.
type lastCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func statePath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last.json"), nil
}

// loadLast reads the saved composition. It returns false when no usable state
// exists (missing or corrupt file).
func loadLast() (lastCommand, bool) {
	p, err := statePath()
	if err != nil {
		return lastCommand{}, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return lastCommand{}, false
	}
	var lc lastCommand
	if err := json.Unmarshal(b, &lc); err != nil {
		return lastCommand{}, false
	}
	return lc, true
}

func saveLast(cmd string, args []string) error {
	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(lastCommand{Command: cmd, Args: args})
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// prefillTokens selects the tokens used to prefill the session. With resume set
// and saved state for the same command, the saved args win and tool-args are
// ignored; otherwise tool-args are used (resume is silently ignored for a
// different command or when no state exists).
func prefillTokens(cmd string, toolArgs []string, resume bool) []string {
	if resume {
		if lc, ok := loadLast(); ok && lc.Command == cmd {
			return lc.Args
		}
	}
	return toolArgs
}
