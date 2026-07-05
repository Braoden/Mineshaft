package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/mail"
	"github.com/steveyegge/mineshaft/internal/style"
	"github.com/steveyegge/mineshaft/internal/witness"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var (
	patrolScanJSON    bool
	patrolScanNotify  bool
	patrolScanRig     string
	patrolScanVerbose bool
)

var patrolScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan miners for zombies, stalls, and completions",
	Long: `Run proactive detection across all miners in a rig.

This command bridges the witness library detection functions to the CLI,
providing a single command for the survey-workers patrol step.

Detections:
  - Zombies: Dead sessions with active agent state, dead agent processes,
    stuck done-intent, closed beads with live sessions
  - Stalls: Agents stuck at startup prompts
  - Completions: Agent bead metadata indicating ms done was called

Actions taken automatically:
  - Zombie restart: Sessions are restarted (not nuked) to preserve worktrees
  - Cleanup wisps: Created for dirty state tracking
  - Completion routing: MR cleanup wisps created, refinery nudged

Use --notify to send mail when zombies with active work are detected.
Long-running scan phases emit progress diagnostics to stderr so JSON stdout
remains machine-readable while operators can see where a slow patrol is stuck.

Examples:
  ms patrol scan                    # Scan current rig
  ms patrol scan --rig mineshaft      # Scan specific rig
  ms patrol scan --json             # Machine-readable output
  ms patrol scan --notify           # Send mail on zombie detection`,
	RunE: runPatrolScan,
}

func init() {
	patrolScanCmd.Flags().BoolVar(&patrolScanJSON, "json", false, "Output as JSON")
	patrolScanCmd.Flags().BoolVar(&patrolScanNotify, "notify", false, "Send mail to witness/overseer when active-work zombies are detected")
	patrolScanCmd.Flags().StringVar(&patrolScanRig, "rig", "", "Rig to scan (default: infer from cwd or MS_RIG)")
	patrolScanCmd.Flags().BoolVarP(&patrolScanVerbose, "verbose", "v", false, "Verbose output")

	patrolCmd.AddCommand(patrolScanCmd)
}

var patrolScanProgressInterval = 10 * time.Second

// PatrolScanOutput is the JSON output format for patrol scan results.
type PatrolScanOutput struct {
	Rig         string                    `json:"rig"`
	Timestamp   string                    `json:"timestamp"`
	Zombies     *PatrolScanZombieOutput   `json:"zombies"`
	Stalls      *PatrolScanStallOutput    `json:"stalls,omitempty"`
	Completions *PatrolScanCompleteOutput `json:"completions,omitempty"`
	Receipts    []witness.PatrolReceipt   `json:"receipts,omitempty"`
}

// PatrolScanZombieOutput holds zombie detection results.
type PatrolScanZombieOutput struct {
	Checked int                    `json:"checked"`
	Found   int                    `json:"found"`
	Zombies []PatrolScanZombieItem `json:"zombies,omitempty"`
	Errors  []string               `json:"errors,omitempty"`
}

// PatrolScanZombieItem is a single zombie detection in scan output.
type PatrolScanZombieItem struct {
	Miner        string `json:"miner"`
	Classification string `json:"classification"`
	AgentState     string `json:"agent_state"`
	HookBead       string `json:"hook_bead,omitempty"`
	CleanupStatus  string `json:"cleanup_status,omitempty"`
	Action         string `json:"action"`
	WasActive      bool   `json:"was_active"`
	Error          string `json:"error,omitempty"`
}

// PatrolScanStallOutput holds stall detection results.
type PatrolScanStallOutput struct {
	Checked int                   `json:"checked"`
	Found   int                   `json:"found"`
	Stalls  []PatrolScanStallItem `json:"stalls,omitempty"`
}

// PatrolScanStallItem is a single stall detection in scan output.
type PatrolScanStallItem struct {
	Miner   string `json:"miner"`
	StallType string `json:"stall_type"`
	Action    string `json:"action"`
	Error     string `json:"error,omitempty"`
}

// PatrolScanCompleteOutput holds completion discovery results.
type PatrolScanCompleteOutput struct {
	Checked   int                      `json:"checked"`
	Found     int                      `json:"found"`
	Completed []PatrolScanCompleteItem `json:"completed,omitempty"`
}

