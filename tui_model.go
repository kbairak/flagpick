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
	screenAction
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
	level  int
	paths  []int
	sel    int
	filter string
}

// actionPopup is the state of the run/copy/print/cancel popup.
type actionPopup struct {
	sel int
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
	actions actionPopup
	confirm confirmPrompt

	errMsg     string
	statusMsg  string
	action     actionKind
	descScroll int
	modes      []Resolved
	mode       int
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
	return model{
		cmd:     cmd,
		cfg:     cfg,
		session: session,
		input:   in,
		picker:  newPicker(dir),
		modes:   modes,
		mode:    detectMode(modes, session),
	}
}

// res returns the active mode.
func (m model) res() Resolved {
	if len(m.modes) == 0 {
		return Resolved{}
	}
	if m.mode < 0 || m.mode >= len(m.modes) {
		return m.modes[0]
	}
	return m.modes[m.mode]
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
