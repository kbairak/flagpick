package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

type model struct {
	cmd          string
	cfg          *CommandConfig
	session      *Session
	filter       string
	cursor       int
	editing      bool
	editingName  string
	editingIndex int
	editingLabel string
	input        textinput.Model
	picking      bool
	pickingName  string
	pickingIndex int
	picker       picker
	helpOverlay  bool
	confirm      bool
	confirmSel   int
	actionMenu   bool
	actionSel    int
	enumPicking  bool
	enumName     string
	enumIndex    int
	enumValues   []string
	enumSel      int
	enumFilter   string
	subPicking   bool
	subLevel     int
	subPaths     []int
	subSel       int
	subFilter    string
	errMsg       string
	statusMsg    string
	action       actionKind
	descScroll   int
	modes        []Resolved
	mode         int
	width        int
	height       int
}

var (
	boxBorder    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	activeStyle  = lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	titleStyle   = lipgloss.NewStyle().Bold(true)
	buttonStyle  = lipgloss.NewStyle().Padding(0, 2)
	previewStyle = lipgloss.NewStyle().BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
)

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

// detectMode returns the first mode that accepts the current state.
func detectMode(modes []Resolved, session *Session) int {
	for i := range modes {
		if modeIncludesApplied(modes[i], session) {
			return i
		}
	}
	return 0
}

// acceptsAt reports whether mode i accepts the applied flags: all applied
// flags are in the mode, and a non-default mode additionally requires at least
// one flag exclusive to it (so e.g. Regexp needs an -e value).
func acceptsAt(modes []Resolved, i int, session *Session) bool {
	r := modes[i]
	exclusive := false
	for name, fs := range session.Flags {
		if !fs.Checked && !fs.Filled() {
			continue
		}
		if !r.HasFlag(name) {
			return false
		}
		if i > 0 && !modes[0].HasFlag(name) {
			exclusive = true
		}
	}
	return i == 0 || exclusive
}

// syncMode keeps the current mode while it still includes every applied flag,
// and otherwise switches to the first mode that accepts the state.
func (m *model) syncMode() {
	if len(m.modes) < 2 {
		return
	}
	if modeIncludesApplied(m.res(), m.session) {
		return
	}
	next := detectMode(m.modes, m.session)
	if next == m.mode {
		return
	}
	if !m.switchMode(next) {
		m.mode = next
		m.reconcileMode()
	}
}

// modeIncludesApplied reports whether r contains every applied flag and
// positional (ignoring the exclusivity rule, so a manually chosen mode or
// subcommand is respected).
func modeIncludesApplied(r Resolved, session *Session) bool {
	for name, fs := range session.Flags {
		if (fs.Checked || fs.Filled()) && !r.HasFlag(name) {
			return false
		}
	}
	for name, vals := range session.PosVals {
		if hasValue(vals) && !r.HasPositional(name) {
			return false
		}
	}
	return true
}

// cycleMode switches to the next/previous mode whose parser accepts the
// current command, re-parsing the composed argv so the state round-trips
// (e.g. PATTERN becomes PATH). Only mode alternatives are considered:
// subcommands are chosen from the options pane. Modes that can't parse are
// skipped.
func (m *model) cycleMode(delta int) {
	cand := m.modeSiblings()
	if len(cand) < 2 {
		return
	}
	pos := 0
	for i, c := range cand {
		if c == m.mode {
			pos = i
			break
		}
	}
	tokens := Assemble(m.res(), m.session)
	empty := sessionEmpty(m.session)
	n := len(cand)
	for step := 1; step <= n; step++ {
		idx := cand[((pos+delta*step)%n+n)%n]
		// With nothing applied there is no composed argv to re-parse, so hop
		// directly to the next mode.
		if empty {
			m.mode = idx
			m.session = NewSession(m.cfg)
			m.filter = ""
			m.cursor = 0
			m.descScroll = 0
			return
		}
		session, err := prefillPath(m.cfg, m.modes[idx], tokens)
		if err != nil {
			continue
		}
		m.mode = idx
		m.session = session
		m.filter = ""
		m.cursor = 0
		m.descScroll = 0
		return
	}
}

// modeSiblings returns the path indices reachable from the current path by
// changing only mode choices (subcommands are held fixed).
func (m *model) modeSiblings() []int {
	curSubs := subcommandChoices(m.res())
	var out []int
	for i, p := range m.modes {
		if reflect.DeepEqual(subcommandChoices(p), curSubs) {
			out = append(out, i)
		}
	}
	return out
}

func subcommandChoices(r Resolved) []BranchChoice {
	var out []BranchChoice
	for _, c := range r.BranchChoices {
		if c.Kind == BranchSubcommand {
			out = append(out, c)
		}
	}
	return out
}

// sessionEmpty reports whether no flag or positional carries a value.
func sessionEmpty(s *Session) bool {
	for _, fs := range s.Flags {
		if fs.Checked || fs.Filled() {
			return false
		}
	}
	for _, vals := range s.PosVals {
		if hasValue(vals) {
			return false
		}
	}
	return true
}

// switchMode re-parses the current command into mode idx, reporting success.
func (m *model) switchMode(idx int) bool {
	tokens := Assemble(m.res(), m.session)
	session, err := prefillPath(m.cfg, m.modes[idx], tokens)
	if err != nil {
		return false
	}
	m.mode = idx
	m.session = session
	return true
}

// reconcileMode clears flag and positional state not present in the active
// mode.
func (m *model) reconcileMode() {
	res := m.res()
	for name, fs := range m.session.Flags {
		if (fs.Checked || fs.Filled()) && !res.HasFlag(name) {
			m.session.Flags[name] = FlagState{}
		}
	}
	for name, vals := range m.session.PosVals {
		if len(vals) > 0 && !res.HasPositional(name) {
			m.session.PosVals[name] = nil
		}
	}
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
		fmt.Fprint(os.Stdout, kittyKeyboardEnable)
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
		if m.picking {
			return m.handlePickerKey(msg)
		}
		updated, cmd := m.handleKey(msg)
		if mm, ok := updated.(model); ok {
			mm.syncMode()
			return mm, cmd
		}
		return updated, cmd
	}
	if m.picking {
		return m, nil
	}
	if cmd, ok := m.handleCSIKey(msg); ok {
		m.syncMode()
		return m, cmd
	}
	return m, nil
}

// ctrlCSIKeys maps a ctrl+<letter> CSI-u codepoint to the legacy key type it
// stands for, so modified keys keep working when a terminal reports them as
// escape sequences.
var ctrlCSIKeys = map[rune]tea.KeyType{
	'c': tea.KeyCtrlC,
	'd': tea.KeyCtrlD,
	'h': tea.KeyCtrlH,
	'n': tea.KeyCtrlN,
	'p': tea.KeyCtrlP,
	'r': tea.KeyCtrlR,
	'u': tea.KeyCtrlU,
	'w': tea.KeyCtrlW,
	'x': tea.KeyCtrlX,
	'y': tea.KeyCtrlY,
}

