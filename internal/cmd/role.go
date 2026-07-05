package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/style"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// Environment variables for role detection
const (
	EnvGTRole     = "GT_ROLE"
	EnvGTRoleHome = "GT_ROLE_HOME"
)

// RoleInfo contains information about a role and its detection source.
// This is the canonical struct for role detection - used by both GetRole()
// and detectRole() functions.
type RoleInfo struct {
	Role          Role   `json:"role"`
	Source        string `json:"source"` // "env", "cwd", or "explicit"
	Home          string `json:"home"`
	Rig           string `json:"rig,omitempty"`
	Miner       string `json:"miner,omitempty"`
	EnvRole       string `json:"env_role,omitempty"`    // Value of GT_ROLE if set
	CwdRole       Role   `json:"cwd_role,omitempty"`    // Role detected from cwd
	Mismatch      bool   `json:"mismatch,omitempty"`    // True if env != cwd detection
	EnvIncomplete bool   `json:"env_incomplete,omitempty"` // True if env was set but missing rig/miner, filled from cwd
	TownRoot      string `json:"town_root,omitempty"`
	WorkDir       string `json:"work_dir,omitempty"`    // Current working directory
}

var roleCmd = &cobra.Command{
	Use:     "role",
	GroupID: GroupAgents,
	Short:   "Show or manage agent role",
	Long: `Display the current agent role and its detection source.

Role is determined by:
1. GT_ROLE environment variable (authoritative if set)
2. Current working directory (fallback)

If both are available and disagree, a warning is shown.`,
	RunE: runRoleShow,
}

var roleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current role",
	Long: `Show the current agent role, its detection source, and associated metadata.

Displays the role name, whether it was detected from the GT_ROLE environment
variable or the current working directory, and the rig/worker identity if
applicable. Warns if the two detection methods disagree.`,
	RunE: runRoleShow,
}

var roleHomeCmd = &cobra.Command{
	Use:   "home [ROLE]",
	Short: "Show home directory for a role",
	Long: `Show the canonical home directory for a role.

If no role is specified, shows the home for the current role.

Examples:
  gt role home           # Home for current role
  gt role home overseer     # Home for overseer
  gt role home witness   # Home for witness (requires --rig)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRoleHome,
}

var roleDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Force cwd-based role detection (debugging)",
	Long: `Detect role from current working directory, ignoring GT_ROLE env var.

This is useful for debugging role detection issues.`,
	RunE: runRoleDetect,
}

var roleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known roles",
	Long: `List all known Mineshaft agent roles and their descriptions.

Roles include overseer, supervisor, witness, refinery, miner, and crew.
Each role has a specific scope and responsibilities within the
Mineshaft multi-agent architecture.`,
	RunE: runRoleList,
}

var roleEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Print export statements for current role",
	Long: `Print shell export statements for the current role.

Role is determined from GT_ROLE environment variable or current working directory.
This is a read-only command that displays the current role's env vars.

Examples:
  eval $(gt role env)    # Export current role's env vars
  gt role env            # View what would be exported`,
	RunE: runRoleEnv,
}

var roleDefCmd = &cobra.Command{
	Use:   "def <role>",
	Short: "Display role definition (session, health, env config)",
	Long: `Display the effective role definition after all overrides are applied.

Role configuration is layered:
  1. Built-in defaults (embedded in binary)
  2. Town-level overrides (<town>/roles/<role>.toml)
  3. Rig-level overrides (<rig>/roles/<role>.toml)

Examples:
  gt role def witness    # Show witness role definition
  gt role def crew       # Show crew role definition`,
	Args: cobra.ExactArgs(1),
	RunE: runRoleDef,
}

// Flags for role home command
var (
	roleRig     string
	roleMiner string
)

func init() {
	rootCmd.AddCommand(roleCmd)
	roleCmd.AddCommand(roleShowCmd)
	roleCmd.AddCommand(roleHomeCmd)
	roleCmd.AddCommand(roleDetectCmd)
	roleCmd.AddCommand(roleListCmd)
	roleCmd.AddCommand(roleEnvCmd)
	roleCmd.AddCommand(roleDefCmd)

	// Add --rig and --miner flags to home command for overrides
	roleHomeCmd.Flags().StringVar(&roleRig, "rig", "", "Rig name (required for rig-specific roles)")
	roleHomeCmd.Flags().StringVar(&roleMiner, "miner", "", "Miner/crew member name")
}

