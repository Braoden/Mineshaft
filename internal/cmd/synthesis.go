package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/excavation/internal/beads"
	"github.com/steveyegge/excavation/internal/formula"
	"github.com/steveyegge/excavation/internal/runtime"
	"github.com/steveyegge/excavation/internal/style"
	"github.com/steveyegge/excavation/internal/workspace"
)

// Synthesis command flags
var (
	synthesisRig      string
	synthesisDryRun   bool
	synthesisForce    bool
	synthesisReviewID string
)

var synthesisCmd = &cobra.Command{
	Use:     "synthesis",
	Aliases: []string{"synth"},
	GroupID: GroupWork,
	Short:   "Manage minecart synthesis steps",
	RunE:    requireSubcommand,
	Long: `Manage synthesis steps for minecart formulas.

Synthesis is the final step in a minecart workflow that combines outputs
from all parallel legs into a unified deliverable.

Commands:
  start     Start synthesis for a minecart (checks all legs complete)
  status    Show synthesis readiness and leg outputs
  close     Close minecart after synthesis complete

Examples:
  gt synthesis status hq-cv-abc     # Check if ready for synthesis
  gt synthesis start hq-cv-abc      # Start synthesis step
  gt synthesis close hq-cv-abc      # Close minecart after synthesis`,
}

var synthesisStartCmd = &cobra.Command{
	Use:   "start <minecart-id>",
	Short: "Start synthesis for a minecart",
	Long: `Start the synthesis step for a minecart.

This command:
  1. Verifies all legs are complete
  2. Collects outputs from all legs
  3. Creates a synthesis bead with combined context
  4. Slings the synthesis to a miner

Options:
  --rig=NAME      Target rig for synthesis miner (default: current)
  --review-id=ID  Override review ID for output paths
  --force         Start synthesis even if some legs incomplete
  --dry-run       Show what would happen without executing`,
	Args: cobra.ExactArgs(1),
	RunE: runSynthesisStart,
}

var synthesisStatusCmd = &cobra.Command{
	Use:   "status <minecart-id>",
	Short: "Show synthesis readiness",
	Long: `Show whether a minecart is ready for synthesis.

Displays:
  - Minecart metadata
  - Leg completion status
  - Available leg outputs
  - Formula synthesis configuration`,
	Args: cobra.ExactArgs(1),
	RunE: runSynthesisStatus,
}

var synthesisCloseCmd = &cobra.Command{
	Use:   "close <minecart-id>",
	Short: "Close minecart after synthesis",
	Long: `Close a minecart after synthesis is complete.

This marks the minecart as complete and triggers any configured notifications.`,
	Args: cobra.ExactArgs(1),
	RunE: runSynthesisClose,
}

func init() {
	// Start flags
	synthesisStartCmd.Flags().StringVar(&synthesisRig, "rig", "", "Target rig for synthesis miner")
	synthesisStartCmd.Flags().BoolVar(&synthesisDryRun, "dry-run", false, "Preview execution")
	synthesisStartCmd.Flags().BoolVar(&synthesisForce, "force", false, "Start even if legs incomplete")
	synthesisStartCmd.Flags().StringVar(&synthesisReviewID, "review-id", "", "Override review ID")

	// Add subcommands
	synthesisCmd.AddCommand(synthesisStartCmd)
	synthesisCmd.AddCommand(synthesisStatusCmd)
	synthesisCmd.AddCommand(synthesisCloseCmd)

	rootCmd.AddCommand(synthesisCmd)
}

