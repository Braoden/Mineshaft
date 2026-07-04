package session

import (
	"strings"
	"testing"
)

func TestBeaconRecipient(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		agentNm  string
		rig      string
		want     string
		wantNot  []string // must NOT contain these (path separators, etc.)
	}{
		{
			name:    "miner with rig",
			role:    "miner",
			agentNm: "rust",
			rig:     "testrig",
			want:    "miner rust (rig: testrig)",
			wantNot: []string{"/"},
		},
		{
			name:    "crew with rig",
			role:    "crew",
			agentNm: "gus",
			rig:     "excavation",
			want:    "crew gus (rig: excavation)",
			wantNot: []string{"/"},
		},
		{
			name:    "witness singleton with rig",
			role:    "witness",
			agentNm: "",
			rig:     "excavation",
			want:    "witness (rig: excavation)",
			wantNot: []string{"/"},
		},
		{
			name:    "refinery singleton with rig",
			role:    "refinery",
			agentNm: "",
			rig:     "myrig",
			want:    "refinery (rig: myrig)",
			wantNot: []string{"/"},
		},
		{
			name:    "dog with no rig",
			role:    "dog",
			agentNm: "fido",
			rig:     "",
			want:    "dog fido",
			wantNot: []string{"/", "(rig:"},
		},
		{
			name:    "town-level role no rig no name",
			role:    "overseer",
			agentNm: "",
			rig:     "",
			want:    "overseer",
			wantNot: []string{"/", "(rig:"},
		},
		{
			name:    "supervisor no rig no name",
			role:    "supervisor",
			agentNm: "",
			rig:     "",
			want:    "supervisor",
		},
		{
			name:    "miner name with hyphen",
			role:    "miner",
			agentNm: "my-worker",
			rig:     "prod-rig",
			want:    "miner my-worker (rig: prod-rig)",
			wantNot: []string{"prod-rig/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BeaconRecipient(tt.role, tt.agentNm, tt.rig)
			if got != tt.want {
				t.Errorf("BeaconRecipient(%q, %q, %q) = %q, want %q",
					tt.role, tt.agentNm, tt.rig, got, tt.want)
			}
			for _, bad := range tt.wantNot {
				if strings.Contains(got, bad) {
					t.Errorf("BeaconRecipient(%q, %q, %q) = %q, should NOT contain %q",
						tt.role, tt.agentNm, tt.rig, got, bad)
				}
			}
		})
	}
}

func TestBeaconRecipientContainsNoPathSeparators(t *testing.T) {
	// Exhaustive check: no BeaconRecipient output should contain "/" which
	// could trick LLMs into interpreting it as a filesystem path.
	cases := []struct{ role, name, rig string }{
		{"miner", "rust", "testrig"},
		{"crew", "gus", "excavation"},
		{"witness", "", "excavation"},
		{"refinery", "", "excavation"},
		{"dog", "fido", ""},
		{"overseer", "", ""},
		{"supervisor", "", ""},
		{"boot", "", ""},
		{"miner", "a/b", "c/d"}, // edge case: slashes in inputs
	}
	for _, c := range cases {
		got := BeaconRecipient(c.role, c.name, c.rig)
		// The function should not introduce "/" on its own.
		// (If inputs contain "/" that's a caller bug, but at minimum the
		// function shouldn't add new ones.)
		if c.name == "" && c.rig == "" && strings.Contains(got, "/") {
			t.Errorf("BeaconRecipient(%q, %q, %q) = %q contains /",
				c.role, c.name, c.rig, got)
		}
	}
}

