package main

import (
	"reflect"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	want := []string{"--ignore-case", "foo"}
	if err := saveLast("rg", want); err != nil {
		t.Fatalf("saveLast: %v", err)
	}
	lc, ok := loadLast()
	if !ok {
		t.Fatal("loadLast returned false")
	}
	if lc.Command != "rg" || !reflect.DeepEqual(lc.Args, want) {
		t.Fatalf("loaded %+v, want rg/%v", lc, want)
	}

	if got := prefillTokens("rg", []string{"ignored"}, true); !reflect.DeepEqual(got, want) {
		t.Errorf("resume same command: got %v, want %v", got, want)
	}
	if got := prefillTokens("other", []string{"keep"}, true); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Errorf("resume other command: got %v, want [keep]", got)
	}
	if got := prefillTokens("rg", []string{"keep"}, false); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Errorf("no resume: got %v, want [keep]", got)
	}
}

func TestStateMissing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if _, ok := loadLast(); ok {
		t.Fatal("loadLast on empty state returned true")
	}
	if got := prefillTokens("rg", []string{"keep"}, true); !reflect.DeepEqual(got, []string{"keep"}) {
		t.Errorf("resume with no state: got %v, want [keep]", got)
	}
}