// LegOutput represents collected output from a minecart leg.
type LegOutput struct {
	LegID    string `json:"leg_id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	FilePath string `json:"file_path,omitempty"`
	Content  string `json:"content,omitempty"`
	HasFile  bool   `json:"has_file"`
}

// MinecartMeta holds metadata about a minecart including its formula.
type MinecartMeta struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Formula     string   `json:"formula,omitempty"`      // Formula name
	FormulaPath string   `json:"formula_path,omitempty"` // Path to formula file
	ReviewID    string   `json:"review_id,omitempty"`    // Review ID for output paths
	LegIssues   []string `json:"leg_issues,omitempty"`   // Tracked leg issue IDs
}

// runSynthesisStart implements gt synthesis start.
func runSynthesisStart(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	// Get minecart metadata
	meta, err := getMinecartMeta(minecartID)
	if err != nil {
		return fmt.Errorf("getting minecart metadata: %w", err)
	}

	fmt.Printf("%s Checking synthesis readiness for %s...\n", style.Bold.Render("🔬"), minecartID)

	// Load formula if specified
	var f *formula.Formula
	if meta.FormulaPath != "" {
		f, err = formula.ParseFile(meta.FormulaPath)
		if err != nil {
			return fmt.Errorf("loading formula: %w", err)
		}
	} else if meta.Formula != "" {
		// Try to find formula by name
		formulaPath, findErr := findFormula(meta.Formula)
		if findErr == nil {
			f, err = formula.ParseFile(formulaPath)
			if err != nil {
				return fmt.Errorf("loading formula: %w", err)
			}
		}
	}

	// Check leg completion status
	legOutputs, allComplete, err := collectLegOutputs(meta, f)
	if err != nil {
		return fmt.Errorf("collecting leg outputs: %w", err)
	}

	// Report status
	completedCount := 0
	for _, leg := range legOutputs {
		if leg.Status == "closed" {
			completedCount++
		}
	}
	fmt.Printf("  Legs: %d/%d complete\n", completedCount, len(legOutputs))

	if !allComplete && !synthesisForce {
		fmt.Printf("\n%s Not all legs complete. Use --force to proceed anyway.\n",
			style.Warning.Render("⚠"))
		fmt.Printf("\nIncomplete legs:\n")
		for _, leg := range legOutputs {
			if leg.Status != "closed" {
				fmt.Printf("  ○ %s: %s [%s]\n", leg.LegID, leg.Title, leg.Status)
			}
		}
		return nil
	}

	// Determine review ID
	reviewID := synthesisReviewID
	if reviewID == "" {
		reviewID = meta.ReviewID
	}
	if reviewID == "" {
		// Extract from minecart ID
		reviewID = strings.TrimPrefix(minecartID, "hq-cv-")
	}

	// Determine target rig
	targetRig := synthesisRig
	if targetRig == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err == nil {
			rigName, _, rigErr := findCurrentRig(townRoot)
			if rigErr == nil && rigName != "" {
				targetRig = rigName
			}
		}
		if targetRig == "" {
			targetRig = "excavation"
		}
	}

	if synthesisDryRun {
		fmt.Printf("\n%s Would start synthesis:\n", style.Dim.Render("[dry-run]"))
		fmt.Printf("  Minecart:    %s\n", minecartID)
		fmt.Printf("  Review ID: %s\n", reviewID)
		fmt.Printf("  Target:    %s\n", targetRig)
		fmt.Printf("  Legs:      %d outputs collected\n", len(legOutputs))
		if f != nil && f.Synthesis != nil {
			fmt.Printf("  Synthesis: %s\n", f.Synthesis.Title)
		}
		return nil
	}

	// Create synthesis bead
	synthesisID, err := createSynthesisBead(minecartID, meta, f, legOutputs, reviewID)
	if err != nil {
		return fmt.Errorf("creating synthesis bead: %w", err)
	}
	fmt.Printf("%s Created synthesis bead: %s\n", style.Bold.Render("✓"), synthesisID)

	// Sling to target rig
	fmt.Printf("  Slinging to %s...\n", targetRig)
	if err := slingSynthesis(synthesisID, targetRig); err != nil {
		return fmt.Errorf("slinging synthesis: %w", err)
	}

	fmt.Printf("%s Synthesis started\n", style.Bold.Render("✓"))
	fmt.Printf("  Monitor: gt minecart status %s\n", minecartID)

	return nil
}

// runSynthesisStatus implements gt synthesis status.
func runSynthesisStatus(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	meta, err := getMinecartMeta(minecartID)
	if err != nil {
		return fmt.Errorf("getting minecart metadata: %w", err)
	}

	// Load formula if available
	var f *formula.Formula
	if meta.FormulaPath != "" {
		f, _ = formula.ParseFile(meta.FormulaPath)
	} else if meta.Formula != "" {
		if path, err := findFormula(meta.Formula); err == nil {
			f, _ = formula.ParseFile(path)
		}
	}

	// Collect leg outputs
	legOutputs, allComplete, err := collectLegOutputs(meta, f)
	if err != nil {
		return fmt.Errorf("collecting leg outputs: %w", err)
	}

	// Display status
	fmt.Printf("🚚 %s %s\n\n", style.Bold.Render(minecartID+":"), meta.Title)
	fmt.Printf("  Status: %s\n", formatMinecartStatus(meta.Status))

	if meta.Formula != "" {
		fmt.Printf("  Formula: %s\n", meta.Formula)
	}

	fmt.Printf("\n  %s\n", style.Bold.Render("Legs:"))
	for _, leg := range legOutputs {
		status := "○"
		if leg.Status == "closed" {
			status = "✓"
		}
		fileStatus := ""
		if leg.HasFile {
			fileStatus = style.Dim.Render(" (output: ✓)")
		}
		fmt.Printf("    %s %s: %s [%s]%s\n", status, leg.LegID, leg.Title, leg.Status, fileStatus)
	}

	// Synthesis readiness
	fmt.Printf("\n  %s\n", style.Bold.Render("Synthesis:"))
	if allComplete {
		fmt.Printf("    %s Ready - all legs complete\n", style.Success.Render("✓"))
		fmt.Printf("    Run: gt synthesis start %s\n", minecartID)
	} else {
		completedCount := 0
		for _, leg := range legOutputs {
			if leg.Status == "closed" {
				completedCount++
			}
		}
		fmt.Printf("    %s Waiting - %d/%d legs complete\n",
			style.Warning.Render("○"), completedCount, len(legOutputs))
	}

	if f != nil && f.Synthesis != nil {
		fmt.Printf("\n  %s\n", style.Bold.Render("Synthesis Config:"))
		fmt.Printf("    Title: %s\n", f.Synthesis.Title)
		if f.Output != nil && f.Output.Synthesis != "" {
			fmt.Printf("    Output: %s\n", f.Output.Synthesis)
		}
	}

	return nil
}

// runSynthesisClose implements gt synthesis close.
func runSynthesisClose(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Read minecart to validate lifecycle state before closing
	showArgs := []string{"show", minecartID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townBeads
	var showOut bytes.Buffer
	showCmd.Stdout = &showOut
	if err := showCmd.Run(); err != nil {
		return fmt.Errorf("reading minecart '%s': %w", minecartID, err)
	}
	var minecarts []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(showOut.Bytes(), &minecarts); err != nil || len(minecarts) == 0 {
		return fmt.Errorf("parsing minecart '%s': invalid response", minecartID)
	}
	status := minecarts[0].Status

	if err := ensureKnownMinecartStatus(status); err != nil {
		return fmt.Errorf("minecart '%s' has invalid lifecycle state: %w", minecartID, err)
	}

	// Idempotent: if already closed, just report it
	if normalizeMinecartStatus(status) == minecartStatusClosed {
		fmt.Printf("%s Minecart %s is already closed\n", style.Dim.Render("○"), minecartID)
		return nil
	}

	// Close the minecart
	closeArgs := []string{"close", minecartID, "--reason=synthesis complete"}
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		closeArgs = append(closeArgs, "--session="+sessionID)
	}
	closeCmd := exec.Command("bd", closeArgs...)
	closeCmd.Dir = townBeads
	closeCmd.Stderr = os.Stderr

	if err := closeCmd.Run(); err != nil {
		return fmt.Errorf("closing minecart: %w", err)
	}

	fmt.Printf("%s Minecart closed: %s\n", style.Bold.Render("✓"), minecartID)

	// TODO: Trigger notification if configured
	// Parse description for "Notify: <address>" and send mail

	return nil
}

// getMinecartMeta retrieves minecart metadata from beads.
func getMinecartMeta(minecartID string) (*MinecartMeta, error) {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return nil, err
	}

	showCmd := exec.Command("bd", "show", minecartID, "--json")
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return nil, fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Description string   `json:"description"`
		Type        string   `json:"issue_type"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &minecarts); err != nil {
		return nil, fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 || !isMinecartIssue(minecarts[0].Type, minecarts[0].Labels) {
		return nil, fmt.Errorf("'%s' is not a minecart", minecartID)
	}

	minecart := minecarts[0]

	// Parse formula and review ID from description
	meta := &MinecartMeta{
		ID:     minecart.ID,
		Title:  minecart.Title,
		Status: minecart.Status,
	}

	// Look for structured fields in description
	for _, line := range strings.Split(minecart.Description, "\n") {
		line = strings.TrimSpace(line)
		if colonIdx := strings.Index(line, ":"); colonIdx != -1 {
			key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
			value := strings.TrimSpace(line[colonIdx+1:])
			switch key {
			case "formula":
				meta.Formula = value
			case "formula_path", "formula-path":
				meta.FormulaPath = value
			case "review_id", "review-id":
				meta.ReviewID = value
			}
		}
	}

	// Get tracked leg issues
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		return nil, fmt.Errorf("getting tracked issues for minecart %s: %w", minecartID, err)
	}
	for _, t := range tracked {
		meta.LegIssues = append(meta.LegIssues, t.ID)
	}

	return meta, nil
}

