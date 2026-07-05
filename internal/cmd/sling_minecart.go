package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/telemetry"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// slingGenerateShortID generates a short random ID (5 lowercase chars).
func slingGenerateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

// isTrackedByMinecart checks if an issue is already being tracked by a minecart.
// Returns the minecart ID if tracked, empty string otherwise.
//
// Uses bdDepListRawIDs for cross-database dep resolution (GH #2624).
// For direction=up queries, the raw SQL approach queries the same table but
// looks for rows where depends_on_id matches the beadID, returning the
// issue_id (which is the minecart). Since this only returns IDs (no issue_type
// or status), we verify each candidate via bd show.
func isTrackedByMinecart(beadID string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}
	townBeads := filepath.Join(townRoot, ".beads")

	// Primary: Use raw dep query to find what tracks this issue (direction=up).
	// This returns minecart IDs that have a "tracks" dep on beadID.
	trackerIDs, err := bdDepListRawIDs(townBeads, beadID, "up", "tracks")
	if err == nil && len(trackerIDs) > 0 {
		// Check each tracker to find an open minecart
		for _, trackerID := range trackerIDs {
			result, err := bdShow(trackerID)
			if err != nil {
				continue
			}
			if isMinecartIssue(result.IssueType, result.Labels) && result.Status == "open" {
				return trackerID
			}
		}
	}

	// Fallback: Query minecarts directly by description pattern
	// This is more robust when cross-rig routing has issues (G19, G21)
	// Auto-minecarts have description "Auto-created minecart tracking <beadID>"
	return findMinecartByDescription(townRoot, beadID)
}

// findMinecartByDescription searches open minecarts for one tracking the given beadID.
// Checks both minecart descriptions (for auto-created minecarts) and tracked deps
// (for manually-created minecarts where the description won't match).
// Returns minecart ID if found, empty string otherwise.
func findMinecartByDescription(townRoot, beadID string) string {
	townBeads := filepath.Join(townRoot, ".beads")

	minecarts, err := listMinecartIssues(townBeads, "open", false)
	if err != nil {
		return ""
	}

	// Check if any minecart's description mentions tracking this beadID
	// (matches auto-created minecarts with "Auto-created minecart tracking <beadID>")
	trackingPattern := fmt.Sprintf("tracking %s", beadID)
	for _, minecart := range minecarts {
		if strings.Contains(minecart.Description, trackingPattern) {
			return minecart.ID
		}
	}

	// Check tracked deps of each minecart (for manually-created minecarts).
	// This handles the case where cross-rig dep resolution (direction=up) fails
	// but the minecart does have a tracks dependency on the bead.
	for _, minecart := range minecarts {
		if minecartTracksBead(townBeads, minecart.ID, beadID) {
			return minecart.ID
		}
	}

	return ""
}

// minecartTracksBead checks if a minecart has a tracks dependency on the given beadID.
// Uses bdDepListRawIDs for cross-database dep resolution (GH #2624).
func minecartTracksBead(beadsDir, minecartID, beadID string) bool {
	trackedIDs, err := bdDepListRawIDs(beadsDir, minecartID, "down", "tracks")
	if err != nil {
		return false
	}

	for _, id := range trackedIDs {
		if id == beadID {
			return true
		}
	}
	return false
}

// MinecartInfo holds minecart details for an issue's tracking minecart.
type MinecartInfo struct {
	ID            string // Minecart bead ID (e.g., "hq-cv-abc")
	Owned         bool   // true if minecart has ms:owned label
	MergeStrategy string // "direct", "mr", "local", or "" (default = mr)
}

// IsOwnedDirect returns true if the minecart is owned with direct merge strategy.
// This is the key check for skipping witness/refinery merge pipeline.
func (c *MinecartInfo) IsOwnedDirect() bool {
	return c != nil && c.Owned && c.MergeStrategy == "direct"
}

