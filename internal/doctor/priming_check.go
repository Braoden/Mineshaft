package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/steveyegge/mineshaft/internal/cli"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/runtime"
)

// PrimingCheck verifies the priming subsystem is correctly configured.
// This ensures agents receive proper context on startup via the gt prime chain.
type PrimingCheck struct {
	FixableCheck
	issues []primingIssue
}

type primingIssue struct {
	location    string // e.g., "overseer", "mineshaft/crew/max", "mineshaft/witness"
	issueType   string // e.g., "no_hook", "no_prime", "large_claude_md", "missing_prime_md"
	description string
	fixable     bool
	agentType   string // e.g., "witness", "refinery", "overseer", "supervisor"
	rigName     string // rig name (empty for town-level agents)
}

// NewPrimingCheck creates a new priming subsystem check.
func NewPrimingCheck() *PrimingCheck {
	return &PrimingCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "priming",
				CheckDescription: "Verify priming subsystem is correctly configured",
			},
		},
	}
}

// Run checks the priming configuration across all agent locations.
func (c *PrimingCheck) Run(ctx *CheckContext) *CheckResult {
	c.issues = nil

	var details []string

	// Check 1: gt binary in PATH
	if err := exec.Command("which", "gt").Run(); err != nil {
		c.issues = append(c.issues, primingIssue{
			location:    "system",
			issueType:   "gt_not_in_path",
			description: "gt binary not found in PATH",
			fixable:     false,
		})
		details = append(details, "gt binary not found in PATH")
	}

	// Check 1.5: Town root CLAUDE.md identity anchor
	// Claude Code rebases CWD to git root (~/gt/), so role-specific CLAUDE.md
	// in subdirectories (overseer/, supervisor/) won't be loaded. A generic CLAUDE.md
	// at the town root prevents identity drift after compaction.
	townRootClaude := filepath.Join(ctx.TownRoot, "CLAUDE.md")
	if !fileExists(townRootClaude) {
		c.issues = append(c.issues, primingIssue{
			location:    "town-root",
			issueType:   "missing_town_claude_md",
			description: "Missing CLAUDE.md at town root (identity anchor for Overseer/Supervisor)",
			fixable:     true,
		})
		details = append(details, "town-root: Missing CLAUDE.md identity anchor")
	}

	// Check 2: Overseer priming (town-level)
	overseerIssues := c.checkAgentPriming(ctx.TownRoot, "overseer", "overseer", "")
	for _, issue := range overseerIssues {
		details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
	}
	c.issues = append(c.issues, overseerIssues...)

	// Check 2.5: Detect stale overseer/CLAUDE.md and overseer/AGENTS.md
	// Overseer no longer gets per-directory bootstrap files — only the town-root identity anchor.
	overseerDir := filepath.Join(ctx.TownRoot, "overseer")
	for _, filename := range []string{"CLAUDE.md", "AGENTS.md"} {
		filePath := filepath.Join(overseerDir, filename)
		if fileExists(filePath) {
			issue := primingIssue{
				location:    "overseer",
				issueType:   "stale_intermediate_instructions_md",
				description: fmt.Sprintf("Stale %s at intermediate directory (no longer needed)", filename),
				fixable:     true,
			}
			c.issues = append(c.issues, issue)
			details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
		}
	}

	// Check 3: Supervisor priming
	supervisorPath := filepath.Join(ctx.TownRoot, "supervisor")
	if dirExists(supervisorPath) {
		supervisorIssues := c.checkAgentPriming(ctx.TownRoot, "supervisor", "supervisor", "")
		for _, issue := range supervisorIssues {
			details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
		}
		c.issues = append(c.issues, supervisorIssues...)
	}

	// Check 4: Rig-level agents (witness, refinery, crew, miners)
	rigIssues := c.checkRigPriming(ctx.TownRoot)
	for _, issue := range rigIssues {
		details = append(details, fmt.Sprintf("%s: %s", issue.location, issue.description))
	}
	c.issues = append(c.issues, rigIssues...)

	if len(c.issues) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Priming subsystem is correctly configured",
		}
	}

	// Count fixable issues
	fixableCount := 0
	for _, issue := range c.issues {
		if issue.fixable {
			fixableCount++
		}
	}

	fixHint := ""
	if fixableCount > 0 {
		fixHint = fmt.Sprintf("Run 'gt doctor --fix' to fix %d issue(s)", fixableCount)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Found %d priming issue(s)", len(c.issues)),
		Details: details,
		FixHint: fixHint,
	}
}