// PatrolScanCompleteItem is a single completion discovery in scan output.
type PatrolScanCompleteItem struct {
	Miner        string `json:"miner"`
	ExitType       string `json:"exit_type"`
	IssueID        string `json:"issue_id,omitempty"`
	MRID           string `json:"mr_id,omitempty"`
	Branch         string `json:"branch,omitempty"`
	Action         string `json:"action"`
	WispCreated    string `json:"wisp_created,omitempty"`
	CompletionTime string `json:"completion_time,omitempty"`
}

func runPatrolScan(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Mineshaft workspace: %w", err)
	}

	// Determine rig name
	rigName := patrolScanRig
	if rigName == "" {
		// Try MS_RIG env, then infer from cwd
		rigName = os.Getenv("MS_RIG")
		if rigName == "" {
			rigName, err = inferRigFromCwd(townRoot)
			if err != nil {
				return fmt.Errorf("could not determine rig: %w\nUse --rig to specify", err)
			}
		}
	}

	bd := witness.DefaultBdCli()
	router := mail.NewRouter(townRoot)
	workDir := townRoot

	timestamp := time.Now().UTC().Format(time.RFC3339)

	// Run all three detection passes.
	// Note: DetectZombieMiners takes a router param but does NOT send mail
	// internally — it only uses the router for workspace context. Notifications
	// are sent exclusively below via --notify, avoiding double-send.
	diagnostics := cmd.ErrOrStderr()
	zombieResult := runPatrolScanPhase(diagnostics, "zombie detection", func() *witness.DetectZombieMinersResult {
		return witness.DetectZombieMiners(bd, workDir, rigName, router)
	})
	stallResult := runPatrolScanPhase(diagnostics, "stall detection", func() *witness.DetectStalledMinersResult {
		return witness.DetectStalledMiners(workDir, rigName)
	})
	completionResult := runPatrolScanPhase(diagnostics, "completion discovery", func() *witness.DiscoverCompletionsResult {
		return witness.DiscoverCompletions(bd, workDir, rigName, router)
	})

	// Build patrol receipts for zombies
	receipts := witness.BuildPatrolReceipts(rigName, zombieResult)

	// Notify when zombies with active work are detected.
	// Always notify the overseer for active-work zombies (dead miners with hooked
	// beads) — this is the primary mechanism for detecting failed work. (GH #3584)
	// Use --notify=false to suppress (e.g., in dry-run/testing contexts).
	if zombieResult != nil {
		activeZombies := countActiveWorkZombies(zombieResult)
		if activeZombies > 0 {
			sendZombieNotification(router, rigName, zombieResult, activeZombies)
		}
	}

	if patrolScanJSON {
		return outputPatrolScanJSON(rigName, timestamp, zombieResult, stallResult, completionResult, receipts)
	}

	return outputPatrolScanHuman(rigName, zombieResult, stallResult, completionResult, receipts)
}

func runPatrolScanPhase[T any](diagnostics io.Writer, name string, fn func() T) T {
	start := time.Now()
	if diagnostics != nil {
		fmt.Fprintf(diagnostics, "ms patrol scan: starting %s\n", name)
	}

	done := make(chan T, 1)
	go func() {
		done <- fn()
	}()

	if patrolScanProgressInterval <= 0 {
		result := <-done
		if diagnostics != nil {
			fmt.Fprintf(diagnostics, "ms patrol scan: finished %s in %s\n", name, formatPatrolScanElapsed(time.Since(start)))
		}
		return result
	}

	ticker := time.NewTicker(patrolScanProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case result := <-done:
			if diagnostics != nil {
				fmt.Fprintf(diagnostics, "ms patrol scan: finished %s in %s\n", name, formatPatrolScanElapsed(time.Since(start)))
			}
			return result
		case <-ticker.C:
			if diagnostics != nil {
				fmt.Fprintf(diagnostics, "ms patrol scan: still running %s after %s\n", name, formatPatrolScanElapsed(time.Since(start)))
			}
		}
	}
}

func formatPatrolScanElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return elapsed.Round(time.Millisecond).String()
	}
	return elapsed.Round(time.Second).String()
}

func countActiveWorkZombies(result *witness.DetectZombieMinersResult) int {
	count := 0
	for _, z := range result.Zombies {
		if z.WasActive {
			count++
		}
	}
	return count
}

