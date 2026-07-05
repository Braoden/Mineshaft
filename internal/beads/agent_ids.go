// Package beads provides a wrapper for the bd (beads) CLI.
package beads

import (
	"fmt"
	"strings"

	"github.com/steveyegge/mineshaft/internal/constants"
)

// TownBeadsPrefix is the prefix used for town-level agent beads stored in ~/gt/.beads/.
// This distinguishes them from rig-level beads (which use project prefixes like "gt-").
const TownBeadsPrefix = "hq"

// Town-level agent bead IDs use the "hq-" prefix and are stored in town beads.
// These are global agents that operate at the town level (overseer, supervisor, dogs).
//
// The naming convention is:
//   - hq-<role>       for singletons (overseer, supervisor)
//   - hq-dog-<name>   for named agents (dogs)
//   - hq-<role>-role  for role definition beads

// OverseerBeadIDTown returns the Overseer agent bead ID for town-level beads.
// This uses the "hq-" prefix for town-level storage.
func OverseerBeadIDTown() string {
	return TownBeadsPrefix + "-overseer"
}

// SupervisorBeadIDTown returns the Supervisor agent bead ID for town-level beads.
// This uses the "hq-" prefix for town-level storage.
func SupervisorBeadIDTown() string {
	return TownBeadsPrefix + "-supervisor"
}

// DogBeadIDTown returns a Dog agent bead ID for town-level beads.
// Dogs are town-level agents, so they follow the pattern: hq-dog-<name>
func DogBeadIDTown(name string) string {
	return fmt.Sprintf("%s-dog-%s", TownBeadsPrefix, name)
}

// ===== Agent ID validation =====

// ValidAgentRoles are the known agent role types for ID pattern validation.
var ValidAgentRoles = []string{
	constants.RoleOverseer,    // Town-level: gt-overseer
	constants.RoleSupervisor,   // Town-level: gt-supervisor
	"dog",                  // Town-level with name: gt-dog-<name>
	constants.RoleWitness,  // Per-rig: gt-<rig>-witness
	constants.RoleRefinery, // Per-rig: gt-<rig>-refinery
	constants.RoleCrew,    // Per-rig with name: gt-<rig>-crew-<name>
	constants.RoleMiner, // Per-rig with name: gt-<rig>-miner-<name>
}

// TownLevelRoles are agent roles that don't have a rig.
var TownLevelRoles = []string{constants.RoleOverseer, constants.RoleSupervisor}

// TownLevelNamedRoles are town-level agent roles that include a name.
var TownLevelNamedRoles = []string{"dog"}

// RigLevelRoles are agent roles that have a rig but no name.
var RigLevelRoles = []string{constants.RoleWitness, constants.RoleRefinery}

// NamedRoles are agent roles that include a worker name (rig-level).
var NamedRoles = []string{constants.RoleCrew, constants.RoleMiner}

// isValidRole checks if a string is a valid agent role.
func isValidRole(s string) bool {
	for _, r := range ValidAgentRoles {
		if s == r {
			return true
		}
	}
	return false
}

// isTownLevelRole checks if a role is a town-level role (no rig, no name).
func isTownLevelRole(s string) bool {
	for _, r := range TownLevelRoles {
		if s == r {
			return true
		}
	}
	return false
}

// isTownLevelNamedRole checks if a role is a town-level role with a name.
func isTownLevelNamedRole(s string) bool {
	for _, r := range TownLevelNamedRoles {
		if s == r {
			return true
		}
	}
	return false
}

// isRigLevelRole checks if a role is a rig-level singleton role.
func isRigLevelRole(s string) bool {
	for _, r := range RigLevelRoles {
		if s == r {
			return true
		}
	}
	return false
}

// isNamedRole checks if a role requires a worker name (rig-level).
func isNamedRole(s string) bool {
	for _, r := range NamedRoles {
		if s == r {
			return true
		}
	}
	return false
}

// ExtractAgentPrefix extracts the prefix from an agent ID.
// Agent IDs have the format: prefix-rig-role-name or prefix-role
// The prefix is always the part before the first hyphen.
// Examples:
//   - "gt-mineshaft-miner-nux" -> "gt"
//   - "nx-nexus-miner-nux" -> "nx"
//   - "gt-overseer" -> "gt"
//   - "bd-beads-witness" -> "bd"
func ExtractAgentPrefix(id string) string {
	hyphenIdx := strings.Index(id, "-")
	if hyphenIdx <= 0 {
		return ""
	}
	return id[:hyphenIdx]
}

