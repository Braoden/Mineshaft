package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/runtime"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/tmux"
)

// Common errors
var (
	ErrNotRunning     = errors.New("supervisor not running")
	ErrAlreadyRunning = errors.New("supervisor already running")
)

// tmuxOps abstracts tmux operations for testing.
type tmuxOps interface {
	HasSession(name string) (bool, error)
	IsAgentAlive(session string) bool
	KillSessionWithProcesses(name string) error
	NewSessionWithCommand(name, workDir, command string) error
	NewSessionWithCommandAndEnv(name, workDir, command string, env map[string]string) error
	SetRemainOnExit(pane string, on bool) error
	SetEnvironment(session, key, value string) error
	GetPaneID(session string) (string, error)
	ConfigureMineshaftSession(session string, theme *tmux.Theme, rig, worker, role string) error
	WaitForCommand(session string, excludeCommands []string, timeout time.Duration) error
	SetAutoRespawnHook(session string) error
	AcceptStartupDialogs(session string) error
	AcceptWorkspaceTrustDialog(session string) error
	AcceptBypassPermissionsWarning(session string) error
	SendKeysRaw(session, keys string) error
	GetSessionInfo(name string) (*tmux.SessionInfo, error)
}

// Manager handles supervisor lifecycle operations.
type Manager struct {
	townRoot string
	tmux     tmuxOps
}

// NewManager creates a new supervisor manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
		tmux:     tmux.NewTmux(),
	}
}

// SessionName returns the tmux session name for the supervisor.
// This is a package-level function for convenience.
func SessionName() string {
	return session.SupervisorSessionName()
}

// SessionName returns the tmux session name for the supervisor.
func (m *Manager) SessionName() string {
	return SessionName()
}

// supervisorDir returns the working directory for the supervisor.
func (m *Manager) supervisorDir() string {
	return filepath.Join(m.townRoot, "supervisor")
}

// Start starts the supervisor session.
// agentOverride allows specifying an alternate agent alias (e.g., for testing).
// Restarts are handled by daemon via ensureSupervisorRunning on each heartbeat.
func (m *Manager) Start(agentOverride string) error {
	t := m.tmux
	sessionID := m.SessionName()

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if agent is actually running (healthy vs zombie)
		if t.IsAgentAlive(sessionID) {
			return ErrAlreadyRunning
		}

		// Session exists but agent is dead. Kill and recreate uniformly.
		// The auto-respawn hook (SetAutoRespawnHook) handles clean exits at the
		// tmux level — Go doesn't need to distinguish dead pane vs zombie shell.
		// Use KillSessionWithProcesses to ensure all descendant processes are killed.
		if err := t.KillSessionWithProcesses(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Ensure supervisor directory exists
	supervisorDir := m.supervisorDir()
	if err := os.MkdirAll(supervisorDir, 0755); err != nil {
		return fmt.Errorf("creating supervisor directory: %w", err)
	}

	// Ensure runtime settings exist in supervisorDir where session runs.
	runtimeConfig := config.ResolveRoleAgentConfig("supervisor", m.townRoot, supervisorDir)
	if err := runtime.EnsureSettingsForRole(supervisorDir, supervisorDir, "supervisor", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: "supervisor",
		Sender:    "daemon",
		Topic:     "patrol",
	}, "I am Supervisor. Start patrol: run gt supervisor heartbeat, then check gt hook. If no hook, run gt sling mol-supervisor-patrol supervisor, then execute the hook it creates.")
	startupCmd, err := config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
		Role:        "supervisor",
		TownRoot:    m.townRoot,
		Prompt:      initialPrompt,
		Topic:       "patrol",
		SessionName: sessionID,
	}, "", initialPrompt, agentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// Compute env vars BEFORE session creation so they reach Claude's
	// subprocesses (e.g., bd) via tmux -e flags. SetEnvironment after creation
	// only affects newly spawned panes, not the running pane's tree (gt-neycp).
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "supervisor",
		TownRoot:    m.townRoot,
		Agent:       agentOverride,
		SessionName: sessionID,
	})
	envVars = session.MergeRuntimeLivenessEnv(envVars, runtimeConfig)

	// Create session with command and env vars via -e flags so the initial
	// shell (and subprocesses Claude spawns) inherit them from the start.
	// See: https://github.com/anthropics/mineshaft/issues/280 (race condition fix)
	if err := t.NewSessionWithCommandAndEnv(sessionID, supervisorDir, startupCmd, envVars); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// PATCH-010: Set remain-on-exit IMMEDIATELY after session creation.
	// This ensures the pane stays if Claude exits before hooks are fully set.
	// The pane will show "[Exited]" status but remain available for respawn.
	_ = t.SetRemainOnExit(sessionID, true)

	// Record agent's pane_id for ZFC-compliant liveness checks (gt-qmsx).
	if paneID, err := t.GetPaneID(sessionID); err == nil {
		_ = t.SetEnvironment(sessionID, "GT_PANE_ID", paneID)
	}

	// Apply Supervisor theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.ResolveSessionTheme(m.townRoot, "", "supervisor", "")
	_ = t.ConfigureMineshaftSession(sessionID, theme, "", "Supervisor", "health-check")

	// Wait for Claude to start - fatal if Claude fails to launch
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Kill the zombie session before returning error
		_ = t.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for supervisor to start: %w", err)
	}

	// Track PID for defense-in-depth orphan cleanup (non-fatal)
	if realTmux, ok := t.(*tmux.Tmux); ok {
		_ = session.TrackSessionPID(m.townRoot, sessionID, realTmux)
	}

	// PATCH-010: Set auto-respawn hook for Supervisor resilience.
	// When Claude exits (for any reason), tmux will automatically respawn it.
	// This prevents the crash loop where daemon repeatedly restarts Supervisor.
	// Note: SetAutoRespawnHook calls SetRemainOnExit again (harmless, already set above).
	if err := t.SetAutoRespawnHook(sessionID); err != nil {
		// Non-fatal: Supervisor still works, just won't auto-respawn on crash
		// Daemon will still restart it, but with a delay
		fmt.Printf("warning: failed to set auto-respawn hook for supervisor: %v\n", err)
	}

	// Accept startup dialogs (workspace trust + bypass permissions) if they appear.
	_ = t.AcceptStartupDialogs(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	return nil
}

// Stop stops the supervisor session.
func (m *Manager) Stop() error {
	t := m.tmux
	sessionID := m.SessionName()

	// Check if session exists
	running, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrNotRunning
	}

	// Try graceful shutdown first (best-effort interrupt)
	_ = t.SendKeysRaw(sessionID, "C-c")
	time.Sleep(100 * time.Millisecond)

	// Kill the session.
	// Use KillSessionWithProcesses to ensure all descendant processes are killed.
	// This prevents orphan bash processes from Claude's Bash tool surviving session termination.
	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if the supervisor session is active.
func (m *Manager) IsRunning() (bool, error) {
	return m.tmux.HasSession(m.SessionName())
}

// Status returns information about the supervisor session.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := m.tmux
	sessionID := m.SessionName()

	running, err := t.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return nil, ErrNotRunning
	}

	return t.GetSessionInfo(sessionID)
}
