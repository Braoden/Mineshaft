package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/miner"
	"github.com/steveyegge/mineshaft/internal/rig"
	"github.com/steveyegge/mineshaft/internal/style"
)

// minerTarget represents a miner to operate on.
type minerTarget struct {
	rigName     string
	minerName string
	mgr         *miner.Manager
	r           *rig.Rig
}

// resolveMinerTargets builds a list of miners from command args.
// If useAll is true, the first arg is treated as a rig name and all miners in it are returned.
// Otherwise, args are parsed as rig/miner addresses.
func resolveMinerTargets(args []string, useAll bool) ([]minerTarget, error) {
	var targets []minerTarget

	if useAll {
		// --all flag: first arg is just the rig name
		rigName := args[0]
		// Check if it looks like rig/miner format
		if _, _, err := parseAddress(rigName); err == nil {
			return nil, fmt.Errorf("with --all, provide just the rig name (e.g., 'ms miner <cmd> %s --all')", strings.Split(rigName, "/")[0])
		}

		mgr, r, err := getMinerManager(rigName)
		if err != nil {
			return nil, err
		}

		miners, err := mgr.List()
		if err != nil {
			return nil, fmt.Errorf("listing miners: %w", err)
		}

		for _, p := range miners {
			targets = append(targets, minerTarget{
				rigName:     rigName,
				minerName: p.Name,
				mgr:         mgr,
				r:           r,
			})
		}
	} else {
		// Multiple rig/miner arguments - require explicit rig/miner format
		for _, arg := range args {
			// Validate format: must contain "/" to avoid misinterpreting rig names as miner names
			if !strings.Contains(arg, "/") {
				return nil, fmt.Errorf("invalid address '%s': must be in 'rig/miner' format (e.g., 'mineshaft/Toast')", arg)
			}

			rigName, minerName, err := parseAddress(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid address '%s': %w", arg, err)
			}

			mgr, r, err := getMinerManager(rigName)
			if err != nil {
				return nil, err
			}

			targets = append(targets, minerTarget{
				rigName:     rigName,
				minerName: minerName,
				mgr:         mgr,
				r:           r,
			})
		}
	}

	return targets, nil
}

// SafetyCheckResult holds the result of safety checks for a miner.
type SafetyCheckResult struct {
	Miner       string
	Blocked       bool
	Reasons       []string
	CleanupStatus miner.CleanupStatus
	HookBead      string
	HookStale     bool // true if hooked bead is closed
	ActiveMR      string
	OpenMR        string
	GitState      *GitState
}