// GetRole returns the current role, checking GT_ROLE first then falling back to cwd.
// This is the canonical function for role detection.
func GetRole() (RoleInfo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return RoleInfo{}, fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return RoleInfo{}, fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return RoleInfo{}, fmt.Errorf("not in a Mineshaft workspace")
	}

	return GetRoleWithContext(cwd, townRoot)
}

// GetRoleWithContext returns role info given explicit cwd and town root.
func GetRoleWithContext(cwd, townRoot string) (RoleInfo, error) {
	info := RoleInfo{
		TownRoot: townRoot,
		WorkDir:  cwd,
	}

	// Check environment variable first
	envRole := os.Getenv(EnvGTRole)
	info.EnvRole = envRole

	// Always detect from cwd for comparison/fallback
	cwdCtx := detectRole(cwd, townRoot)
	info.CwdRole = cwdCtx.Role

	// Determine authoritative role
	if envRole != "" {
		// Parse env role - it might be simple ("overseer") or compound ("mineshaft/witness")
		parsedRole, rig, miner := parseRoleString(envRole)
		info.Role = parsedRole
		info.Rig = rig
		info.Miner = miner
		info.Source = "env"

		// For simple role strings like "crew" or "miner", also check
		// GT_RIG and GT_CREW/GT_MINER env vars for the full identity
		if info.Rig == "" {
			if envRig := os.Getenv("GT_RIG"); envRig != "" {
				info.Rig = envRig
			}
		}
		if info.Miner == "" {
			if envCrew := os.Getenv("GT_CREW"); envCrew != "" {
				info.Miner = envCrew
			} else if envMiner := os.Getenv("GT_MINER"); envMiner != "" {
				info.Miner = envMiner
			}
		}

		// If env is incomplete (missing rig/miner for roles that need them),
		// fill gaps from cwd detection and mark as incomplete
		needsRig := parsedRole == RoleWitness || parsedRole == RoleRefinery || parsedRole == RoleMiner || parsedRole == RoleCrew
		needsMiner := parsedRole == RoleMiner || parsedRole == RoleCrew || parsedRole == RoleDog

		if needsRig && info.Rig == "" && cwdCtx.Rig != "" {
			info.Rig = cwdCtx.Rig
			info.EnvIncomplete = true
		}
		if needsMiner && info.Miner == "" && cwdCtx.Miner != "" {
			info.Miner = cwdCtx.Miner
			info.EnvIncomplete = true
		}

		// Check for mismatch with cwd detection
		if cwdCtx.Role != RoleUnknown && cwdCtx.Role != parsedRole {
			info.Mismatch = true
		}
	} else {
		// Fall back to cwd detection - copy all fields from cwdCtx
		info.Role = cwdCtx.Role
		info.Rig = cwdCtx.Rig
		info.Miner = cwdCtx.Miner
		info.Source = "cwd"
	}

	// Determine home directory
	info.Home = getRoleHome(info.Role, info.Rig, info.Miner, townRoot)

	return info, nil
}

// detectRole detects the agent role from the current working directory path.
// This is the cwd-based fallback used by GetRoleWithContext when GT_ROLE is not set.
func detectRole(cwd, townRoot string) RoleInfo {
	ctx := RoleInfo{
		Role:     RoleUnknown,
		TownRoot: townRoot,
		WorkDir:  cwd,
		Source:   "cwd",
	}

	// Get relative path from town root
	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return ctx
	}

	// Normalize and split path
	relPath = filepath.ToSlash(relPath)
	parts := strings.Split(relPath, "/")

	// Town root is a neutral location — don't infer any role from it.
	// The overseer's actual home is overseer/ (matched below).
	if relPath == "." || relPath == "" {
		return ctx
	}

	// Check for overseer role: overseer/ or overseer/rig/
	if len(parts) >= 1 && parts[0] == "overseer" {
		ctx.Role = RoleOverseer
		return ctx
	}

	// Check for boot role: supervisor/dogs/boot/
	// Must check before supervisor since boot is under supervisor directory
	if len(parts) >= 3 && parts[0] == "supervisor" && parts[1] == "dogs" && parts[2] == "boot" {
		ctx.Role = RoleBoot
		return ctx
	}

	// Check for dog role: supervisor/dogs/<name>/
	// Must check before supervisor since dogs are under supervisor directory
	if len(parts) >= 3 && parts[0] == "supervisor" && parts[1] == "dogs" {
		ctx.Role = RoleDog
		ctx.Miner = parts[2] // dog name stored in Miner field
		return ctx
	}

	// Check for supervisor role: supervisor/
	if len(parts) >= 1 && parts[0] == "supervisor" {
		ctx.Role = RoleSupervisor
		return ctx
	}

	// At this point, first part should be a rig name
	if len(parts) < 1 {
		return ctx
	}
	rigName := parts[0]
	ctx.Rig = rigName

	// Check for overseer: <rig>/overseer/ or <rig>/overseer/rig/
	if len(parts) >= 2 && parts[1] == "overseer" {
		ctx.Role = RoleOverseer
		return ctx
	}

	// Check for witness: <rig>/witness/rig/
	if len(parts) >= 2 && parts[1] == "witness" {
		ctx.Role = RoleWitness
		return ctx
	}

	// Check for refinery: <rig>/refinery/rig/
	if len(parts) >= 2 && parts[1] == "refinery" {
		ctx.Role = RoleRefinery
		return ctx
	}

	// Check for miner: <rig>/miners/<name>/
	if len(parts) >= 3 && parts[1] == "miners" {
		ctx.Role = RoleMiner
		ctx.Miner = parts[2]
		return ctx
	}

	// Check for crew: <rig>/crew/<name>/
	if len(parts) >= 3 && parts[1] == "crew" {
		ctx.Role = RoleCrew
		ctx.Miner = parts[2] // Use Miner field for crew member name
		return ctx
	}

	// Default: could be rig root - treat as unknown
	return ctx
}

