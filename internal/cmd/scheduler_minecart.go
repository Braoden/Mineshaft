package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/style"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// minecartScheduleOpts holds options for minecart schedule operations.
type minecartScheduleOpts struct {
	Formula     string
	HookRawBead bool
	Force       bool
	DryRun      bool
	NoBoot      bool
}

// runMinecartScheduleByID schedules all open tracked issues of a minecart.
func runMinecartScheduleByID(minecartID string, opts minecartScheduleOpts) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	if err := verifyBeadExists(minecartID); err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	townBeads := filepath.Join(townRoot, ".beads")
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		return fmt.Errorf("getting tracked issues: %w", err)
	}

	if len(tracked) == 0 {
		fmt.Printf("Minecart %s has no tracked issues.\n", minecartID)
		return nil
	}

	type scheduleCandidate struct {
		ID      string
		Title   string
		RigName string
	}
	var candidates []scheduleCandidate
	skippedClosed := 0
	skippedAssigned := 0
	skippedScheduled := 0
	skippedNoRig := 0

	// Batch-check scheduling status for all tracked issues (single DB query).
	var beadIDs []string
	for _, t := range tracked {
		beadIDs = append(beadIDs, t.ID)
	}
	scheduledSet := areScheduled(beadIDs)

	for _, t := range tracked {
		if t.Status == "closed" || t.Status == "tombstone" {
			skippedClosed++
			continue
		}

		if t.Assignee != "" && !opts.Force {
			skippedAssigned++
			continue
		}

		if scheduledSet[t.ID] {
			skippedScheduled++
			continue
		}

		rigName := resolveRigForBead(townRoot, t.ID)
		if rigName == "" {
			skippedNoRig++
			prefix := beads.ExtractPrefix(t.ID)
			fmt.Printf("  %s %s: cannot resolve rig from prefix %q (town-root or unknown)\n",
				style.Dim.Render("○"), t.ID, prefix)
			continue
		}

		candidates = append(candidates, scheduleCandidate{ID: t.ID, Title: t.Title, RigName: rigName})
	}

	if len(candidates) == 0 {
		fmt.Printf("No issues to schedule from minecart %s", minecartID)
		if skippedClosed > 0 || skippedAssigned > 0 || skippedScheduled > 0 || skippedNoRig > 0 {
			fmt.Printf(" (%d closed, %d assigned, %d already scheduled, %d no rig)",
				skippedClosed, skippedAssigned, skippedScheduled, skippedNoRig)
		}
		fmt.Println()
		return nil
	}

	formula := opts.Formula

	if opts.DryRun {
		fmt.Printf("%s Would schedule %d issue(s) from minecart %s:\n",
			style.Bold.Render("DRY-RUN"), len(candidates), minecartID)
		if formula != "" {
			fmt.Printf("  Formula: %s\n", formula)
		} else {
			fmt.Printf("  Hook raw beads (no formula)\n")
		}
		for _, c := range candidates {
			fmt.Printf("  Would schedule: %s -> %s (%s)\n", c.ID, c.RigName, c.Title)
		}
		if skippedClosed > 0 || skippedAssigned > 0 || skippedScheduled > 0 || skippedNoRig > 0 {
			fmt.Printf("\nSkipped: %d closed, %d assigned, %d already scheduled, %d no rig\n",
				skippedClosed, skippedAssigned, skippedScheduled, skippedNoRig)
		}
		return nil
	}

	fmt.Printf("%s Scheduling %d issue(s) from minecart %s...\n",
		style.Bold.Render("📋"), len(candidates), minecartID)

	successCount := 0
	for _, c := range candidates {
		err := scheduleBead(c.ID, c.RigName, ScheduleOptions{
			Formula:     formula,
			NoMinecart:    true, // Already tracked by this minecart
			Force:       opts.Force,
			HookRawBead: opts.HookRawBead,
		})
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Dim.Render("✗"), c.ID, err)
			continue
		}
		successCount++
	}

	fmt.Printf("\n%s Scheduled %d/%d issue(s) from minecart %s\n",
		style.Bold.Render("📊"), successCount, len(candidates), minecartID)
	if skippedClosed > 0 || skippedAssigned > 0 || skippedScheduled > 0 || skippedNoRig > 0 {
		fmt.Printf("  Skipped: %d closed, %d assigned, %d already scheduled, %d no rig\n",
			skippedClosed, skippedAssigned, skippedScheduled, skippedNoRig)
	}

	if successCount == 0 {
		return fmt.Errorf("all %d schedule attempts failed for minecart %s", len(candidates), minecartID)
	}
	return nil
}

