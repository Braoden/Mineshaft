// Package session provides miner session lifecycle management.
package session

import (
	"fmt"
)

// DefaultPrefix is the default beads prefix used when no rig-specific prefix is known.
const DefaultPrefix = "gt"

// HQPrefix is the prefix for town-level services (Overseer, Supervisor).
const HQPrefix = "hq-"

// OverseerSessionName returns the session name for the Overseer agent.
// One overseer per machine - multi-town requires containers/VMs for isolation.
func OverseerSessionName() string {
	return HQPrefix + "overseer"
}

// SupervisorSessionName returns the session name for the Supervisor agent.
// One supervisor per machine - multi-town requires containers/VMs for isolation.
func SupervisorSessionName() string {
	return HQPrefix + "supervisor"
}

// WitnessSessionName returns the session name for a rig's Witness agent.
// rigPrefix is the rig's beads prefix (e.g., "gt" for mineshaft, "bd" for beads).
func WitnessSessionName(rigPrefix string) string {
	return fmt.Sprintf("%s-witness", rigPrefix)
}

// RefinerySessionName returns the session name for a rig's Refinery agent.
// rigPrefix is the rig's beads prefix (e.g., "gt" for mineshaft, "bd" for beads).
func RefinerySessionName(rigPrefix string) string {
	return fmt.Sprintf("%s-refinery", rigPrefix)
}

// CrewSessionName returns the session name for a crew worker in a rig.
// rigPrefix is the rig's beads prefix (e.g., "gt" for mineshaft, "bd" for beads).
func CrewSessionName(rigPrefix, name string) string {
	return fmt.Sprintf("%s-crew-%s", rigPrefix, name)
}

// MinerSessionName returns the session name for a miner in a rig.
// rigPrefix is the rig's beads prefix (e.g., "gt" for mineshaft, "bd" for beads).
func MinerSessionName(rigPrefix, name string) string {
	return fmt.Sprintf("%s-%s", rigPrefix, name)
}

// BossSessionName returns the session name for the human operator.
// The boss is the human who controls Mineshaft, not an AI agent.
func BossSessionName() string {
	return HQPrefix + "boss"
}

// BootSessionName returns the session name for the Boot watchdog.
// Boot is town-level (launched by supervisor), so it uses the hq- prefix.
// "hq-boot" avoids tmux prefix-matching collisions with "hq-supervisor".
func BootSessionName() string {
	return HQPrefix + "boot"
}

// DogSessionName returns the session name for a named dog agent.
// Dogs are town-level (managed by supervisor), so they use the hq- prefix.
// Pattern: hq-dog-<name> (e.g., hq-dog-alpha).
func DogSessionName(name string) string {
	return fmt.Sprintf("%sdog-%s", HQPrefix, name)
}
