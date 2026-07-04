package cmd

import "github.com/steveyegge/excavation/internal/beads"

func isMergeRequestReadyForSelection(issue *beads.Issue) bool {
	return issue != nil && issue.Status == "open" && !beads.HasUnresolvedBlockers(issue)
}
