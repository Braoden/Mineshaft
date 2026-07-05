package beads

import (
	"strings"
	"testing"
)

// TestOverseerBeadIDTown tests the town-level Overseer bead ID.
func TestOverseerBeadIDTown(t *testing.T) {
	got := OverseerBeadIDTown()
	want := "hq-overseer"
	if got != want {
		t.Errorf("OverseerBeadIDTown() = %q, want %q", got, want)
	}
}

// TestSupervisorBeadIDTown tests the town-level Supervisor bead ID.
func TestSupervisorBeadIDTown(t *testing.T) {
	got := SupervisorBeadIDTown()
	want := "hq-supervisor"
	if got != want {
		t.Errorf("SupervisorBeadIDTown() = %q, want %q", got, want)
	}
}

// TestDogBeadIDTown tests town-level Dog bead IDs.
func TestDogBeadIDTown(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"alpha", "hq-dog-alpha"},
		{"rex", "hq-dog-rex"},
		{"spot", "hq-dog-spot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DogBeadIDTown(tt.name)
			if got != tt.want {
				t.Errorf("DogBeadIDTown(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestAgentBeadIDWithPrefix tests agent bead ID generation, including dedup when prefix == rig.
func TestAgentBeadIDWithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		role   string
		wname  string
		want   string
	}{
		// Normal cases (prefix != rig)
		{"town-level overseer", "gt", "", "overseer", "", "gt-overseer"},
		{"rig witness", "gt", "mineshaft", "witness", "", "gt-mineshaft-witness"},
		{"rig miner", "gt", "mineshaft", "miner", "nux", "gt-mineshaft-miner-nux"},
		{"rig crew", "bd", "beads", "crew", "dave", "bd-beads-crew-dave"},

		// Collapsed cases (prefix == rig) — should NOT stutter
		{"dedup witness", "ff", "ff", "witness", "", "ff-witness"},
		{"dedup refinery", "ff", "ff", "refinery", "", "ff-refinery"},
		{"dedup miner", "ff", "ff", "miner", "nux", "ff-miner-nux"},
		{"dedup crew", "ff", "ff", "crew", "dave", "ff-crew-dave"},
		{"dedup bd-beads", "bd", "bd", "witness", "", "bd-witness"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentBeadIDWithPrefix(tt.prefix, tt.rig, tt.role, tt.wname)
			if got != tt.want {
				t.Errorf("AgentBeadIDWithPrefix(%q, %q, %q, %q) = %q, want %q",
					tt.prefix, tt.rig, tt.role, tt.wname, got, tt.want)
			}
		})
	}
}

// TestValidateAgentID tests agent ID validation.
func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		wantError     bool
		errorContains string
	}{
		// Town-level agents (no rig)
		{"valid overseer", "gt-overseer", false, ""},
		{"valid supervisor", "gt-supervisor", false, ""},

		// Town-level named agents (dogs)
		{"valid dog", "gt-dog-alpha", false, ""},
		{"valid dog with hyphen", "gt-dog-war-boy", false, ""},

		// Per-rig agents (canonical format: gt-<rig>-<role>)
		{"valid witness mineshaft", "gt-mineshaft-witness", false, ""},
		{"valid refinery beads", "gt-beads-refinery", false, ""},

		// Named agents (canonical format: gt-<rig>-<role>-<name>)
		{"valid miner", "gt-mineshaft-miner-nux", false, ""},
		{"valid crew", "gt-beads-crew-dave", false, ""},
		{"valid miner with complex name", "gt-mineshaft-miner-war-boy-1", false, ""},

		// Valid: alternative prefixes (beads uses bd-)
		{"valid bd-overseer", "bd-overseer", false, ""},
		{"valid bd-beads-miner-pearl", "bd-beads-miner-pearl", false, ""},
		{"valid bd-beads-witness", "bd-beads-witness", false, ""},

		// Valid: hyphenated rig names
		{"hyphenated rig witness", "ob-my-project-witness", false, ""},
		{"hyphenated rig refinery", "gt-foo-bar-refinery", false, ""},
		{"hyphenated rig crew", "bd-my-cool-project-crew-fang", false, ""},
		{"hyphenated rig miner", "gt-some-long-rig-name-miner-nux", false, ""},
		{"hyphenated rig and name", "gt-my-rig-miner-war-boy", false, ""},
		{"multi-hyphen rig crew", "ob-a-b-c-d-crew-dave", false, ""},

		// Invalid: no prefix (missing hyphen)
		{"no prefix", "overseer", true, "must have a prefix followed by '-'"},

		// Invalid: empty
		{"empty id", "", true, "agent ID is required"},

		// Invalid: unknown role in position 2
		{"unknown role", "gt-mineshaft-admin", true, "invalid agent format"},

		// Invalid: town-level with rig (put role first)
		{"overseer with rig suffix", "gt-mineshaft-overseer", true, "cannot have rig/name suffixes"},
		{"supervisor with rig suffix", "gt-beads-supervisor", true, "cannot have rig/name suffixes"},

		// Collapsed form: rig-level role without rig (prefix == rig)
		{"collapsed witness", "gt-witness", false, ""},
		{"collapsed refinery", "gt-refinery", false, ""},
		{"collapsed miner", "ff-miner-nux", false, ""},
		{"collapsed crew", "ff-crew-dave", false, ""},

		// Invalid: named agent without name
		{"crew no name", "gt-beads-crew", true, "requires name"},
		{"miner no name", "gt-mineshaft-miner", true, "requires name"},
		{"dog no name", "gt-dog", true, "requires name"},

		// Valid: worker name collides with role keyword
		{"miner named witness", "gt-mineshaft-miner-witness", false, ""},
		{"miner named refinery", "gt-mineshaft-miner-refinery", false, ""},
		{"crew named witness", "gt-mineshaft-crew-witness", false, ""},
		{"crew named refinery", "gt-mineshaft-crew-refinery", false, ""},
		{"miner named crew", "gt-mineshaft-miner-crew", false, ""},
		{"crew named miner", "gt-mineshaft-crew-miner", false, ""},

		// Invalid: witness/refinery with extra parts (no named role to the left)
		{"witness with name", "gt-mineshaft-witness-extra", true, "cannot have name suffix"},
		{"refinery with name", "gt-beads-refinery-extra", true, "cannot have name suffix"},

		// Invalid: empty components
		{"empty after prefix", "gt-", true, "must include content after prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentID(tt.id)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateAgentID(%q) error = %v, wantError %v", tt.id, err, tt.wantError)
				return
			}
			if err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ValidateAgentID(%q) error = %q, should contain %q", tt.id, err.Error(), tt.errorContains)
				}
			}
		})
	}
}

