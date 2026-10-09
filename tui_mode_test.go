package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestDetectMode checks that the prefilled shape is selected.
func TestDetectMode(t *testing.T) {
	m := testModesModel(t)
	if len(m.modes) != 2 || m.mode != 0 {
		t.Fatalf("modes=%d mode=%d, want 2/0", len(m.modes), m.mode)
	}
	if m.validate() {
		t.Fatal("empty Pattern mode should not validate")
	}

	// A session carrying -e selects the Regexp shape.
	s := NewSession(m.cfg)
	s.Flags["E"] = FlagState{Values: []string{"bar"}}
	got := detectMode(m.modes, s)
	if got != 1 {
		t.Fatalf("detectMode = %d, want 1", got)
	}
	m.mode = got
	m.session = s
	if !m.validate() {
		t.Fatalf("Regexp mode should validate: %s", m.errMsg)
	}
}

// TestCycleMode checks ctrl-n/ctrl-p mode cycling, including the PATTERN/PATH
// round-trip, and that the title shows the mode segments.
func TestCycleMode(t *testing.T) {
	m := testModesModel(t)
	if len(m.modeSiblings()) != 2 {
		t.Fatalf("siblings = %d, want 2", len(m.modeSiblings()))
	}

	// Empty session: cycling hops directly to the sibling.
	m.cycleMode(1)
	if m.mode != 1 {
		t.Fatalf("after cycle: mode=%d, want 1", m.mode)
	}
	m.cycleMode(-1)
	if m.mode != 0 {
		t.Fatalf("after cycle back: mode=%d, want 0", m.mode)
	}

	// Round-trip: PATTERN becomes PATH and back.
	m.session.PosVals["PATTERN"] = []string{"func"}
	m.cycleMode(1)
	if m.mode != 1 {
		t.Fatalf("cycle to Regexp: mode=%d", m.mode)
	}
	if !reflect.DeepEqual(m.session.PosVals["PATH"], []string{"func"}) || hasValue(m.session.PosVals["PATTERN"]) {
		t.Fatalf("reparse: PATH=%v PATTERN=%v", m.session.PosVals["PATH"], m.session.PosVals["PATTERN"])
	}
	m.cycleMode(-1)
	if m.mode != 0 || !reflect.DeepEqual(m.session.PosVals["PATTERN"], []string{"func"}) {
		t.Fatalf("round-trip back: mode=%d PATTERN=%v", m.mode, m.session.PosVals["PATTERN"])
	}

	// The title shows the mode segments.
	m.width, m.height = 100, 24
	view := m.View()
	if !strings.Contains(view, "Pattern") || !strings.Contains(view, "Regexp") {
		t.Fatalf("view does not show the mode segments:\n%s", view)
	}
}

// TestRegexpModeRequiresRegexp checks that the Regexp mode of rg.yaml requires
// at least one --regexp value, while Pattern does not.
func TestRegexpModeRequiresRegexp(t *testing.T) {
	b, err := os.ReadFile("config/rg.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var pattern, regexp Resolved
	for _, r := range cfg.Resolved() {
		switch r.Name {
		case "Pattern":
			pattern = r
		case "Regexp":
			regexp = r
		}
	}

	s := NewSession(cfg)
	if missing := MissingRequired(regexp, s); !reflect.DeepEqual(missing, []string{"Regular expression"}) {
		t.Fatalf("Regexp missing = %v, want [Regular expression]", missing)
	}
	if missing := MissingRequired(pattern, s); !reflect.DeepEqual(missing, []string{"PATTERN"}) {
		t.Fatalf("Pattern missing = %v, want [PATTERN] (--regexp must not apply)", missing)
	}

	s.Flags["Regular expression"] = FlagState{Values: []string{"foo"}}
	if missing := MissingRequired(regexp, s); len(missing) != 0 {
		t.Fatalf("Regexp with -e missing = %v, want none", missing)
	}
}
