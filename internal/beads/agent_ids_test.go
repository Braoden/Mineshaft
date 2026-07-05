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
		{"town-level overseer", "ms", "", "overseer", "", "ms-overseer"},
		{"rig witness", "ms", "mineshaft", "witness", "", "ms-mineshaft-witness"},
		{"rig miner", "ms", "mineshaft", "miner", "nux", "ms-mineshaft-miner-nux"},
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
		{"valid overseer", "ms-overseer", false, ""},
		{"valid supervisor", "ms-supervisor", false, ""},

		// Town-level named agents (dogs)
		{"valid dog", "ms-dog-alpha", false, ""},
		{"valid dog with hyphen", "ms-dog-war-boy", false, ""},

		// Per-rig agents (canonical format: ms-<rig>-<role>)
		{"valid witness mineshaft", "ms-mineshaft-witness", false, ""},
		{"valid refinery beads", "ms-beads-refinery", false, ""},

		// Named agents (canonical format: ms-<rig>-<role>-<name>)
		{"valid miner", "ms-mineshaft-miner-nux", false, ""},
		{"valid crew", "ms-beads-crew-dave", false, ""},
		{"valid miner with complex name", "ms-mineshaft-miner-war-boy-1", false, ""},

		// Valid: alternative prefixes (beads uses bd-)
		{"valid bd-overseer", "bd-overseer", false, ""},
		{"valid bd-beads-miner-pearl", "bd-beads-miner-pearl", false, ""},
		{"valid bd-beads-witness", "bd-beads-witness", false, ""},

		// Valid: hyphenated rig names
		{"hyphenated rig witness", "ob-my-project-witness", false, ""},
		{"hyphenated rig refinery", "ms-foo-bar-refinery", false, ""},
		{"hyphenated rig crew", "bd-my-cool-project-crew-fang", false, ""},
		{"hyphenated rig miner", "ms-some-long-rig-name-miner-nux", false, ""},
		{"hyphenated rig and name", "ms-my-rig-miner-war-boy", false, ""},
		{"multi-hyphen rig crew", "ob-a-b-c-d-crew-dave", false, ""},

		// Invalid: no prefix (missing hyphen)
		{"no prefix", "overseer", true, "must have a prefix followed by '-'"},

		// Invalid: empty
		{"empty id", "", true, "agent ID is required"},

		// Invalid: unknown role in position 2
		{"unknown role", "ms-mineshaft-admin", true, "invalid agent format"},

		// Invalid: town-level with rig (put role first)
		{"overseer with rig suffix", "ms-mineshaft-overseer", true, "cannot have rig/name suffixes"},
		{"supervisor with rig suffix", "ms-beads-supervisor", true, "cannot have rig/name suffixes"},

		// Collapsed form: rig-level role without rig (prefix == rig)
		{"collapsed witness", "ms-witness", false, ""},
		{"collapsed refinery", "ms-refinery", false, ""},
		{"collapsed miner", "ff-miner-nux", false, ""},
		{"collapsed crew", "ff-crew-dave", false, ""},

		// Invalid: named agent without name
		{"crew no name", "ms-beads-crew", true, "requires name"},
		{"miner no name", "ms-mineshaft-miner", true, "requires name"},
		{"dog no name", "ms-dog", true, "requires name"},

		// Valid: worker name collides with role keyword
		{"miner named witness", "ms-mineshaft-miner-witness", false, ""},
		{"miner named refinery", "ms-mineshaft-miner-refinery", false, ""},
		{"crew named witness", "ms-mineshaft-crew-witness", false, ""},
		{"crew named refinery", "ms-mineshaft-crew-refinery", false, ""},
		{"miner named crew", "ms-mineshaft-miner-crew", false, ""},
		{"crew named miner", "ms-mineshaft-crew-miner", false, ""},

		// Invalid: witness/refinery with extra parts (no named role to the left)
		{"witness with name", "ms-mineshaft-witness-extra", true, "cannot have name suffix"},
		{"refinery with name", "ms-beads-refinery-extra", true, "cannot have name suffix"},

		// Invalid: empty components
		{"empty after prefix", "ms-", true, "must include content after prefix"},
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
		{"overseer", "ms-overseer", "ms"},
		{"supervisor", "ms-supervisor", "ms"},
		{"bd overseer", "bd-overseer", "bd"},

		// Town-level named (dogs)
		{"dog", "ms-dog-alpha", "ms"},
		{"dog hyphen name", "ms-dog-war-boy", "ms"},

		// Per-rig agents
		{"witness", "ms-mineshaft-witness", "ms"},
		{"refinery", "bd-beads-refinery", "bd"},

		// Named agents - the bug case
		{"miner 3-char name", "nx-nexus-miner-nux", "nx"},
		{"miner regular", "ms-mineshaft-miner-phoenix", "ms"},
		{"crew", "ms-beads-crew-dave", "ms"},

		// Hyphenated rig names
		{"hyphenated rig", "ms-my-project-witness", "ms"},
		{"multi-hyphen rig miner", "bd-my-cool-app-miner-bob", "bd"},

		// Edge cases
		{"no hyphen", "nohyphen", ""},
		{"empty", "", ""},
		{"just prefix", "ms-", "ms"},
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
		{"normal witness", "ms", "mineshaft", "witness", ""},
		{"normal miner", "ms", "mineshaft", "miner", "nux"},
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

