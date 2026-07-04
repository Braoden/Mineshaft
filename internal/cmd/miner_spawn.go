// Package cmd provides miner spawning utilities for gt sling.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/excavation/internal/beads"
	"github.com/steveyegge/excavation/internal/config"
	"github.com/steveyegge/excavation/internal/constants"
	"github.com/steveyegge/excavation/internal/events"
	"github.com/steveyegge/excavation/internal/git"
	"github.com/steveyegge/excavation/internal/miner"
	"github.com/steveyegge/excavation/internal/rig"
	"github.com/steveyegge/excavation/internal/style"
	"github.com/steveyegge/excavation/internal/tmux"
	"github.com/steveyegge/excavation/internal/witness"
	"github.com/steveyegge/excavation/internal/workspace"
)

const minMinerDirsPerRig = 30

// SpawnedMinerInfo contains info about a spawned miner session.
type SpawnedMinerInfo struct {
	RigName     string // Rig name (e.g., "excavation")
	MinerName string // Miner name (e.g., "Toast")
	ClonePath   string // Path to miner's git worktree
	SessionName string // Tmux session name (e.g., "gt-excavation-p-Toast")
	Pane        string // Tmux pane ID (empty until StartSession is called)
	BaseBranch  string // Effective base branch (e.g., "main", "integration/epic-id")
	Branch      string // Git branch name (for cleanup on rollback)

	// Internal fields for deferred session start
	account string
	agent   string
}

// AgentID returns the agent identifier (e.g., "excavation/miners/Toast")
func (s *SpawnedMinerInfo) AgentID() string {
	return fmt.Sprintf("%s/miners/%s", s.RigName, s.MinerName)
}

// SessionStarted returns true if the tmux session has been started.
func (s *SpawnedMinerInfo) SessionStarted() bool {
	return s.Pane != ""
}

// SlingSpawnOptions contains options for spawning a miner via sling.
type SlingSpawnOptions struct {
	TownRoot      string // Excavation Site workspace root; falls back to cwd when empty
	Force         bool   // Force spawn even if miner has uncommitted work
	Account       string // Claude Code account handle to use
	Create        bool   // Create miner if it doesn't exist (currently always true for sling)
	HookBead      string // Bead ID to set as hook_bead at spawn time (atomic assignment)
	Agent         string // Agent override for this spawn (e.g., "gemini", "codex", "claude-haiku")
	BaseBranch    string // Override base branch for miner worktree (e.g., "develop", "release/v2")
	ResumeBranch  string // Resume an existing branch (e.g. PR head) instead of creating miner/<name>/<bead>@<ts>
	SkipAdmission bool   // Caller already holds a miner admission reservation
}

func effectiveMinerDirCap(configured int) int {
	if configured < minMinerDirsPerRig {
		return minMinerDirsPerRig
	}
	return configured
}