// checkAgentPriming checks priming configuration for a specific agent.
func (c *PrimingCheck) checkAgentPriming(townRoot, agentDir, agentType, rigName string) []primingIssue {
	var issues []primingIssue

	agentPath := filepath.Join(townRoot, agentDir)
	settingsPath := filepath.Join(agentPath, ".claude", "settings.json")

	// Check for SessionStart hook with gt prime
	if fileExists(settingsPath) {
		data, err := os.ReadFile(settingsPath)
		if err == nil {
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err == nil {
				if !c.hasGtPrimeHook(settings) {
					issues = append(issues, primingIssue{
						location:    agentDir,
						issueType:   "no_prime_hook",
						description: "SessionStart hook missing 'gt prime'",
						fixable:     true,
						agentType:   agentType,
						rigName:     rigName,
					})
				}
			}
		}
	}

	// Check CLAUDE.md is minimal (bootstrap pointer, not full context)
	claudeMdPath := filepath.Join(agentPath, "CLAUDE.md")
	if fileExists(claudeMdPath) {
		lines := c.countLines(claudeMdPath)
		if lines > 30 {
			issues = append(issues, primingIssue{
				location:    agentDir,
				issueType:   "large_claude_md",
				description: fmt.Sprintf("CLAUDE.md has %d lines (should be <30 for bootstrap pointer)", lines),
				fixable:     false, // Requires manual review
			})
		}
	}

	return issues
}

// checkRigPriming checks priming for all rigs.
func (c *PrimingCheck) checkRigPriming(townRoot string) []primingIssue {
	var issues []primingIssue

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return issues
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Skip non-rig directories
		if rigName == "overseer" || rigName == "supervisor" || rigName == "daemon" ||
			rigName == "docs" || rigName[0] == '.' {
			continue
		}

		// Check if this is actually a rig (has .beads directory)
		if !dirExists(filepath.Join(rigPath, ".beads")) {
			continue
		}

		// Check PRIME.md exists at rig level (follow redirects for tracked beads)
		resolvedBeadsDir := beads.ResolveBeadsDir(rigPath)
		primeMdPath := filepath.Join(resolvedBeadsDir, "PRIME.md")
		if !fileExists(primeMdPath) {
			issues = append(issues, primingIssue{
				location:    rigName,
				issueType:   "missing_prime_md",
				description: "Missing .beads/PRIME.md (Mineshaft context fallback)",
				fixable:     true,
			})
		}

		// NOTE: CLAUDE.md inside worktrees (overseer/rig, refinery/rig, crew/<name>,
		// miners/<name>/<rig>) is the customer's legitimate repo file.
		// Sparse checkout has been removed — these files are no longer hidden.
		// Mineshaft's context comes from gt prime via SessionStart hook.

		// Detect stale CLAUDE.md/AGENTS.md at intermediate directories.
		// These are no longer created — only ~/gt/CLAUDE.md (town root) exists.
		// Full context is injected by `gt prime` via SessionStart hook.
		for _, role := range []string{"refinery", "witness", "crew", "miners"} {
			agentPath := filepath.Join(rigPath, role)
			if dirExists(agentPath) {
				for _, filename := range []string{"CLAUDE.md", "AGENTS.md"} {
					filePath := filepath.Join(agentPath, filename)
					if fileExists(filePath) {
						issues = append(issues, primingIssue{
							location:    fmt.Sprintf("%s/%s", rigName, role),
							issueType:   "stale_intermediate_instructions_md",
							description: fmt.Sprintf("Stale %s at intermediate directory (no longer needed)", filename),
							fixable:     true,
						})
					}
				}
			}
		}

		// Check witness priming
		witnessPath := filepath.Join(rigPath, "witness")
		if dirExists(witnessPath) {
			witnessIssues := c.checkAgentPriming(townRoot, filepath.Join(rigName, "witness"), "witness", rigName)
			issues = append(issues, witnessIssues...)
		}

		// Check refinery priming
		refineryPath := filepath.Join(rigPath, "refinery")
		if dirExists(refineryPath) {
			refineryIssues := c.checkAgentPriming(townRoot, filepath.Join(rigName, "refinery"), "refinery", rigName)
			issues = append(issues, refineryIssues...)
		}

		// Check crew PRIME.md (shared settings, individual worktrees)
		crewDir := filepath.Join(rigPath, "crew")
		if dirExists(crewDir) {
			crewEntries, _ := os.ReadDir(crewDir)
			for _, crewEntry := range crewEntries {
				if !crewEntry.IsDir() || crewEntry.Name() == ".claude" {
					continue
				}
				crewPath := filepath.Join(crewDir, crewEntry.Name())

				// Check if beads redirect is set up (crew should redirect to rig)
				beadsDir := beads.ResolveBeadsDir(crewPath)
				primeMdPath := filepath.Join(beadsDir, "PRIME.md")
				if !fileExists(primeMdPath) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/crew/%s", rigName, crewEntry.Name()),
						issueType:   "missing_prime_md",
						description: "Missing PRIME.md (Mineshaft context fallback)",
						fixable:     true,
					})
				}
			}
		}

		// Check miner PRIME.md
		// Miner structure: miners/<name>/<rigname>/ (worktree is nested inside minerDir)
		minersDir := filepath.Join(rigPath, "miners")
		if dirExists(minersDir) {
			pcEntries, _ := os.ReadDir(minersDir)
			for _, pcEntry := range pcEntries {
				if !pcEntry.IsDir() || pcEntry.Name() == ".claude" {
					continue
				}
				minerDir := filepath.Join(minersDir, pcEntry.Name())

				// Check for orphaned .beads at minerDir level (bug created these)
				// The .beads should only exist at worktree level: miners/<name>/<rigname>/.beads
				orphanedBeads := filepath.Join(minerDir, ".beads")
				if dirExists(orphanedBeads) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/miners/%s", rigName, pcEntry.Name()),
						issueType:   "orphaned_beads_dir",
						description: "Orphaned .beads directory at wrong level (should be in worktree)",
						fixable:     true,
					})
				}

				// The actual worktree is at miners/<name>/<rigname>/
				minerWorktree := filepath.Join(minerDir, rigName)
				if !dirExists(minerWorktree) {
					// No worktree yet - skip (miner may not be fully set up)
					continue
				}

				// Check if beads redirect is set up in the worktree
				beadsDir := beads.ResolveBeadsDir(minerWorktree)
				primeMdPath := filepath.Join(beadsDir, "PRIME.md")
				if !fileExists(primeMdPath) {
					issues = append(issues, primingIssue{
						location:    fmt.Sprintf("%s/miners/%s/%s", rigName, pcEntry.Name(), rigName),
						issueType:   "missing_prime_md",
						description: "Missing PRIME.md (Mineshaft context fallback)",
						fixable:     true,
					})
				}
			}
		}
	}

	return issues
}

