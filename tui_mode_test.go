package main

import (
	"reflect"
	"testing"
)

// TestModes covers mode selection, auto-switch, and validation.
func TestModes(t *testing.T) {
	m := testModesModel(t)
	if len(m.modes) != 2 || m.mode != 0 {
		t.Fatalf("modes=%d mode=%d, want 2/0", len(m.modes), m.mode)
	}
	if m.validate() {
		t.Fatal("empty Pattern mode should not validate")
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
	m.cycleMode(1)
	if m.mode != 0 || !reflect.DeepEqual(m.session.PosVals["PATTERN"], []string{"func"}) {
		t.Fatalf("round-trip back: mode=%d PATTERN=%v", m.mode, m.session.PosVals["PATTERN"])
	}

	// A mode that can't parse the command is skipped.
	m.mode = 1
	m.session.Flags["E"] = FlagState{Values: []string{"bar"}}
	m.cycleMode(1) // Pattern can't parse --regexp -> stay
	if m.mode != 1 {
		t.Fatalf("unparseable mode not skipped: mode=%d", m.mode)
	}
	if !m.validate() {
		t.Fatalf("Regexp mode should validate: %s", m.errMsg)
	}
}
