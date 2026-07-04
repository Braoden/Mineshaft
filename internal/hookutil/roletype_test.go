package hookutil

import "testing"

func TestIsAutonomousRole(t *testing.T) {
	autonomous := []string{"miner", "witness", "refinery", "supervisor", "boot"}
	for _, role := range autonomous {
		if !IsAutonomousRole(role) {
			t.Errorf("IsAutonomousRole(%q) = false, want true", role)
		}
	}

	interactive := []string{"overseer", "crew", "unknown", ""}
	for _, role := range interactive {
		if IsAutonomousRole(role) {
			t.Errorf("IsAutonomousRole(%q) = true, want false", role)
		}
	}
}