// checkMinerSafety performs safety checks before destructive operations.
// Returns nil if the miner is safe to operate on, or a SafetyCheckResult with reasons if blocked.
func checkMinerSafety(target minerTarget) *SafetyCheckResult {
	result := &SafetyCheckResult{
		Miner: fmt.Sprintf("%s/%s", target.rigName, target.minerName),
	}

	// Get miner info for branch name
	minerInfo, infoErr := target.mgr.Get(target.minerName)

	// Check 1: Unpushed commits via cleanup_status or git state
	bd := beads.New(target.r.Path)
	agentBeadID := minerBeadIDForRig(target.r, target.rigName, target.minerName)
	agentIssue, fields, err := bd.GetAgentBead(agentBeadID)

	if err != nil || fields == nil {
		// No agent bead - fall back to git check
		if infoErr == nil && minerInfo != nil {
			gitState, gitErr := getGitState(minerInfo.ClonePath)
			result.GitState = gitState
			if gitErr != nil {
				result.Reasons = append(result.Reasons, "cannot check git state")
			} else if !gitState.Clean {
				if gitState.UnpushedCommits > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d unpushed commit(s)", gitState.UnpushedCommits))
				} else if len(gitState.UncommittedFiles) > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d uncommitted file(s)", len(gitState.UncommittedFiles)))
				} else if gitState.StashCount > 0 {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has %d stash(es)", gitState.StashCount))
				}
			}
		}
	} else {
		currentIssue := ""
		if infoErr == nil && minerInfo != nil {
			currentIssue = minerInfo.Issue
		}
		sourceHint := agentSourceIssueHint(currentIssue, fields)
		hookBead := agentHookBead(agentIssue, fields)
		var gitState *GitState
		gitStateLoaded := false
		loadGitState := func() {
			if gitStateLoaded || infoErr != nil || minerInfo == nil {
				return
			}
			gitState, _ = getGitState(minerInfo.ClonePath)
			result.GitState = gitState
			gitStateLoaded = true
		}
		activeMRAssessment := miner.ActiveMRAssessment{}
		if fields.ActiveMR != "" {
			loadGitState()
			gitSafe := false
			if minerInfo != nil {
				gitSafe = activeMRGitSafeForWorktree(minerInfo.ClonePath)
			}
			activeMRAssessment = miner.AssessActiveMR(bd, miner.ActiveMRInput{ActiveMR: fields.ActiveMR, SourceIssueHint: sourceHint, RequireGitSafe: true, GitSafe: gitSafe})
		}
		beadTerminal := isAssignedBeadTerminal(bd, sourceHint)
		if activeMRAssessment.SourceTerminal {
			beadTerminal = true
		}

		// Check cleanup_status from agent bead
		result.CleanupStatus = miner.CleanupStatus(fields.CleanupStatus)
		switch result.CleanupStatus {
		case miner.CleanupClean:
			// OK
		default:
			if result.CleanupStatus == miner.CleanupUnpushed {
				loadGitState()
			}
			gitSafe := false
			if minerInfo != nil {
				gitSafe = activeMRGitSafeForWorktree(minerInfo.ClonePath)
			}
			hookSafe, hookTerminal, _ := hookBeadSafeForCleanup(bd, hookBead)
			activeMRSafe := !activeMRAssessment.Pending
			if miner.CanIgnoreStaleCleanupStatus(result.CleanupStatus, beadTerminal || hookTerminal, hookSafe, activeMRSafe, gitSafe) {
				// OK: stale self-report after terminal source and direct clean git.
			} else {
				result.Reasons = append(result.Reasons, cleanupStatusBlocker(result.CleanupStatus))
			}
		}

		// Check 3: Work on hook
		if hookBead != "" {
			result.HookBead = hookBead
			// Check if hooked bead is still active (not closed)
			hookedIssue, err := bd.Show(hookBead)
			if err == nil && hookedIssue != nil {
				if hookedIssue.Status != "closed" {
					result.Reasons = append(result.Reasons, fmt.Sprintf("has work on hook (%s)", hookBead))
				} else {
					result.HookStale = true
				}
			} else {
				result.Reasons = append(result.Reasons, fmt.Sprintf("has work on hook (%s, unverified)", hookBead))
			}
		}

		if fields.ActiveMR != "" {
			result.ActiveMR = fields.ActiveMR
			if blocker := activeMRAssessment.Reason; activeMRAssessment.Pending && blocker != "" {
				result.Reasons = append(result.Reasons, blocker)
			}
		}
	}

	// Check 2: Open MR beads for this branch
	if infoErr == nil && minerInfo != nil && minerInfo.Branch != "" {
		mr, mrErr := bd.FindMRForBranch(minerInfo.Branch)
		if mrErr != nil {
			result.Reasons = append(result.Reasons, fmt.Sprintf("open_mr_lookup_error: %v", mrErr))
		} else if mr != nil {
			result.OpenMR = mr.ID
			result.Reasons = append(result.Reasons, fmt.Sprintf("has open MR (%s)", mr.ID))
		}
	}

	result.Blocked = len(result.Reasons) > 0
	return result
}

func rigPrefix(r *rig.Rig) string {
	townRoot := filepath.Dir(r.Path)
	return beads.GetPrefixForRig(townRoot, r.Name)
}

func minerBeadIDForRig(r *rig.Rig, rigName, minerName string) string {
	return beads.MinerBeadIDWithPrefix(rigPrefix(r), rigName, minerName)
}

// displaySafetyCheckBlocked prints blocked miners and guidance.
func displaySafetyCheckBlocked(blocked []*SafetyCheckResult) {
	displaySafetyCheckBlockedTo(os.Stderr, blocked)
}

func displaySafetyCheckBlockedTo(w io.Writer, blocked []*SafetyCheckResult) {
	fmt.Fprintf(w, "%s Cannot nuke the following miners:\n\n", style.Error.Render("Error:"))
	var minerList []string
	for _, b := range blocked {
		fmt.Fprintf(w, "  %s:\n", style.Bold.Render(b.Miner))
		for _, r := range b.Reasons {
			fmt.Fprintf(w, "    - %s\n", r)
		}
		minerList = append(minerList, b.Miner)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Safety checks failed. Resolve issues before nuking, or use --force.")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  1. Complete work: ms done (from miner session)")
	fmt.Fprintln(w, "  2. Push changes: git push (from miner worktree)")
	fmt.Fprintln(w, "  3. Escalate: ms mail send overseer/ -s \"RECOVERY_NEEDED\" -m \"...\"")
	fmt.Fprintf(w, "  4. Force nuke (LOSES WORK): ms miner nuke --force %s\n", strings.Join(minerList, " "))
	fmt.Fprintln(w)
}

