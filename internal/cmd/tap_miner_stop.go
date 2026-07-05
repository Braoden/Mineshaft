package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/miner"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var tapMinerStopCmd = &cobra.Command{
	Use:   "miner-stop-check",
	Short: "Auto-run ms done on session Stop if miner has pending work",
	Long: `Safety net for the "idle miner" problem: miners that finish work
but forget to call ms done before the session ends.

This command is designed to run from a Claude Code Stop hook. It checks:
1. Whether this is a miner session (MS_MINER env var)
2. Whether ms done has already run (heartbeat state is "exiting" or "idle")
3. Whether the miner has commits on its branch

If the miner has pending work that wasn't submitted, this command
runs ms done to submit it. If ms done already ran or there's nothing
to submit, it exits silently.

Exit codes:
  0 - No action needed (not a miner, already done, or ms done succeeded)
  1 - ms done was attempted but failed`,
	RunE:         runTapMinerStop,
	SilenceUsage: true,
}

func init() {
	tapCmd.AddCommand(tapMinerStopCmd)
}

func runTapMinerStop(cmd *cobra.Command, args []string) error {
	// Only applies to miners
	minerName := os.Getenv("MS_MINER")
	if minerName == "" {
		return nil // Not a miner session — nothing to do
	}

	sessionName := os.Getenv("MS_SESSION")
	if sessionName == "" {
		return nil // No session tracking — can't check state
	}

	// Find town root for heartbeat check
	townRoot, _, _ := workspace.FindFromCwdWithFallback()
	if townRoot == "" {
		townRoot = os.Getenv("MS_TOWN_ROOT")
	}
	if townRoot == "" {
		return nil // Can't find workspace — exit quietly
	}

	// Check heartbeat state: if already "exiting" or "idle", ms done already ran
	hb := miner.ReadSessionHeartbeat(townRoot, sessionName)
	if hb != nil {
		state := hb.EffectiveState()
		if state == miner.HeartbeatExiting || state == miner.HeartbeatIdle {
			return nil // ms done already ran or miner is idle — nothing to do
		}
	}

	// Check if the miner is on a feature branch with commits
	rigName := os.Getenv("MS_RIG")
	if rigName == "" {
		return nil
	}

	// Reconstruct miner worktree path
	minerDir := filepath.Join(townRoot, rigName, "miners", minerName)
	// Try the nested clone layout first (miners/<name>/<rig>/)
	cloneDir := filepath.Join(minerDir, rigName)
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
		// Fall back to flat layout
		cloneDir = minerDir
		if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err != nil {
			return nil // No git repo found — exit quietly
		}
	}

	// Check current branch — skip if on main/master
	branchCmd := exec.Command("git", "-C", cloneDir, "rev-parse", "--abbrev-ref", "HEAD")
	branchOut, err := branchCmd.Output()
	if err != nil {
		return nil // Can't determine branch — exit quietly
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch == "main" || branch == "master" || branch == "HEAD" {
		return nil // On default branch — nothing to submit
	}

	// Check for commits ahead of origin/main
	aheadCmd := exec.Command("git", "-C", cloneDir, "rev-list", "--count", "origin/main..HEAD")
	aheadOut, err := aheadCmd.Output()
	if err != nil {
		return nil // Can't check — exit quietly (don't block session stop)
	}
	ahead := strings.TrimSpace(string(aheadOut))
	if ahead == "0" {
		return nil // No commits ahead — nothing to submit
	}

	// Miner has pending work! Run ms done as a safety net.
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "⚠️  Miner %s has %s unpushed commit(s) on branch %s\n", minerName, ahead, branch)
	fmt.Fprintf(os.Stderr, "   Auto-running ms done as safety net...\n")
	fmt.Fprintf(os.Stderr, "\n")

	// Find ms binary path
	gtBin, err := os.Executable()
	if err != nil {
		gtBin = "ms"
	}

	// Run ms done in the miner's worktree context
	doneCmd := exec.Command(gtBin, "done")
	doneCmd.Dir = cloneDir
	doneCmd.Stdout = os.Stdout
	doneCmd.Stderr = os.Stderr
	// Inherit environment (MS_MINER, MS_RIG, etc. are already set)
	doneCmd.Env = os.Environ()

	if err := doneCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Auto ms done failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Witness will handle cleanup.\n")
		// Don't return error — don't block session stop
		return nil
	}

	return nil
}