// handleCSIKey decodes a CSI-u key event (kitty keyboard protocol), which is
// how modified keys like ctrl+shift+j/k arrive. It reports whether the event
// was recognized.
func (m *model) handleCSIKey(msg tea.Msg) (tea.Cmd, bool) {
	rv := reflect.ValueOf(msg)
	if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Uint8 {
		return nil, false
	}
	b := trimCSIPrefix(rv.Bytes())
	if len(b) == 0 || b[len(b)-1] != 'u' {
		return nil, false
	}
	parts := strings.Split(string(b[:len(b)-1]), ";")
	cp, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, false
	}
	mods := 0
	if len(parts) > 1 {
		mods, _ = strconv.Atoi(parts[1])
	}
	bits := mods - 1
	// Ambiguous special keys also come through as CSI-u.
	switch cp {
	case 27:
		return m.dispatchKey(tea.KeyEscape)
	case 13:
		return m.dispatchKey(tea.KeyEnter)
	case 9:
		return m.dispatchKey(tea.KeyTab)
	case 127:
		return m.dispatchKey(tea.KeyBackspace)
	}
	if bits&4 == 0 { // require ctrl
		return nil, false
	}
	shift := bits&1 != 0
	if bits&2 != 0 { // alt/modified beyond ctrl(+shift): ignore
		return nil, false
	}
	switch cp {
	case 'k', 'K':
		if shift {
			m.reorder(-1)
		} else {
			m.moveCursor(-1)
		}
		return nil, true
	case 'j', 'J':
		if shift {
			m.reorder(1)
		} else {
			m.moveCursor(1)
		}
		return nil, true
	case 'x', 'X':
		if shift {
			m.clearAll()
		} else {
			m.clearSelected()
		}
		return nil, true
	case 'y', 'Y':
		if shift {
			return m.copyStay(), true
		}
		return m.copyExit(), true
	}
	if kt, ok := ctrlCSIKeys[rune(cp)]; ok {
		return m.dispatchKey(kt)
	}
	return nil, false
}

// dispatchKey routes a synthesized key through the main handler.
func (m *model) dispatchKey(kt tea.KeyType) (tea.Cmd, bool) {
	updated, cmd := m.handleKey(tea.KeyMsg{Type: kt})
	if mm, ok := updated.(model); ok {
		*m = mm
	}
	return cmd, true
}

// trimCSIPrefix removes a leading ESC [ from an unknown CSI sequence.
func trimCSIPrefix(b []byte) []byte {
	if len(b) >= 2 && b[0] == 0x1b && b[1] == '[' {
		return b[2:]
	}
	return b
}

// handlePickerKey routes keys to the filesystem picker: printable runes fuzzy
// filter entries, enter selects, l opens a directory, h/backspace go up, esc
// cancels.
func (m model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.picking = false
		return m, nil
	case "enter":
		if path, ok := m.picker.selectedPath(); ok {
			m.commitPath(path)
			m.picking = false
		}
		return m, nil
	case "right", "l":
		m.picker.descend()
		return m, nil
	case "left", "h":
		m.picker.parent()
		return m, nil
	case "up", "ctrl+k", "ctrl+p":
		m.picker.move(-1)
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.picker.move(1)
		return m, nil
	case "backspace", "ctrl+h":
		if !m.picker.backspace() {
			m.picker.parent()
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.picker.pushFilter(string(msg.Runes))
		return m, nil
	}
	if msg.Type == tea.KeySpace {
		m.picker.pushFilter(" ")
		return m, nil
	}
	return m, nil
}

func (m *model) openPicker(name string, index int) tea.Cmd {
	m.picking = true
	m.pickingName = name
	m.pickingIndex = index
	m.picker = newPicker(m.picker.dir)
	return nil
}