// ValidateAgentID validates that an agent ID follows the expected pattern.
// Canonical format: prefix-rig-role-name
// Patterns:
//   - Town-level: <prefix>-<role> (e.g., gt-overseer, bd-supervisor)
//   - Town-level named: <prefix>-<role>-<name> (e.g., gt-dog-alpha)
//   - Per-rig singleton: <prefix>-<rig>-<role> (e.g., gt-mineshaft-witness)
//   - Per-rig named: <prefix>-<rig>-<role>-<name> (e.g., gt-mineshaft-miner-nux)
//
// The prefix can be any rig's configured prefix (gt-, bd-, etc.).
// Rig names may contain hyphens (e.g., my-project), so we parse by scanning
// for known role tokens from the right side of the ID.
// Returns nil if the ID is valid, or an error describing the issue.
func ValidateAgentID(id string) error {
	if id == "" {
		return fmt.Errorf("agent ID is required")
	}

	// Must contain a hyphen to have a prefix
	hyphenIdx := strings.Index(id, "-")
	if hyphenIdx <= 0 {
		return fmt.Errorf("agent ID must have a prefix followed by '-' (got %q)", id)
	}

	// Split into parts after the prefix
	rest := id[hyphenIdx+1:] // Skip "<prefix>-"
	parts := strings.Split(rest, "-")
	if len(parts) < 1 || parts[0] == "" {
		return fmt.Errorf("agent ID must include content after prefix (got %q)", id)
	}

	// Case 1: Single part after prefix - town-level role or collapsed rig-level
	// (collapsed form: when prefix == rig, e.g., "ff-witness" for rig "ff")
	if len(parts) == 1 {
		role := parts[0]
		if isTownLevelRole(role) {
			return nil // Valid town-level agent
		}
		if isTownLevelNamedRole(role) {
			return fmt.Errorf("agent role %q requires name: <prefix>-%s-<name> (got %q)", role, role, id)
		}
		if isRigLevelRole(role) {
			return nil // Valid collapsed rig-level singleton (prefix == rig)
		}
		if isNamedRole(role) {
			return fmt.Errorf("agent role %q requires name: <prefix>-%s-<name> (got %q)", role, role, id)
		}
		return fmt.Errorf("invalid agent role %q (valid: %s)", role, strings.Join(ValidAgentRoles, ", "))
	}

	// Case 2: Two parts - could be town-level named (dog-alpha), rig-level singleton
	// (mineshaft-witness), or collapsed named agent (miner-nux when prefix == rig)
	if len(parts) == 2 {
		// Check if first part is a town-level named role
		if isTownLevelNamedRole(parts[0]) {
			return nil // Valid town-level named agent: gt-dog-alpha
		}
		// Check if first part is a named role (collapsed form: prefix-role-name)
		if isNamedRole(parts[0]) {
			return nil // Valid collapsed named agent: ff-miner-nux (prefix == rig)
		}
		// Check if second part is a rig-level singleton role
		if isRigLevelRole(parts[1]) {
			return nil // Valid rig-level singleton: gt-mineshaft-witness
		}
		// Check if second part is a named role (missing name)
		if isNamedRole(parts[1]) {
			return fmt.Errorf("agent role %q requires name: <prefix>-<rig>-%s-<name> (got %q)", parts[1], parts[1], id)
		}
		// Check if second part is a town-level role (invalid with rig)
		if isTownLevelRole(parts[1]) {
			return fmt.Errorf("town-level agent %q cannot have rig/name suffixes (expected <prefix>-%s, got %q)", parts[1], parts[1], id)
		}
		return fmt.Errorf("invalid agent format: no valid role found in %q (valid roles: %s)", id, strings.Join(ValidAgentRoles, ", "))
	}

	// For 3+ parts, scan from the right to find a known role.
	// This allows rig names to contain hyphens (e.g., "my-project").
	// When a worker name collides with a role keyword (e.g., miner named
	// "witness"), prefer the named-role interpretation over singleton.
	roleIdx := -1
	var role string
	for i := len(parts) - 1; i >= 0; i-- {
		if !isValidRole(parts[i]) {
			continue
		}
		// Found a role keyword. Check if the part to its left is a named
		// role — if so, the keyword is actually the worker's name.
		if i >= 2 && isNamedRole(parts[i-1]) {
			roleIdx = i - 1
			role = parts[i-1]
			break
		}
		roleIdx = i
		role = parts[i]
		break
	}

	if roleIdx == -1 {
		return fmt.Errorf("invalid agent format: no valid role found in %q (valid roles: %s)", id, strings.Join(ValidAgentRoles, ", "))
	}

	// Extract rig (everything before role) and name (everything after role)
	rig := strings.Join(parts[:roleIdx], "-")
	name := strings.Join(parts[roleIdx+1:], "-")

	// Validate based on role type
	if isTownLevelRole(role) {
		if rig != "" || name != "" {
			return fmt.Errorf("town-level agent %q cannot have rig/name suffixes (expected <prefix>-%s, got %q)", role, role, id)
		}
		return nil
	}

	if isTownLevelNamedRole(role) {
		if rig != "" {
			return fmt.Errorf("town-level agent %q cannot have rig prefix (expected <prefix>-%s-<name>, got %q)", role, role, id)
		}
		if name == "" {
			return fmt.Errorf("agent role %q requires name: <prefix>-%s-<name> (got %q)", role, role, id)
		}
		return nil // Valid town-level named agent
	}

	if isRigLevelRole(role) {
		if rig == "" {
			return fmt.Errorf("agent role %q requires rig: <prefix>-<rig>-%s (got %q)", role, role, id)
		}
		if name != "" {
			return fmt.Errorf("agent role %q cannot have name suffix (expected <prefix>-<rig>-%s, got %q)", role, role, id)
		}
		return nil // Valid rig-level singleton agent
	}

	if isNamedRole(role) {
		if rig == "" {
			return fmt.Errorf("rig name cannot be empty in %q", id)
		}
		if name == "" {
			return fmt.Errorf("agent role %q requires name: <prefix>-<rig>-%s-<name> (got %q)", role, role, id)
		}
		return nil // Valid named agent
	}

	return fmt.Errorf("invalid agent ID format: %q", id)
}