// TestExtractAgentPrefix tests prefix extraction from agent IDs.
func TestExtractAgentPrefix(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantPrefix string
	}{
		// Town-level agents
		{"overseer", "gt-overseer", "gt"},
		{"supervisor", "gt-supervisor", "gt"},
		{"bd overseer", "bd-overseer", "bd"},

		// Town-level named (dogs)
		{"dog", "gt-dog-alpha", "gt"},
		{"dog hyphen name", "gt-dog-war-boy", "gt"},

		// Per-rig agents
		{"witness", "gt-mineshaft-witness", "gt"},
		{"refinery", "bd-beads-refinery", "bd"},

		// Named agents - the bug case
		{"miner 3-char name", "nx-nexus-miner-nux", "nx"},
		{"miner regular", "gt-mineshaft-miner-phoenix", "gt"},
		{"crew", "gt-beads-crew-dave", "gt"},

		// Hyphenated rig names
		{"hyphenated rig", "gt-my-project-witness", "gt"},
		{"multi-hyphen rig miner", "bd-my-cool-app-miner-bob", "bd"},

		// Edge cases
		{"no hyphen", "nohyphen", ""},
		{"empty", "", ""},
		{"just prefix", "gt-", "gt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAgentPrefix(tt.id)
			if got != tt.wantPrefix {
				t.Errorf("ExtractAgentPrefix(%q) = %q, want %q", tt.id, got, tt.wantPrefix)
			}
		})
	}
}

// TestAgentBeadIDRoundTrip verifies that generating an ID and parsing it back
// produces consistent results, especially for the collapsed form (GH#1877).
func TestAgentBeadIDRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		role   string
		wname  string
	}{
		// Normal cases
		{"normal witness", "gt", "mineshaft", "witness", ""},
		{"normal miner", "gt", "mineshaft", "miner", "nux"},
		{"normal crew", "bd", "beads", "crew", "dave"},

		// Collapsed cases (prefix == rig)
		{"collapsed witness", "ff", "ff", "witness", ""},
		{"collapsed refinery", "ff", "ff", "refinery", ""},
		{"collapsed miner", "ff", "ff", "miner", "nux"},
		{"collapsed crew", "ff", "ff", "crew", "dave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := AgentBeadIDWithPrefix(tt.prefix, tt.rig, tt.role, tt.wname)

			// Validate the generated ID
			if err := ValidateAgentID(id); err != nil {
				t.Errorf("AgentBeadIDWithPrefix(%q,%q,%q,%q) = %q, ValidateAgentID error: %v",
					tt.prefix, tt.rig, tt.role, tt.wname, id, err)
			}

			// Parse back and verify role and name
			_, gotRole, gotName, ok := ParseAgentBeadID(id)
			if !ok {
				t.Errorf("ParseAgentBeadID(%q) failed", id)
				return
			}
			if gotRole != tt.role {
				t.Errorf("ParseAgentBeadID(%q) role = %q, want %q", id, gotRole, tt.role)
			}
			if gotName != tt.wname {
				t.Errorf("ParseAgentBeadID(%q) name = %q, want %q", id, gotName, tt.wname)
			}
		})
	}
}

