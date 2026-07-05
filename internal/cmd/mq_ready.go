package cmd

import "github.com/steveyegge/mineshaft/internal/beads"

func isMergeRequestReadyForSelection(issue *beads.Issue) bool {
	return issue != nil && issue.Status == "open" && !beads.HasUnresolvedBlockers(issue)
}
