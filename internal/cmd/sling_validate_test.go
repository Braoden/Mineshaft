package cmd

import (
	"strings"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
		errMsg  string // substring that must appear in error
	}{
		// Valid targets
		{name: "empty target", target: "", wantErr: false},
		{name: "self target", target: ".", wantErr: false},
		{name: "bare rig name", target: "mineshaft", wantErr: false},
		{name: "role shortcut overseer", target: "overseer", wantErr: false},
		{name: "role shortcut supervisor", target: "supervisor", wantErr: false},
		{name: "rig/miners/name", target: "mineshaft/miners/nux", wantErr: false},
		{name: "rig/crew/name", target: "mineshaft/crew/burke", wantErr: false},
		{name: "rig/witness", target: "mineshaft/witness", wantErr: false},
		{name: "rig/refinery", target: "mineshaft/refinery", wantErr: false},
		{name: "supervisor/dogs", target: "supervisor/dogs", wantErr: false},
		{name: "supervisor/dogs/name", target: "supervisor/dogs/rex", wantErr: false},
		{name: "miner shorthand", target: "mineshaft/nux", wantErr: false},
		{name: "crew shorthand", target: "mineshaft/max", wantErr: false},

		// Invalid targets — empty segments
		{name: "trailing slash", target: "mineshaft/", wantErr: true, errMsg: "empty path segment"},
		{name: "double slash", target: "mineshaft//miners", wantErr: true, errMsg: "empty path segment"},
		{name: "leading slash", target: "/miners", wantErr: true, errMsg: "empty path segment"},

		// Invalid targets — unknown role (only rejected with 3+ segments)
		{name: "unknown role 3-seg", target: "mineshaft/badrole/name", wantErr: true, errMsg: "unknown role"},
		{name: "typo in role 3-seg", target: "mineshaft/miner/name", wantErr: true, errMsg: "unknown role"},

		// Invalid targets — missing name
		{name: "crew no name", target: "mineshaft/crew", wantErr: true, errMsg: "requires a worker name"},
		{name: "miners no name", target: "mineshaft/miners", wantErr: true, errMsg: "requires a miner name"},

		// Invalid targets — witness/refinery with sub-agents
		{name: "witness with name", target: "mineshaft/witness/extra", wantErr: true, errMsg: "does not have named sub-agents"},
		{name: "refinery with name", target: "mineshaft/refinery/extra", wantErr: true, errMsg: "does not have named sub-agents"},

		// Invalid targets — too many segments
		{name: "too many segments", target: "mineshaft/crew/burke/extra", wantErr: true, errMsg: "too many path segments"},

		// Invalid targets — overseer sub-paths
		{name: "overseer sub-agent", target: "overseer/something", wantErr: true, errMsg: "does not have sub-agents"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTarget(tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateTarget(%q) = nil, want error containing %q", tc.target, tc.errMsg)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateTarget(%q) = %v, want nil", tc.target, err)
			}
			if tc.wantErr && err != nil && tc.errMsg != "" {
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("ValidateTarget(%q) error = %q, want it to contain %q", tc.target, err.Error(), tc.errMsg)
				}
			}
		})
	}
}