// collectLegOutputs gathers outputs from all minecart legs.
func collectLegOutputs(meta *MinecartMeta, f *formula.Formula) ([]LegOutput, bool, error) { //nolint:unparam // error return kept for future use
	var outputs []LegOutput
	allComplete := true

	// If we have tracked issues, use those as legs
	if len(meta.LegIssues) > 0 {
		for _, issueID := range meta.LegIssues {
			details := getIssueDetails(issueID)
			output := LegOutput{
				LegID: issueID,
				Title: "(unknown)",
			}
			if details != nil {
				output.Title = details.Title
				output.Status = details.Status
			}
			if output.Status != "closed" {
				allComplete = false
			}
			outputs = append(outputs, output)
		}
	}

	// If we have a formula, also try to find output files
	if f != nil && f.Output != nil && meta.ReviewID != "" {
		for _, leg := range f.Legs {
			// Expand output path template
			outputPath := expandOutputPath(f.Output.Directory, f.Output.LegPattern,
				meta.ReviewID, leg.ID)

			// Check if file exists and read content
			if content, err := os.ReadFile(outputPath); err == nil {
				// Find or create leg output entry
				found := false
				for i := range outputs {
					if outputs[i].LegID == leg.ID {
						outputs[i].FilePath = outputPath
						outputs[i].Content = string(content)
						outputs[i].HasFile = true
						found = true
						break
					}
				}
				if !found {
					outputs = append(outputs, LegOutput{
						LegID:    leg.ID,
						Title:    leg.Title,
						Status:   "closed", // If file exists, assume complete
						FilePath: outputPath,
						Content:  string(content),
						HasFile:  true,
					})
				}
			}
		}
	}

	return outputs, allComplete, nil
}