// SpawnMinerForSling creates a fresh miner and optionally starts its session.
// This is used by gt sling when the target is a rig name.
// The caller (sling) handles hook attachment and nudging.
func SpawnMinerForSling(rigName string, opts SlingSpawnOptions) (*SpawnedMinerInfo, error) {
	// Find workspace
	townRoot := opts.TownRoot
	if townRoot == "" {
		var err error
		townRoot, err = workspace.FindFromCwdOrError()
		if err != nil {
			return nil, fmt.Errorf("not in a Excavation Site workspace: %w", err)
		}
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	// Get miner manager (with tmux for session-aware allocation)
	minerGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	minerMgr := miner.NewManager(r, minerGit, t)

	// Pre-spawn Dolt health check (gt-94llt7): verify Dolt is reachable before
	// allocating a miner. Prevents orphaned miners when Dolt is down.
	if err := minerMgr.CheckDoltHealth(); err != nil {
		return nil, fmt.Errorf("pre-spawn health check failed: %w", err)
	}

	// Pre-spawn admission control (gt-1obzke): verify Dolt server has connection
	// capacity before spawning. Prevents connection storms during mass sling.
	if err := minerMgr.CheckDoltServerCapacity(); err != nil {
		return nil, fmt.Errorf("admission control: %w", err)
	}

	if blocked, reason := IsRigParkedOrDocked(townRoot, rigName); blocked {
		undoCmd := "gt rig unpark"
		if reason == "docked" {
			undoCmd = "gt rig undock"
		}
		return nil, fmt.Errorf("cannot sling to %s rig %q\n%s %s", reason, rigName, undoCmd, rigName)
	}

	var admission *minerAdmissionHandle
	if !opts.SkipAdmission {
		admission, _, err = acquireMinerAdmissionFn(townRoot, rigName, opts.HookBead, "spawn-or-reuse")
		if err != nil {
			return nil, err
		}
		defer admission.Release()
	}

	// Per-bead respawn circuit breaker (clown show #22):
	// Track how many times this bead has been slung. Block after N attempts
	// to prevent witness→supervisor→sling feedback loops.
	if opts.HookBead != "" && !opts.Force {
		if witness.ShouldBlockRespawn(townRoot, opts.HookBead) {
			maxRespawns := config.LoadOperationalConfig(townRoot).GetWitnessConfig().MaxBeadRespawnsV()
			return nil, fmt.Errorf("respawn limit reached for %s (%d attempts). "+
				"This bead keeps failing — investigate before re-dispatching.\n"+
				"Override: gt sling %s %s --force\n"+
				"Reset:    gt sling respawn-reset %s",
				opts.HookBead, maxRespawns,
				opts.HookBead, rigName, opts.HookBead)
		}
		witness.RecordBeadRespawn(townRoot, opts.HookBead)
	}

	// Persistent miner model (gt-4ac): try to reuse an idle miner first.
	// Idle miners have completed their work but kept their sandbox (worktree).
	// Reusing avoids the overhead of creating a new worktree.
	idleMiner, findErr := minerMgr.FindIdleMiner()
	if findErr == nil && idleMiner != nil {
		minerName := idleMiner.Name
		fmt.Printf("Reusing idle miner: %s\n", minerName)

		// ResumeBranch takes precedence over BaseBranch / integration auto-detection:
		// when the user (or scheduler) wants to resume an existing PR branch, we
		// must not start from main or an integration branch.
		baseBranch := opts.BaseBranch
		if opts.ResumeBranch == "" {
			if baseBranch == "" && opts.HookBead != "" {
				settingsPath := filepath.Join(r.Path, "settings", "config.json")
				minerIntegrationEnabled := true
				if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
					minerIntegrationEnabled = settings.MergeQueue.IsMinerIntegrationEnabled()
				}
				if minerIntegrationEnabled {
					repoGit, repoErr := getRigGit(r.Path)
					if repoErr == nil {
						bd := beads.New(r.Path)
						detected, detectErr := beads.DetectIntegrationBranch(bd, repoGit, opts.HookBead)
						if detectErr == nil && detected != "" {
							baseBranch = "origin/" + detected
							fmt.Printf("  Auto-detected integration branch: %s\n", detected)
						}
					}
				}
			}
			if baseBranch != "" && !strings.HasPrefix(baseBranch, "origin/") {
				baseBranch = "origin/" + baseBranch
			}
		}

		// Reuse the idle miner with branch-only operations (no worktree add/remove).
		// Phase 3 of persistent-miner-pool: eliminates ~5s worktree creation overhead.
		// If reuse is unsafe or fails, allocate a new miner instead of repairing
		// this worktree destructively.
		addOpts := miner.AddOptions{
			HookBead:     opts.HookBead,
			BaseBranch:   baseBranch,
			ResumeBranch: opts.ResumeBranch,
		}
		reuseOK := false
		if _, err := minerMgr.ReuseIdleMiner(minerName, addOpts); err != nil {
			if errors.Is(err, miner.ErrMinerNeedsRecovery) {
				fmt.Printf("  Idle miner %s needs recovery before reuse: %v; allocating new...\n", minerName, err)
			} else {
				fmt.Printf("  Branch-only reuse failed for idle miner %s: %v; allocating new...\n", minerName, err)
			}
		} else {
			reuseOK = true
		}

		if reuseOK {
			minerObj, err := minerMgr.Get(minerName)
			if err != nil {
				return nil, fmt.Errorf("getting idle miner after reuse: %w", err)
			}
			if err := verifyWorktreeExists(minerObj.ClonePath); err != nil {
				return nil, fmt.Errorf("worktree verification failed for reused %s: %w", minerName, err)
			}

			minerSessMgr := miner.NewSessionManager(t, r)
			sessionName := minerSessMgr.SessionName(minerName)

			fmt.Printf("%s Miner %s reused (idle → working, session start deferred)\n", style.Bold.Render("✓"), minerName)
			_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, minerName))

			effectiveBranch := strings.TrimPrefix(baseBranch, "origin/")
			if effectiveBranch == "" {
				effectiveBranch = r.DefaultBranch()
			}
			if opts.ResumeBranch != "" {
				effectiveBranch = opts.ResumeBranch
			}

			return &SpawnedMinerInfo{
				RigName:     rigName,
				MinerName: minerName,
				ClonePath:   minerObj.ClonePath,
				SessionName: sessionName,
				Pane:        "",
				BaseBranch:  effectiveBranch,
				Branch:      minerObj.Branch,
				account:     opts.Account,
				agent:       opts.Agent,
			}, nil
		}
	}

	// Per-rig directory cap: prevent unbounded worktree accumulation, but only
	// after trying safe reuse. A reusable preserved miner should not be blocked
	// just because the rig is already at the directory cap.
	maxMinerDirsPerRig := effectiveMinerDirCap(r.GetIntConfig("max_miners"))
	rigMinerDir := filepath.Join(townRoot, rigName, "miners")
	if entries, err := os.ReadDir(rigMinerDir); err == nil {
		dirCount := 0
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirCount++
			}
		}
		if dirCount >= maxMinerDirsPerRig {
			return nil, fmt.Errorf("rig %s has %d miner directories (max %d). "+
				"Resolve recovery-needed miners before allocating more slots: gt miner list %s",
				rigName, dirCount, maxMinerDirsPerRig, rigName)
		}
	}

	// Determine base branch for miner worktree.
	// ResumeBranch (gh#3602) takes precedence: when resuming an existing branch
	// we must not start from main or auto-detect an integration branch.
	baseBranch := opts.BaseBranch
	if opts.ResumeBranch == "" {
		if baseBranch == "" && opts.HookBead != "" {
			// Auto-detect: check if the hooked bead's parent epic has an integration branch
			settingsPath := filepath.Join(r.Path, "settings", "config.json")
			minerIntegrationEnabled := true
			if settings, err := config.LoadRigSettings(settingsPath); err == nil && settings.MergeQueue != nil {
				minerIntegrationEnabled = settings.MergeQueue.IsMinerIntegrationEnabled()
			}
			if minerIntegrationEnabled {
				repoGit, repoErr := getRigGit(r.Path)
				if repoErr == nil {
					bd := beads.New(r.Path)
					detected, detectErr := beads.DetectIntegrationBranch(bd, repoGit, opts.HookBead)
					if detectErr == nil && detected != "" {
						baseBranch = "origin/" + detected
						fmt.Printf("  Auto-detected integration branch: %s\n", detected)
					}
				}
			}
		}
		if baseBranch != "" && !strings.HasPrefix(baseBranch, "origin/") {
			baseBranch = "origin/" + baseBranch
		}
	}

	// Build add options with hook_bead set atomically at spawn time
	addOpts := miner.AddOptions{
		HookBead:     opts.HookBead,
		BaseBranch:   baseBranch,
		ResumeBranch: opts.ResumeBranch,
	}

	// No idle miner available — allocate and create atomically (GH#2215).
	// AllocateAndAdd holds the pool lock through directory creation, preventing
	// concurrent processes from allocating the same name.
	minerName, _, err := minerMgr.AllocateAndAdd(addOpts)
	if err != nil {
		return nil, fmt.Errorf("allocating and creating miner: %w", err)
	}
	fmt.Printf("Created miner: %s\n", minerName)

	// Get miner object for path info
	minerObj, err := minerMgr.Get(minerName)
	if err != nil {
		return nil, fmt.Errorf("getting miner after creation: %w", err)
	}

	// Verify worktree was actually created (fixes #1070)
	// The identity bead may exist but worktree creation can fail silently
	if err := verifyWorktreeExists(minerObj.ClonePath); err != nil {
		// Clean up the partial state before returning error
		_ = minerMgr.Remove(minerName, true) // force=true to clean up partial state
		return nil, fmt.Errorf("worktree verification failed for %s: %w\nHint: try 'gt miner nuke %s/%s --force' to clean up",
			minerName, err, rigName, minerName)
	}

	// Get session manager for session name (session start is deferred)
	minerSessMgr := miner.NewSessionManager(t, r)
	sessionName := minerSessMgr.SessionName(minerName)

	fmt.Printf("%s Miner %s spawned (session start deferred)\n", style.Bold.Render("✓"), minerName)

	// Log spawn event to activity feed
	_ = events.LogFeed(events.TypeSpawn, "gt", events.SpawnPayload(rigName, minerName))

	// Compute effective base branch (strip origin/ prefix since formula prepends it)
	effectiveBranch := strings.TrimPrefix(baseBranch, "origin/")
	if effectiveBranch == "" {
		effectiveBranch = r.DefaultBranch()
	}
	if opts.ResumeBranch != "" {
		effectiveBranch = opts.ResumeBranch
	}

	return &SpawnedMinerInfo{
		RigName:     rigName,
		MinerName: minerName,
		ClonePath:   minerObj.ClonePath,
		SessionName: sessionName,
		Pane:        "", // Empty until StartSession is called
		BaseBranch:  effectiveBranch,
		Branch:      minerObj.Branch,
		account:     opts.Account,
		agent:       opts.Agent,
	}, nil
}

