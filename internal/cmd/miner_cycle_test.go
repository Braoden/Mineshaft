package cmd

import (
	"reflect"
	"testing"

	"github.com/steveyegge/mineshaft/internal/session"
)

func setupMinerTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("ms", "mineshaft")
	reg.Register("gp", "greenplace")
	reg.Register("bd", "beads")
	reg.Register("mr", "my-rig")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestParseMinerSessionName(t *testing.T) {
	setupMinerTestRegistry(t)
	tests := []struct {
		name        string
		sessionName string
		wantRig     string
		wantMiner string
		wantOk      bool
	}{
		// Valid miner sessions (using rig prefixes)
		{
			name:        "simple miner",
			sessionName: "gp-Toast",
			wantRig:     "greenplace",
			wantMiner: "Toast",
			wantOk:      true,
		},
		{
			name:        "another miner",
			sessionName: "gp-Nux",
			wantRig:     "greenplace",
			wantMiner: "Nux",
			wantOk:      true,
		},
		{
			name:        "miner in different rig",
			sessionName: "bd-Worker",
			wantRig:     "beads",
			wantMiner: "Worker",
			wantOk:      true,
		},
		{
			name:        "hyphenated rig name",
			sessionName: "mr-Toast",
			wantRig:     "my-rig",
			wantMiner: "Toast",
			wantOk:      true,
		},

		// Not miner sessions (should return false)
		{
			name:        "crew session",
			sessionName: "gp-crew-jack",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "witness session",
			sessionName: "gp-witness",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "refinery session",
			sessionName: "gp-refinery",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "overseer session",
			sessionName: "hq-overseer",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "supervisor session",
			sessionName: "hq-supervisor",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "no known prefix",
			sessionName: "plaintext",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "empty string",
			sessionName: "",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "just ms prefix",
			sessionName: "ms-",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
		{
			name:        "no name after rig",
			sessionName: "gp-",
			wantRig:     "",
			wantMiner: "",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRig, gotMiner, gotOk := parseMinerSessionName(tt.sessionName)
			if gotRig != tt.wantRig || gotMiner != tt.wantMiner || gotOk != tt.wantOk {
				t.Errorf("parseMinerSessionName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.sessionName, gotRig, gotMiner, gotOk, tt.wantRig, tt.wantMiner, tt.wantOk)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple lines",
			input: "a\nb\nc",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "trailing newline filtered",
			input: "a\nb\n",
			want:  []string{"a", "b"},
		},
		{
			name:  "multiple trailing newlines filtered",
			input: "a\n\n\n",
			want:  []string{"a"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single line no newline",
			input: "hello",
			want:  []string{"hello"},
		},
		{
			name:  "only newlines",
			input: "\n\n\n",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
