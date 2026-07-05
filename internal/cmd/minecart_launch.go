package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// minecartLaunchForce controls whether to launch a minecart with warnings.
var minecartLaunchForce bool

// DispatchResult records the outcome of dispatching a single task.
type DispatchResult struct {
	BeadID  string
	Rig     string
	Success bool
	Error   error
}

// dispatchTaskDirect dispatches a single task to its rig.
// In production, this delegates to ms sling. Tests override this variable
// with a stub to avoid spawning real processes.
var dispatchTaskDirect = func(townRoot, beadID, rig string) error {
	cmd := exec.Command("ms", "sling", beadID, rig)
	cmd.Dir = townRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ms sling %s %s: %w\nstderr: %s", beadID, rig, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

var minecartLaunchCmd = &cobra.Command{
	Use:   "launch <minecart-id | epic-id | task-id...>",
	Short: "Launch a staged minecart: transition to open and dispatch Wave 1",
	Long: `Launch a staged minecart by transitioning its status from staged to open
and dispatching Wave 1 tasks.

For staged minecart-id input: transitions directly and dispatches.
For epic/task input: runs stage + launch in one step.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMinecartLaunch,
}

func init() {
	minecartLaunchCmd.Flags().BoolVar(&minecartLaunchForce, "force", false, "Launch even with warnings")
}

// transitionMinecartToOpen transitions a staged minecart to open status.
// If the minecart is staged_ready, it transitions unconditionally.
// If the minecart is staged_warnings and force is true, it transitions.
// If the minecart is staged_warnings and force is false, it returns an error.
// If the minecart is already open or closed, it returns an error.
func transitionMinecartToOpen(minecartID string, force bool) error {
	result, err := bdShow(minecartID)
	if err != nil {
		return fmt.Errorf("cannot resolve minecart %s: %w", minecartID, err)
	}

	status := normalizeMinecartStatus(result.Status)

	switch status {
	case minecartStatusStagedReady:
		// Transition directly to open.
		return bdUpdateStatus(minecartID, minecartStatusOpen)

	case minecartStatusStagedWarnings:
		if !force {
			return fmt.Errorf("minecart %s has warnings, use --force to launch", minecartID)
		}
		return bdUpdateStatus(minecartID, minecartStatusOpen)

	case minecartStatusOpen:
		return fmt.Errorf("minecart %s is already launched", minecartID)

	case minecartStatusClosed:
		return fmt.Errorf("minecart %s is closed", minecartID)

	default:
		return fmt.Errorf("minecart %s has unexpected status %q", minecartID, result.Status)
	}
}

// bdUpdateStatus runs `bd update <id> --status=<status>` against the town beads
// database, since minecarts live at the HQ level.
func bdUpdateStatus(beadID, status string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}
	cmd := exec.Command("bd", "update", beadID, "--status="+status)
	cmd.Dir = townBeads
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --status=%s: %w\noutput: %s", beadID, status, err, out)
	}
	return nil
}

// collectBlockedRigsInDAG returns a map of parked/docked rig names to the
// bead IDs that target them. Only considers slingable nodes. (ms-4owfd.1)
func collectBlockedRigsInDAG(dag *MinecartDAG, townRoot string) map[string][]string {
	blockedRigBeads := make(map[string][]string)
	for _, node := range dag.Nodes {
		if !isSlingableType(node.Type) {
			continue
		}
		if node.Rig == "" {
			continue
		}
		if blocked, _ := IsRigParkedOrDocked(townRoot, node.Rig); blocked {
			blockedRigBeads[node.Rig] = append(blockedRigBeads[node.Rig], node.ID)
		}
	}
	return blockedRigBeads
}

// checkBlockedRigsForLaunch checks if any target rigs are parked or docked.
// Returns an error listing all blocked rigs if any are found and force is false.
// (ms-4owfd.1)
func checkBlockedRigsForLaunch(dag *MinecartDAG, townRoot string, force bool) error {
	blockedRigBeads := collectBlockedRigsInDAG(dag, townRoot)
	if len(blockedRigBeads) == 0 {
		return nil
	}

	// Build sorted list of blocked rigs for deterministic output
	var rigs []string
	for rig := range blockedRigBeads {
		rigs = append(rigs, rig)
	}
	sort.Strings(rigs)

	if force {
		// Warn but proceed
		fmt.Printf("Warning: %d non-operational rig(s) in minecart: %s\n", len(rigs), strings.Join(rigs, ", "))
		fmt.Printf("  Proceeding with --force (tasks may fail)\n")
		return nil
	}

	// Build detailed error message
	var details []string
	for _, rig := range rigs {
		beadIDs := blockedRigBeads[rig]
		sort.Strings(beadIDs)
		details = append(details, fmt.Sprintf("  %s: %s", rig, strings.Join(beadIDs, ", ")))
	}

	return fmt.Errorf("cannot launch: %d target rig(s) are parked or docked:\n%s\n\nUse 'ms rig unpark' or 'ms rig undock' to restore, or --force to proceed anyway",
		len(rigs), strings.Join(details, "\n"))
}

// dispatchWave1 dispatches all tasks in Wave 1 of the computed waves.
// Individual task failures do not abort remaining dispatches (I-14).
// Returns a result for every Wave 1 task and a non-nil error only if waves
// are empty or contain no Wave 1.
func dispatchWave1(minecartID string, dag *MinecartDAG, waves []Wave, townRoot string) ([]DispatchResult, error) {
	if len(waves) == 0 {
		return nil, fmt.Errorf("minecart %s: no waves to dispatch", minecartID)
	}

	wave1 := waves[0]
	if wave1.Number != 1 {
		return nil, fmt.Errorf("minecart %s: first wave has unexpected number %d", minecartID, wave1.Number)
	}

	var results []DispatchResult
	for _, taskID := range wave1.Tasks {
		node := dag.Nodes[taskID]
		rig := ""
		if node != nil {
			rig = node.Rig
		}

		err := dispatchTaskDirect(townRoot, taskID, rig)
		results = append(results, DispatchResult{
			BeadID:  taskID,
			Rig:     rig,
			Success: err == nil,
			Error:   err,
		})
	}

	return results, nil
}

// renderLaunchOutput formats the post-launch console output showing minecart ID,
// wave summary, dispatched tasks with status, and helpful hints.
func renderLaunchOutput(minecartID string, waves []Wave, results []DispatchResult, dag *MinecartDAG) string {
	var b strings.Builder

	// Section 1: Minecart ID with status.
	fmt.Fprintf(&b, "Minecart launched: %s (status: open)\n", minecartID)
	b.WriteString("\n")

	// Section 2: Monitor command hint.
	fmt.Fprintf(&b, "  Monitor: ms minecart status %s\n", minecartID)
	b.WriteString("\n")

	// Section 3: Wave summary.
	totalTasks := 0
	for _, w := range waves {
		totalTasks += len(w.Tasks)
	}
	b.WriteString("Wave summary:\n")
	fmt.Fprintf(&b, "  %d waves, %d tasks total\n", len(waves), totalTasks)
	for _, w := range waves {
		status := "pending"
		if w.Number == 1 {
			status = "dispatched"
		}
		fmt.Fprintf(&b, "  Wave %d: %d tasks (%s)\n", w.Number, len(w.Tasks), status)
	}
	b.WriteString("\n")

	// Section 4: Dispatched tasks (Wave 1).
	b.WriteString("Dispatched (Wave 1):\n")

	// Sort results by BeadID for deterministic output.
	sorted := make([]DispatchResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].BeadID < sorted[j].BeadID
	})

	for _, r := range sorted {
		marker := "✓"
		if !r.Success {
			marker = "✗"
		}

		title := ""
		if node := dag.Nodes[r.BeadID]; node != nil {
			title = node.Title
		}

		rigInfo := ""
		if r.Rig != "" {
			rigInfo = fmt.Sprintf("  (rig: %s)", r.Rig)
		}

		errInfo := ""
		if r.Error != nil {
			errInfo = fmt.Sprintf("    error: %v", r.Error)
		}

		fmt.Fprintf(&b, "  %s %s  %s%s%s\n", marker, r.BeadID, title, rigInfo, errInfo)
	}
	b.WriteString("\n")

	// Section 5: TUI hint.
	b.WriteString("  Hint: ms minecart -i for interactive monitoring\n")
	b.WriteString("\n")

	// Section 6: Daemon explanation.
	b.WriteString("Subsequent waves will be dispatched automatically by the daemon as tasks complete.\n")

	return b.String()
}

// runMinecartLaunch is the handler for `ms minecart launch`.
func runMinecartLaunch(cmd *cobra.Command, args []string) error {
	// Step 1: Validate args.
	if err := validateStageArgs(args); err != nil {
		return err
	}

	// Step 2: Resolve bead types via bd show for each arg.
	beadTypes := make(map[string]*bdShowResult)
	for _, arg := range args {
		result, err := bdShow(arg)
		if err != nil {
			return fmt.Errorf("cannot resolve bead %s: %w", arg, err)
		}
		beadTypes[arg] = result
	}

	// Step 3: If single arg is a minecart with staged status, transition to open
	// and dispatch Wave 1.
	if len(args) == 1 {
		result := beadTypes[args[0]]
		if isMinecartIssue(result.IssueType, result.Labels) && isStagedStatus(normalizeMinecartStatus(result.Status)) {
			minecartID := args[0]

			if err := transitionMinecartToOpen(minecartID, minecartLaunchForce); err != nil {
				return err
			}

			// Rebuild DAG from tracked beads and dispatch Wave 1.
			beads, deps, err := collectMinecartBeads(minecartID)
			if err != nil {
				return fmt.Errorf("collect beads for dispatch: %w", err)
			}

			dag := buildMinecartDAG(beads, deps)
			waves, _, err := computeWaves(dag)
			if err != nil {
				return fmt.Errorf("compute waves for dispatch: %w", err)
			}

			townRoot, err := workspace.FindFromCwdOrError()
			if err != nil {
				return fmt.Errorf("resolve town root for dispatch: %w", err)
			}

			// Check for parked/docked rigs before dispatch (ms-4owfd.1, #2120)
			if err := checkBlockedRigsForLaunch(dag, townRoot, minecartLaunchForce); err != nil {
				return err
			}

			results, err := dispatchWave1(minecartID, dag, waves, townRoot)
			if err != nil {
				return fmt.Errorf("dispatch wave 1: %w", err)
			}

			// Report results.
			fmt.Print(renderLaunchOutput(minecartID, waves, results, dag))
			return nil
		}
	}

	// Step 4: For non-minecart or non-staged input, delegate to stage+launch flow.
	// Set the --launch flag on minecartStageCmd and delegate to runMinecartStage.
	minecartStageLaunch = true
	defer func() { minecartStageLaunch = false }()
	return runMinecartStage(cmd, args)
}
