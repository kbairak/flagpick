package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// maxPickerEntries caps the recursive walk so huge trees stay responsive.
const maxPickerEntries = 10000

// pickerEntry is one candidate path found under the picker root.
type pickerEntry struct {
	path string // display path relative to the root, using / separators
	dir  bool
}

// pickerMatch is a filtered entry plus the rune indices the filter matched, for
// highlighting.
type pickerMatch struct {
	entry pickerEntry
	idx   []int
}

// picker is a recursive, fuzzy-filtered path chooser. It walks every path under
// the root once, filters them with sahilm/fuzzy, and lets the user pick one.
// The query line is a bubbles/textinput so all editing keys work unchanged.
type picker struct {
	dir     string
	all     []pickerEntry
	matches []pickerMatch
	cursor  int
	filter  string
	input   textinput.Model
	errMsg  string
}

func newPicker(dir string) picker {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 0
	in.Focus()
	p := picker{dir: dir, input: in}
	p.reload()
	return p
}

// reload walks the root (skipping hidden directories) and rebuilds the matches.
func (p *picker) reload() {
	p.errMsg = ""
	p.all = nil
	root := p.dir
	_ = fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if len(p.all) >= maxPickerEntries {
			return fs.SkipAll
		}
		p.all = append(p.all, pickerEntry{path: path, dir: d.IsDir()})
		return nil
	})
	p.refilter()
}

// refilter recomputes the matches for the current filter.
func (p *picker) refilter() {
	p.cursor = 0
	if p.filter == "" {
		p.matches = make([]pickerMatch, 0, len(p.all))
		for _, e := range p.all {
			p.matches = append(p.matches, pickerMatch{entry: e})
		}
		return
	}
	targets := make([]string, len(p.all))
	for i, e := range p.all {
		targets[i] = e.path
	}
	ranks := fuzzy.Find(p.filter, targets)
	p.matches = make([]pickerMatch, 0, len(ranks))
	for _, r := range ranks {
		p.matches = append(p.matches, pickerMatch{entry: p.all[r.Index], idx: r.MatchedIndexes})
	}
}

// move shifts the cursor without wrapping.
func (p *picker) move(delta int) {
	n := len(p.matches)
	if n == 0 {
		p.cursor = 0
		return
	}
	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= n {
		p.cursor = n - 1
	}
}

func (p picker) selected() (pickerEntry, bool) {
	if p.cursor < 0 || p.cursor >= len(p.matches) {
		return pickerEntry{}, false
	}
	return p.matches[p.cursor].entry, true
}

// selectedPath returns the absolute path of the highlighted entry.
func (p picker) selectedPath() (string, bool) {
	e, ok := p.selected()
	if !ok {
		return "", false
	}
	return filepath.Join(p.dir, filepath.FromSlash(e.path)), true
}

// update forwards a message to the query input and refilters when it changes.
// Up/down/enter/esc are handled by the caller and never reach here.
func (p picker) update(msg tea.Msg) (picker, tea.Cmd) {
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if v := p.input.Value(); v != p.filter {
		p.filter = v
		p.refilter()
	}
	return p, cmd
}

// Picker styles, loosely following fzf.
var (
	pickerDirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	pickerMatchStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	pickerPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	pickerCountStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// render draws the picker full-screen: a dim header with the directory, the
// match list, an n/m count, and a bottom `> query` prompt.
func (p picker) render(width, height int) string {
	cw := width - 1
	if cw < 1 {
		cw = 1
	}
	if height < 3 {
		height = 3
	}
	listH := height - 3

	lines := make([]string, 0, height)
	lines = append(lines, dimStyle.Render(truncate("  "+p.dir, cw)))

	var rows []string
	switch {
	case p.errMsg != "":
		rows = append(rows, errStyle.Render(truncate("  "+p.errMsg, cw)))
	case len(p.matches) == 0:
		rows = append(rows, dimStyle.Render(truncate("  (no match)", cw)))
	default:
		start := 0
		if p.cursor >= listH {
			start = p.cursor - listH + 1
		}
		if start > len(p.matches)-listH {
			start = len(p.matches) - listH
		}
		if start < 0 {
			start = 0
		}
		end := start + listH
		if end > len(p.matches) {
			end = len(p.matches)
		}
		for i := start; i < end; i++ {
			if i == p.cursor {
				rows = append(rows, cursorStyle.Render(truncate("> "+plainLine(p.matches[i].entry), cw)))
			} else {
				rows = append(rows, "  "+p.nameLine(p.matches[i], cw-2))
			}
		}
	}
	for len(rows) < listH {
		rows = append(rows, "")
	}
	if len(rows) > listH {
		rows = rows[:listH]
	}
	lines = append(lines, rows...)

	count := "0/0"
	if len(p.matches) > 0 {
		count = fmt.Sprintf("%d/%d", p.cursor+1, len(p.matches))
	}
	lines = append(lines, pickerCountStyle.Render(truncate("  "+count, cw)))

	in := p.input
	// textinput renders one cell wider than Width (the cursor), so leave room.
	in.Width = cw - 3
	if in.Width < 1 {
		in.Width = 1
	}
	lines = append(lines, pickerPromptStyle.Render("> ")+in.View())

	return strings.Join(lines, "\n")
}

// plainLine is the unstyled path of an entry (with a trailing slash for dirs).
func plainLine(e pickerEntry) string {
	if e.dir {
		return e.path + "/"
	}
	return e.path
}

// nameLine renders an entry's path, coloring directories and highlighting the
// characters the filter matched, truncated to width cells.
func (p picker) nameLine(m pickerMatch, width int) string {
	if width < 1 {
		return ""
	}
	disp := []rune(m.entry.path)
	if m.entry.dir {
		disp = append(disp, '/')
	}
	truncated := len(disp) > width
	if truncated {
		if width == 1 {
			return "…"
		}
		disp = disp[:width-1]
	}
	matched := map[int]bool{}
	for _, i := range m.idx {
		matched[i] = true
	}
	var b strings.Builder
	for i, r := range disp {
		switch {
		case matched[i] && p.filter != "":
			b.WriteString(pickerMatchStyle.Render(string(r)))
		case m.entry.dir:
			b.WriteString(pickerDirStyle.Render(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	if truncated {
		b.WriteString("…")
	}
	return b.String()
}