// expandOutputPath expands template variables in output paths.
// Supports Go template output syntax plus legacy bare placeholders.
func expandOutputPath(directory, pattern, reviewID, legID string) string {
	dir := expandOutputTemplate(directory, reviewID, legID)
	file := expandOutputTemplate(pattern, reviewID, legID)
	return filepath.Join(dir, file)
}

func expandOutputTemplate(tmplText, reviewID, legID string) string {
	ctx := map[string]interface{}{
		"review_id": reviewID,
		"leg": map[string]interface{}{
			"id": legID,
		},
	}
	if rendered, err := renderTemplate(tmplText, ctx); err == nil {
		return rendered
	}

	text := strings.ReplaceAll(tmplText, "{{review_id}}", reviewID)
	return strings.ReplaceAll(text, "{{leg.id}}", legID)
}

// createSynthesisBead creates a bead for the synthesis step.
func createSynthesisBead(minecartID string, meta *MinecartMeta, f *formula.Formula,
	legOutputs []LegOutput, reviewID string) (string, error) {

	// Build synthesis title
	title := "Synthesis: " + meta.Title
	if f != nil && f.Synthesis != nil && f.Synthesis.Title != "" {
		title = f.Synthesis.Title + ": " + meta.Title
	}

	// Build synthesis description with leg outputs
	var desc strings.Builder
	desc.WriteString(fmt.Sprintf("minecart: %s\n", minecartID))
	desc.WriteString(fmt.Sprintf("review_id: %s\n", reviewID))
	desc.WriteString("\n")

	var outputDir, outputSynthesis string
	if f != nil && f.Output != nil {
		outputDir = expandOutputTemplate(f.Output.Directory, reviewID, "")
		outputSynthesis = f.Output.Synthesis
	}

	// Add synthesis instructions from formula
	if f != nil && f.Synthesis != nil && f.Synthesis.Description != "" {
		formulaName := meta.Formula
		if formulaName == "" {
			formulaName = f.Name
		}
		synCtx := formulaTemplateContext(formulaName, meta.Title, reviewID, 0, "", nil, nil, nil)
		synCtx["problem"] = meta.Title
		addOutputTemplateContext(synCtx, outputDir, outputSynthesis)
		synDesc := renderTemplateOrDefault(f.Synthesis.Description, synCtx, f.Synthesis.Description)

		desc.WriteString("## Instructions\n\n")
		desc.WriteString(synDesc)
		desc.WriteString("\n\n")
	}

	// Add collected leg outputs
	desc.WriteString("## Leg Outputs\n\n")
	for _, leg := range legOutputs {
		desc.WriteString(fmt.Sprintf("### %s: %s\n\n", leg.LegID, leg.Title))
		if leg.Content != "" {
			desc.WriteString(leg.Content)
			desc.WriteString("\n\n")
		} else if leg.FilePath != "" {
			desc.WriteString(fmt.Sprintf("Output file: %s\n\n", leg.FilePath))
		} else {
			desc.WriteString("(no output available)\n\n")
		}
	}

	// Add output path if configured
	if f != nil && f.Output != nil && f.Output.Synthesis != "" {
		outputPath := filepath.Join(outputDir, f.Output.Synthesis)
		desc.WriteString(fmt.Sprintf("\n## Output\n\nWrite synthesis to: %s\n", outputPath))
	}

	// Guard against flag-like synthesis titles (gt-e0kx5)
	if beads.IsFlagLikeTitle(title) {
		return "", fmt.Errorf("refusing to create synthesis bead: title %q looks like a CLI flag", title)
	}

	// Create the bead
	createArgs := []string{
		"create",
		"--type=task",
		"--title=" + title,
		"--description=" + desc.String(),
		"--json",
	}

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return "", err
	}

	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeads
	var stdout bytes.Buffer
	createCmd.Stdout = &stdout
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating synthesis bead: %w", err)
	}

	// Parse created bead ID
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		// Try to extract ID from non-JSON output (bead IDs have format: prefix-id)
		out := strings.TrimSpace(stdout.String())
		if looksLikeIssueID(out) {
			return out, nil
		}
		return "", fmt.Errorf("parsing created bead: %w", err)
	}

	// Add tracking relation: minecart tracks synthesis.
	_ = addTrackingRelationFn(townBeads, minecartID, result.ID) // Non-fatal if this fails

	return result.ID, nil
}

