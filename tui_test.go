package main

import (
	"testing"
)

const tuiFixture = `pool:
  options:
    - name: Ignore case
      kind: flag
      type: bool
      long: --ignore-case
      short: -i
      description: >
        Toggle case-insensitive matching. This description is intentionally long so
        that the description pane has to wrap it across several lines and, at small
        heights, scroll it.
    - name: Smart case
      kind: flag
      type: bool
      long: --smart-case
      short: -S
      description: Smart case matching.
    - name: PATTERN
      kind: positional
      type: string
      required: true
      description: A regular expression used for searching.
    - name: PATH
      kind: positional
      type: string
      required: false
      variadic: true
      description: A file or directory to search.
options: [Ignore case, Smart case, PATTERN, PATH]
`

func testModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(tuiFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

const pathFixture = `pool:
  options:
    - name: PATH
      kind: positional
      type: path
      required: false
      variadic: true
      description: A file or directory to search.
options: [PATH]
`

func testPathModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(pathFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

const valueFixture = `pool:
  options:
    - name: Replace
      kind: flag
      type: string
      long: --replace
      short: -r
    - name: After context
      kind: flag
      type: int
      long: --after-context
      short: -A
    - name: PATTERN
      kind: positional
      type: string
      required: true
options: [Replace, After context, PATTERN]
`

func testValueModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(valueFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

// variadicRows returns the rows of the PATH positional (filtered to PATH).
func variadicRows(t *testing.T, m model) []row {
	t.Helper()
	m.filter = "PATH"
	var out []row
	for _, r := range m.rows() {
		if r.opt != nil && r.opt.OptName() == "PATH" {
			out = append(out, r)
		}
	}
	return out
}

const modesFixture = `pool:
  options:
    - name: E
      kind: flag
      type: string
      long: --regexp
      short: -e
      repeatable: true
    - name: PATTERN
      kind: positional
      type: string
      required: true
    - name: PATH
      kind: positional
      type: path
      variadic: true
modes:
  - name: Pattern
    usage: rg [OPTIONS] PATTERN [PATH ...]
    options: [PATTERN, PATH]
  - name: Regexp
    usage: rg [OPTIONS] -e PATTERN ... [PATH ...]
    options: [E, PATH]
`

func testModesModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(modesFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}
