package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// pickerEntry is one row in the filesystem picker.
type pickerEntry struct {
	name string
	dir  bool
}

// picker is a minimal filesystem browser with fuzzy filtering. It is used by
// the TUI to choose `path` positional values.
type picker struct {
	dir     string
	entries []pickerEntry
	cursor  int
	filter  string
	errMsg  string
}

func newPicker(dir string) picker {
	p := picker{dir: dir}
	p.reload()
	return p
}

// reload re-reads the current directory, hiding dotfiles and listing
// directories before files.
func (p *picker) reload() {
	p.errMsg = ""
	p.cursor = 0
	ents, err := os.ReadDir(p.dir)
	if err != nil {
		p.errMsg = err.Error()
		p.entries = nil
		return
	}
	var dirs, files []pickerEntry
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, pickerEntry{name: name, dir: true})
		} else {
			files = append(files, pickerEntry{name: name})
		}
	}
	p.entries = append(dirs, files...)
	// Start on the first real entry; index 0 is the synthetic ".." row.
	if p.filter == "" && p.hasParent() && len(p.entries) > 0 {
		p.cursor = 1
	}
}

// visible returns the entries matching the current filter. A ".." row is
// prepended when not at the filesystem root and no filter is active.
func (p picker) visible() []pickerEntry {
	var out []pickerEntry
	if p.filter == "" && p.hasParent() {
		out = append(out, pickerEntry{name: "..", dir: true})
	}
	for _, e := range p.entries {
		if p.filter == "" || fuzzyMatch(e.name, p.filter) {
			out = append(out, e)
		}
	}
	return out
}

func (p picker) hasParent() bool {
	return filepath.Dir(p.dir) != p.dir
}

func (p *picker) move(delta int) {
	n := len(p.visible())
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
	list := p.visible()
	if p.cursor < 0 || p.cursor >= len(list) {
		return pickerEntry{}, false
	}
	return list[p.cursor], true
}

// selectedPath returns the absolute path of the highlighted entry. It returns
// false for ".." and for an empty list.
func (p picker) selectedPath() (string, bool) {
	e, ok := p.selected()
	if !ok || e.name == ".." {
		return "", false
	}
	return filepath.Join(p.dir, e.name), true
}

// descend enters the highlighted directory.
func (p *picker) descend() {
	e, ok := p.selected()
	if !ok || !e.dir || e.name == ".." {
		return
	}
	p.dir = filepath.Join(p.dir, e.name)
	p.filter = ""
	p.reload()
}

// parent moves to the parent directory. It reports whether it moved.
func (p *picker) parent() bool {
	up := filepath.Dir(p.dir)
	if up == p.dir {
		return false
	}
	p.dir = up
	p.filter = ""
	p.reload()
	return true
}

// pushFilter appends printable characters to the filter.
func (p *picker) pushFilter(s string) {
	p.filter += s
	p.cursor = 0
}

// backspace removes the last filter rune. It reports whether it consumed the
// key (i.e. the filter was non-empty).
func (p *picker) backspace() bool {
	if p.filter == "" {
		return false
	}
	r := []rune(p.filter)
	p.filter = string(r[:len(r)-1])
	p.cursor = 0
	return true
}

func (p picker) render(width, height int) string {
	cw := width - 1
	if cw < 1 {
		cw = 1
	}
	innerW := cw - 2
	if innerW < 10 {
		innerW = 10
	}
	listH := height - 5
	if listH < 1 {
		listH = 1
	}

	head := titleStyle.Render(truncate("Pick "+p.dir, innerW))
	filterLine := truncate("filter: "+p.filter+"▏", innerW)

	var b strings.Builder
	list := p.visible()
	switch {
	case p.errMsg != "":
		b.WriteString(errStyle.Render(truncate(p.errMsg, innerW)))
	case len(list) == 0:
		b.WriteString(dimStyle.Render("  (empty)"))
	default:
		start := 0
		if p.cursor >= listH {
			start = p.cursor - listH + 1
		}
		end := start + listH
		if end > len(list) {
			end = len(list)
		}
		for i := start; i < end; i++ {
			line := p.entryLine(list[i], innerW-2)
			if i == p.cursor {
				line = cursorStyle.Render("> " + line)
			} else {
				line = "  " + line
			}
			b.WriteString(line)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	content := head + "\n" + filterLine + "\n" + b.String()
	box := boxBorder.Width(innerW).Height(listH + 2).Render(content)
	hint := dimStyle.Render(truncate("enter select · l open · h back · type to filter · esc cancel", cw))
	return box + "\n" + hint
}

func (p picker) entryLine(e pickerEntry, width int) string {
	if e.dir {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(truncate(e.name+"/", width))
	}
	return truncate(e.name, width)
}
