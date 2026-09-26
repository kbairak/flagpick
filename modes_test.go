package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestNestedGroupExpansion(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
options:
  - name: A
    kind: flag
    type: bool
    long: --a
  - name: B
    kind: flag
    type: bool
    long: --b
  - name: C
    kind: flag
    type: bool
    long: --c
groups:
  inner: [A, B]
  outer: [inner, C]
modes:
  - name: M
    options: [outer]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := cfg.Resolved()[0]
	var names []string
	for _, f := range res.Flags {
		names = append(names, f.OptName())
	}
	if !reflect.DeepEqual(names, []string{"A", "B", "C"}) {
		t.Fatalf("expanded = %v, want [A B C]", names)
	}
}

func TestNestedGroupErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name:    "cycle",
			yaml:    "options:\n  - name: A\n    kind: flag\n    type: bool\n    long: --a\ngroups:\n  x: [y]\n  y: [x]\nmodes:\n  - name: M\n    options: [x]\n",
			wantSub: "cycle",
		},
		{
			name:    "unknown",
			yaml:    "options:\n  - name: A\n    kind: flag\n    type: bool\n    long: --a\ngroups:\n  x: [nope]\nmodes:\n  - name: M\n    options: [x]\n",
			wantSub: "unknown option or group",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfigBytes(t, []byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}