// ===== Rig-level agent bead ID helpers (gt- prefix) =====

// Agent bead ID naming convention:
//   prefix-rig-role-name
//
// Examples:
//   - gt-overseer (town-level, no rig)
//   - gt-supervisor (town-level, no rig)
//   - gt-mineshaft-witness (rig-level singleton)
//   - gt-mineshaft-refinery (rig-level singleton)
//   - gt-mineshaft-crew-max (rig-level named agent)
//   - gt-mineshaft-miner-Toast (rig-level named agent)

// AgentBeadIDWithPrefix generates an agent bead ID using the specified prefix.
// The prefix should NOT include the hyphen (e.g., "gt", "bd", not "gt-", "bd-").
// For town-level agents (overseer, supervisor), pass empty rig and name.
// For rig-level singletons (witness, refinery), pass empty name.
// For named agents (crew, miner), pass all three.
//
// When prefix == rig (e.g., rig "ff" with derived prefix "ff"), the rig component
// is omitted to avoid stuttered IDs like "ff-ff-witness". Instead produces "ff-witness".
func AgentBeadIDWithPrefix(prefix, rig, role, name string) string {
	if rig == "" || rig == prefix {
		// Town-level agent (rig=="") or collapsed form (rig==prefix):
		//   prefix-role or prefix-role-name
		if name == "" {
			return prefix + "-" + role
		}
		return prefix + "-" + role + "-" + name
	}
	if name == "" {
		// Rig-level singleton: prefix-rig-witness, prefix-rig-refinery
		return prefix + "-" + rig + "-" + role
	}
	// Rig-level named agent: prefix-rig-role-name
	return prefix + "-" + rig + "-" + role + "-" + name
}

// AgentBeadID generates the canonical agent bead ID using "gt" prefix.
// For non-mineshaft rigs, use AgentBeadIDWithPrefix with the rig's configured prefix.
func AgentBeadID(rig, role, name string) string {
	return AgentBeadIDWithPrefix("gt", rig, role, name)
}

// WitnessBeadIDWithPrefix returns the Witness agent bead ID for a rig using the specified prefix.
func WitnessBeadIDWithPrefix(prefix, rig string) string {
	return AgentBeadIDWithPrefix(prefix, rig, constants.RoleWitness, "")
}

// WitnessBeadID returns the Witness agent bead ID for a rig using "gt" prefix.
func WitnessBeadID(rig string) string {
	return WitnessBeadIDWithPrefix("gt", rig)
}

// RefineryBeadIDWithPrefix returns the Refinery agent bead ID for a rig using the specified prefix.
func RefineryBeadIDWithPrefix(prefix, rig string) string {
	return AgentBeadIDWithPrefix(prefix, rig, constants.RoleRefinery, "")
}

// RefineryBeadID returns the Refinery agent bead ID for a rig using "gt" prefix.
func RefineryBeadID(rig string) string {
	return RefineryBeadIDWithPrefix("gt", rig)
}

// CrewBeadIDWithPrefix returns a Crew worker agent bead ID using the specified prefix.
func CrewBeadIDWithPrefix(prefix, rig, name string) string {
	return AgentBeadIDWithPrefix(prefix, rig, constants.RoleCrew, name)
}

// CrewBeadID returns a Crew worker agent bead ID using "gt" prefix.
func CrewBeadID(rig, name string) string {
	return CrewBeadIDWithPrefix("gt", rig, name)
}

