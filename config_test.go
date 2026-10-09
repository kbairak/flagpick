package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var (
	_ Flag       = (*BoolFlag)(nil)
	_ Positional = (*StringPositional)(nil)
)

func parseConfigBytes(t *testing.T, b []byte) (*CommandConfig, error) {
	t.Helper()
	return parseConfig(b)
}

func TestParseRealConfig(t *testing.T) {
	b, err := os.ReadFile("config/rg.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Flags) != 105 {
		t.Fatalf("flags = %d, want 105", len(cfg.Flags))
	}
	modes := cfg.Resolved()
	if len(modes) != 4 {
		t.Fatalf("modes = %d, want 4", len(modes))
	}
	if modes[0].Name != "Pattern" || !modes[0].HasPositional("PATTERN") || !modes[0].HasFlag("Ignore case") {
		t.Fatalf("Pattern mode = %+v", modes[0])
	}
	if modes[1].Name != "Regexp" || modes[1].HasPositional("PATTERN") || !modes[1].HasFlag("Regular expression") {
		t.Fatalf("Regexp mode = %+v", modes[1])
	}
	if modes[2].Name != "Info" || !modes[2].HasFlag("Help") || modes[2].HasPositional("PATTERN") {
		t.Fatalf("Info mode = %+v", modes[2])
	}
	if modes[0].HasFlag("Help") {
		t.Fatal("Info flags should be exclusive to the Info mode")
	}
	if modes[3].Name != "Generate" || !modes[3].HasFlag("Generate") || modes[3].HasPositional("PATTERN") {
		t.Fatalf("Generate mode = %+v", modes[3])
	}
	if modes[0].HasFlag("Generate") {
		t.Fatal("Generate flag should be exclusive to the Generate mode")
	}
	// Nullable bool: Heading has a negative form.
	var heading Flag
	for _, fl := range cfg.Flags {
		if fl.OptName() == "Heading" {
			heading = fl
		}
	}
	if heading == nil || heading.Negative() != "--no-heading" {
		t.Fatalf("Heading negative = %q", heading.Negative())
	}
	if modes[0].HasFlag("Regular expression") {
		t.Fatal("Regexp flag should be exclusive to the Regexp mode")
	}
	var regexpFlag Flag
	for _, fl := range modes[1].Flags {
		if fl.OptName() == "Regular expression" {
			regexpFlag = fl
		}
	}
	if regexpFlag == nil || !regexpFlag.Required() {
		t.Fatal("Regular expression should be required in the Regexp mode")
	}
	if len(cfg.Positionals) != 2 {
		t.Fatalf("positionals = %d, want 2", len(cfg.Positionals))
	}
	var ignoreCase Flag
	for _, fl := range cfg.Flags {
		if fl.OptName() == "Ignore case" {
			ignoreCase = fl
		}
	}
	if _, ok := ignoreCase.(*BoolFlag); !ok {
		t.Fatalf("Ignore case type = %T, want *BoolFlag", ignoreCase)
	}
	if ignoreCase.Short() != "-i" {
		t.Errorf("short = %q, want %q", ignoreCase.Short(), "-i")
	}
	if got := ignoreCase.Conflicts(); len(got) != 2 || got[0] != "Case sensitive" || got[1] != "Smart case" {
		t.Errorf("conflicts = %v, want [Case sensitive Smart case]", got)
	}
	p, ok := cfg.Positionals[0].(*StringPositional)
	if !ok {
		t.Fatalf("Positionals[0] type = %T, want *StringPositional", cfg.Positionals[0])
	}
	if !p.Required() {
		t.Errorf("PATTERN.Required() = false, want true")
	}
	path, ok := cfg.Positionals[1].(*PathPositional)
	if !ok {
		t.Fatalf("Positionals[1] type = %T, want *PathPositional", cfg.Positionals[1])
	}
	if path.OptName() != "PATH" {
		t.Errorf("name = %q, want PATH", path.OptName())
	}
	if path.Required() {
		t.Errorf("PATH.Required() = true, want false")
	}
	if !path.Variadic() {
		t.Errorf("PATH.Variadic() = false, want true")
	}
	var replace Flag
	for _, f := range cfg.Flags {
		if f.OptName() == "Replace" {
			replace = f
		}
	}
	if replace == nil {
		t.Fatal("Replace flag not found")
	}
	if _, ok := replace.(*StringFlag); !ok {
		t.Fatalf("Replace type = %T, want *StringFlag", replace)
	}
	if !replace.TakesValue() {
		t.Errorf("Replace.TakesValue() = false, want true")
	}
	var after Flag
	for _, f := range cfg.Flags {
		if f.OptName() == "After context" {
			after = f
		}
	}
	if _, ok := after.(*IntFlag); !ok {
		t.Fatalf("After context type = %T, want *IntFlag", after)
	}
}

