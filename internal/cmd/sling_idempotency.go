package cmd

import "strings"

// normalizeAgentID trims surrounding whitespace and trailing slash for comparison.
func normalizeAgentID(v string) string {
	return strings.TrimSuffix(strings.TrimSpace(v), "/")
}

// matchesSlingTarget returns true when target should be treated as equivalent
// to the existing assignee for idempotent sling behavior.
//
// Only matches unambiguous equivalences. Ambiguous shorthand targets
// (e.g., "rig/name" which could resolve to miners or crew) and pool
// targets (e.g., "supervisor/dogs" which dispatches to an idle dog) are NOT
// matched — these must go through normal resolution to pick the right agent.
func matchesSlingTarget(target, assignee, selfAgent string) bool {
	assigneeNorm := normalizeAgentID(assignee)
	if assigneeNorm == "" {
		return false
	}

	target = strings.TrimSpace(target)
	if target == "" || target == "." {
		selfNorm := normalizeAgentID(selfAgent)
		return selfNorm != "" && selfNorm == assigneeNorm
	}

	targetNorm := normalizeAgentID(target)
	if targetNorm == assigneeNorm {
		return true
	}

	// Rig-only target maps to miner dispatch within that rig.
	// Intentionally excludes crew/witness/refinery: rig-name targets resolve
	// exclusively to miners via IsRigName, so "mineshaft" + "mineshaft/crew/alex"
	// is NOT a match (different dispatch path).
	parts := strings.Split(targetNorm, "/")
	if len(parts) == 1 && strings.HasPrefix(assigneeNorm, targetNorm+"/miners/") {
		return true
	}

	// NOTE: Two-segment shorthand targets (e.g., "mineshaft/alex") and pool
	// targets (e.g., "supervisor/dogs") are intentionally NOT matched here.
	// - Shorthand: the real resolver has priority logic (prefers crew when
	//   crew dir exists) that this pure function cannot replicate.
	// - Pool: "supervisor/dogs" means "dispatch to an idle dog", not "keep the
	//   current dog". Matching would prevent reassignment to idle workers.
	// Users can use full paths (e.g., "mineshaft/miners/toast") for
	// unambiguous idempotent behavior with these targets.

	return false
}