// MinerBeadIDWithPrefix returns a Miner agent bead ID using the specified prefix.
func MinerBeadIDWithPrefix(prefix, rig, name string) string {
	return AgentBeadIDWithPrefix(prefix, rig, constants.RoleMiner, name)
}

// MinerBeadID returns a Miner agent bead ID using "gt" prefix.
func MinerBeadID(rig, name string) string {
	return MinerBeadIDWithPrefix("gt", rig, name)
}

// ParseAgentBeadID parses an agent bead ID into its components.
// Returns rig, role, name, and whether parsing succeeded.
// For town-level agents, rig will be empty.
// For singletons, name will be empty.
// Accepts any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
//
// Handles the collapsed form where prefix == rig (e.g., "ff-witness" for rig "ff").
// In collapsed form, the prefix is returned as the rig:
//   - "ff-witness"     → rig="ff", role="witness", name=""
//   - "ff-miner-nux" → rig="ff", role="miner", name="nux"
func ParseAgentBeadID(id string) (rig, role, name string, ok bool) {
	// Find the prefix (everything before the first hyphen)
	// Valid prefixes are 2-3 characters (e.g., "gt", "bd", "hq")
	hyphenIdx := strings.Index(id, "-")
	if hyphenIdx < 2 || hyphenIdx > 3 {
		return "", "", "", false
	}

	prefix := id[:hyphenIdx]
	rest := id[hyphenIdx+1:]
	parts := strings.Split(rest, "-")

	if len(parts) == 0 {
		return "", "", "", false
	}

	// Single part: town-level role (gt-overseer) or collapsed rig-level (ff-witness)
	if len(parts) == 1 {
		r := parts[0]
		if isTownLevelRole(r) {
			return "", r, "", true
		}
		// Collapsed rig-level singleton: prefix is the rig (e.g., ff-witness)
		if isRigLevelRole(r) {
			return prefix, r, "", true
		}
		// Unknown single-part — return as-is for backward compat
		return "", r, "", true
	}

	// Check for town-level named roles (dog) first
	if parts[0] == "dog" {
		return "", "dog", strings.Join(parts[1:], "-"), true
	}

	// Check for collapsed named agent: prefix-role-name (e.g., ff-miner-nux)
	// This happens when prefix == rig, so the rig component was omitted.
	if isNamedRole(parts[0]) {
		return prefix, parts[0], strings.Join(parts[1:], "-"), true
	}

	// Scan from right for known role markers to handle hyphenated rig names.
	// Format: <rig>-<role>[-<name>] where rig may contain hyphens.
	//
	// When a worker name collides with a role keyword (e.g., a miner named
	// "witness"), we prefer the named-role interpretation. A named role like
	// "miner" at position i-1 consuming the keyword at position i as its
	// name is more specific than treating the keyword as a singleton role.
	for i := len(parts) - 1; i >= 1; i-- {
		p := parts[i]
		if isNamedRole(p) && i < len(parts)-1 {
			// Named roles with a name following: crew, miner
			return strings.Join(parts[:i], "-"), p, strings.Join(parts[i+1:], "-"), true
		}
		if isRigLevelRole(p) {
			// Before accepting as singleton, check if the part to the left
			// is a named role — if so, this keyword is actually the worker's
			// name, not a singleton role marker.
			if i >= 2 && isNamedRole(parts[i-1]) {
				return strings.Join(parts[:i-1], "-"), parts[i-1], strings.Join(parts[i:], "-"), true
			}
			// Genuine singleton role: witness, refinery
			return strings.Join(parts[:i], "-"), p, "", true
		}
		if isNamedRole(p) && i == len(parts)-1 {
			// Named role at the end without a following name. Check if the
			// part to the left is also a named role — if so, this keyword
			// is the worker's name for that role.
			if i >= 2 && isNamedRole(parts[i-1]) {
				return strings.Join(parts[:i-1], "-"), parts[i-1], p, true
			}
			// Named role without a name (invalid but handle gracefully)
			return strings.Join(parts[:i], "-"), p, "", true
		}
	}

	// Fallback: assume 2-part rig/role pattern
	if len(parts) == 2 {
		return parts[0], parts[1], "", true
	}

	return "", "", "", false
}

// IsAgentSessionBead returns true if the bead ID represents an agent session molecule.
// Agent session beads follow patterns like gt-overseer, bd-beads-witness, gt-mineshaft-crew-joe.
// Supports any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
// These are used to track agent state and update frequently, which can create noise.
func IsAgentSessionBead(beadID string) bool {
	_, role, _, ok := ParseAgentBeadID(beadID)
	if !ok {
		return false
	}
	// Known agent roles
	switch role {
	case constants.RoleOverseer, constants.RoleSupervisor, constants.RoleWitness, constants.RoleRefinery, constants.RoleCrew, constants.RoleMiner, "dog":
		return true
	default:
		return false
	}
}