func TestParseDirenvConfig(t *testing.T) {
	b, err := os.ReadFile("config/direnv.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var names []string
	for _, r := range cfg.Resolved() {
		names = append(names, r.Name)
	}
	want := []string{
		"allow", "block", "edit", "exec", "export", "fetchurl", "help",
		"hook", "prune", "reload", "status", "stdlib", "version", "log",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("paths = %v, want %v", names, want)
	}

	// A dispatcher opens with no tokens (the TUI picks the first path).
	if _, err := Prefill(cfg, nil); err != nil {
		t.Fatalf("empty prefill: %v", err)
	}

	// Aliases resolve to the canonical verb, and exec's tail is a passthrough.
	s, err := Prefill(cfg, []string{"grant"})
	if err != nil {
		t.Fatalf("prefill alias: %v", err)
	}
	if got := Assemble(cfg.Resolved()[0], s); !reflect.DeepEqual(got, []string{"allow"}) {
		t.Fatalf("alias assemble = %v, want [allow]", got)
	}
	s, err = Prefill(cfg, []string{"exec", "./proj", "rg", "--json", "-i", "foo"})
	if err != nil {
		t.Fatalf("prefill exec: %v", err)
	}
	got := Assemble(cfg.Resolved()[3], s)
	wantArgv := []string{"exec", "./proj", "rg", "--json", "-i", "foo"}
	if !reflect.DeepEqual(got, wantArgv) {
		t.Fatalf("exec assemble = %v, want %v", got, wantArgv)
	}
}

// resolvedByName returns the selectable path whose Name equals name.
func resolvedByName(t *testing.T, cfg *CommandConfig, name string) Resolved {
	t.Helper()
	for _, r := range cfg.Resolved() {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no resolved path named %q", name)
	return Resolved{}
}

func TestSubcommandDescriptions(t *testing.T) {
	b, err := os.ReadFile("config/direnv.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// A subcommand path carries its own description.
	allow := resolvedByName(t, cfg, "allow")
	if !strings.Contains(allow.Description, "Grants direnv permission") {
		t.Fatalf("description = %q, want it to mention the permission grant", allow.Description)
	}

	// A flag-less subcommand assembles to just its canonical verb.
	help := resolvedByName(t, cfg, "help")
	if got := Assemble(help, NewSession(cfg)); !reflect.DeepEqual(got, []string{"help"}) {
		t.Fatalf("assemble = %v, want [help]", got)
	}
}

func TestRequiredSubcommandArgBlocksRun(t *testing.T) {
	b, err := os.ReadFile("config/direnv.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// A subcommand with a required positional reports it as missing.
	hook := resolvedByName(t, cfg, "hook")
	missing := MissingRequired(hook, NewSession(cfg))
	if len(missing) == 0 || !strings.Contains(strings.Join(missing, ", "), "Hook shell") {
		t.Fatalf("missing = %v, want Hook shell", missing)
	}
}

func TestParseGitConfig(t *testing.T) {
	b, err := os.ReadFile("config/git.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byName := map[string]Resolved{}
	for _, r := range cfg.Resolved() {
		byName[r.Name] = r
	}
	for _, want := range []string{"add", "commit", "rebase", "remote add", "remote set-url", "stash pop", "config set"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing path %q", want)
		}
	}
	// Global flags live on the root, so a subcommand path sees them.
	if r, ok := byName["commit"]; !ok || !r.HasFlag("Help") || !r.HasFlag("Git dir") {
		t.Fatalf("commit path missing inherited global flags")
	}
	if r := byName["remote add"]; !r.HasPositional("Remote name") {
		t.Fatalf("remote add missing positional")
	}
}

func TestInvalidConfigs(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name: "malformed yaml",
			yaml: "options:\n  - name: x\n    kind: flag\n    type: bool\n",
		},
		{
			name:    "unknown top-level field",
			yaml:    "options: []\nbogus: 1\n",
			wantSub: "bogus",
		},
		{
			name:    "unknown option field",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n      long: --x\n      nope: 1\n",
			wantSub: "unknown field",
		},
		{
			name:    "unsupported flag type",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: number\n      long: --x\n",
			wantSub: "unsupported flag type",
		},
		{
			name:    "unsupported positional type",
			yaml:    "pool:\n  options:\n    - name: X\n      kind: positional\n      type: integer\n",
			wantSub: "unsupported positional type",
		},
		{
			name:    "missing flag name",
			yaml:    "pool:\n  options:\n    - kind: flag\n      type: bool\n      long: --x\n",
			wantSub: "missing a name",
		},
		{
			name:    "missing long form",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n",
			wantSub: "missing a long form",
		},
		{
			name:    "duplicate flag names",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n      long: --x\n    - name: x\n      kind: flag\n      type: bool\n      long: --y\n",
			wantSub: "duplicate option name",
		},
		{
			name:    "positional after variadic",
			yaml:    "pool:\n  options:\n    - {name: X, kind: positional, type: string, variadic: true}\n    - {name: Y, kind: positional, type: string}\noptions: [X, Y]\n",
			wantSub: "must not follow a variadic",
		},
		{
			name:    "two variadics",
			yaml:    "pool:\n  options:\n    - {name: X, kind: positional, type: string, variadic: true}\n    - {name: Y, kind: positional, type: string, variadic: true}\noptions: [X, Y]\n",
			wantSub: "only one variadic",
		},
		{
			name:    "negative short without negative",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n      long: --x\n      negative_short: \"-y\"\n",
			wantSub: "requires a negative form",
		},
		{
			name:    "negative short double dash",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n      long: --x\n      negative: --no-x\n      negative_short: \"--y\"\n",
			wantSub: "negative short must start with a single -",
		},
		{
			name:    "negative short equals short",
			yaml:    "pool:\n  options:\n    - name: x\n      kind: flag\n      type: bool\n      long: --x\n      short: \"-x\"\n      negative: --no-x\n      negative_short: \"-x\"\n",
			wantSub: "negative short must differ from short",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfigBytes(t, []byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestConfigNotFound(t *testing.T) {
	_, err := LoadConfig("definitely-not-a-real-command-xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*ConfigNotFoundError); !ok {
		t.Fatalf("error type = %T, want *ConfigNotFoundError", err)
	}
}

func minimalConfig(flagName string) []byte {
	return []byte("pool:\n  options:\n    - name: " + flagName + "\n      kind: flag\n      type: bool\n      long: --x\noptions: [" + flagName + "]\n")
}

func TestFindConfig(t *testing.T) {
	data := t.TempDir()
	configDir := t.TempDir()

	// Config (user override) wins when both define the command.
	if err := os.WriteFile(filepath.Join(data, "rg.yaml"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "rg.yaml"), []byte("config"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, ok := findConfig(configDir, data, "rg")
	if !ok {
		t.Fatal("findConfig returned false")
	}
	if path != filepath.Join(configDir, "rg.yaml") {
		t.Fatalf("path = %q, want config path", path)
	}

	// Miss.
	if _, ok := findConfig(configDir, data, "nope"); ok {
		t.Fatal("findConfig found a nonexistent command")
	}
}

func TestLoadConfigPrecedence(t *testing.T) {
	dataHome := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dataDir := filepath.Join(dataHome, "flagpick")
	configDir := filepath.Join(configHome, "flagpick")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Data version and config version differ; config (override) must win.
	if err := os.WriteFile(filepath.Join(dataDir, "rg.yaml"), minimalConfig("from-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "rg.yaml"), minimalConfig("from-config"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig("rg")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Flags) != 1 || cfg.Flags[0].OptName() != "from-config" {
		t.Fatalf("LoadConfig picked %v, want config version", cfg.Flags)
	}

	// A command present only in the data dir is found.
	if err := os.WriteFile(filepath.Join(dataDir, "solo.yaml"), minimalConfig("solo"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadConfig("solo")
	if err != nil {
		t.Fatalf("LoadConfig(solo): %v", err)
	}
	if len(cfg.Flags) != 1 || cfg.Flags[0].OptName() != "solo" {
		t.Fatalf("LoadConfig(solo) picked %v", cfg.Flags)
	}
}

func TestManifestUpToDate(t *testing.T) {
	b, err := os.ReadFile("config/manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	files, err := filepath.Glob("config/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want, ok := manifest[name]
		if !ok {
			t.Errorf("config %s missing from manifest; run 'make manifest'", name)
			continue
		}
		if got := md5Hex(data); got != want {
			t.Errorf("manifest stale for %s: manifest %s, computed %s; run 'make manifest'", name, want, got)
		}
	}
}

// TestFlagRequired checks that the flag `required` field parses and applies
// only to the shape that selects it.
func TestFlagRequired(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - {name: X, kind: flag, type: string, long: --x, required: true}
    - {name: Y, kind: flag, type: string, long: --y}
options: [X, Y]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byName := map[string]Flag{}
	for _, f := range cfg.Resolved()[0].Flags {
		byName[f.OptName()] = f
	}
	if !byName["X"].Required() {
		t.Error("X.Required() = false, want true")
	}
	if byName["Y"].Required() {
		t.Error("Y.Required() = true, want false")
	}
}