func sendZombieNotification(router *mail.Router, rigName string, result *witness.DetectZombieMinersResult, activeCount int) {
	var lines []string
	lines = append(lines, fmt.Sprintf("Patrol scan detected %d zombie(s) with active work in rig %s:", activeCount, rigName))
	lines = append(lines, "")
	for _, z := range result.Zombies {
		if !z.WasActive {
			continue
		}
		line := fmt.Sprintf("- %s: %s (hook=%s, action=%s)",
			z.MinerName, string(z.Classification), z.HookBead, z.Action)
		if z.Error != nil {
			line += fmt.Sprintf(" [error: %v]", z.Error)
		}
		lines = append(lines, line)
	}

	body := strings.Join(lines, "\n")
	subject := fmt.Sprintf("ZOMBIE_DETECTED: %d active-work zombie(s) in %s", activeCount, rigName)

	// Send to witness (best-effort)
	witMsg := &mail.Message{
		From:    fmt.Sprintf("%s/witness", rigName),
		To:      fmt.Sprintf("%s/witness", rigName),
		Subject: subject,
		Body:    body,
	}
	_ = router.Send(witMsg)

	// Also notify the overseer so dead miners don't go unnoticed. (GH #3584)
	// The overseer needs to know so work can be reslung.
	overseerBody := strings.Join(lines, "\n") +
		"\n\nResling instructions:\n" +
		"  ms sling <bead-id> <rig> --create --force"
	overseerMsg := &mail.Message{
		From:    fmt.Sprintf("%s/witness", rigName),
		To:      "overseer/",
		Subject: fmt.Sprintf("MINER_DIED: %d miner(s) died with active work in %s", activeCount, rigName),
		Body:    overseerBody,
	}
	_ = router.Send(overseerMsg)
}