// parseRoleString parses a role string like "overseer", "mineshaft/witness", or "mineshaft/miners/alpha".
func parseRoleString(s string) (Role, string, string) {
	s = strings.TrimSpace(s)

	// Normalize consecutive slashes (e.g. "gamestore//refinery" → "gamestore/refinery")
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	s = strings.TrimSuffix(s, "/")

	// Simple roles
	switch s {
	case constants.RoleOverseer:
		return RoleOverseer, "", ""
	case constants.RoleSupervisor:
		return RoleSupervisor, "", ""
	case "boot":
		return RoleBoot, "", ""
	case "dog":
		return RoleDog, "", ""
	}

	// Compound roles: rig/role or rig/miners/name or rig/crew/name
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		// Unknown format, try to match as simple role
		return Role(s), "", ""
	}

	rig := parts[0]

	switch parts[1] {
	case "boot":
		// Handle compound "supervisor/boot" format from GT_ROLE env var
		if rig == "supervisor" && len(parts) == 2 {
			return RoleBoot, "", ""
		}
		return Role(s), "", ""
	case constants.RoleWitness:
		return RoleWitness, rig, ""
	case constants.RoleRefinery:
		return RoleRefinery, rig, ""
	case "miners":
		if len(parts) >= 3 {
			return RoleMiner, rig, parts[2]
		}
		return RoleMiner, rig, ""
	case constants.RoleCrew:
		if len(parts) >= 3 {
			return RoleCrew, rig, parts[2]
		}
		return RoleCrew, rig, ""
	default:
		// Might be rig/minerName format
		return RoleMiner, rig, parts[1]
	}
}

// ActorString returns the actor identity string for beads attribution.
// Format matches beads created_by convention:
//   - Simple roles: "overseer", "supervisor"
//   - Dog roles: "supervisor-boot" (hyphenated, matching BD_ACTOR)
//   - Rig-specific: "mineshaft/witness", "mineshaft/refinery"
//   - Workers: "mineshaft/crew/max", "mineshaft/miners/Toast"
func (info RoleInfo) ActorString() string {
	switch info.Role {
	case RoleOverseer:
		return "overseer"
	case RoleSupervisor:
		return "supervisor"
	case RoleWitness:
		if info.Rig != "" {
			return fmt.Sprintf("%s/witness", info.Rig)
		}
		return "witness"
	case RoleRefinery:
		if info.Rig != "" {
			return fmt.Sprintf("%s/refinery", info.Rig)
		}
		return "refinery"
	case RoleMiner:
		if info.Rig != "" && info.Miner != "" {
			return fmt.Sprintf("%s/miners/%s", info.Rig, info.Miner)
		}
		return "miner"
	case RoleCrew:
		if info.Rig != "" && info.Miner != "" {
			return fmt.Sprintf("%s/crew/%s", info.Rig, info.Miner)
		}
		return "crew"
	case RoleBoot:
		return "supervisor-boot"
	default:
		return string(info.Role)
	}
}