// StartSession starts the tmux session for a spawned miner.
// This is called after the molecule/bead is attached, so the miner
// sees its work when gt prime runs on session start.
// Returns the pane ID after session start.
func (s *SpawnedMinerInfo) StartSession() (string, error) {
	if s.SessionStarted() {
		return s.Pane, nil
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Excavation Site workspace: %w", err)
	}

	// Load rig config
	rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(s.RigName)
	if err != nil {
		return "", fmt.Errorf("rig '%s' not found", s.RigName)
	}

	// Resolve account
	accountsPath := constants.OverseerAccountsPath(townRoot)
	claudeConfigDir, _, err := config.ResolveAccountConfigDir(accountsPath, s.account)
	if err != nil {
		return "", fmt.Errorf("resolving account: %w", err)
	}

	// Start session
	t := tmux.NewTmux()
	minerSessMgr := miner.NewSessionManager(t, r)

	fmt.Printf("Starting session for %s/%s...\n", s.RigName, s.MinerName)
	startOpts := miner.SessionStartOptions{
		RuntimeConfigDir: claudeConfigDir,
		Agent:            s.agent,
	}
	if err := minerSessMgr.Start(s.MinerName, startOpts); err != nil {
		return "", fmt.Errorf("starting session: %w", err)
	}

	// Wait for runtime to be fully ready before returning.
	// When an agent override is specified (e.g., --agent codex), resolve the runtime
	// config from the override so WaitForRuntimeReady uses the correct readiness
	// strategy (delay-based for Codex vs prompt-polling for Claude). Without this,
	// ResolveRoleAgentConfig returns the default agent (Claude) and polls for "❯ "
	// in a Codex session, always timing out after 30 seconds (gt-1j3m).
	spawnTownRoot := filepath.Dir(r.Path)
	var runtimeConfig *config.RuntimeConfig
	if s.agent != "" {
		rc, _, err := config.ResolveAgentConfigWithOverride(spawnTownRoot, r.Path, s.agent)
		if err != nil {
			style.PrintWarning("resolving agent config for %s: %v (using default)", s.agent, err)
			runtimeConfig = config.ResolveRoleAgentConfig("miner", spawnTownRoot, r.Path)
		} else {
			runtimeConfig = rc
		}
	} else {
		runtimeConfig = config.ResolveRoleAgentConfig("miner", spawnTownRoot, r.Path)
	}
	if err := t.WaitForRuntimeReady(s.SessionName, runtimeConfig, 30*time.Second); err != nil {
		style.PrintWarning("runtime may not be fully ready: %v", err)
	}

	// Update agent state with retry logic (gt-94llt7: fail-safe Dolt writes).
	// Note: warn-only, not fail-hard. The tmux session is already started above,
	// so returning an error here would leave an orphaned session with no cleanup path.
	// The miner can still function without the agent state update — it only affects
	// monitoring visibility, not correctness. Compare with createAgentBeadWithRetry
	// which fails hard because a miner without an agent bead is untrackable.
	minerGit := git.NewGit(r.Path)
	minerMgr := miner.NewManager(r, minerGit, t)
	if err := minerMgr.SetAgentStateWithRetry(s.MinerName, "working"); err != nil {
		style.PrintWarning("could not update agent state after retries: %v", err)
	}

	// Update issue status from hooked to in_progress.
	// Also warn-only for the same reason: session is already running.
	if err := minerMgr.SetState(s.MinerName, miner.StateWorking); err != nil {
		style.PrintWarning("could not update issue status to in_progress: %v", err)
	}

	// Get pane — if this fails, the session may have died during startup.
	// Kill the dead session to prevent "session already running" on next attempt (gt-jn40ft).
	pane, err := getSessionPane(s.SessionName)
	if err != nil {
		// Session likely died — clean up the tmux session so it doesn't block re-sling
		_ = t.KillSession(s.SessionName)
		return "", fmt.Errorf("getting pane for %s (session likely died during startup): %w", s.SessionName, err)
	}

	s.Pane = pane
	return pane, nil
}