func outputPatrolScanJSON(rigName, timestamp string, zombieResult *witness.DetectZombieMinersResult, stallResult *witness.DetectStalledMinersResult, completionResult *witness.DiscoverCompletionsResult, receipts []witness.PatrolReceipt) error {
	output := PatrolScanOutput{
		Rig:       rigName,
		Timestamp: timestamp,
		Receipts:  receipts,
	}

	// Zombies
	if zombieResult != nil {
		zo := &PatrolScanZombieOutput{
			Checked: zombieResult.Checked,
			Found:   len(zombieResult.Zombies),
		}
		for _, z := range zombieResult.Zombies {
			item := PatrolScanZombieItem{
				Miner:        z.MinerName,
				Classification: string(z.Classification),
				AgentState:     z.AgentState,
				HookBead:       z.HookBead,
				CleanupStatus:  z.CleanupStatus,
				Action:         z.Action,
				WasActive:      z.WasActive,
			}
			if z.Error != nil {
				item.Error = z.Error.Error()
			}
			zo.Zombies = append(zo.Zombies, item)
		}
		for _, e := range zombieResult.Errors {
			zo.Errors = append(zo.Errors, e.Error())
		}
		output.Zombies = zo
	}

	// Stalls
	if stallResult != nil {
		so := &PatrolScanStallOutput{
			Checked: stallResult.Checked,
			Found:   len(stallResult.Stalled),
		}
		for _, s := range stallResult.Stalled {
			item := PatrolScanStallItem{
				Miner:   s.MinerName,
				StallType: s.StallType,
				Action:    s.Action,
			}
			if s.Error != nil {
				item.Error = s.Error.Error()
			}
			so.Stalls = append(so.Stalls, item)
		}
		output.Stalls = so
	}

	// Completions
	if completionResult != nil {
		co := &PatrolScanCompleteOutput{
			Checked: completionResult.Checked,
			Found:   len(completionResult.Discovered),
		}
		for _, d := range completionResult.Discovered {
			item := PatrolScanCompleteItem{
				Miner:        d.MinerName,
				ExitType:       d.ExitType,
				IssueID:        d.IssueID,
				MRID:           d.MRID,
				Branch:         d.Branch,
				Action:         d.Action,
				WispCreated:    d.WispCreated,
				CompletionTime: d.CompletionTime,
			}
			co.Completed = append(co.Completed, item)
		}
		output.Completions = co
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputPatrolScanHuman(rigName string, zombieResult *witness.DetectZombieMinersResult, stallResult *witness.DetectStalledMinersResult, completionResult *witness.DiscoverCompletionsResult, _ []witness.PatrolReceipt) error {
	fmt.Printf("%s Patrol scan: %s\n\n", style.Bold.Render("🔍"), rigName)

	// Zombies
	if zombieResult != nil {
		fmt.Printf("%s Zombie Detection: checked %d miner(s)\n",
			style.Bold.Render("👻"), zombieResult.Checked)

		if len(zombieResult.Zombies) == 0 {
			fmt.Printf("  %s\n", style.Dim.Render("No zombies detected"))
		} else {
			for _, z := range zombieResult.Zombies {
				icon := "⚠"
				if z.WasActive {
					icon = "🚨"
				}
				fmt.Printf("  %s %s: %s\n", icon, z.MinerName, z.Classification)
				fmt.Printf("    State: %s", z.AgentState)
				if z.HookBead != "" {
					fmt.Printf("  Hook: %s", z.HookBead)
				}
				if z.CleanupStatus != "" {
					fmt.Printf("  Cleanup: %s", z.CleanupStatus)
				}
				fmt.Println()
				fmt.Printf("    Action: %s\n", z.Action)
				if z.Error != nil {
					fmt.Printf("    %s\n", style.Dim.Render(fmt.Sprintf("Error: %v", z.Error)))
				}
			}
		}

		if len(zombieResult.Errors) > 0 && patrolScanVerbose {
			fmt.Printf("  Errors: %d\n", len(zombieResult.Errors))
			for _, e := range zombieResult.Errors {
				fmt.Printf("    - %v\n", e)
			}
		}

		if len(zombieResult.MinecartFailures) > 0 {
			fmt.Printf("  Minecart failures: %d\n", len(zombieResult.MinecartFailures))
		}
		fmt.Println()
	}

	// Stalls
	if stallResult != nil && (len(stallResult.Stalled) > 0 || patrolScanVerbose) {
		fmt.Printf("%s Stall Detection: checked %d miner(s)\n",
			style.Bold.Render("⏳"), stallResult.Checked)

		if len(stallResult.Stalled) == 0 {
			fmt.Printf("  %s\n", style.Dim.Render("No stalls detected"))
		} else {
			for _, s := range stallResult.Stalled {
				fmt.Printf("  ⚠ %s: %s → %s\n", s.MinerName, s.StallType, s.Action)
				if s.Error != nil {
					fmt.Printf("    %s\n", style.Dim.Render(fmt.Sprintf("Error: %v", s.Error)))
				}
			}
		}
		fmt.Println()
	}

	// Completions
	if completionResult != nil && (len(completionResult.Discovered) > 0 || patrolScanVerbose) {
		fmt.Printf("%s Completion Discovery: checked %d miner(s)\n",
			style.Bold.Render("✅"), completionResult.Checked)

		if len(completionResult.Discovered) == 0 {
			fmt.Printf("  %s\n", style.Dim.Render("No completions discovered"))
		} else {
			for _, d := range completionResult.Discovered {
				fmt.Printf("  ● %s: exit=%s", d.MinerName, d.ExitType)
				if d.IssueID != "" {
					fmt.Printf("  issue=%s", d.IssueID)
				}
				if d.MRID != "" {
					fmt.Printf("  mr=%s", d.MRID)
				}
				fmt.Println()
				fmt.Printf("    Action: %s\n", d.Action)
			}
		}
		fmt.Println()
	}

	// Summary
	zombieCount := 0
	activeCount := 0
	if zombieResult != nil {
		zombieCount = len(zombieResult.Zombies)
		activeCount = countActiveWorkZombies(zombieResult)
	}
	stallCount := 0
	if stallResult != nil {
		stallCount = len(stallResult.Stalled)
	}
	completionCount := 0
	if completionResult != nil {
		completionCount = len(completionResult.Discovered)
	}

	if zombieCount == 0 && stallCount == 0 && completionCount == 0 {
		fmt.Printf("%s All clear — no issues detected\n", style.Success.Render("✓"))
	} else {
		fmt.Printf("Summary: %d zombie(s) (%d active-work), %d stall(s), %d completion(s)\n",
			zombieCount, activeCount, stallCount, completionCount)
	}

	return nil
}