// slingSynthesis slings the synthesis bead to a rig.
func slingSynthesis(beadID, targetRig string) error {
	slingArgs := []string{"sling", beadID, targetRig}
	slingCmd := exec.Command("gt", slingArgs...)
	slingCmd.Stdout = os.Stdout
	slingCmd.Stderr = os.Stderr

	return slingCmd.Run()
}

// findFormula searches for a formula file by name.
func findFormula(name string) (string, error) {
	// Search paths
	searchPaths := []string{
		".beads/formulas",
	}

	// Add home directory formulas
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".beads", "formulas"))
	}

	// Add GT_ROOT formulas if set
	if gtRoot := os.Getenv("GT_ROOT"); gtRoot != "" {
		searchPaths = append(searchPaths, filepath.Join(gtRoot, ".beads", "formulas"))
	}

	// Try each search path
	for _, searchPath := range searchPaths {
		// Try with .formula.toml extension
		path := filepath.Join(searchPath, name+".formula.toml")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		// Try with .formula.json extension
		path = filepath.Join(searchPath, name+".formula.json")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("formula '%s' not found", name)
}

// CheckSynthesisReady checks if a minecart is ready for synthesis.
// Returns true if all tracked legs are complete.
func CheckSynthesisReady(minecartID string) (bool, error) {
	meta, err := getMinecartMeta(minecartID)
	if err != nil {
		return false, err
	}

	_, allComplete, err := collectLegOutputs(meta, nil)
	return allComplete, err
}

// TriggerSynthesisIfReady checks minecart status and starts synthesis if ready.
// This can be called by the witness when a leg completes.
func TriggerSynthesisIfReady(minecartID, targetRig string) error {
	ready, err := CheckSynthesisReady(minecartID)
	if err != nil {
		return err
	}

	if !ready {
		return nil // Not ready yet
	}

	// Synthesis is ready - start it
	fmt.Printf("%s All legs complete, starting synthesis...\n", style.Bold.Render("🔬"))

	meta, err := getMinecartMeta(minecartID)
	if err != nil {
		return err
	}

	// Load formula if available
	var f *formula.Formula
	if meta.FormulaPath != "" {
		f, _ = formula.ParseFile(meta.FormulaPath)
	} else if meta.Formula != "" {
		if path, err := findFormula(meta.Formula); err == nil {
			f, _ = formula.ParseFile(path)
		}
	}

	legOutputs, _, _ := collectLegOutputs(meta, f)
	reviewID := meta.ReviewID
	if reviewID == "" {
		reviewID = strings.TrimPrefix(minecartID, "hq-cv-")
	}

	synthesisID, err := createSynthesisBead(minecartID, meta, f, legOutputs, reviewID)
	if err != nil {
		return fmt.Errorf("creating synthesis bead: %w", err)
	}

	if err := slingSynthesis(synthesisID, targetRig); err != nil {
		return fmt.Errorf("slinging synthesis: %w", err)
	}

	return nil
}