func (m *model) commitPath(path string) {
	name := m.pickingName
	vals := m.session.PosVals[name]
	switch {
	case m.pickingIndex < 0:
		if m.isVariadic(name) {
			m.session.PosVals[name] = append(vals, path)
		} else {
			m.session.PosVals[name] = []string{path}
		}
	case m.pickingIndex < len(vals):
		vals[m.pickingIndex] = path
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.helpOverlay {
		m.helpOverlay = false
		return m, nil
	}

	if m.confirm {
		return m.handleConfirmKey(msg)
	}

	if m.actionMenu {
		return m.handleActionKey(msg)
	}

	if m.enumPicking {
		return m.handleEnumKey(msg)
	}

	if m.subPicking {
		return m.handleSubKey(msg)
	}

	if m.editing {
		switch key {
		case "enter":
			m.commitEdit()
			m.editing = false
			m.input.Blur()
			if m.editingIndex < 0 && m.isVariadic(m.editingName) {
				m.setCursor(m.rowIndexOfKind(m.editingName, rowAdd))
			} else {
				m.setCursor(m.rowIndexOfOpt(m.editingName) + 1)
			}
			return m, nil
		case "esc":
			m.editing = false
			m.input.Blur()
			return m, nil
		default:
			if m.isIntFlag(m.editingName) && !isDigitInput(msg) {
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	m.errMsg = ""
	m.statusMsg = ""

	switch key {
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
			m.descScroll = 0
			return m, nil
		}
		m.confirm = true
		m.confirmSel = 1
		return m, nil
	case "ctrl+j", "down":
		m.moveCursor(1)
		return m, nil
	case "ctrl+k", "up":
		m.moveCursor(-1)
		return m, nil
	case "ctrl+d":
		m.moveCursorClamped(halfPage(m.pageSize()))
		return m, nil
	case "ctrl+u":
		m.moveCursorClamped(-halfPage(m.pageSize()))
		return m, nil
	case "ctrl+x":
		m.clearSelected()
		return m, nil
	case "ctrl+shift+x":
		m.clearAll()
		return m, nil
	case "pgdown":
		m.descScroll += m.pageSize()
		return m, nil
	case "pgup":
		m.descScroll -= m.pageSize()
		return m, nil
	// Reorder. ctrl-shift-j/k is preferred; terminals that don't emit a
	// distinct sequence for it fall back to ctrl+up/down.
	case "ctrl+shift+k", "ctrl+up":
		m.reorder(-1)
		return m, nil
	case "ctrl+shift+j", "ctrl+down":
		m.reorder(1)
		return m, nil
	case "enter":
		m.actionMenu = true
		m.actionSel = 0
		return m, nil
	case " ":
		return m, m.activate()
	case "ctrl+r":
		return m, m.runCommand()
	case "ctrl+y":
		return m, m.copyCommand(false)
	case "ctrl+shift+y":
		return m, m.copyCommand(true)
	case "ctrl+p":
		return m, m.printCommand()
	case "ctrl+?", "ctrl+/":
		m.helpOverlay = true
		return m, nil
	case "tab":
		m.cycleMode(1)
		return m, nil
	case "shift+tab":
		m.cycleMode(-1)
		return m, nil
	case "backspace", "ctrl+h":
		if m.filter != "" {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.cursor = 0
			m.descScroll = 0
		}
		return m, nil
	case "ctrl+w":
		m.filter = trimLastWord(m.filter)
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.filter += string(msg.Runes)
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	}
	return m, nil
}

// runCommand validates and quits to exec the command.
func (m *model) runCommand() tea.Cmd {
	if !m.validate() {
		return nil
	}
	m.action = actionRun
	return tea.Quit
}

// copyCommand validates and copies the composed command, staying open.
// copyCommand validates and copies the composed command. On failure it stays
// open with an error; on success it sets actionCopy and, unless stay is set,
// quits.
func (m *model) copyCommand(stay bool) tea.Cmd {
	if !m.validate() {
		return nil
	}
	if err := copyFn(ShellQuote(append([]string{m.cmd}, Assemble(m.res(), m.session)...))); err != nil {
		m.errMsg = "copy: " + err.Error()
		return nil
	}
	m.statusMsg = "copied"
	m.action = actionCopy
	if stay {
		return nil
	}
	return tea.Quit
}

// printCommand validates and quits to print the command.
func (m *model) printCommand() tea.Cmd {
	if !m.validate() {
		return nil
	}
	m.action = actionPrint
	return tea.Quit
}

// handleEnumKey drives the enum value menu (up/down choose, enter selects,
// esc cancels). Printable runes fuzzy-filter the values.
func (m model) handleEnumKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.enumPicking = false
		return m, nil
	case "up", "ctrl+k":
		if m.enumSel > 0 {
			m.enumSel--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.enumSel < len(m.filteredEnum())-1 {
			m.enumSel++
		}
		return m, nil
	case "backspace", "ctrl+h":
		m.enumFilter = trimLastRune(m.enumFilter)
		m.enumSel = 0
		return m, nil
	case "ctrl+w":
		m.enumFilter = trimLastWord(m.enumFilter)
		m.enumSel = 0
		return m, nil
	case "enter":
		if f := m.filteredEnum(); m.enumSel < len(f) {
			m.commitEnum(f[m.enumSel])
		}
		m.enumPicking = false
		m.setCursor(m.rowIndexOfOpt(m.enumName))
		return m, nil
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	if msg.Type == tea.KeyRunes {
		m.enumFilter += string(msg.Runes)
		m.enumSel = 0
	}
	if msg.Type == tea.KeySpace {
		m.enumFilter += " "
		m.enumSel = 0
	}
	return m, nil
}

// filteredEnum returns the enum values matching the current filter.
func (m model) filteredEnum() []string {
	var out []string
	for _, v := range m.enumValues {
		if fuzzyMatch(v, m.enumFilter) {
			out = append(out, v)
		}
	}
	return out
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// openEnum opens the value menu for an enum flag.
func (m *model) openEnum(name string, index int) {
	vals := enumValues(m.cfg, name)
	if len(vals) == 0 {
		return
	}
	cur := ""
	pos := m.isPositionalName(name)
	if index >= 0 {
		vs := m.enumState(name, pos)
		if index < len(vs) {
			cur = vs[index]
		}
	} else {
		if vs := m.enumState(name, pos); len(vs) > 0 {
			cur = vs[0]
		}
	}
	m.enumPicking = true
	m.enumName = name
	m.enumIndex = index
	m.enumValues = vals
	m.enumFilter = ""
	m.enumSel = 0
	for i, v := range vals {
		if v == cur {
			m.enumSel = i
			break
		}
	}
}

// enumState returns the current value list backing an enum option: flag
// values or positional values.
func (m *model) enumState(name string, pos bool) []string {
	if pos {
		return m.session.PosVals[name]
	}
	return m.session.Flags[name].Values
}

// commitEnum stores the chosen enum value.
func (m *model) commitEnum(val string) {
	if m.isPositionalName(m.enumName) {
		vals := m.session.PosVals[m.enumName]
		if m.enumIndex < 0 {
			if m.isVariadic(m.enumName) {
				m.session.PosVals[m.enumName] = append(vals, val)
			} else {
				m.session.PosVals[m.enumName] = []string{val}
			}
		} else if m.enumIndex < len(vals) {
			vals[m.enumIndex] = val
		}
		return
	}
	state := m.session.Flags[m.enumName]
	if m.enumIndex < 0 {
		if m.isRepeatableFlag(m.enumName) {
			state.Values = append(state.Values, val)
		} else {
			state.Values = []string{val}
		}
	} else if m.enumIndex < len(state.Values) {
		state.Values[m.enumIndex] = val
	}
	m.session.Flags[m.enumName] = state
}

// openSub opens the subcommand menu for the branch at level.
func (m *model) openSub(level int) {
	idx := m.cfg.SiblingIndices(m.res(), level)
	if len(idx) == 0 {
		return
	}
	m.subPicking = true
	m.subLevel = level
	m.subPaths = idx
	m.subFilter = ""
	m.subSel = 0
	cur := m.res().BranchChoices[level].Index
	for i, pi := range idx {
		if m.modes[pi].BranchChoices[level].Index == cur {
			m.subSel = i
			break
		}
	}
}

// filteredSub returns the sibling path indices matching the current filter.
func (m model) filteredSub() []int {
	var out []int
	for _, pi := range m.subPaths {
		if fuzzyMatch(m.modes[pi].Name, m.subFilter) {
			out = append(out, pi)
		}
	}
	return out
}

// handleSubKey drives the subcommand menu (up/down choose, enter selects, esc
// cancels). Printable runes fuzzy-filter the subcommands. Choosing one
// switches the active path and keeps the state the new path still accepts.
func (m model) handleSubKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.subPicking = false
		return m, nil
	case "up", "ctrl+k":
		if m.subSel > 0 {
			m.subSel--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.subSel < len(m.filteredSub())-1 {
			m.subSel++
		}
		return m, nil
	case "backspace", "ctrl+h":
		m.subFilter = trimLastRune(m.subFilter)
		m.subSel = 0
		return m, nil
	case "ctrl+w":
		m.subFilter = trimLastWord(m.subFilter)
		m.subSel = 0
		return m, nil
	case "enter":
		if f := m.filteredSub(); m.subSel < len(f) {
			m.mode = f[m.subSel]
		}
		m.subPicking = false
		m.reconcileMode()
		m.filter = ""
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	if msg.Type == tea.KeyRunes {
		m.subFilter += string(msg.Runes)
		m.subSel = 0
	}
	if msg.Type == tea.KeySpace {
		m.subFilter += " "
		m.subSel = 0
	}
	return m, nil
}

// subView renders the subcommand menu.
func (m model) subView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}
	normal := lipgloss.NewStyle().Padding(0, 2)
	selected := normal.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))

	lines := []string{titleStyle.Render(truncate("Select subcommand", inner)), ""}
	subs := m.filteredSub()
	if len(subs) == 0 {
		lines = append(lines, dimStyle.Render("(no match)"))
	}
	for i, pi := range subs {
		pc := m.modes[pi].BranchChoices[m.subLevel]
		label := m.modes[pi].Name
		if d := strings.TrimSpace(pc.Description); d != "" {
			label += " — " + d
		}
		label = truncate(label, inner-2)
		if i == m.subSel {
			lines = append(lines, selected.Render(label))
		} else {
			lines = append(lines, normal.Render(label))
		}
	}
	filterLine := truncate("filter: "+m.subFilter+"▏", inner)
	lines = append(lines, "", filterLine, dimStyle.Render(truncate("type to filter · ctrl-w word · enter choose · up/down · esc cancel", inner)))
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

// enumValues returns an enum flag's or positional's allowed values.
func enumValues(cfg *CommandConfig, name string) []string {
	for _, f := range cfg.Flags {
		if f.OptName() == name {
			if e, ok := f.(*EnumFlag); ok {
				return e.Allowed()
			}
		}
	}
	for _, p := range cfg.Positionals {
		if p.OptName() == name {
			if e, ok := p.(*EnumPositional); ok {
				return e.Allowed()
			}
		}
	}
	return nil
}

// isPositionalName reports whether name is a positional in this config.
func (m model) isPositionalName(name string) bool {
	for _, p := range m.cfg.Positionals {
		if p.OptName() == name {
			return true
		}
	}
	return false
}

// handleActionKey drives the run/copy/print/cancel popup.
func (m model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.actionMenu = false
		return m, nil
	case "up", "ctrl+k", "k":
		if m.actionSel > 0 {
			m.actionSel--
		}
		return m, nil
	case "down", "ctrl+j", "ctrl+n", "j":
		if m.actionSel < 4 {
			m.actionSel++
		}
		return m, nil
	case "enter":
		switch m.actionSel {
		case 0:
			cmd := m.runCommand()
			if cmd == nil {
				m.actionMenu = false
			}
			return m, cmd
		case 1:
			return m, m.copyExit()
		case 2:
			return m, m.copyStay()
		case 3:
			cmd := m.printCommand()
			if cmd == nil {
				m.actionMenu = false
			}
			return m, cmd
		default:
			m.actionMenu = false
			return m, nil
		}
	case "ctrl+r":
		m.actionMenu = false
		return m, m.runCommand()
	case "ctrl+y":
		return m, m.copyExit()
	case "ctrl+shift+y":
		return m, m.copyStay()
	case "ctrl+p":
		m.actionMenu = false
		return m, m.printCommand()
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	return m, nil
}

// copyExit copies and, on success, quits.
func (m *model) copyExit() tea.Cmd {
	cmd := m.copyCommand(false)
	if cmd == nil {
		// Copy failed (stay with error) or validation failed.
		if m.errMsg == "" {
			m.actionMenu = false
		}
		return nil
	}
	m.actionMenu = false
	return cmd
}

// copyStay copies and stays open.
func (m *model) copyStay() tea.Cmd {
	cmd := m.copyCommand(true)
	m.actionMenu = false
	return cmd
}

// handleConfirmKey drives the quit-confirmation popup. esc cancels, enter
// activates the focused button (Cancel or Exit), left/right/tab move focus,
// and ctrl+c always exits.
func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.confirm = false
		return m, nil
	case "left", "h":
		m.confirmSel = 0
		return m, nil
	case "right", "l":
		m.confirmSel = 1
		return m, nil
	case "tab", "shift+tab":
		m.confirmSel = 1 - m.confirmSel
		return m, nil
	case "enter":
		if m.confirmSel == 0 {
			m.confirm = false
			return m, nil
		}
		m.action = actionNone
		return m, tea.Quit
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	return m, nil
}

