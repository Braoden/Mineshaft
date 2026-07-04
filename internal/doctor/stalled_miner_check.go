package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/excavation/internal/git"
	"github.com/steveyegge/excavation/internal/session"
	"github.com/steveyegge/excavation/internal/tmux"
)

// StalledMinerCheck detects miners whose tmux sessions have died but whose
// worktrees still contain unpushed commits. These are the most dangerous failure
// mode after disk space exhaustion: the miner appears dead, and nuking it
// would permanently lose the committed work on its branch.
//
// This check warns about at-risk branches so they can be pushed before cleanup.
type StalledMinerCheck struct {
	FixableCheck
	stalledMiners []stalledMinerInfo // Cached during Run for use in Fix
}

type stalledMinerInfo struct {
	name          string
	rigName       string
	branch        string
	unpushedCount int
	clonePath     string
}

// NewStalledMinerCheck creates a new stalled miner check.
func NewStalledMinerCheck() *StalledMinerCheck {
	return &StalledMinerCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "stalled-miners",
				CheckDescription: "Detect miners with dead sessions and unpushed work",
				CheckCategory:    CategoryCleanup,
			},
		},
	}
}

// Run checks all rigs for miners with dead sessions and unpushed commits.
func (c *StalledMinerCheck) Run(ctx *CheckContext) *CheckResult {
	t := tmux.NewTmux()
	var stalled []stalledMinerInfo
	var checked int

	// Iterate over all rigs (or single rig if specified)
	rigsToCheck := c.findRigs(ctx)
	for _, rigName := range rigsToCheck {
		minersDir := filepath.Join(ctx.TownRoot, rigName, "miners")
		entries, err := os.ReadDir(minersDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			minerName := entry.Name()
			checked++

			// Check if tmux session is alive
			sessionName := session.MinerSessionName(session.PrefixFor(rigName), minerName)
			alive, err := t.HasSession(sessionName)
			if err != nil || alive {
				continue // Session alive or can't check — skip
			}

			// Session is dead. Check for unpushed commits.
			clonePath := c.resolveClonePath(ctx.TownRoot, rigName, minerName)
			if clonePath == "" {
				continue
			}

			minerGit := git.NewGit(clonePath)
			branch, brErr := minerGit.CurrentBranch()
			if brErr != nil || branch == "" {
				continue
			}

			pushed, unpushedCount, checkErr := minerGit.BranchPushedToRemote(branch, "origin")
			if checkErr != nil || pushed {
				continue // Already pushed or can't check
			}

			if unpushedCount > 0 {
				stalled = append(stalled, stalledMinerInfo{
					name:          minerName,
					rigName:       rigName,
					branch:        branch,
					unpushedCount: unpushedCount,
					clonePath:     clonePath,
				})
			}
		}
	}

	c.stalledMiners = stalled

	if len(stalled) == 0 {
		msg := "No stalled miners with unpushed work"
		if checked > 0 {
			msg = fmt.Sprintf("Checked %d miner(s), no unpushed work at risk", checked)
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: msg,
		}
	}

	details := make([]string, len(stalled))
	for i, s := range stalled {
		details[i] = fmt.Sprintf("STALLED: %s/%s — branch %s has %d unpushed commit(s)",
			s.rigName, s.name, s.branch, s.unpushedCount)
	}

	return &CheckResult{
		Name:   c.Name(),
		Status: StatusWarning,
		Message: fmt.Sprintf("Found %d stalled miner(s) with unpushed work at risk of loss",
			len(stalled)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to push stalled branches to remote",
	}
}

// Fix pushes branches from stalled miners to the remote.
func (c *StalledMinerCheck) Fix(ctx *CheckContext) error {
	if len(c.stalledMiners) == 0 {
		return nil
	}

	var lastErr error
	for _, s := range c.stalledMiners {
		minerGit := git.NewGit(s.clonePath)
		if err := minerGit.Push("origin", s.branch, false); err != nil {
			lastErr = fmt.Errorf("pushing %s/%s branch %s: %w", s.rigName, s.name, s.branch, err)
		}
	}
	return lastErr
}

// findRigs returns the list of rig names to check.
func (c *StalledMinerCheck) findRigs(ctx *CheckContext) []string {
	if ctx.RigName != "" {
		return []string{ctx.RigName}
	}

	// Scan town root for rig directories (directories containing miners/)
	entries, err := os.ReadDir(ctx.TownRoot)
	if err != nil {
		return nil
	}

	var rigs []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "overseer" {
			continue
		}
		minersDir := filepath.Join(ctx.TownRoot, entry.Name(), "miners")
		if info, err := os.Stat(minersDir); err == nil && info.IsDir() {
			rigs = append(rigs, entry.Name())
		}
	}
	return rigs
}

// resolveClonePath finds the worktree path for a miner.
// Handles both new (miners/<name>/<rigname>/) and old (miners/<name>/) structures.
func (c *StalledMinerCheck) resolveClonePath(townRoot, rigName, minerName string) string {
	// New structure: miners/<name>/<rigname>/
	newPath := filepath.Join(townRoot, rigName, "miners", minerName, rigName)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		return newPath
	}

	// Old structure: miners/<name>/
	oldPath := filepath.Join(townRoot, rigName, "miners", minerName)
	if info, err := os.Stat(filepath.Join(oldPath, ".git")); err == nil && !info.IsDir() {
		return oldPath
	}

	return ""
}
