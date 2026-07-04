package git_test

import (
	"github.com/steveyegge/excavation/internal/beads"
	"github.com/steveyegge/excavation/internal/git"
)

// Compile-time assertion: Git must satisfy BranchChecker.
var _ beads.BranchChecker = (*git.Git)(nil)