// runMinecartSlingByID immediately dispatches all open tracked issues of a minecart.
// Used when max_miners=-1 (direct dispatch mode). Each tracked issue gets its
// own miner via executeSling(). Sets NoMinecart=true since issues are already tracked.
func runMinecartSlingByID(minecartID string, opts minecartScheduleOpts) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	if err := verifyBeadExists(minecartID); err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	townBeads := filepath.Join(townRoot, ".beads")
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		return fmt.Errorf("getting tracked issues: %w", err)
	}

	if len(tracked) == 0 {
		fmt.Printf("Minecart %s has no tracked issues.\n", minecartID)
		return nil
	}

	type slingCandidate struct {
		ID      string
		Title   string
		RigName string
	}
	var candidates []slingCandidate
	skippedClosed := 0
	skippedAssigned := 0
	skippedNoRig := 0

	for _, t := range tracked {
		if t.Status == "closed" || t.Status == "tombstone" {
			skippedClosed++
			continue
		}
		if t.Assignee != "" && !opts.Force {
			skippedAssigned++
			continue
		}
		rigName := resolveRigForBead(townRoot, t.ID)
		if rigName == "" {
			skippedNoRig++
			prefix := beads.ExtractPrefix(t.ID)
			fmt.Printf("  %s %s: cannot resolve rig from prefix %q (town-root or unknown)\n",
				style.Dim.Render("○"), t.ID, prefix)
			continue
		}
		candidates = append(candidates, slingCandidate{ID: t.ID, Title: t.Title, RigName: rigName})
	}

	if len(candidates) == 0 {
		fmt.Printf("No issues to dispatch from minecart %s", minecartID)
		if skippedClosed > 0 || skippedAssigned > 0 || skippedNoRig > 0 {
			fmt.Printf(" (%d closed, %d assigned, %d no rig)",
				skippedClosed, skippedAssigned, skippedNoRig)
		}
		fmt.Println()
		return nil
	}

	formula := opts.Formula

	if opts.DryRun {
		fmt.Printf("%s Would dispatch %d issue(s) from minecart %s:\n",
			style.Bold.Render("DRY-RUN"), len(candidates), minecartID)
		for _, c := range candidates {
			fmt.Printf("  Would dispatch: %s -> %s (%s)\n", c.ID, c.RigName, c.Title)
		}
		if skippedClosed > 0 || skippedAssigned > 0 || skippedNoRig > 0 {
			fmt.Printf("\nSkipped: %d closed, %d assigned, %d no rig\n",
				skippedClosed, skippedAssigned, skippedNoRig)
		}
		return nil
	}

	fmt.Printf("%s Dispatching %d issue(s) from minecart %s...\n",
		style.Bold.Render("▶"), len(candidates), minecartID)

	successCount := 0
	successfulRigs := make(map[string]bool)
	for i, c := range candidates {
		if slingMaxConcurrent > 0 && i >= slingMaxConcurrent {
			fmt.Printf("  %s Reached --max-concurrent spawn batch size (%d), remaining will be scheduled next cycle\n", style.Dim.Render("○"), slingMaxConcurrent)
			break
		}

		fmt.Printf("\n[%d/%d] Dispatching %s → %s...\n", i+1, len(candidates), c.ID, c.RigName)
		_, err := executeSling(SlingParams{
			BeadID:        c.ID,
			RigName:       c.RigName,
			FormulaName:   formula,
			Force:         opts.Force,
			HookRawBead:   opts.HookRawBead,
			NoMinecart:      true, // Already tracked by this minecart
			NoBoot:        opts.NoBoot,
			CallerContext: "minecart-sling",
			TownRoot:      townRoot,
			BeadsDir:      filepath.Join(townRoot, ".beads"),
		})
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Dim.Render("✗"), c.ID, err)
			continue
		}
		successCount++
		successfulRigs[c.RigName] = true

		// Brief delay between spawns to avoid Dolt contention
		if i < len(candidates)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Wake rig agents for each unique rig that had successful dispatches
	if !opts.NoBoot {
		for rig := range successfulRigs {
			wakeRigAgents(rig)
		}
	}

	fmt.Printf("\n%s Dispatched %d/%d issue(s) from minecart %s\n",
		style.Bold.Render("📊"), successCount, len(candidates), minecartID)
	if skippedClosed > 0 || skippedAssigned > 0 || skippedNoRig > 0 {
		fmt.Printf("  Skipped: %d closed, %d assigned, %d no rig\n",
			skippedClosed, skippedAssigned, skippedNoRig)
	}

	if successCount == 0 {
		return fmt.Errorf("all %d dispatch attempts failed for minecart %s", len(candidates), minecartID)
	}
	return nil
}
