// Package hookutil provides shared utilities for agent hook installers.
package hookutil

import "github.com/steveyegge/mineshaft/internal/constants"

// IsAutonomousRole returns true if the given role operates without human
// prompting and needs automatic mail injection on startup.
//
// Autonomous roles: miner, witness, refinery, supervisor, boot.
// Interactive roles: overseer, crew (and anything else).
//
// This is the single source of truth for the autonomous/interactive
// classification used by all hook installer packages (claude, gemini,
// cursor, etc.) and the runtime fallback logic.
func IsAutonomousRole(role string) bool {
	switch role {
	case constants.RoleMiner, constants.RoleWitness, constants.RoleRefinery, constants.RoleSupervisor, "boot":
		return true
	default:
		return false
	}
}
