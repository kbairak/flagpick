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
	_, _ = fmt.Fprint(w, `flagpick - TUI for building and running complex CLI commands
usage: fp [--fp-flags] <command> [tool-args...]

flags:
  -l, --list      list known commands
  -V, --version   print version
  -h, --help      print this help
  -r, --resume    prefill with the last composition for <command>
  --update        download the latest upstream configs
`)
}

// cliAction is a terminal flagpick action selected on the command line before
// any wrapped command is considered.
type cliAction int

const (
	actionBuild cliAction = iota // wrap a command and open the TUI
	actionHelp
	actionVersion
	actionList
	actionUpdate
)

// cliArgs is the result of parsing flagpick's own arguments.
type cliArgs struct {
	action   cliAction
	cmdName  string
	toolArgs []string
	resume   bool
}

// parseArgs scans flagpick's flags up to the first non-flag token, which begins
// the wrapped command. It performs no I/O; the caller dispatches the action.
func parseArgs(args []string) (cliArgs, error) {
	var out cliArgs
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--version", "-V":
			out.action = actionVersion
			return out, nil
		case "--help", "-h":
			out.action = actionHelp
			return out, nil
		case "--list", "-l":
			out.action = actionList
			return out, nil
		case "--resume", "-r":
			out.resume = true
		case "--update":
			out.action = actionUpdate
			return out, nil
		default:
			if strings.HasPrefix(a, "-") {
				return out, fmt.Errorf("unknown flag: %s", a)
			}
			out.cmdName = a
			out.toolArgs = args[i+1:]
			return out, nil
		}
	}
	return out, nil
}

func main() {
	parsed, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage(os.Stderr)
		os.Exit(2)
	}
	switch parsed.action {
	case actionVersion:
		fmt.Println("fp " + version)
		return
	case actionHelp:
		usage(os.Stdout)
		return
	case actionList:
		list, err := ListCommands()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, n := range list {
			fmt.Println(n)
		}
		return
	case actionUpdate:
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
	}
	if parsed.cmdName == "" {
		usage(os.Stderr)
		os.Exit(2)
	}
	runCommand(parsed.cmdName, parsed.toolArgs, parsed.resume)
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
	_, _ = fmt.Fprint(os.Stdout, kittyKeyboardDisable)
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
		_, _ = fmt.Fprintln(os.Stdout, ShellQuote(append([]string{cmd}, Assemble(m.res(), m.session)...)))
	case actionRun:
		argv := append([]string{cmd}, Assemble(m.res(), m.session)...)
		_, _ = fmt.Fprintln(os.Stdout, "$ "+ShellQuote(argv))
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