// listOptions mirrors the preview order (flags, then positionals) while
// surfacing filled options first: filled flags, filled positionals, then empty
// flags, then empty positionals.
// listOptions returns the active mode's options in the order written in the
// mode's `options:` list, with flag slots filled in session emission order (so
// reorder is visible) and filled options hoisted first.
func (m model) listOptions() []Option {
	res := m.res()
	byName := make(map[string]Flag, len(res.Flags))
	for _, f := range res.Flags {
		byName[f.OptName()] = f
	}

	seq := append([]Option(nil), res.Options...)
	// Fill flag slots in session order.
	var ordered []Flag
	for _, name := range m.session.Order {
		if f, ok := byName[name]; ok {
			ordered = append(ordered, f)
		}
	}
	k := 0
	for i, opt := range seq {
		if _, isFlag := opt.(Flag); isFlag && k < len(ordered) {
			seq[i] = ordered[k]
			k++
		}
	}

	var filled, empty []Option
	for _, opt := range seq {
		if m.isFilled(opt) {
			filled = append(filled, opt)
		} else {
			empty = append(empty, opt)
		}
	}
	return append(filled, empty...)
}

func filterOptions(opts []Option, filter string) []Option {
	if filter == "" {
		return opts
	}
	var out []Option
	for _, o := range opts {
		if fuzzyMatch(o.OptName(), filter) {
			out = append(out, o)
		}
	}
	return out
}

func fuzzyMatch(label, filter string) bool {
	fr := []rune(strings.ToLower(filter))
	i := 0
	for _, r := range strings.ToLower(label) {
		if i < len(fr) && r == fr[i] {
			i++
		}
	}
	return i == len(fr)
}

// trimLastWord removes the trailing word from a filter (ctrl-w), including any
// whitespace before it but keeping the whitespace separator.
func trimLastWord(s string) string {
	s = strings.TrimRight(s, " \t")
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[:i+1]
	}
	return ""
}

func (m model) filtered() []Option {
	return filterOptions(m.listOptions(), m.filter)
}

// rows expands the filtered options into selectable lines. A variadic
// positional becomes one rowValue per value plus a trailing rowAdd.
func (m model) rows() []row {
	var rows []row
	seenFilled := false
	sepInserted := false
	for _, opt := range m.filtered() {
		filled := m.isFilled(opt)
		if filled {
			seenFilled = true
		} else if seenFilled && !sepInserted {
			rows = append(rows, row{kind: rowSep})
			sepInserted = true
		}
		if m.isRepeatableOption(opt) {
			for i, v := range m.optionValues(opt) {
				rows = append(rows, row{opt: opt, kind: rowValue, valIdx: i, value: v})
			}
			rows = append(rows, row{opt: opt, kind: rowAdd})
			continue
		}
		rows = append(rows, row{opt: opt, kind: rowOption})
	}
	return rows
}

