package cmd

import (
	"testing"

	"github.com/steveyegge/excavation/internal/session"
)

func setupDndTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "excavation")
	reg.Register("bd", "beads")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestAddressToAgentBeadID(t *testing.T) {
	setupDndTestRegistry(t)
	tests := []struct {
		address  string
		expected string
	}{
		// Overseer and supervisor use hq- prefix (town-level)
		{"overseer", "hq-overseer"},
		{"supervisor", "hq-supervisor"},
		{"excavation/witness", "gt-witness"},
		{"excavation/refinery", "gt-refinery"},
		{"excavation/alpha", "gt-alpha"},
		{"excavation/crew/max", "gt-crew-max"},
		{"excavation/miners/alpha", "gt-alpha"},
		{"beads/miners/beta", "bd-beta"},
		{"beads/witness", "bd-witness"},
		{"beads/beta", "bd-beta"},
		// Invalid addresses should return empty string
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToAgentBeadID(tt.address)
			if got != tt.expected {
				t.Errorf("addressToAgentBeadID(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}
