package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

func usage(w io.Writer) {
	fmt.Fprint(w, `flagpick - TUI for building and running complex CLI commands

usage: fp [--fp-flags] <command> [tool-args...]

flags:
  -l, --list      list known commands
  -V, --version   print version
  -h, --help      print this help
  -r, --resume    prefill with the last composition for <command>
  --update        download the latest upstream configs
`)
}

func main() {
	args := os.Args[1:]
	var cmdName string
	var toolArgs []string
	resume := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--version", "-V":
			fmt.Println("fp " + version)
			return
		case "--help", "-h":
			usage(os.Stdout)
			return
		case "--list", "-l":
			list, err := ListCommands()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			for _, n := range list {
				fmt.Println(n)
			}
			return
		case "--resume", "-r":
			resume = true
		case "--update":
			data, err := dataDir()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			if err := updateConfigs(remoteBaseURL, data); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", a)
				usage(os.Stderr)
				os.Exit(2)
			}
			cmdName = a
			toolArgs = args[i+1:]
			i = len(args)
		}
	}
	if cmdName == "" {
		usage(os.Stderr)
		os.Exit(2)
	}
	runCommand(cmdName, toolArgs, resume)
}

func runCommand(cmd string, toolArgs []string, resume bool) {
	cfg, err := LoadConfig(cmd)
	if err != nil {
		var notFound *ConfigNotFoundError
		if errors.As(err, &notFound) {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprintf(os.Stderr, "failed to parse config for '%s': %v\n", cmd, err)
		}
		os.Exit(1)
	}

	tokens := prefillTokens(cmd, toolArgs, resume)
	session, err := Prefill(cfg, tokens)
	if err != nil && resume && len(tokens) > 0 {
		// Saved state may no longer fit the config; fall back to tool-args.
		tokens = toolArgs
		session, err = Prefill(cfg, tokens)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "prefill: %v\n", err)
		os.Exit(1)
	}

	final, err := tea.NewProgram(newModel(cmd, cfg, session), tea.WithAltScreen()).Run()
	fmt.Fprint(os.Stdout, kittyKeyboardDisable)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m, ok := final.(model)
	if !ok {
		return
	}
	// Persist the composition for --resume. A bare peek (quit with no
	// options and no action) leaves any previous state untouched.
	if args := Assemble(m.res(), m.session); m.action != actionNone || len(args) > 0 {
		_ = saveLast(cmd, args)
	}
	switch m.action {
	case actionPrint:
		fmt.Fprintln(os.Stdout, ShellQuote(append([]string{cmd}, Assemble(m.res(), m.session)...)))
	case actionRun:
		argv := append([]string{cmd}, Assemble(m.res(), m.session)...)
		fmt.Fprintln(os.Stdout, ShellQuote(argv))
		path, err := exec.LookPath(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot run %q: %v\n", cmd, err)
			os.Exit(1)
		}
		if err := syscall.Exec(path, argv, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "exec %q: %v\n", cmd, err)
			os.Exit(1)
		}
	}
}
