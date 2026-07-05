package cmd

import (
	"fmt"
	"strings"
)

// knownRoles lists valid second-segment roles in path-style sling targets.
var knownRoles = map[string]bool{
	"miners": true,
	"crew":     true,
	"witness":  true,
	"refinery": true,
}

// ValidateTarget performs lightweight pre-checks on a sling target string,
// catching common mistakes before resolveTarget can trigger side-effects
// like miner spawning. It returns a non-nil error with a helpful message
// when the target is clearly malformed.
//
// It intentionally does NOT duplicate the full resolution logic — valid
// targets that pass this check are still resolved by resolveTarget.
func ValidateTarget(target string) error {
	// Self, empty, and role shortcuts are always fine.
	if target == "" || target == "." {
		return nil
	}

	// No slashes → could be rig name or role shortcut; let resolveTarget decide.
	if !strings.Contains(target, "/") {
		return nil
	}

	parts := strings.Split(target, "/")

	// Reject empty segments: "rig//miners", "/miners", "rig/miners/"
	for i, p := range parts {
		if p == "" {
			return fmt.Errorf("invalid target %q: empty path segment at position %d\n"+
				"Valid formats:\n"+
				"  <rig>                  auto-spawn miner\n"+
				"  <rig>/miners/<name>  specific miner\n"+
				"  <rig>/crew/<name>      crew worker\n"+
				"  <rig>/witness          rig witness\n"+
				"  <rig>/refinery         rig refinery\n"+
				"  supervisor/dogs            dog pool\n"+
				"  overseer                  town overseer",
				target, i)
		}
	}

	// Dog targets are valid at any depth (supervisor/dogs, supervisor/dogs/<name>).
	// Supervisor sub-path validation is handled downstream by IsDogTarget/resolveTarget.
	if strings.ToLower(parts[0]) == "supervisor" {
		return nil
	}

	// Overseer has no sub-agents.
	if strings.ToLower(parts[0]) == "overseer" {
		return fmt.Errorf("invalid target %q: overseer does not have sub-agents\n"+
			"Use 'overseer' to target the overseer directly", target)
	}

	// Path targets: parts[0] = rig, parts[1] = role or shorthand name.
	// Two-segment paths like "mineshaft/nux" are miner/crew shorthand —
	// resolvePathToSession handles these by trying miner then crew lookup.
	// We only validate when the second segment IS a known role.
	if len(parts) >= 2 {
		role := strings.ToLower(parts[1])
		if knownRoles[role] {
			// Known role: apply role-specific constraints.
			if role == "witness" || role == "refinery" {
				// Witness and refinery are singleton roles — no sub-agents.
				if len(parts) > 2 {
					return fmt.Errorf("invalid target %q: %s does not have named sub-agents\n"+
						"Usage: %s/%s", target, role, parts[0], role)
				}
			} else if len(parts) == 2 {
				// Crew and miners require a name segment.
				if role == "crew" {
					return fmt.Errorf("invalid target %q: crew requires a worker name\n"+
						"Usage: %s/crew/<name>", target, parts[0])
				}
				return fmt.Errorf("invalid target %q: miners requires a miner name\n"+
					"Usage: %s/miners/<name>\n"+
					"Or use just %q to auto-spawn a miner", target, parts[0], parts[0])
			}
			// Too many segments for role paths: rig/role/name/extra
			if len(parts) > 3 {
				return fmt.Errorf("invalid target %q: too many path segments (max 3: rig/role/name)", target)
			}
		} else if len(parts) > 2 {
			// Not a known role but has 3+ segments — not a valid shorthand.
			return fmt.Errorf("invalid target %q: unknown role %q\n"+
				"Valid roles after a rig name:\n"+
				"  %s/miners/<name>  specific miner\n"+
				"  %s/crew/<name>      crew worker\n"+
				"  %s/witness          rig witness\n"+
				"  %s/refinery         rig refinery\n"+
				"Or use just %q to target by name shorthand",
				target, parts[1], parts[0], parts[0], parts[0], parts[0], parts[0])
		}
		// else: 2-segment with unknown role → miner/crew shorthand, let resolveTarget handle.
	}

	return nil
}