// getMinecartInfoForIssue checks if an issue is tracked by a minecart and returns its info.
// Returns nil if not tracked by any minecart.
func getMinecartInfoForIssue(issueID string) *MinecartInfo {
	minecartID := isTrackedByMinecart(issueID)
	if minecartID == "" {
		return nil
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil
	}
	townBeads := filepath.Join(townRoot, ".beads")

	var stderr bytes.Buffer
	stdout, err := BdCmd("show", minecartID, "--json").
		AllowStale().
		Dir(townRoot).
		WithBeadsDir(townBeads).
		Stderr(&stderr).
		Output()

	if err != nil {
		// Check if this is a "not found" error (phantom minecart) vs transient error.
		// Phantom minecarts occur when a minecart bead is deleted from HQ but tracking
		// deps still exist in local beads DB (ms-9xum2). Return nil to treat as
		// untracked, allowing normal MR flow to proceed.
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "not found") ||
			strings.Contains(stderrStr, "Issue not found") ||
			strings.Contains(stderrStr, "no issue found") {
			return nil // Phantom minecart - proceed without minecart context
		}
		// Other error (transient) - return basic info as fallback
		return &MinecartInfo{ID: minecartID}
	}

	var minecarts []struct {
		Labels      []string `json:"labels"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal(stdout, &minecarts); err != nil || len(minecarts) == 0 {
		return &MinecartInfo{ID: minecartID}
	}

	info := &MinecartInfo{ID: minecartID}

	// Check for ms:owned label
	for _, label := range minecarts[0].Labels {
		if label == "ms:owned" {
			info.Owned = true
			break
		}
	}

	// Parse merge strategy from description using typed accessor
	info.MergeStrategy = minecartMergeFromFields(minecarts[0].Description)

	return info
}

// getMinecartInfoFromIssue reads minecart info directly from the issue's attachment fields.
// This is the primary lookup method (ms-7b6wf fix): ms sling stores minecart_id and
// merge_strategy on the issue when dispatching, avoiding unreliable cross-rig dep
// resolution. Returns nil if the issue has no minecart fields in its description.
func getMinecartInfoFromIssue(issueID, cwd string) *MinecartInfo {
	if issueID == "" {
		return nil
	}

	bd := beads.New(beads.ResolveBeadsDir(cwd))
	issue, err := bd.Show(issueID)
	if err != nil {
		return nil
	}

	attachment := beads.ParseAttachmentFields(issue)
	if attachment == nil || attachment.MinecartID == "" {
		return nil
	}

	return &MinecartInfo{
		ID:            attachment.MinecartID,
		MergeStrategy: attachment.MergeStrategy,
		Owned:         attachment.MinecartOwned,
	}
}

// printMinecartConflict prints detailed information about a bead that is already
// tracked by another minecart, including all beads in that minecart with their
// statuses, and recommended actions the user can take.
func printMinecartConflict(beadID, minecartID string) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		fmt.Printf("\n  %s is already tracked by minecart %s\n", beadID, minecartID)
		return
	}
	townBeads := filepath.Join(townRoot, ".beads")

	var minecartTitle string
	showOut, err := BdCmd("show", minecartID, "--json").
		AllowStale().
		Dir(townRoot).
		WithBeadsDir(townBeads).
		Stderr(io.Discard).
		Output()
	if err == nil {
		var items []struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(showOut, &items) == nil && len(items) > 0 {
			minecartTitle = items[0].Title
		}
	}

	fmt.Printf("\n  Conflict: %s is already tracked by minecart %s", beadID, minecartID)
	if minecartTitle != "" {
		fmt.Printf(" (%s)", minecartTitle)
	}
	fmt.Println()

	// Get all beads in the conflicting minecart
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err == nil && len(tracked) > 0 {
		fmt.Printf("\n  Beads in minecart %s:\n", minecartID)
		for _, t := range tracked {
			marker := " "
			if t.ID == beadID {
				marker = "→"
			}
			statusIcon := "○"
			switch t.Status {
			case "open":
				statusIcon = "●"
			case "closed":
				statusIcon = "✓"
			case "hooked", "pinned":
				statusIcon = "◆"
			}
			title := t.Title
			if title == "" {
				title = "(no title)"
			}
			suffix := ""
			if t.ID == beadID {
				suffix = "  ← conflict"
			}
			fmt.Printf("    %s %s %s  %s [%s]%s\n", marker, statusIcon, t.ID, title, t.Status, suffix)
		}
	}

	fmt.Printf("\n  Options:\n")
	fmt.Printf("    1. Remove the bead from this batch:\n")
	fmt.Printf("         ms sling <other-beads...> <rig>   (without %s)\n", beadID)
	fmt.Printf("    2. Move the bead to the new batch (remove from existing minecart first):\n")
	fmt.Printf("         bd dep remove %s %s --type=tracks\n", minecartID, beadID)
	fmt.Printf("         ms sling <all-beads...> <rig>\n")
	fmt.Printf("    3. Close the existing minecart and re-sling all beads together:\n")
	fmt.Printf("         ms minecart close %s --reason \"re-batching\"\n", minecartID)
	fmt.Printf("         ms sling <all-beads...> <rig>\n")
	fmt.Printf("    4. Add the other beads to the existing minecart instead:\n")
	fmt.Printf("         ms minecart add %s <other-beads...>\n", minecartID)
	fmt.Println()
}

// createBatchMinecart creates a single auto-minecart that tracks all beads in a batch sling.
// Returns the minecart ID and the list of bead IDs that were successfully tracked.
// Callers should only stamp MinecartID on beads in the tracked set — a bead whose
// dep add failed should not reference a minecart that has no knowledge of it.
// If owned is true, the minecart is marked with ms:owned label.
// beadIDs must be non-empty. The minecart title uses the rig name and bead count.
func createBatchMinecart(beadIDs []string, rigName string, owned bool, mergeStrategy, baseBranch string) (string, []string, error) {
	if len(beadIDs) == 0 {
		return "", nil, fmt.Errorf("no beads to track")
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", nil, fmt.Errorf("finding town root: %w", err)
	}

	townBeads := filepath.Join(townRoot, ".beads")

	minecartID := fmt.Sprintf("hq-cv-%s", slingGenerateShortID())

	minecartTitle := fmt.Sprintf("Batch: %d beads to %s", len(beadIDs), rigName)
	prose := fmt.Sprintf("Auto-created minecart tracking %d beads", len(beadIDs))
	description := beads.SetMinecartFields(&beads.Issue{Description: prose}, &beads.MinecartFields{
		Merge:      mergeStrategy,
		BaseBranch: baseBranch,
	})

	createArgs := []string{
		"create",
		"--type=task",
		"--id=" + minecartID,
		"--title=" + minecartTitle,
		"--description=" + description,
		"--labels=" + minecartLabels(owned),
	}
	if beads.NeedsForceForID(minecartID) {
		createArgs = append(createArgs, "--force")
	}

	// Use BdCmd with WithAutoCommit to ensure minecart is persisted even when
	// ms sling has set BD_DOLT_AUTO_COMMIT=off globally (ms-9xum2 root cause fix).
	if out, err := BdCmd(createArgs...).Dir(townBeads).WithAutoCommit().CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("creating batch minecart: %w\noutput: %s", err, out)
	}

	// Add tracking relations for all beads, recording which succeed.
	// Use WithAutoCommit for the same reason as above.
	var tracked []string
	for _, beadID := range beadIDs {
		if err := addTrackingRelationFn(townRoot, minecartID, beadID); err != nil {
			// Log but continue — partial tracking is better than no tracking
			fmt.Printf("  Warning: could not track %s in minecart: %v\n", beadID, err)
		} else {
			tracked = append(tracked, beadID)
		}
	}

	return minecartID, tracked, nil
}

// createAutoMinecart creates an auto-minecart for a single issue and tracks it.
// If owned is true, the minecart is marked with the ms:owned label for caller-managed lifecycle.
// mergeStrategy is optional: "direct", "mr", or "local" (empty = default mr).
// Returns the created minecart ID.
func createAutoMinecart(beadID, beadTitle string, owned bool, mergeStrategy, baseBranch string) (_ string, retErr error) {
	defer func() { telemetry.RecordMinecartCreate(context.Background(), beadID, retErr) }()
	// Guard against flag-like titles propagating into minecart names (ms-e0kx5)
	if beads.IsFlagLikeTitle(beadTitle) {
		return "", fmt.Errorf("refusing to create minecart: bead title %q looks like a CLI flag", beadTitle)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", fmt.Errorf("finding town root: %w", err)
	}

	townBeads := filepath.Join(townRoot, ".beads")

	// Generate minecart ID with hq-cv- prefix for visual distinction
	// The hq-cv- prefix is registered in routes during ms install
	minecartID := fmt.Sprintf("hq-cv-%s", slingGenerateShortID())

	// Create minecart with title "Work: <issue-title>"
	minecartTitle := fmt.Sprintf("Work: %s", beadTitle)
	prose := fmt.Sprintf("Auto-created minecart tracking %s", beadID)
	description := beads.SetMinecartFields(&beads.Issue{Description: prose}, &beads.MinecartFields{
		Merge:      mergeStrategy,
		BaseBranch: baseBranch,
	})

	createArgs := []string{
		"create",
		"--type=task",
		"--id=" + minecartID,
		"--title=" + minecartTitle,
		"--description=" + description,
		"--labels=" + minecartLabels(owned),
	}
	if beads.NeedsForceForID(minecartID) {
		createArgs = append(createArgs, "--force")
	}

	// Use BdCmd with WithAutoCommit to ensure minecart is persisted even when
	// ms sling has set BD_DOLT_AUTO_COMMIT=off globally (ms-9xum2 root cause fix).
	if out, err := BdCmd(createArgs...).Dir(townBeads).WithAutoCommit().CombinedOutput(); err != nil {
		return "", fmt.Errorf("creating minecart: %w\noutput: %s", err, out)
	}

	// Add tracking relation: minecart tracks the issue.
	if err := addTrackingRelationFn(townRoot, minecartID, beadID); err != nil {
		fmt.Printf("Warning: Could not create auto-minecart tracking: %v\n", err)
	}

	return minecartID, nil
}