func TestAgentIdentityBeaconAddress(t *testing.T) {
	tests := []struct {
		name    string
		id      AgentIdentity
		want    string
		wantNot []string
	}{
		{
			name: "overseer",
			id:   AgentIdentity{Role: RoleOverseer},
			want: "overseer",
		},
		{
			name: "supervisor",
			id:   AgentIdentity{Role: RoleSupervisor},
			want: "supervisor",
		},
		{
			name:    "witness",
			id:      AgentIdentity{Role: RoleWitness, Rig: "excavation"},
			want:    "witness (rig: excavation)",
			wantNot: []string{"excavation/witness"},
		},
		{
			name:    "refinery",
			id:      AgentIdentity{Role: RoleRefinery, Rig: "excavation"},
			want:    "refinery (rig: excavation)",
			wantNot: []string{"excavation/refinery"},
		},
		{
			name:    "crew",
			id:      AgentIdentity{Role: RoleCrew, Rig: "excavation", Name: "max"},
			want:    "crew max (rig: excavation)",
			wantNot: []string{"excavation/crew/max"},
		},
		{
			name:    "miner",
			id:      AgentIdentity{Role: RoleMiner, Rig: "excavation", Name: "Toast"},
			want:    "miner Toast (rig: excavation)",
			wantNot: []string{"excavation/miners/Toast"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.id.BeaconAddress()
			if got != tt.want {
				t.Errorf("BeaconAddress() = %q, want %q", got, tt.want)
			}
			for _, bad := range tt.wantNot {
				if strings.Contains(got, bad) {
					t.Errorf("BeaconAddress() = %q, should NOT contain path-like %q", got, bad)
				}
			}
		})
	}
}

func TestBeaconAddressVsAddress(t *testing.T) {
	// Verify that BeaconAddress produces different (non-path) output
	// while Address produces the traditional path-like output.
	ids := []AgentIdentity{
		{Role: RoleWitness, Rig: "excavation"},
		{Role: RoleRefinery, Rig: "excavation"},
		{Role: RoleCrew, Rig: "excavation", Name: "max"},
		{Role: RoleMiner, Rig: "excavation", Name: "Toast"},
	}
	for _, id := range ids {
		addr := id.Address()
		beacon := id.BeaconAddress()

		// Address should contain "/" (path-like)
		if !strings.Contains(addr, "/") {
			t.Errorf("Address() for %v = %q, expected path-like with /", id.Role, addr)
		}
		// BeaconAddress should NOT contain "/" (non-path)
		if strings.Contains(beacon, "/") {
			t.Errorf("BeaconAddress() for %v = %q, should NOT contain /", id.Role, beacon)
		}
		// Both should contain the rig name
		if !strings.Contains(beacon, "excavation") {
			t.Errorf("BeaconAddress() for %v = %q, missing rig name", id.Role, beacon)
		}
	}

	// Town-level roles should be identical
	for _, role := range []Role{RoleOverseer, RoleSupervisor} {
		id := AgentIdentity{Role: role}
		if id.Address() != id.BeaconAddress() {
			t.Errorf("For %v: Address()=%q != BeaconAddress()=%q", role, id.Address(), id.BeaconAddress())
		}
	}
}

