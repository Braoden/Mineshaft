package constants

import (
	"testing"
)

func TestRoleEmoji(t *testing.T) {
	tests := []struct {
		role   string
		expect string
	}{
		{RoleOverseer, EmojiOverseer},
		{RoleSupervisor, EmojiSupervisor},
		{RoleWitness, EmojiWitness},
		{RoleRefinery, EmojiRefinery},
		{RoleCrew, EmojiCrew},
		{RoleMiner, EmojiMiner},
		{"unknown", "❓"},
		{"", "❓"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := RoleEmoji(tt.role)
			if got != tt.expect {
				t.Errorf("RoleEmoji(%q) = %q, want %q", tt.role, got, tt.expect)
			}
		})
	}
}

func TestBeadsCustomTypesList(t *testing.T) {
	types := BeadsCustomTypesList()
	expected := []string{"agent", "role", "rig", "minecart", "slot", "queue", "event", "message", "molecule", "gate", "merge-request"}

	if len(types) != len(expected) {
		t.Fatalf("BeadsCustomTypesList() returned %d items, want %d", len(types), len(expected))
	}
	for i, typ := range types {
		if typ != expected[i] {
			t.Errorf("BeadsCustomTypesList()[%d] = %q, want %q", i, typ, expected[i])
		}
	}
}

func TestOverseerRigsPath(t *testing.T) {
	got := OverseerRigsPath("/town")
	expect := "/town/overseer/rigs.json"
	if got != expect {
		t.Errorf("OverseerRigsPath = %q, want %q", got, expect)
	}
}

func TestOverseerTownPath(t *testing.T) {
	got := OverseerTownPath("/town")
	expect := "/town/overseer/town.json"
	if got != expect {
		t.Errorf("OverseerTownPath = %q, want %q", got, expect)
	}
}

func TestRigOverseerPath(t *testing.T) {
	got := RigOverseerPath("/rig")
	expect := "/rig/overseer/rig"
	if got != expect {
		t.Errorf("RigOverseerPath = %q, want %q", got, expect)
	}
}

func TestRigBeadsPath(t *testing.T) {
	got := RigBeadsPath("/rig")
	expect := "/rig/overseer/rig/.beads"
	if got != expect {
		t.Errorf("RigBeadsPath = %q, want %q", got, expect)
	}
}

func TestRigMinersPath(t *testing.T) {
	got := RigMinersPath("/rig")
	expect := "/rig/miners"
	if got != expect {
		t.Errorf("RigMinersPath = %q, want %q", got, expect)
	}
}

func TestRigCrewPath(t *testing.T) {
	got := RigCrewPath("/rig")
	expect := "/rig/crew"
	if got != expect {
		t.Errorf("RigCrewPath = %q, want %q", got, expect)
	}
}

func TestOverseerConfigPath(t *testing.T) {
	got := OverseerConfigPath("/town")
	expect := "/town/overseer/config.json"
	if got != expect {
		t.Errorf("OverseerConfigPath = %q, want %q", got, expect)
	}
}

func TestTownRuntimePath(t *testing.T) {
	got := TownRuntimePath("/town")
	expect := "/town/.runtime"
	if got != expect {
		t.Errorf("TownRuntimePath = %q, want %q", got, expect)
	}
}

func TestRigRuntimePath(t *testing.T) {
	got := RigRuntimePath("/rig")
	expect := "/rig/.runtime"
	if got != expect {
		t.Errorf("RigRuntimePath = %q, want %q", got, expect)
	}
}

func TestRigSettingsPath(t *testing.T) {
	got := RigSettingsPath("/rig")
	expect := "/rig/settings"
	if got != expect {
		t.Errorf("RigSettingsPath = %q, want %q", got, expect)
	}
}

func TestOverseerAccountsPath(t *testing.T) {
	got := OverseerAccountsPath("/town")
	expect := "/town/overseer/accounts.json"
	if got != expect {
		t.Errorf("OverseerAccountsPath = %q, want %q", got, expect)
	}
}

func TestOverseerQuotaPath(t *testing.T) {
	got := OverseerQuotaPath("/town")
	expect := "/town/overseer/quota.json"
	if got != expect {
		t.Errorf("OverseerQuotaPath = %q, want %q", got, expect)
	}
}