func formatSafetyCheckBlockers(blocked []*SafetyCheckResult) string {
	parts := make([]string, 0, len(blocked))
	for _, b := range blocked {
		parts = append(parts, fmt.Sprintf("%s: %s", b.Miner, strings.Join(b.Reasons, "; ")))
	}
	return strings.Join(parts, " | ")
}

// displayDryRunSafetyCheck shows safety check status for dry-run mode. It returns true when a normal nuke would refuse.
func displayDryRunSafetyCheck(target minerTarget) bool {
	fmt.Printf("\n  Safety checks:\n")
	result := checkMinerSafety(target)
	minerInfo, infoErr := target.mgr.Get(target.minerName)
	bd := beads.New(target.r.Path)
	agentBeadID := minerBeadIDForRig(target.r, target.rigName, target.minerName)
	agentIssue, fields, err := bd.GetAgentBead(agentBeadID)

	// Check 1: cleanup status or fallback git state
	if err != nil || fields == nil {
		if infoErr == nil && minerInfo != nil {
			gitState, gitErr := getGitState(minerInfo.ClonePath)
			if gitErr != nil {
				fmt.Printf("    - Git state: %s\n", style.Warning.Render("cannot check"))
			} else if gitState.Clean {
				fmt.Printf("    - Git state: %s\n", style.Success.Render("clean"))
			} else {
				fmt.Printf("    - Git state: %s\n", style.Error.Render("dirty"))
			}
		} else {
			fmt.Printf("    - Git state: %s\n", style.Dim.Render("unknown (no miner info)"))
		}
		fmt.Printf("    - Hook: %s\n", style.Dim.Render("unknown (no agent bead)"))
	} else {
		cleanupStatus := miner.CleanupStatus(fields.CleanupStatus)
		if cleanupStatus.IsSafe() {
			fmt.Printf("    - Cleanup status: %s\n", style.Success.Render(string(cleanupStatus)))
		} else if cleanupStatus.RequiresRecovery() {
			fmt.Printf("    - Cleanup status: %s\n", style.Error.Render(string(cleanupStatus)))
		} else {
			statusText := string(cleanupStatus)
			if statusText == "" {
				statusText = "<missing>"
			}
			fmt.Printf("    - Cleanup status: %s\n", style.Warning.Render(statusText))
		}

		hookBead := agentIssue.HookBead
		if hookBead == "" {
			hookBead = fields.HookBead
		}
		if hookBead != "" {
			hookedIssue, err := bd.Show(hookBead)
			if err == nil && hookedIssue != nil && hookedIssue.Status == "closed" {
				fmt.Printf("    - Hook: %s (%s, closed - stale)\n", style.Warning.Render("stale"), hookBead)
			} else {
				fmt.Printf("    - Hook: %s (%s)\n", style.Error.Render("has work"), hookBead)
			}
		} else {
			fmt.Printf("    - Hook: %s\n", style.Success.Render("empty"))
		}

		if fields.ActiveMR != "" {
			sourceHint := agentSourceIssueHint("", fields)
			gitSafe := false
			if infoErr == nil && minerInfo != nil {
				gitSafe = activeMRGitSafeForWorktree(minerInfo.ClonePath)
			}
			if blocker := activeMRBlocker(bd, fields.ActiveMR, sourceHint, true, gitSafe); blocker != "" {
				fmt.Printf("    - Active MR: %s (%s)\n", style.Error.Render("blocked"), blocker)
			} else {
				fmt.Printf("    - Active MR: %s (%s)\n", style.Success.Render("terminal"), fields.ActiveMR)
			}
		}
	}

	// Check 2: Open MR
	if infoErr == nil && minerInfo != nil && minerInfo.Branch != "" {
		mr, mrErr := bd.FindMRForBranch(minerInfo.Branch)
		if mrErr == nil && mr != nil {
			fmt.Printf("    - Open MR: %s (%s)\n", style.Error.Render("yes"), mr.ID)
		} else {
			fmt.Printf("    - Open MR: %s\n", style.Success.Render("none"))
		}
	} else {
		fmt.Printf("    - Open MR: %s\n", style.Dim.Render("unknown (no branch info)"))
	}

	return result.Blocked
}