// isRepeatableOption reports whether opt expands into a list of values (a
// variadic positional or a repeatable value flag).
func (m model) isRepeatableOption(opt Option) bool {
	switch o := opt.(type) {
	case Positional:
		return o.Variadic()
	case Flag:
		return o.Repeatable() && o.TakesValue()
	}
	return false
}

// optionValues returns the current values backing a repeatable option.
func (m model) optionValues(opt Option) []string {
	switch o := opt.(type) {
	case Positional:
		return m.session.PosVals[o.OptName()]
	case Flag:
		return m.session.Flags[o.OptName()].Values
	}
	return nil
}

// isFilled reports whether an option currently carries a value (checked bool,
// non-empty value flag, or a positional with a value).
func (m model) isFilled(opt Option) bool {
	switch o := opt.(type) {
	case Positional:
		return hasValue(m.session.PosVals[o.OptName()])
	case Flag:
		fs := m.session.Flags[o.OptName()]
		return fs.Checked || fs.Filled()
	}
	return false
}

func (m *model) moveCursor(delta int) {
	m.descScroll = 0
	list := m.rows()
	// Collect the selectable rows so movement cycles and skips separators.
	var idx []int
	for i, r := range list {
		if r.kind != rowSep {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		m.cursor = 0
		return
	}
	pos := 0
	for j, i := range idx {
		if i == m.cursor {
			pos = j
			break
		}
	}
	pos = ((pos+delta)%len(idx) + len(idx)) % len(idx)
	m.cursor = idx[pos]
}

// halfPage is half the visible page, at least one row.
func halfPage(pageSize int) int {
	n := pageSize / 2
	if n < 1 {
		n = 1
	}
	return n
}

// moveCursorClamped moves by delta without wrapping (stops at the ends).
func (m *model) moveCursorClamped(delta int) {
	m.descScroll = 0
	var idx []int
	for i, r := range m.rows() {
		if r.kind != rowSep {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		m.cursor = 0
		return
	}
	pos := 0
	for j, i := range idx {
		if i == m.cursor {
			pos = j
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(idx) {
		pos = len(idx) - 1
	}
	m.cursor = idx[pos]
}

// setCursor clamps the cursor to the current row list and resets the
// description scroll.
func (m *model) setCursor(i int) {
	m.descScroll = 0
	list := m.rows()
	n := len(list)
	if n == 0 {
		m.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	// Separators are not selectable; step in the direction of travel, then
	// fall back the other way.
	if list[i].kind == rowSep {
		if j := nextSelectable(list, i, 1); j >= 0 {
			i = j
		} else if j := nextSelectable(list, i, -1); j >= 0 {
			i = j
		}
	}
	m.cursor = i
}

// nextSelectable returns the nearest selectable row index from start moving in
// direction dir, or -1 if none exists.
func nextSelectable(rows []row, start, dir int) int {
	for i := start + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].kind != rowSep {
			return i
		}
	}
	return -1
}

func (m model) selectedRow() (row, bool) {
	list := m.rows()
	if m.cursor < 0 || m.cursor >= len(list) {
		return row{}, false
	}
	return list[m.cursor], true
}

func (m *model) activate() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	switch r.kind {
	case rowAdd:
		if m.isPath(r.opt) {
			return m.openPicker(r.opt.OptName(), -1)
		}
		if m.isEnum(r.opt) {
			m.openEnum(r.opt.OptName(), -1)
			return nil
		}
		m.beginEdit(r.opt.OptName(), -1, "", "+ "+r.opt.OptName()+": ")
	case rowValue:
		if m.isPath(r.opt) {
			return m.openPicker(r.opt.OptName(), r.valIdx)
		}
		if m.isEnum(r.opt) {
			m.openEnum(r.opt.OptName(), r.valIdx)
			return nil
		}
		m.beginEdit(r.opt.OptName(), r.valIdx, r.value, valueLabel(r.opt.OptName(), r.valIdx))
		return nil
	}
	switch v := r.opt.(type) {
	case *SubcommandOption:
		m.openSub(v.Level)
	case *BoolFlag:
		m.cycleBool(r.opt.OptName(), v.Negative() != "")
		if !v.Repeatable() {
			m.filter = ""
			m.descScroll = 0
			// The toggled flag may have moved into the checked group; keep
			// the cursor on it.
			m.cursor = m.rowIndexOf(r.opt.OptName())
		}
	case *StringPositional:
		m.beginEdit(r.opt.OptName(), -1, firstValue(m.session.PosVals[r.opt.OptName()]), r.opt.OptName()+": ")
	case *EnumFlag:
		m.openEnum(r.opt.OptName(), -1)
	case *EnumPositional:
		m.openEnum(r.opt.OptName(), -1)
	case *CountFlag:
		state := m.session.Flags[r.opt.OptName()]
		state.Count = (state.Count + 1) % 4
		m.session.Flags[r.opt.OptName()] = state
		m.setCursor(m.rowIndexOfOpt(r.opt.OptName()))
	case *StringFlag, *IntFlag, *SizeFlag:
		m.beginEdit(r.opt.OptName(), -1, m.session.Flags[r.opt.OptName()].First(), r.opt.OptName()+": ")
	case *PathPositional:
		return m.openPicker(r.opt.OptName(), -1)
	}
	return nil
}

// isEnum reports whether opt is an enum flag or enum positional.
func (m model) isEnum(opt Option) bool {
	switch opt.(type) {
	case *EnumFlag, *EnumPositional:
		return true
	}
	return false
}

func (m model) isPath(opt Option) bool {
	_, ok := opt.(*PathPositional)
	return ok
}

// isValueFlag reports whether name is a flag that takes a value.
func (m model) isValueFlag(name string) bool {
	for _, f := range m.cfg.Flags {
		if f.OptName() == name {
			return f.TakesValue()
		}
	}
	return false
}

// isIntFlag reports whether name is an integer-valued flag.
func (m model) isIntFlag(name string) bool {
	for _, f := range m.cfg.Flags {
		if f.OptName() == name {
			_, ok := f.(*IntFlag)
			return ok
		}
	}
	return false
}

// isDigitInput reports whether a key may be applied to an integer field:
// control keys pass through, rune input must be ASCII digits.
func isDigitInput(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes {
		return true
	}
	for _, r := range msg.Runes {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// rowIndexOf returns the row index of the plain option named name, or 0 if it
// is not present.
func (m model) rowIndexOf(name string) int {
	return m.rowIndexOfKind(name, rowOption)
}

// rowIndexOfKind returns the row index of the option named name with the given
// kind, falling back to the first row with that name, or 0.
func (m model) rowIndexOfKind(name string, kind rowKind) int {
	fallback := -1
	for i, r := range m.rows() {
		if r.opt == nil || r.opt.OptName() != name {
			continue
		}
		if r.kind == kind {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	if fallback >= 0 {
		return fallback
	}
	return 0
}

// rowIndexOfOpt returns the first row (any kind) for the named option, or 0.
func (m model) rowIndexOfOpt(name string) int {
	return m.rowIndexOfKind(name, rowOption)
}

func valueLabel(name string, idx int) string {
	return name + "[" + strconv.Itoa(idx) + "]: "
}

func (m *model) beginEdit(name string, index int, value, label string) {
	leftW, _ := m.paneWidths()
	content := leftW - 2
	if content < 2 {
		content = 2
	}
	label = truncate(label, content-1)
	// Reserve one cell for the input's cursor, which is rendered in addition
	// to the width-limited value.
	w := content - lipgloss.Width(label) - 1
	if w < 1 {
		w = 1
	}
	m.input.Width = w
	m.editing = true
	m.editingName = name
	m.editingIndex = index
	m.editingLabel = label
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *model) commitEdit() {
	val := m.input.Value()
	name := m.editingName
	if m.isValueFlag(name) {
		if err := m.validateValue(name, val); err != nil {
			m.errMsg = err.Error()
			return
		}
		state := m.session.Flags[name]
		if m.isRepeatableFlag(name) {
			state.Values = editValueList(state.Values, m.editingIndex, val)
		} else if val == "" {
			state.Values = nil
		} else {
			state.Values = []string{val}
		}
		m.session.Flags[name] = state
		return
	}
	vals := m.session.PosVals[name]
	m.session.PosVals[name] = editValueList(vals, m.editingIndex, val)
}

// editValueList applies an edit to a list of values: index < 0 appends a
// non-empty value; otherwise the indexed value is replaced (or removed when
// empty).
func editValueList(vals []string, index int, val string) []string {
	if index < 0 {
		if val == "" {
			return vals
		}
		return append(vals, val)
	}
	if index >= len(vals) {
		return vals
	}
	if val == "" {
		return append(vals[:index], vals[index+1:]...)
	}
	vals[index] = val
	return vals
}

// validateValue enforces kind-specific value rules.
func (m *model) validateValue(name, val string) error {
	for _, f := range m.cfg.Flags {
		if f.OptName() == name {
			return validateFlagValue(f, val)
		}
	}
	return nil
}

// deleteValue removes the variadic value under the cursor. It is a no-op on
// any other row.
func (m *model) deleteValue() {
	r, ok := m.selectedRow()
	if !ok || r.kind != rowValue {
		return
	}
	name := r.opt.OptName()
	if f, ok := r.opt.(Flag); ok && f.Repeatable() {
		state := m.session.Flags[name]
		if r.valIdx < len(state.Values) {
			state.Values = append(state.Values[:r.valIdx], state.Values[r.valIdx+1:]...)
			m.session.Flags[name] = state
		}
	} else {
		vals := m.session.PosVals[name]
		if r.valIdx >= len(vals) {
			return
		}
		m.session.PosVals[name] = append(vals[:r.valIdx], vals[r.valIdx+1:]...)
	}
	list := m.rows()
	if m.cursor >= len(list) && m.cursor > 0 {
		m.cursor--
	}
}

// clearAll resets every option: unchecks bools, empties value flags, and
// clears positionals.
func (m *model) clearAll() {
	for name := range m.session.Flags {
		m.session.Flags[name] = FlagState{}
	}
	for name := range m.session.PosVals {
		m.session.PosVals[name] = nil
	}
	m.filter = ""
	m.cursor = 0
	m.descScroll = 0
	m.errMsg = ""
	m.statusMsg = ""
}

// cycleBool advances a bool flag. Non-nullable flags toggle; nullable flags
// cycle unset -> on -> off -> unset.
func (m *model) cycleBool(name string, nullable bool) {
	if !nullable {
		ToggleFlag(m.cfg, m.session, name)
		return
	}
	state := m.session.Flags[name]
	switch {
	case !state.Checked && !state.Neg:
		ToggleFlag(m.cfg, m.session, name)
	case state.Checked:
		state.Checked = false
		state.Neg = true
		m.session.Flags[name] = state
	default:
		state.Neg = false
		m.session.Flags[name] = state
	}
}

// clearSelected empties the selected entry: a checked bool is unchecked (like
// space), a text/number value is cleared, and a variadic value is deleted. It
// is a no-op on an already-empty entry, the add row, and separators.
func (m *model) clearSelected() {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	switch r.kind {
	case rowValue:
		m.deleteValue()
		return
	case rowAdd, rowSep:
		return
	}
	name := r.opt.OptName()
	switch r.opt.(type) {
	case *BoolFlag:
		if !m.session.Flags[name].Filled() {
			return
		}
		m.session.Flags[name] = FlagState{}
	case *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		if !m.session.Flags[name].Filled() {
			return
		}
		state := m.session.Flags[name]
		state.Values = nil
		m.session.Flags[name] = state
	case *CountFlag:
		if m.session.Flags[name].Count == 0 {
			return
		}
		state := m.session.Flags[name]
		state.Count = 0
		m.session.Flags[name] = state
	case *StringPositional, *PathPositional, *EnumPositional:
		if !hasValue(m.session.PosVals[name]) {
			return
		}
		m.session.PosVals[name] = nil
	default:
		return
	}
	// The entry may have moved into the empty group; keep the cursor on it.
	m.setCursor(m.rowIndexOfOpt(name))
}

func (m model) isVariadic(name string) bool {
	for _, p := range m.cfg.Positionals {
		if p.OptName() == name {
			return p.Variadic()
		}
	}
	return false
}

func firstValue(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func (m *model) reorder(delta int) {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	if _, isFlag := r.opt.(Flag); !isFlag {
		return
	}
	name := r.opt.OptName()
	// Only applied (enabled) flags can be reordered; unapplied ones emit
	// nothing, so moving them has no visible effect.
	if !m.flagEnabled(name) {
		return
	}
	idx := -1
	for i, n := range m.session.Order {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	// Swap with the nearest enabled flag in the given direction, so the
	// change is visible in the preview (disabled flags emit nothing).
	target := -1
	for j := idx + delta; j >= 0 && j < len(m.session.Order); j += delta {
		if m.flagEnabled(m.session.Order[j]) {
			target = j
			break
		}
	}
	if target < 0 {
		return
	}
	m.session.Order[idx], m.session.Order[target] = m.session.Order[target], m.session.Order[idx]
	// Keep the cursor on the moved flag as it changes position.
	m.setCursor(m.rowIndexOfOpt(name))
}

// flagEnabled reports whether a flag currently contributes to the command.
func (m model) flagEnabled(name string) bool {
	fs, ok := m.session.Flags[name]
	if !ok {
		return false
	}
	return fs.Checked || fs.Filled()
}

// isRepeatableFlag reports whether name is a repeatable value flag.
func (m model) isRepeatableFlag(name string) bool {
	for _, f := range m.cfg.Flags {
		if f.OptName() == name {
			return f.Repeatable() && f.TakesValue()
		}
	}
	return false
}

// validate checks the active path. If exactly one *mode alternative* of the
// same subcommand accepts the current state and validates, it is adopted.
func (m *model) validate() bool {
	if modeIncludesApplied(m.res(), m.session) && len(MissingRequired(m.res(), m.session)) == 0 {
		return true
	}
	var valid []int
	for _, i := range m.modeSiblings() {
		if acceptsAt(m.modes, i, m.session) && len(MissingRequired(m.modes[i], m.session)) == 0 {
			valid = append(valid, i)
		}
	}
	if len(valid) == 1 {
		m.mode = valid[0]
		return true
	}
	if missing := MissingRequired(m.res(), m.session); len(missing) > 0 {
		m.errMsg = "required: " + strings.Join(missing, ", ")
	} else {
		m.errMsg = "no mode matches the current options"
	}
	m.statusMsg = ""
	return false
}

func (m model) preview() string {
	return ShellQuote(append([]string{m.cmd}, Assemble(m.res(), m.session)...))
}

func (m model) View() string {
	if m.helpOverlay {
		return m.helpView()
	}
	if m.confirm {
		return m.confirmView()
	}
	if m.actionMenu {
		return m.actionView()
	}
	if m.enumPicking {
		return m.enumView()
	}
	if m.subPicking {
		return m.subView()
	}
	if m.picking {
		return m.pickerView()
	}

	leftW, rightW := m.paneWidths()
	paneH := m.paneHeight()
	bodyH := m.pageSize()

	list := boxBorder.Width(leftW).Height(paneH).Render(titleStyle.Render("Options") + "\n" + m.renderList(bodyH, leftW))
	desc := boxBorder.Width(rightW).Height(paneH).Render(titleStyle.Render("Description") + "\n" + m.renderDescription(rightW, bodyH))

	panes := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", desc)

	cw := m.contentWidth()

	filterLine := truncate("filter: "+m.filter+"▏", cw)

	preview := previewStyle.Width(cw).Render(truncate("$ "+m.preview(), cw))

	plain := m.statusMsg
	isErr := false
	if m.errMsg != "" {
		plain = m.errMsg
		isErr = true
	}
	if plain == "" {
		plain = "space toggle/edit · enter menu · ctrl-x clear · ctrl-j/k cycle · ctrl-d/u half-page · ctrl-shift-j/k reorder · pgup/pgdn scroll desc · ctrl-r run · ctrl-y copy+exit · ctrl-shift-y copy · ctrl-p print · ctrl-? help · esc confirm · ctrl-c quit"
	}
	plain = truncate(plain, cw)

	var status string
	switch {
	case isErr:
		status = errStyle.Render(plain)
	case m.statusMsg != "":
		status = statusStyle.Render(plain)
	default:
		status = dimStyle.Render(plain)
	}

	top := []string{preview, filterLine}
	if m.modeSelectorVisible() {
		top = append(top, m.modePicker(), m.modeUsage())
	}
	top = append(top, panes, status)
	return strings.Join(top, "\n")
}

// modeSelectorVisible reports whether the horizontal mode picker should render.
// Subcommands are chosen from the options pane instead, so a tree with no mode
// forks (a pure dispatcher like direnv) hides it.
func (m model) modeSelectorVisible() bool {
	return len(m.modes) > 1 && m.res().HasBranchKind(BranchMode)
}

// modePicker renders the horizontal mode selector.
func (m model) modePicker() string {
	var parts []string
	for i, r := range m.modes {
		name := r.Name
		if name == "" {
			name = "mode"
		}
		if i == m.mode {
			parts = append(parts, activeStyle.Render(name))
		} else {
			parts = append(parts, dimStyle.Render(name))
		}
	}
	return truncate("mode: "+strings.Join(parts, "  "), m.contentWidth())
}

// modeUsage renders the one-line usage for the active mode.
func (m model) modeUsage() string {
	return dimStyle.Render(truncate(m.res().Usage, m.contentWidth()))
}

// paneWidths returns the content widths of the left (options) and right
// (description) panes so that, with their borders and the one-column gap,
// they fill the usable width.
func (m model) paneWidths() (int, int) {
	const gap = 1
	cw := m.contentWidth()
	total := cw - 4 - gap
	if total < 20 {
		return 15, 15
	}
	left := cw / 3
	if left < 20 {
		left = 20
	}
	if left > 40 {
		left = 40
	}
	right := total - left
	if right < 15 {
		right = 15
		left = total - right
	}
	if left < 5 {
		left = 5
	}
	return left, right
}

// contentWidth is the usable width, one column short of the terminal so that
// no rendered line reaches the last cell (which makes terminals auto-wrap and
// scroll the whole UI).
func (m model) contentWidth() int {
	w := m.width - 1
	if w < 1 {
		w = 1
	}
	return w
}

// paneHeight returns the content height of the panes so the panes plus the
// three leading lines (filter, preview border, preview) and the trailing
// status line fit the terminal height. Each pane also has a top and bottom
// border.
func (m model) paneHeight() int {
	extra := 0
	if m.modeSelectorVisible() {
		extra = 2 // mode picker + usage line
	}
	h := m.height - 6 - extra
	if h < 1 {
		h = 1
	}
	return h
}

// truncate cuts s to at most w display cells, appending an ellipsis when it
// has to drop text. Intended for plain (unstyled) strings.
func truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// pageSize is the number of body lines shown in a pane (pane height minus
// the title line) and the step used to scroll the description.
func (m model) pageSize() int {
	h := m.paneHeight() - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) renderDescription(width, maxLines int) string {
	r, ok := m.selectedRow()
	if !ok {
		return dimStyle.Render(truncate("(none)", width))
	}
	desc := strings.TrimSpace(r.opt.OptDescription())
	if desc == "" {
		return dimStyle.Render(truncate("(no description)", width))
	}
	lines := wrapText(desc, width)
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	off := m.descScroll
	maxOff := len(lines) - maxLines
	if off > maxOff {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}
	window := append([]string(nil), lines[off:off+maxLines]...)
	if off > 0 {
		window[0] = dimStyle.Render("↑ more")
	}
	if off+maxLines < len(lines) {
		window[len(window)-1] = dimStyle.Render("↓ more")
	}
	return strings.Join(window, "\n")
}

// wrapText word-wraps s to the given width, preserving paragraph breaks.
func wrapText(s string, width int) []string {
	if width < 1 {
		return strings.Split(s, "\n")
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
}

// clipLines truncates s to at most max lines.
func clipLines(s string, max int) string {
	if max < 1 {
		max = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n")
}

func (m model) renderList(max, width int) string {
	rows := m.rows()
	if len(rows) == 0 {
		return dimStyle.Render("  (none)")
	}
	if max < 1 {
		max = 1
	}
	content := width - 2
	if content < 1 {
		content = 1
	}
	start := 0
	if m.cursor >= max {
		start = m.cursor - max + 1
	}
	if start > len(rows)-max {
		start = len(rows) - max
	}
	if start < 0 {
		start = 0
	}
	end := start + max
	if end > len(rows) {
		end = len(rows)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		line := renderRow(m, rows[i], content)
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		if m.activeRow(rows[i]) {
			line = prefix + activeStyle.Render(line)
		} else {
			line = prefix + line
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// activeRow reports whether r is the row currently open for editing.
func (m model) activeRow(r row) bool {
	if !m.editing || r.opt == nil || r.opt.OptName() != m.editingName {
		return false
	}
	switch r.kind {
	case rowAdd:
		return m.editingIndex < 0
	case rowValue:
		return m.editingIndex == r.valIdx
	}
	switch r.opt.(type) {
	case *StringPositional, *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		return m.editingIndex < 0
	}
	return false
}

func renderRow(m model, r row, content int) string {
	if r.kind == rowSep {
		return dimStyle.Render(strings.Repeat("─", content))
	}
	name := r.opt.OptName()
	switch r.kind {
	case rowAdd:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		verb := "+ add "
		if m.isPath(r.opt) {
			verb = "+ pick "
		}
		return truncate(verb+name, content)
	case rowValue:
		if m.editing && m.editingName == name && m.editingIndex == r.valIdx {
			return m.editingLabel + m.input.View()
		}
		return truncate(valueLabel(name, r.valIdx)+r.value, content)
	}

	switch v := r.opt.(type) {
	case *SubcommandOption:
		return truncate("Command: "+v.Label, content)
	case *BoolFlag:
		fs := m.session.Flags[name]
		glyph := "[ ]"
		switch {
		case fs.Checked:
			glyph = "[✔]"
		case fs.Neg && v.Negative() != "":
			glyph = "[x]"
		}
		line := glyph + " " + truncate(name, content-4)
		if v.Short() != "" && len([]rune(line))+1+len([]rune(v.Short())) <= content {
			line += " " + dimStyle.Render(v.Short())
		}
		return line
	case *StringPositional:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *EnumPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		return truncate(name+": "+m.session.Flags[name].First(), content)
	case *CountFlag:
		n := m.session.Flags[name].Count
		line := name
		if n > 0 {
			line += ": " + strings.Repeat("×", n)
		}
		if v, ok := r.opt.(*CountFlag); ok && v.Short() != "" && len([]rune(line))+1+len([]rune(v.Short())) <= content {
			line += " " + dimStyle.Render(v.Short())
		}
		return truncate(line, content)
	case *PathPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	default:
		return "? " + name
	}
}

// pickerView renders the filesystem picker as a full-screen overlay.
func (m model) pickerView() string {
	return m.picker.render(m.contentWidth(), m.height)
}

// enumView renders the enum value menu.
func (m model) enumView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}
	normal := lipgloss.NewStyle().Padding(0, 2)
	selected := normal.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))

	lines := []string{titleStyle.Render(truncate("Select "+m.enumName, inner)), ""}
	values := m.filteredEnum()
	if len(values) == 0 {
		lines = append(lines, dimStyle.Render("(no match)"))
	}
	for i, v := range values {
		label := truncate(v, inner-2)
		if i == m.enumSel {
			lines = append(lines, selected.Render(label))
		} else {
			lines = append(lines, normal.Render(label))
		}
	}
	filterLine := truncate("filter: "+m.enumFilter+"▏", inner)
	lines = append(lines, "", filterLine, dimStyle.Render(truncate("type to filter · ctrl-w word · enter choose · up/down · esc cancel", inner)))
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

// actionView renders the vertical action popup: run, copy, print, cancel.
func (m model) actionView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}

	normal := lipgloss.NewStyle().Padding(0, 2)
	selected := normal.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))

	labels := []string{"Run", "Copy and exit", "Copy and stay", "Print composed command", "Cancel"}
	buttons := make([]string, len(labels))
	for i, l := range labels {
		plain := truncate(l, inner-2)
		if i == m.actionSel {
			buttons[i] = selected.Render(plain)
		} else {
			buttons[i] = normal.Render(plain)
		}
	}

	lines := []string{
		titleStyle.Render(truncate("Command", inner)),
		"",
		truncate("$ "+m.preview(), inner),
		"",
	}
	lines = append(lines, buttons...)
	lines = append(lines, "", dimStyle.Render(truncate("enter confirm · up/down or j/k choose · esc cancel", inner)))
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

// confirmView renders the quit-confirmation popup with Cancel on the left and
// Exit on the right.
func (m model) confirmView() string {
	cw := m.contentWidth()
	inner := cw - 6 // border (2) + horizontal padding (4)
	if inner < 10 {
		inner = 10
	}

	normal := lipgloss.NewStyle().Padding(0, 2)
	selected := normal.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))

	cancel := normal.Render("[ Cancel ]")
	exit := normal.Render("[ Exit ]")
	if m.confirmSel == 0 {
		cancel = selected.Render("[ Cancel ]")
	} else {
		exit = selected.Render("[ Exit ]")
	}

	lines := []string{
		titleStyle.Render(truncate("Quit flagpick?", inner)),
		"",
		truncate("$ "+m.preview(), inner),
		"",
		cancel + "  " + exit,
		"",
		dimStyle.Render(truncate("enter confirm · left/right choose · esc cancel", inner)),
	}
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) helpView() string {
	body := strings.Join([]string{
		titleStyle.Render("flagpick keymap"),
		"",
		"any printable   fuzzy-filter options",
		"backspace       delete last filter char",
		"ctrl-w          delete last filter word",
		"ctrl-j / ctrl-k cycle selection",
		"up / down       cycle selection (alt)",
		"ctrl-d / ctrl-u half-page down / up",
		"pgup / pgdown   scroll description",
		"space           toggle flag / edit value / add value",
		"space on path   open the file/dir picker",
		"space on enum   choose value from a menu",
		"space on command choose the subcommand",
		"space on count  cycle repeats (0-3)",
		"enter           open command menu (run/copy/print/cancel)",
		"ctrl-x          clear selected entry (uncheck/empty)",
		"ctrl-shift-x    clear all options",
		"ctrl-shift-j/k  reorder selected flag (or ctrl-up/down)",
		"tab / shift-tab switch mode",
		"ctrl-r          run (exec)",
		"ctrl-y          copy to clipboard and exit",
		"ctrl-shift-y    copy to clipboard and stay",
		"ctrl-p          print after exit",
		"esc             open quit confirm · esc cancel · esc clear filter",
		"ctrl-c          quit immediately",
		"ctrl-?          toggle this help",
		"",
		dimStyle.Render("press any key to close"),
	}, "\n")
	box := boxBorder.Padding(1, 2).Render(body)
	return lipgloss.Place(m.contentWidth(), m.height, lipgloss.Center, lipgloss.Center, box)
}