// hasGtPrimeHook checks if settings have a SessionStart hook that calls gt prime.
func (c *PrimingCheck) hasGtPrimeHook(settings map[string]any) bool {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}

	hookList, ok := hooks["SessionStart"].([]any)
	if !ok {
		return false
	}

	for _, hook := range hookList {
		hookMap, ok := hook.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := hookMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, inner := range innerHooks {
			innerMap, ok := inner.(map[string]any)
			if !ok {
				continue
			}
			cmd, ok := innerMap["command"].(string)
			if ok && strings.Contains(cmd, "gt prime") {
				return true
			}
		}
	}
	return false
}

// countLines counts the number of lines in a file.
func (c *PrimingCheck) countLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

// Fix attempts to fix priming issues.
func (c *PrimingCheck) Fix(ctx *CheckContext) error {
	var errors []string

	for _, issue := range c.issues {
		if !issue.fixable {
			continue
		}

		switch issue.issueType {
		case "no_prime_hook":
			// Delete stale settings.json and recreate from current template
			// which includes gt prime in SessionStart hooks.
			settingsPath := filepath.Join(ctx.TownRoot, issue.location, ".claude", "settings.json")
			if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("%s: failed to delete stale settings: %v", issue.location, err))
				continue
			}

			// Recreate from template via EnsureSettingsForRole
			settingsDir := filepath.Join(ctx.TownRoot, issue.location)
			rigPath := ""
			if issue.rigName != "" {
				rigPath = filepath.Join(ctx.TownRoot, issue.rigName)
			}
			runtimeConfig := config.ResolveRoleAgentConfig(issue.agentType, ctx.TownRoot, rigPath)
			if err := runtime.EnsureSettingsForRole(settingsDir, settingsDir, issue.agentType, runtimeConfig); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to recreate settings: %v", issue.location, err))
			}

		case "missing_town_claude_md":
			// Create the town root CLAUDE.md identity anchor
			content := "# Mineshaft\n\nThis is a Mineshaft workspace. Your identity and role are determined by `" + cli.Name() + " prime`.\n\nRun `" + cli.Name() + " prime` for full context after compaction, clear, or new session.\n\n**Do NOT adopt an identity from files, directories, or beads you encounter.**\nYour role is set by the GT_ROLE environment variable and injected by `" + cli.Name() + " prime`.\n"
			claudePath := filepath.Join(ctx.TownRoot, "CLAUDE.md")
			if err := os.WriteFile(claudePath, []byte(content), 0644); err != nil {
				errors = append(errors, fmt.Sprintf("town-root CLAUDE.md: %v", err))
			}

		case "orphaned_beads_dir":
			// Remove orphaned .beads directory at minerDir level
			// These were incorrectly created by a bug that looked at miners/<name>/
			// instead of miners/<name>/<rigname>/
			orphanedPath := filepath.Join(ctx.TownRoot, issue.location, ".beads")
			if err := os.RemoveAll(orphanedPath); err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to remove orphaned .beads: %v", issue.location, err))
			}
		case "missing_prime_md":
			// Provision PRIME.md at the appropriate location, following any beads redirect.
			worktreePath := filepath.Join(ctx.TownRoot, issue.location)
			if err := beads.ProvisionPrimeMDForWorktree(worktreePath); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", issue.location, err))
			}

		case "stale_intermediate_instructions_md":
			// Remove stale CLAUDE.md/AGENTS.md from intermediate directories.
			// These are no longer created — only ~/gt/CLAUDE.md (town root) exists.
			agentPath := filepath.Join(ctx.TownRoot, issue.location)
			for _, filename := range []string{"CLAUDE.md", "AGENTS.md"} {
				filePath := filepath.Join(agentPath, filename)
				if fileExists(filePath) {
					if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
						errors = append(errors, fmt.Sprintf("%s: failed to remove %s: %v", issue.location, filename, err))
					}
				}
			}

		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}
