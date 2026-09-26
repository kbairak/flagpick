package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Copy places text on the system clipboard, with an OSC52 fallback.
func Copy(text string) error {
	switch runtime.GOOS {
	case "darwin":
		if err := pipeTo("pbcopy", text); err == nil {
			return nil
		} else if _, ok := err.(*exec.Error); !ok {
			return err
		}
	default:
		if err := pipeTo("xclip", text, "-selection", "clipboard"); err == nil {
			return nil
		}
	}
	return osc52(text)
}

// copyFn is the clipboard writer; a variable so tests can stub it.
var copyFn = Copy

func pipeTo(name, text string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return &exec.Error{Name: name, Err: err}
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func osc52(text string) error {
	f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	payload := base64.StdEncoding.EncodeToString([]byte(text))
	_, err = fmt.Fprintf(f, "\x1b]52;c;%s\x07", payload)
	return err
}