// IsRigName checks if a target string is a rig name (not a role or path).
// Returns the rig name and true if it's a valid rig.
func IsRigName(target string) (string, bool) {
	// If it contains a slash, it's a path format (rig/role or rig/crew/name)
	if strings.Contains(target, "/") {
		return "", false
	}

	// Check known non-rig role names
	switch strings.ToLower(target) {
	case constants.RoleOverseer, "may", constants.RoleSupervisor, "dea", constants.RoleCrew, constants.RoleWitness, "wit", constants.RoleRefinery, "ref":
		return "", false
	}

	// Try to load as a rig
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", false
	}

	rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return "", false
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	_, err = rigMgr.GetRig(target)
	if err != nil {
		return "", false
	}

	return target, true
}

// verifyWorktreeExists checks that a git worktree was actually created at the given path
// and that it is a functional git repository. Returns an error if the worktree is missing,
// has a broken .git reference, or fails basic git validation. (GH#2056)
func verifyWorktreeExists(clonePath string) error {
	// Check if directory exists
	info, err := os.Stat(clonePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree directory does not exist: %s", clonePath)
		}
		return fmt.Errorf("checking worktree directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path is not a directory: %s", clonePath)
	}

	// Check for .git file (worktrees have a .git file, not a .git directory)
	gitPath := filepath.Join(clonePath, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree missing .git file (not a valid git worktree): %s", clonePath)
		}
		return fmt.Errorf("checking .git: %w", err)
	}

	// For worktree .git files, verify the gitdir reference points to a valid path.
	// A broken reference (e.g., from os.Rename instead of git worktree move) causes
	// "fatal: not a git repository" for every git operation.
	gitContent, err := os.ReadFile(gitPath)
	if err == nil {
		content := strings.TrimSpace(string(gitContent))
		if strings.HasPrefix(content, "gitdir: ") {
			gitdirPath := strings.TrimPrefix(content, "gitdir: ")
			if !filepath.IsAbs(gitdirPath) {
				gitdirPath = filepath.Join(clonePath, gitdirPath)
			}
			if _, err := os.Stat(gitdirPath); err != nil {
				return fmt.Errorf("worktree .git references nonexistent gitdir %s: %w", gitdirPath, err)
			}
		}
	}

	// Final validation: run git rev-parse to confirm the worktree is functional
	cmd := exec.Command("git", "-C", clonePath, "rev-parse", "--git-dir")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree at %s is not a valid git repository: %s", clonePath, strings.TrimSpace(string(output)))
	}

	return nil
}
