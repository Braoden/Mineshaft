package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/estop"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var (
	statusLineSession string
)

var statusLineCmd = &cobra.Command{
	Use:   "status-line",
	Short: "Output status line content for tmux (internal use)",
	Long: `Output formatted status line content for the tmux status bar.

Called internally by the tmux status-right configuration. Displays
the current rig, role, worker name, and active issue. Pass --session
to specify which tmux session to query.`,
	Hidden: true, // Internal command called by tmux
	RunE:   runStatusLine,
}

func init() {
	rootCmd.AddCommand(statusLineCmd)
	statusLineCmd.Flags().StringVar(&statusLineSession, "session", "", "Tmux session name")
}

func runStatusLine(cmd *cobra.Command, args []string) error {
	// Check E-stop first — prepend red indicator if active
	if townRoot, twErr := workspace.FindFromCwd(); twErr == nil {
		showEstop := false
		var info *estop.Info
		if estop.IsActive(townRoot) {
			showEstop = true
			info = estop.Read(townRoot)
		} else {
			// Check per-rig E-stop
			rigEnv := os.Getenv("GT_RIG")
			if rigEnv != "" && estop.IsRigActive(townRoot, rigEnv) {
				showEstop = true
				info = estop.ReadRig(townRoot, rigEnv)
			}
		}
		if showEstop {
			ts := ""
			if info != nil && !info.Timestamp.IsZero() {
				ts = info.Timestamp.Format("15:04")
			}
			fmt.Printf("#[bg=red,fg=white,bold] ESTOP %s #[default] ", ts)
		}
	}

	t := tmux.NewTmux()

	// Get session environment
	var rigName, miner, crew, issue, role string

	if statusLineSession != "" {
		// Non-fatal: missing env vars are handled gracefully below
		rigName, _ = t.GetEnvironment(statusLineSession, "GT_RIG")
		miner, _ = t.GetEnvironment(statusLineSession, "GT_MINER")
		crew, _ = t.GetEnvironment(statusLineSession, "GT_CREW")
		issue, _ = t.GetEnvironment(statusLineSession, "GT_ISSUE")
		role, _ = t.GetEnvironment(statusLineSession, "GT_ROLE")
	} else {
		// Fallback to process environment
		rigName = os.Getenv("GT_RIG")
		miner = os.Getenv("GT_MINER")
		crew = os.Getenv("GT_CREW")
		issue = os.Getenv("GT_ISSUE")
		role = os.Getenv("GT_ROLE")
	}

	// Get session names for comparison
	overseerSession := getOverseerSessionName()
	supervisorSession := getSupervisorSessionName()

	// Determine identity and output based on role
	if role == "overseer" || statusLineSession == overseerSession {
		return runOverseerStatusLine(t)
	}

	// Supervisor status line
	if role == "supervisor" || statusLineSession == supervisorSession {
		return runSupervisorStatusLine(t)
	}

	// Witness status line (session naming: gt-<rig>-witness)
	if role == "witness" || strings.HasSuffix(statusLineSession, "-witness") {
		return runWitnessStatusLine(t, rigName)
	}

	// Refinery status line
	if role == "refinery" || strings.HasSuffix(statusLineSession, "-refinery") {
		return runRefineryStatusLine(rigName)
	}

	// Crew/Miner status line
	return runWorkerStatusLine(miner, crew, issue)
}

// runWorkerStatusLine outputs status for crew or miner sessions.
func runWorkerStatusLine(miner, crew, issue string) error {
	// Determine agent type and identity
	var icon string
	if miner != "" {
		icon = AgentTypeIcons[AgentMiner]
	} else if crew != "" {
		icon = AgentTypeIcons[AgentCrew]
	}

	// Build status parts
	var parts []string
	currentWork := issue
	if currentWork != "" {
		if icon != "" {
			parts = append(parts, fmt.Sprintf("%s %s", icon, currentWork))
		} else {
			parts = append(parts, currentWork)
		}
	} else if icon != "" {
		parts = append(parts, icon)
	}

	// Output
	if len(parts) > 0 {
		fmt.Print(strings.Join(parts, " | ") + " |")
	}

	return nil
}

