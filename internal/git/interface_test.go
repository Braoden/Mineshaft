package git_test

import (
	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/git"
)

// Compile-time assertion: Git must satisfy BranchChecker.
var _ beads.BranchChecker = (*git.Git)(nil)
