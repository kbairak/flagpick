package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want cliArgs
	}{
		{
			name: "no args",
			args: nil,
			want: cliArgs{action: actionBuild},
		},
		{
			name: "version long",
			args: []string{"--version"},
			want: cliArgs{action: actionVersion},
		},
		{
			name: "version short",
			args: []string{"-V"},
			want: cliArgs{action: actionVersion},
		},
		{
			name: "help long",
			args: []string{"--help"},
			want: cliArgs{action: actionHelp},
		},
		{
			name: "help short",
			args: []string{"-h"},
			want: cliArgs{action: actionHelp},
		},
		{
			name: "list long",
			args: []string{"--list"},
			want: cliArgs{action: actionList},
		},
		{
			name: "list short",
			args: []string{"-l"},
			want: cliArgs{action: actionList},
		},
		{
			name: "update",
			args: []string{"--update"},
			want: cliArgs{action: actionUpdate},
		},
		{
			name: "resume with command and tool args",
			args: []string{"--resume", "rg", "-i", "foo"},
			want: cliArgs{action: actionBuild, cmdName: "rg", toolArgs: []string{"-i", "foo"}, resume: true},
		},
		{
			name: "command with tool args",
			args: []string{"rg", "-i", "foo"},
			want: cliArgs{action: actionBuild, cmdName: "rg", toolArgs: []string{"-i", "foo"}},
		},
		{
			name: "flagpick flag after command is passed through",
			args: []string{"rg", "--version"},
			want: cliArgs{action: actionBuild, cmdName: "rg", toolArgs: []string{"--version"}},
		},
		{
			name: "resume without a command",
			args: []string{"--resume"},
			want: cliArgs{action: actionBuild, resume: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if err != nil {
				t.Fatalf("parseArgs(%v) error: %v", tc.args, err)
			}
			if got.action != tc.want.action {
				t.Errorf("action = %d, want %d", got.action, tc.want.action)
			}
			if got.cmdName != tc.want.cmdName {
				t.Errorf("cmdName = %q, want %q", got.cmdName, tc.want.cmdName)
			}
			if got.resume != tc.want.resume {
				t.Errorf("resume = %v, want %v", got.resume, tc.want.resume)
			}
			if !reflect.DeepEqual(got.toolArgs, tc.want.toolArgs) {
				t.Errorf("toolArgs = %v, want %v", got.toolArgs, tc.want.toolArgs)
			}
		})
	}
}

func TestParseArgsUnknownFlag(t *testing.T) {
	for _, args := range [][]string{{"-x"}, {"--bogus"}} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) = nil error, want error", args)
		}
	}
}