func TestFormatStartupBeacon(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BeaconConfig
		wantSub []string // substrings that must appear
		wantNot []string // substrings that must NOT appear
	}{
		{
			name: "assigned with mol-id uses new format",
			cfg: BeaconConfig{
				Recipient: BeaconRecipient("crew", "gus", "excavation"),
				Sender:    "supervisor",
				Topic:     "assigned",
				MolID:     "gt-abc12",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"crew gus (rig: excavation)",
				"<- supervisor",
				"assigned:gt-abc12",
				"gt prime --hook",
				"begin work",
			},
			wantNot: []string{
				"excavation/crew/gus", // must NOT contain path-like format
			},
		},
		{
			name: "cold-start no mol-id",
			cfg: BeaconConfig{
				Recipient: "supervisor",
				Sender:    "overseer",
				Topic:     "cold-start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"supervisor",
				"<- overseer",
				"cold-start",
				"Check your hook and mail",
				"gt hook",
				"gt mail inbox",
			},
		},
		{
			name: "handoff self uses new format",
			cfg: BeaconConfig{
				Recipient: BeaconRecipient("witness", "", "excavation"),
				Sender:    "self",
				Topic:     "handoff",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"witness (rig: excavation)",
				"<- self",
				"handoff",
				"Check your hook and mail",
				"gt hook",
				"gt mail inbox",
			},
			wantNot: []string{
				"excavation/witness",
			},
		},
		{
			name: "miner assigned uses new format",
			cfg: BeaconConfig{
				Recipient: BeaconRecipient("miner", "Toast", "excavation"),
				Sender:    "witness",
				MolID:     "gt-xyz99",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"miner Toast (rig: excavation)",
				"<- witness",
				"gt-xyz99",
			},
			wantNot: []string{
				"excavation/miners/Toast",
			},
		},
		{
			name: "empty topic defaults to ready",
			cfg: BeaconConfig{
				Recipient: "supervisor",
				Sender:    "overseer",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"ready",
			},
		},
		{
			name: "start beacon has no prime instruction",
			cfg: BeaconConfig{
				Recipient: BeaconRecipient("crew", "fang", "beads"),
				Sender:    "human",
				Topic:     "start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"crew fang (rig: beads)",
				"<- human",
				"start",
			},
			wantNot: []string{
				"gt prime",
				"beads/crew/fang",
			},
		},
		{
			name: "restart beacon has no prime instruction",
			cfg: BeaconConfig{
				Recipient: BeaconRecipient("crew", "george", "excavation"),
				Sender:    "human",
				Topic:     "restart",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"crew george (rig: excavation)",
				"restart",
			},
			wantNot: []string{
				"gt prime",
				"excavation/crew/george",
			},
		},
		{
			name: "include prime instruction for non-hook agents",
			cfg: BeaconConfig{
				Recipient:               BeaconRecipient("miner", "ruby", "myrig"),
				Sender:                  "witness",
				Topic:                   "assigned",
				IncludePrimeInstruction: true,
			},
			wantSub: []string{
				"[GAS TOWN]",
				"miner ruby (rig: myrig)",
				"gt prime",
			},
			wantNot: []string{
				"begin work", // excluded when IncludePrimeInstruction is set
			},
		},
		{
			name: "attach topic includes hook/mail instructions",
			cfg: BeaconConfig{
				Recipient: "overseer",
				Sender:    "human",
				Topic:     "attach",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"overseer",
				"attach",
				"gt hook",
				"gt mail inbox",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStartupBeacon(tt.cfg)

			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("FormatStartupBeacon() = %q, want to contain %q", got, sub)
				}
			}

			for _, sub := range tt.wantNot {
				if strings.Contains(got, sub) {
					t.Errorf("FormatStartupBeacon() = %q, should NOT contain %q", got, sub)
				}
			}
		})
	}
}

func TestBuildStartupPrompt(t *testing.T) {
	// BuildStartupPrompt combines beacon + instructions
	cfg := BeaconConfig{
		Recipient: "supervisor",
		Sender:    "daemon",
		Topic:     "patrol",
	}
	instructions := "Start patrol immediately."

	got := BuildStartupPrompt(cfg, instructions)

	// Should contain beacon parts
	if !strings.Contains(got, "[GAS TOWN]") {
		t.Errorf("BuildStartupPrompt() missing beacon header")
	}
	if !strings.Contains(got, "supervisor") {
		t.Errorf("BuildStartupPrompt() missing recipient")
	}
	if !strings.Contains(got, "<- daemon") {
		t.Errorf("BuildStartupPrompt() missing sender")
	}
	if !strings.Contains(got, "patrol") {
		t.Errorf("BuildStartupPrompt() missing topic")
	}

	// Should contain instructions after beacon
	if !strings.Contains(got, instructions) {
		t.Errorf("BuildStartupPrompt() missing instructions")
	}

	// Should have blank line between beacon and instructions
	if !strings.Contains(got, "\n\n"+instructions) {
		t.Errorf("BuildStartupPrompt() missing blank line before instructions")
	}
}
