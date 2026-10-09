// This file defines the TUI model, its message dispatch, and kitty-keyboard setup.
package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type actionKind int

const (
	actionNone actionKind = iota
	actionRun
	actionCopy
	actionPrint
)

// rowKind distinguishes the selectable lines of the options pane. A plain
// option is one row; a variadic positional contributes one rowValue per value
// plus a trailing rowAdd.
type rowKind int

const (
	rowOption rowKind = iota
	rowValue
	rowAdd
	rowSep
)

type row struct {
	opt    Option
	kind   rowKind
	valIdx int
	value  string
}

// screen is the active full-screen overlay. The main view is the default;
// editing is a sub-state of the main view, not a screen of its own.
type screen int

const (
	screenMain screen = iota
	screenPicker
	screenEnum
	screenSub
	screenConfirm
	screenHelp
)

// enumMenu is the state of the enum value menu.
type enumMenu struct {
	name   string
	index  int
	values []string
	sel    int
	filter string
}

// subMenu is the state of the subcommand menu.
type subMenu struct {
	entries []subEntry
	sel     int
	filter  string
	depth   int // subcommand depth being selected
}

// subEntry is one selectable subcommand in the menu.
type subEntry struct {
	name  string
	desc  string
	index int // index into the node's Subcommands
}

// confirmPrompt is the state of the quit-confirmation popup.
type confirmPrompt struct {
	sel int
}

// pickerState records which positional a picker selection targets.
type pickerState struct {
	name  string
	index int
}

type model struct {
	cmd     string
	cfg     *CommandConfig
	session *Session
	filter  string
	cursor  int
	focus   int

	editing      bool
	editingName  string
	editingIndex int
	editingLabel string
	input        textinput.Model

	screen  screen
	picker  picker
	pick    pickerState
	enum    enumMenu
	sub     subMenu
	confirm confirmPrompt

	errMsg     string
	statusMsg  string
	action     actionKind
	descScroll int
	modes      []Resolved
	mode       int
	subDepth   int
	width      int
	height     int
}

func newModel(cmd string, cfg *CommandConfig, session *Session) model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 0
	in.TextStyle = activeStyle

	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}

	modes := cfg.Resolved()
	mode := detectMode(modes, session)
	subDepth := 0
	if len(modes) > 0 {
		if session.path >= 0 && session.path < len(modes) {
			mode = session.path
			subDepth = countSubs(modes[mode])
		} else if !sessionEmpty(session) {
			subDepth = countSubs(modes[mode])
		}
	}
	m := model{
		cmd:      cmd,
		cfg:      cfg,
		session:  session,
		input:    in,
		picker:   newPicker(dir),
		modes:    modes,
		mode:     mode,
		subDepth: subDepth,
	}
	m.focusEmptySlot()
	return m
}

// res returns the active path, truncated to the chosen subcommand depth.
func (m model) res() Resolved {
	if len(m.modes) == 0 {
		return Resolved{}
	}
	idx := m.mode
	if idx < 0 || idx >= len(m.modes) {
		idx = 0
	}
	return prefixResolved(m.modes[idx], m.subDepth)
}

// kittyKeyboardEnable turns on the kitty keyboard protocol so the terminal
// reports modified keys (e.g. ctrl+shift+j/k) as distinguishable CSI-u
// sequences. Terminals that don't support it ignore the escape.
const (
	kittyKeyboardEnable  = "\x1b[>1u"
	kittyKeyboardDisable = "\x1b[<u"
)

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		_, _ = fmt.Fprint(os.Stdout, kittyKeyboardEnable)
		return nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.screen == screenPicker {
			return m.handlePickerKey(msg)
		}
		updated, cmd := m.handleKey(msg)
		if mm, ok := updated.(model); ok {
			mm.syncMode()
			return mm, cmd
		}
		return updated, cmd
	}
	if m.screen == screenPicker {
		return m, nil
	}
	if cmd, ok := m.handleCSIKey(msg); ok {
		m.syncMode()
		return m, cmd
	}
	return m, nil
}