// getRoleHome returns the canonical home directory for a role.
func getRoleHome(role Role, rig, miner, townRoot string) string {
	switch role {
	case RoleOverseer:
		return filepath.Join(townRoot, "overseer")
	case RoleSupervisor:
		return filepath.Join(townRoot, "supervisor")
	case RoleWitness:
		if rig == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "witness")
	case RoleRefinery:
		if rig == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "refinery", "rig")
	case RoleMiner:
		if rig == "" || miner == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "miners", miner)
	case RoleCrew:
		if rig == "" || miner == "" {
			return ""
		}
		return filepath.Join(townRoot, rig, "crew", miner)
	case RoleBoot:
		return filepath.Join(townRoot, "supervisor", "dogs", "boot")
	case RoleDog:
		if miner == "" {
			return ""
		}
		return filepath.Join(townRoot, "supervisor", "dogs", miner)
	default:
		return ""
	}
}

func runRoleShow(cmd *cobra.Command, args []string) error {
	info, err := GetRole()
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("%s\n", style.Bold.Render(string(info.Role)))
	fmt.Printf("Source: %s\n", info.Source)

	if info.Home != "" {
		fmt.Printf("Home: %s\n", info.Home)
	}

	if info.Rig != "" {
		fmt.Printf("Rig: %s\n", info.Rig)
	}

	if info.Miner != "" {
		fmt.Printf("Worker: %s\n", info.Miner)
	}

	// Show mismatch warning
	if info.Mismatch {
		fmt.Println()
		fmt.Printf("%s\n", style.Bold.Render("⚠️  ROLE MISMATCH"))
		fmt.Printf("  GT_ROLE=%s (authoritative)\n", info.EnvRole)
		fmt.Printf("  cwd suggests: %s\n", info.CwdRole)
		fmt.Println()
		fmt.Println("The GT_ROLE env var takes precedence, but you may be in the wrong directory.")
		fmt.Printf("Expected home: %s\n", info.Home)
	}

	return nil
}

func runRoleHome(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Mineshaft workspace")
	}

	// Validate flag combinations: --miner requires --rig to prevent strange merges
	if roleMiner != "" && roleRig == "" {
		return fmt.Errorf("--miner requires --rig to be specified")
	}

	// Start with current role detection (from env vars or cwd)
	info, err := GetRole()
	if err != nil {
		return err
	}
	role := info.Role
	rig := info.Rig
	miner := info.Miner

	// Apply overrides from arguments/flags
	if len(args) > 0 {
		role, _, _ = parseRoleString(args[0])
	}
	if roleRig != "" {
		rig = roleRig
	}
	if roleMiner != "" {
		miner = roleMiner
	}

	home := getRoleHome(role, rig, miner, townRoot)
	if home == "" {
		return fmt.Errorf("cannot determine home for role %s (rig=%q, miner=%q)", role, rig, miner)
	}

	// Warn if computed home doesn't match cwd
	if home != cwd && !strings.HasPrefix(cwd, home) {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: cwd (%s) is not within role home (%s)\n", cwd, home)
	}

	fmt.Println(home)
	return nil
}

func runRoleDetect(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Mineshaft workspace")
	}

	ctx := detectRole(cwd, townRoot)

	fmt.Printf("%s (from cwd)\n", style.Bold.Render(string(ctx.Role)))
	fmt.Printf("Directory: %s\n", cwd)

	if ctx.Rig != "" {
		fmt.Printf("Rig: %s\n", ctx.Rig)
	}
	if ctx.Miner != "" {
		fmt.Printf("Worker: %s\n", ctx.Miner)
	}

	// Check if env var disagrees
	envRole := os.Getenv(EnvGTRole)
	if envRole != "" {
		parsedRole, _, _ := parseRoleString(envRole)
		if parsedRole != ctx.Role {
			fmt.Println()
			fmt.Printf("%s\n", style.Bold.Render("⚠️  Mismatch with $GT_ROLE"))
			fmt.Printf("  $GT_ROLE=%s\n", envRole)
			fmt.Println("  The env var takes precedence in normal operation.")
		}
	}

	return nil
}

