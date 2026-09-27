package main

import (
	"os"
	"reflect"
	"testing"
)

func loadTestConfig(t *testing.T) *CommandConfig {
	t.Helper()
	b, err := os.ReadFile("config/rg.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return cfg
}

// res1 resolves a config to its first mode for tests.
func res1(cfg *CommandConfig) Resolved { return cfg.Resolved()[0] }

func TestAssembleGolden(t *testing.T) {
	cfg := loadTestConfig(t)

	t.Run("empty session", func(t *testing.T) {
		got := Assemble(res1(cfg), NewSession(cfg))
		if !reflect.DeepEqual(got, []string{}) {
			t.Fatalf("got %#v, want []string{}", got)
		}
	})

	t.Run("only ignore case", func(t *testing.T) {
		s := NewSession(cfg)
		ToggleFlag(cfg, s, "Ignore case")
		got := Assemble(res1(cfg), s)
		want := []string{"--ignore-case"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("conflict resolution", func(t *testing.T) {
		s := NewSession(cfg)
		ToggleFlag(cfg, s, "Smart case")
		ToggleFlag(cfg, s, "Ignore case")
		got := Assemble(res1(cfg), s)
		want := []string{"--ignore-case"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("order swap", func(t *testing.T) {
		cfg2, err := parseConfigBytes(t, []byte(
			"options:\n  - name: A\n    kind: flag\n    type: bool\n    long: --a\n  - name: B\n    kind: flag\n    type: bool\n    long: --b\n",
		))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		s := NewSession(cfg2)
		ToggleFlag(cfg2, s, "A")
		ToggleFlag(cfg2, s, "B")
		s.Order = []string{"B", "A"}
		got := Assemble(res1(cfg2), s)
		want := []string{"--b", "--a"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("positional plus flag", func(t *testing.T) {
		s := NewSession(cfg)
		ToggleFlag(cfg, s, "Ignore case")
		s.PosVals["PATTERN"] = []string{"foo"}
		got := Assemble(res1(cfg), s)
		want := []string{"--ignore-case", "foo"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("variadic positional", func(t *testing.T) {
		s := NewSession(cfg)
		s.PosVals["PATTERN"] = []string{"foo"}
		s.PosVals["PATH"] = []string{"a", "b c", "d"}
		got := Assemble(res1(cfg), s)
		want := []string{"foo", "a", "b c", "d"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("nullable bool", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"-n"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if !s.Flags["Line numbers"].Checked || s.Flags["Line numbers"].Neg {
			t.Fatalf("state = %+v", s.Flags["Line numbers"])
		}
		if got := Assemble(res1(cfg), s); !reflect.DeepEqual(got, []string{"--line-number"}) {
			t.Fatalf("on: %v", got)
		}
		s, err = Prefill(cfg, []string{"--no-line-number"})
		if err != nil {
			t.Fatalf("prefill neg: %v", err)
		}
		if s.Flags["Line numbers"].Checked || !s.Flags["Line numbers"].Neg {
			t.Fatalf("neg state = %+v", s.Flags["Line numbers"])
		}
		if got := Assemble(res1(cfg), s); !reflect.DeepEqual(got, []string{"--no-line-number"}) {
			t.Fatalf("off: %v", got)
		}
		s, err = Prefill(cfg, []string{"-N"})
		if err != nil {
			t.Fatalf("prefill neg short: %v", err)
		}
		if s.Flags["Line numbers"].Checked || !s.Flags["Line numbers"].Neg {
			t.Fatalf("neg short state = %+v", s.Flags["Line numbers"])
		}
		if got := Assemble(res1(cfg), s); !reflect.DeepEqual(got, []string{"--no-line-number"}) {
			t.Fatalf("neg short: %v", got)
		}
	})

	t.Run("negative short", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"-I"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if s.Flags["With filename"].Checked || !s.Flags["With filename"].Neg {
			t.Fatalf("state = %+v", s.Flags["With filename"])
		}
		if got := Assemble(res1(cfg), s); !reflect.DeepEqual(got, []string{"--no-filename"}) {
			t.Fatalf("got %v, want [--no-filename]", got)
		}
	})

	t.Run("string value flag", func(t *testing.T) {
		s := NewSession(cfg)
		s.Flags["Replace"] = FlagState{Values: []string{"bar"}}
		s.PosVals["PATTERN"] = []string{"foo"}
		got := Assemble(res1(cfg), s)
		want := []string{"--replace=bar", "foo"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("empty value flag omitted", func(t *testing.T) {
		s := NewSession(cfg)
		s.PosVals["PATTERN"] = []string{"foo"}
		got := Assemble(res1(cfg), s)
		want := []string{"foo"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("int value flag", func(t *testing.T) {
		s := NewSession(cfg)
		s.Flags["After context"] = FlagState{Values: []string{"3"}}
		s.PosVals["PATTERN"] = []string{"foo"}
		got := Assemble(res1(cfg), s)
		want := []string{"--after-context=3", "foo"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})
}

func TestRepeatableValueFlag(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(
		"options:\n  - name: E\n    kind: flag\n    type: string\n    long: --regexp\n    short: -e\n    repeatable: true\n  - name: PATH\n    kind: positional\n    type: string\n    variadic: true\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"-e", "a", "-e", "b", "-eb", "x"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	if got := s.Flags["E"].Values; !reflect.DeepEqual(got, []string{"a", "b", "b"}) {
		t.Fatalf("values = %v, want [a b b]", got)
	}
	if !reflect.DeepEqual(s.PosVals["PATH"], []string{"x"}) {
		t.Fatalf("PATH = %v, want [x]", s.PosVals["PATH"])
	}
	got := Assemble(res1(cfg), s)
	want := []string{"--regexp=a", "--regexp=b", "--regexp=b", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assemble = %v, want %v", got, want)
	}
}

func TestPrefillModes(t *testing.T) {
	cfg := loadTestConfig(t)

	// Default (Pattern) mode.
	s, err := Prefill(cfg, []string{"foo"})
	if err != nil {
		t.Fatalf("prefill foo: %v", err)
	}
	if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"foo"}) {
		t.Fatalf("PATTERN = %v, want [foo]", s.PosVals["PATTERN"])
	}

	// Regexp mode: -e makes all positionals paths.
	s, err = Prefill(cfg, []string{"-e", "foo", "src"})
	if err != nil {
		t.Fatalf("prefill -e: %v", err)
	}
	if !reflect.DeepEqual(s.Flags["Regular expression"].Values, []string{"foo"}) {
		t.Fatalf("regexp = %v, want [foo]", s.Flags["Regular expression"].Values)
	}
	if !reflect.DeepEqual(s.PosVals["PATH"], []string{"src"}) {
		t.Fatalf("PATH = %v, want [src]", s.PosVals["PATH"])
	}
	if hasValue(s.PosVals["PATTERN"]) {
		t.Fatalf("PATTERN should be empty in regexp mode: %v", s.PosVals["PATTERN"])
	}
}

func TestEnumAndSizeFlags(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(
		"options:\n  - name: C\n    kind: flag\n    type: enum\n    long: --color\n    values: [never, auto, always]\n  - name: M\n    kind: flag\n    type: size\n    long: --max-filesize\n  - name: P\n    kind: positional\n    type: string\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"--color=auto", "--max-filesize=10M", "x"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	got := Assemble(res1(cfg), s)
	want := []string{"--color=auto", "--max-filesize=10M", "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assemble = %v, want %v", got, want)
	}
	if _, err := Prefill(cfg, []string{"--color=sometimes", "x"}); err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	if _, err := Prefill(cfg, []string{"--max-filesize=10X", "x"}); err == nil {
		t.Fatal("expected error for invalid size value")
	}
}

func TestInfoMode(t *testing.T) {
	cfg := loadTestConfig(t)
	info := cfg.Resolved()[2]
	if info.Name != "Info" {
		t.Fatalf("mode[2] = %q, want Info", info.Name)
	}

	s, err := Prefill(cfg, []string{"--help"})
	if err != nil {
		t.Fatalf("prefill --help: %v", err)
	}
	if !s.Flags["Help"].Checked {
		t.Fatalf("Help not checked: %+v", s.Flags["Help"])
	}
	if got := Assemble(info, s); !reflect.DeepEqual(got, []string{"--help"}) {
		t.Fatalf("assemble = %v, want [--help]", got)
	}

	// Conflicting info flags: last wins.
	s, err = Prefill(cfg, []string{"-h", "-V"})
	if err != nil {
		t.Fatalf("prefill -h -V: %v", err)
	}
	if s.Flags["Help"].Checked || !s.Flags["Version"].Checked {
		t.Fatalf("conflict: help=%v version=%v", s.Flags["Help"].Checked, s.Flags["Version"].Checked)
	}
}

func TestGenerateMode(t *testing.T) {
	cfg := loadTestConfig(t)
	gen := cfg.Resolved()[3]
	if gen.Name != "Generate" {
		t.Fatalf("mode[3] = %q, want Generate", gen.Name)
	}
	s, err := Prefill(cfg, []string{"--generate", "man"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	if got := Assemble(gen, s); !reflect.DeepEqual(got, []string{"--generate=man"}) {
		t.Fatalf("assemble = %v, want [--generate=man]", got)
	}
	if _, err := Prefill(cfg, []string{"--generate", "bogus"}); err == nil {
		t.Fatal("expected error for invalid --generate value")
	}
}

func TestShellQuote(t *testing.T) {
	got := ShellQuote([]string{"rg", "--ignore-case", "foo bar", "it's", "", "a.b/c"})
	want := `rg --ignore-case 'foo bar' 'it'\''s' '' a.b/c`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrefill(t *testing.T) {
	cfg := loadTestConfig(t)

	t.Run("short flag and positional", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"-i", "foo"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if !s.Flags["Ignore case"].Checked {
			t.Errorf("Ignore case not checked")
		}
		if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"foo"}) {
			t.Errorf("PATTERN = %v, want [foo]", s.PosVals["PATTERN"])
		}
	})

	t.Run("long flag and positional", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"--ignore-case", "foo"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if !s.Flags["Ignore case"].Checked {
			t.Errorf("Ignore case not checked")
		}
		if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"foo"}) {
			t.Errorf("PATTERN = %v, want [foo]", s.PosVals["PATTERN"])
		}
	})

	t.Run("string value flag forms", func(t *testing.T) {
		forms := [][]string{
			{"-r", "bar", "foo"},
			{"-rbar", "foo"},
			{"--replace", "bar", "foo"},
			{"--replace=bar", "foo"},
		}
		for _, in := range forms {
			s, err := Prefill(cfg, in)
			if err != nil {
				t.Fatalf("prefill %v: %v", in, err)
			}
			if s.Flags["Replace"].First() != "bar" {
				t.Errorf("%v: Replace = %q, want bar", in, s.Flags["Replace"].First())
			}
			if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"foo"}) {
				t.Errorf("%v: PATTERN = %v, want [foo]", in, s.PosVals["PATTERN"])
			}
		}
	})

	t.Run("missing value flag value", func(t *testing.T) {
		if _, err := Prefill(cfg, []string{"--replace"}); err == nil {
			t.Fatal("expected error for --replace with no value")
		}
		if _, err := Prefill(cfg, []string{"-r"}); err == nil {
			t.Fatal("expected error for -r with no value")
		}
	})

	t.Run("int value flag forms", func(t *testing.T) {
		for _, in := range [][]string{
			{"-A", "3", "foo"},
			{"-A3", "foo"},
			{"--after-context", "3", "foo"},
			{"--after-context=3", "foo"},
		} {
			s, err := Prefill(cfg, in)
			if err != nil {
				t.Fatalf("prefill %v: %v", in, err)
			}
			if s.Flags["After context"].First() != "3" {
				t.Errorf("%v: After context = %q, want 3", in, s.Flags["After context"].First())
			}
		}
	})

	t.Run("invalid int value flag", func(t *testing.T) {
		if _, err := Prefill(cfg, []string{"-A", "x", "foo"}); err == nil {
			t.Fatal("expected error for non-numeric --after-context")
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		_, err := Prefill(cfg, []string{"-Q"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("variadic absorbs extra positionals", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"a"}) {
			t.Errorf("PATTERN = %v, want [a]", s.PosVals["PATTERN"])
		}
		if !reflect.DeepEqual(s.PosVals["PATH"], []string{"b", "c"}) {
			t.Errorf("PATH = %v, want [b c]", s.PosVals["PATH"])
		}
	})

	t.Run("too many positionals", func(t *testing.T) {
		cfg2, err := parseConfigBytes(t, []byte(
			"options:\n  - name: A\n    kind: positional\n    type: string\n  - name: B\n    kind: positional\n    type: string\n",
		))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if _, err := Prefill(cfg2, []string{"a", "b", "c"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("conflict last wins", func(t *testing.T) {
		s, err := Prefill(cfg, []string{"-i", "-s", "foo"})
		if err != nil {
			t.Fatalf("prefill: %v", err)
		}
		if s.Flags["Ignore case"].Checked {
			t.Errorf("Ignore case should be unchecked")
		}
		if !s.Flags["Case sensitive"].Checked {
			t.Errorf("Case sensitive should be checked")
		}
		if !reflect.DeepEqual(s.PosVals["PATTERN"], []string{"foo"}) {
			t.Errorf("PATTERN = %v, want [foo]", s.PosVals["PATTERN"])
		}
	})
}

func TestMissingRequired(t *testing.T) {
	cfg := loadTestConfig(t)
	s := NewSession(cfg)
	got := MissingRequired(res1(cfg), s)
	want := []string{"PATTERN"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	s.PosVals["PATTERN"] = []string{"foo"}
	if got := MissingRequired(res1(cfg), s); len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
	}

	cfg2, err := parseConfigBytes(t, []byte(
		"options:\n  - name: FILES\n    kind: positional\n    type: string\n    required: true\n    variadic: true\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s2 := NewSession(cfg2)
	if got := MissingRequired(res1(cfg2), s2); !reflect.DeepEqual(got, []string{"FILES"}) {
		t.Fatalf("empty variadic: got %#v, want [FILES]", got)
	}
	s2.PosVals["FILES"] = []string{"a"}
	if got := MissingRequired(res1(cfg2), s2); len(got) != 0 {
		t.Fatalf("filled variadic: got %#v, want empty", got)
	}
}

func TestCountFlag(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(
		"options:\n  - name: U\n    kind: flag\n    type: count\n    long: --unrestricted\n    short: -u\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"-u", "-u", "--unrestricted"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	if s.Flags["U"].Count != 3 {
		t.Fatalf("count = %d, want 3", s.Flags["U"].Count)
	}
	got := Assemble(res1(cfg), s)
	want := []string{"--unrestricted", "--unrestricted", "--unrestricted"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assemble = %v, want %v", got, want)
	}

	s2, err := Prefill(cfg, []string{"-uu"})
	if err != nil {
		t.Fatalf("prefill -uu: %v", err)
	}
	if s2.Flags["U"].Count != 2 {
		t.Fatalf("-uu count = %d, want 2", s2.Flags["U"].Count)
	}
}