func runOverseerStatusLine(t *tmux.Tmux) error {
	// Count active sessions by listing tmux sessions
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	// Get town root from overseer pane's working directory
	var townRoot string
	overseerSession := getOverseerSessionName()
	paneDir, err := t.GetPaneWorkDir(overseerSession)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Load registered rigs to validate against
	registeredRigs := make(map[string]bool)
	if townRoot != "" {
		rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
		if rigsConfig, err := config.LoadRigsConfig(rigsConfigPath); err == nil {
			for rigName := range rigsConfig.Rigs {
				registeredRigs[rigName] = true
			}
		}
	}

	// Track per-rig status for LED indicators and sorting
	type rigStatus struct {
		hasWitness  bool
		hasRefinery bool
		opState     string // "OPERATIONAL", "PARKED", or "DOCKED"
	}
	rigStatuses := make(map[string]*rigStatus)

	// Initialize for all registered rigs
	for rigName := range registeredRigs {
		rigStatuses[rigName] = &rigStatus{}
	}

	// Track per-agent-type health (working/zombie counts)
	type agentHealth struct {
		total   int
		working int
	}
	healthByType := map[AgentType]*agentHealth{
		AgentWitness:  {},
		AgentRefinery: {},
	}

	// Track supervisor presence (just icon, no count)
	hasSupervisor := false

	// Single pass: track rig status AND agent health
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}

		// Track rig-level status (witness/refinery presence)
		// Miners are not tracked in tmux - they're a GC concern, not a display concern
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			if rigStatuses[agent.Rig] == nil {
				rigStatuses[agent.Rig] = &rigStatus{}
			}
			switch agent.Type {
			case AgentWitness:
				rigStatuses[agent.Rig].hasWitness = true
			case AgentRefinery:
				rigStatuses[agent.Rig].hasRefinery = true
			}
		}

		// Track agent health (skip Overseer and Crew)
		if health := healthByType[agent.Type]; health != nil {
			health.total++
			// Detect working state via ✻ symbol
			if isSessionWorking(t, s) {
				health.working++
			}
		}

		// Track supervisor presence (just the icon, no count)
		if agent.Type == AgentSupervisor {
			hasSupervisor = true
		}
	}

	// Status-line is a tmux hot path. Do not query beads for dock/park state here;
	// `gt rig list/status` remains the authoritative live status view.
	for _, status := range rigStatuses {
		status.opState = "OPERATIONAL"
	}

	// Build status
	var parts []string

	// Add per-agent-type health in consistent order
	// Format: "1/3 👁️" = 1 working out of 3 total
	// Only show agent types that have sessions
	// Note: Miners excluded - idle state is misleading noise
	// Supervisor gets just an icon (no count) - shown separately below
	agentOrder := []AgentType{AgentWitness, AgentRefinery}
	var agentParts []string
	for _, agentType := range agentOrder {
		health := healthByType[agentType]
		if health.total == 0 {
			continue
		}
		icon := AgentTypeIcons[agentType]
		agentParts = append(agentParts, fmt.Sprintf("%d/%d %s", health.working, health.total, icon))
	}
	if len(agentParts) > 0 {
		parts = append(parts, strings.Join(agentParts, " "))
	}

	// Add supervisor icon if running (just presence, no count)
	if hasSupervisor {
		parts = append(parts, AgentTypeIcons[AgentSupervisor])
	}

	// Build rig status display with LED indicators (see GetRigLED for definitions)

	// Create sortable rig list
	type rigInfo struct {
		name   string
		status *rigStatus
	}
	var rigs []rigInfo
	for rigName, status := range rigStatuses {
		// Skip docked rigs — they're intentionally disabled and don't need display.
		// Reserve 🛑 for error states (crashed agents, unreachable Dolt, etc.).
		if status.opState == "DOCKED" {
			continue
		}
		rigs = append(rigs, rigInfo{name: rigName, status: status})
	}

	// Sort by: 1) running state, 2) operational state, 3) alphabetical
	sort.Slice(rigs, func(i, j int) bool {
		isRunningI := rigs[i].status.hasWitness || rigs[i].status.hasRefinery
		isRunningJ := rigs[j].status.hasWitness || rigs[j].status.hasRefinery

		// Primary sort: running rigs before non-running rigs
		if isRunningI != isRunningJ {
			return isRunningI
		}

		// Secondary sort: operational state (for non-running rigs: OPERATIONAL < PARKED < DOCKED)
		stateOrder := map[string]int{"OPERATIONAL": 0, "PARKED": 1, "DOCKED": 2}
		stateI := stateOrder[rigs[i].status.opState]
		stateJ := stateOrder[rigs[j].status.opState]
		if stateI != stateJ {
			return stateI < stateJ
		}

		// Tertiary sort: alphabetical
		return rigs[i].name < rigs[j].name
	})

	// Build display with group separators
	var rigParts []string
	var lastGroup string
	for _, rig := range rigs {
		isRunning := rig.status.hasWitness || rig.status.hasRefinery
		var currentGroup string
		if isRunning {
			currentGroup = "running"
		} else {
			currentGroup = "idle-" + rig.status.opState
		}

		// Add separator when group changes (running -> non-running, or different opStates within non-running)
		if lastGroup != "" && lastGroup != currentGroup {
			rigParts = append(rigParts, "|")
		}
		lastGroup = currentGroup

		status := rig.status
		led := GetRigLED(status.hasWitness, status.hasRefinery, status.opState)

		// All icons get 1 space, Park gets 2
		space := " "
		if led == "🅿️" {
			space = "  "
		}
		// Abbreviate rig names to beads prefix when >2 rigs
		displayName := rig.name
		if len(rigs) > 2 && townRoot != "" {
			if prefix := config.GetRigPrefix(townRoot, rig.name); prefix != "" {
				displayName = prefix
			}
		}
		rigParts = append(rigParts, led+space+displayName)
	}

	if len(rigParts) > 0 {
		parts = append(parts, strings.Join(rigParts, " "))
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runSupervisorStatusLine outputs status for the supervisor session.
// Shows: active rigs, miner count, hook or mail preview
func runSupervisorStatusLine(t *tmux.Tmux) error {
	// Count active rigs and miners
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	// Get town root from supervisor pane's working directory. Config files only; no beads.
	var townRoot string
	supervisorSession := getSupervisorSessionName()
	paneDir, err := t.GetPaneWorkDir(supervisorSession)
	if err == nil && paneDir != "" {
		townRoot, _ = workspace.Find(paneDir)
	}

	// Load registered rigs to validate against
	registeredRigs := make(map[string]bool)
	if townRoot != "" {
		rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
		if rigsConfig, err := config.LoadRigsConfig(rigsConfigPath); err == nil {
			for rigName := range rigsConfig.Rigs {
				registeredRigs[rigName] = true
			}
		}
	}

	rigs := make(map[string]bool)
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		// Only count registered rigs
		if agent.Rig != "" && registeredRigs[agent.Rig] {
			rigs[agent.Rig] = true
		}
	}
	rigCount := len(rigs)

	// Build status
	// Note: Miners excluded - their sessions are ephemeral and idle detection is a GC concern
	var parts []string
	parts = append(parts, fmt.Sprintf("%d rigs", rigCount))

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runWitnessStatusLine outputs status for a witness session.
// Shows: crew count, hook or mail preview
// Note: Miners excluded - their sessions are ephemeral and idle detection is a GC concern
func runWitnessStatusLine(t *tmux.Tmux, rigName string) error {
	if rigName == "" {
		// Try to extract from session name: <prefix>-witness
		if identity, err := session.ParseSessionName(statusLineSession); err == nil && identity.Role == session.RoleWitness {
			rigName = identity.Rig
		}
	}

	// Count crew in this rig (crew are persistent, worth tracking)
	sessions, err := t.ListSessions()
	if err != nil {
		return nil // Silent fail
	}

	crewCount := 0
	for _, s := range sessions {
		agent := categorizeSession(s)
		if agent == nil {
			continue
		}
		if agent.Rig == rigName && agent.Type == AgentCrew {
			crewCount++
		}
	}

	// Build status
	var parts []string
	if crewCount > 0 {
		parts = append(parts, fmt.Sprintf("%d crew", crewCount))
	}
	if len(parts) == 0 {
		parts = append(parts, "patrol")
	}

	fmt.Print(strings.Join(parts, " | ") + " |")
	return nil
}

// runRefineryStatusLine outputs status for a refinery session.
// Shows: MQ length, current item, hook or mail preview
func runRefineryStatusLine(rigName string) error {
	if rigName == "" {
		// Try to extract from session name: <prefix>-refinery
		if identity, err := session.ParseSessionName(statusLineSession); err == nil && identity.Role == session.RoleRefinery {
			rigName = identity.Rig
		}
	}

	if rigName == "" {
		fmt.Printf("%s ? |", AgentTypeIcons[AgentRefinery])
		return nil
	}

	fmt.Print("idle |")
	return nil
}

// isSessionWorking detects if a Claude Code session is actively working.
// Returns true if the ✻ symbol is visible in the pane (indicates Claude is processing).
// Returns false for idle sessions (showing ❯ prompt) or if state cannot be determined.
func isSessionWorking(t *tmux.Tmux, session string) bool {
	// Capture last few lines of the pane
	lines, err := t.CapturePaneLines(session, 5)
	if err != nil || len(lines) == 0 {
		return false
	}

	// Check all captured lines for the working indicator
	// ✻ appears in Claude's status line when actively processing
	for _, line := range lines {
		if strings.Contains(line, "✻") {
			return true
		}
	}

	return false
}