func runRoleList(cmd *cobra.Command, args []string) error {
	roles := []struct {
		name Role
		desc string
	}{
		{RoleOverseer, "Global coordinator at overseer/"},
		{RoleSupervisor, "Background supervisor daemon"},
		{RoleWitness, "Per-rig miner lifecycle manager"},
		{RoleRefinery, "Per-rig merge queue processor"},
		{RoleMiner, "Worker with persistent identity, ephemeral sessions"},
		{RoleCrew, "Persistent worker with own worktree"},
	}

	fmt.Println("Available roles:")
	fmt.Println()
	for _, r := range roles {
		fmt.Printf("  %-10s  %s\n", style.Bold.Render(string(r.name)), r.desc)
	}
	return nil
}

func runRoleEnv(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Mineshaft workspace")
	}

	// Get current role (read-only - from env vars or cwd)
	info, err := GetRole()
	if err != nil {
		return err
	}

	home := getRoleHome(info.Role, info.Rig, info.Miner, townRoot)
	if home == "" {
		return fmt.Errorf("cannot determine home for role %s (rig=%q, miner=%q)", info.Role, info.Rig, info.Miner)
	}

	// Warn if env was incomplete and we filled from cwd
	if info.EnvIncomplete {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: env vars incomplete, filled from cwd\n")
	}

	// Warn if computed home doesn't match cwd
	if home != cwd && !strings.HasPrefix(cwd, home) {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: cwd (%s) is not within role home (%s)\n", cwd, home)
	}

	// Get canonical env vars from shared source of truth
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:      string(info.Role),
		Rig:       info.Rig,
		AgentName: info.Miner,
		TownRoot:  townRoot,
	})
	envVars[EnvGTRoleHome] = home

	// Output in sorted order for consistent output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if runtime.GOOS == "windows" {
			fmt.Printf("$env:%s=%s\n", k, envVars[k])
		} else {
			fmt.Printf("export %s=%s\n", k, envVars[k])
		}
	}

	return nil
}

func runRoleDef(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	// Validate role name
	validRoles := config.AllRoles()
	isValid := false
	for _, r := range validRoles {
		if r == roleName {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("unknown role %q - valid roles: %s", roleName, strings.Join(validRoles, ", "))
	}

	// Determine town root and rig path
	townRoot, _ := workspace.FindFromCwd()
	rigPath := ""
	if townRoot != "" {
		// Try to get rig path if we're in a rig directory
		if rigInfo, err := GetRole(); err == nil && rigInfo.Rig != "" {
			rigPath = filepath.Join(townRoot, rigInfo.Rig)
		}
	}

	// Load role definition with overrides
	def, err := config.LoadRoleDefinition(townRoot, rigPath, roleName)
	if err != nil {
		return fmt.Errorf("loading role definition: %w", err)
	}

	// Display role info
	fmt.Printf("%s %s\n", style.Bold.Render("Role:"), def.Role)
	fmt.Printf("%s %s\n", style.Bold.Render("Scope:"), def.Scope)
	fmt.Println()

	// Session config
	fmt.Println(style.Bold.Render("[session]"))
	fmt.Printf("  pattern        = %q\n", def.Session.Pattern)
	fmt.Printf("  work_dir       = %q\n", def.Session.WorkDir)
	fmt.Printf("  needs_pre_sync = %v\n", def.Session.NeedsPreSync)
	if def.Session.StartCommand != "" {
		fmt.Printf("  start_command  = %q\n", def.Session.StartCommand)
	}
	fmt.Println()

	// Environment variables
	if len(def.Env) > 0 {
		fmt.Println(style.Bold.Render("[env]"))
		envKeys := make([]string, 0, len(def.Env))
		for k := range def.Env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			fmt.Printf("  %s = %q\n", k, def.Env[k])
		}
		fmt.Println()
	}

	// Health config
	fmt.Println(style.Bold.Render("[health]"))
	fmt.Printf("  ping_timeout         = %q\n", def.Health.PingTimeout.String())
	fmt.Printf("  consecutive_failures = %d\n", def.Health.ConsecutiveFailures)
	fmt.Printf("  kill_cooldown        = %q\n", def.Health.KillCooldown.String())
	fmt.Printf("  stuck_threshold      = %q\n", def.Health.StuckThreshold.String())
	if def.Health.HungSessionThreshold.Duration != 0 {
		fmt.Printf("  hung_session_threshold = %q\n", def.Health.HungSessionThreshold.String())
	}
	fmt.Println()

	// Prompts
	if def.Nudge != "" {
		fmt.Printf("%s %s\n", style.Bold.Render("Nudge:"), def.Nudge)
	}
	if def.PromptTemplate != "" {
		fmt.Printf("%s %s\n", style.Bold.Render("Template:"), def.PromptTemplate)
	}

	return nil
}
