package overseer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/mineshaft/internal/acp"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/templates"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// Common errors
var (
	ErrNotRunning     = errors.New("overseer not running")
	ErrAlreadyRunning = errors.New("overseer already running")
	ErrACPActive      = errors.New("ACP overseer is active")
)

// Mode represents the overseer session mode.
type Mode string

const (
	ModeTMUX Mode = "tmux"
	ModeACP  Mode = "acp"
	ModeBoth Mode = "both"
	ModeNone Mode = "none"
)

// OverseerStatus represents the combined status of the overseer across all modes.
type OverseerStatus struct {
	Active  bool
	Mode    Mode
	Tmux    *tmux.SessionInfo
	ACPPid  int
	Running bool // Deprecated: use Active
}

// Manager handles overseer lifecycle operations.
type Manager struct {
	townRoot string
}

// CombinedStatus returns the combined status of the overseer across all modes.
func (m *Manager) CombinedStatus() (*OverseerStatus, error) {
	status := &OverseerStatus{
		Mode: ModeNone,
	}

	// Check TMUX
	tmuxRunning, _ := m.IsRunning()
	if tmuxRunning {
		info, err := m.Status()
		if err == nil {
			status.Tmux = info
			status.Active = true
			status.Mode = ModeTMUX
		}
	}

	// Check ACP
	if IsACPActive(m.townRoot) {
		status.Active = true
		if status.Mode == ModeTMUX {
			status.Mode = ModeBoth
		} else {
			status.Mode = ModeACP
		}
		pid, _ := GetACPPid(m.townRoot)
		status.ACPPid = pid
	}

	return status, nil
}

// IsActive checks if the overseer session is active in any mode.
func (m *Manager) IsActive() (bool, Mode) {
	status, _ := m.CombinedStatus()
	return status.Active, status.Mode
}

// NewManager creates a new overseer manager for a town.
func NewManager(townRoot string) *Manager {
	return &Manager{
		townRoot: townRoot,
	}
}

// SessionName returns the tmux session name for the overseer.
// This is a package-level function for convenience.
func SessionName() string {
	return session.OverseerSessionName()
}

// SessionName returns the tmux session name for the overseer.
func (m *Manager) SessionName() string {
	return SessionName()
}

// overseerDir returns the working directory for the overseer.
func (m *Manager) overseerDir() string {
	return filepath.Join(m.townRoot, "overseer")
}

// Start starts the overseer session.
// It checks both TMUX and ACP modes and returns ErrAlreadyRunning if active.
// agentOverride optionally specifies a different agent alias to use.
func (m *Manager) Start(agentOverride string) error {
	status, err := m.CombinedStatus()
	if err == nil && status.Active {
		switch status.Mode {
		case ModeACP, ModeBoth:
			return ErrACPActive
		case ModeTMUX:
			return ErrAlreadyRunning
		}
	}
	return m.StartTMUX(agentOverride)
}

// StartTMUX starts the overseer session in TMUX mode.
// agentOverride optionally specifies a different agent alias to use.
func (m *Manager) StartTMUX(agentOverride string) error {
	if IsACPActive(m.townRoot) {
		return ErrAlreadyRunning
	}

	t := tmux.NewTmux()
	sessionID := m.SessionName()

	// Kill any existing zombie session (tmux alive but agent dead).
	// Returns error if session is healthy and already running.
	_, err := session.KillExistingSession(t, sessionID, true)
	if err != nil {
		return ErrAlreadyRunning
	}

	// Ensure overseer directory exists (for Claude settings)
	overseerDir := m.overseerDir()
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		return fmt.Errorf("creating overseer directory: %w", err)
	}

	// Resolve CLAUDE_CONFIG_DIR from accounts.json so the overseer session
	// uses the correct account. Same pattern as crew startup (start.go).
	accountsPath := constants.OverseerAccountsPath(m.townRoot)
	claudeConfigDir, _, _ := config.ResolveAccountConfigDir(accountsPath, "")
	if claudeConfigDir == "" {
		claudeConfigDir = os.Getenv("CLAUDE_CONFIG_DIR")
	}

	// Use unified session lifecycle for config → settings → command → create → env → theme → wait.
	theme := tmux.ResolveSessionTheme(m.townRoot, "", "overseer", "")
	_, err = session.StartSession(t, session.SessionConfig{
		SessionID:        sessionID,
		WorkDir:          overseerDir,
		Role:             "overseer",
		TownRoot:         m.townRoot,
		AgentName:        "Overseer",
		RuntimeConfigDir: claudeConfigDir,
		Beacon: session.BeaconConfig{
			Recipient: "overseer",
			Sender:    "human",
			Topic:     "cold-start",
		},
		AgentOverride: agentOverride,
		Theme:         theme,
		WaitForAgent:  true,
		WaitFatal:     true,
		AutoRespawn:   true,
		AcceptBypass:  true,
	})
	if err != nil {
		return err
	}

	time.Sleep(session.ShutdownDelay())

	return nil
}

// StartACP starts the overseer session in ACP mode.
// This handles the transition from TMUX to ACP mode.
func (m *Manager) StartACP(ctx context.Context, agentOverride, rigName string) error {
	// Check if an ACP session is already running - only one ACP session is allowed
	// because they share the same PID file. Starting a second one would overwrite
	// the PID file, causing the first session's proxy to detect "PID file removed"
	// and shut down unexpectedly.
	if IsACPActive(m.townRoot) {
		return fmt.Errorf("ACP Overseer is already running. Only one ACP session is allowed at a time")
	}

	rc, agentName, err := config.ResolveAgentConfigWithOverride(m.townRoot, "", agentOverride)
	if err != nil {
		return fmt.Errorf("resolving agent config: %w", err)
	}

	if !config.RuntimeConfigSupportsACP(rc) {
		return fmt.Errorf("agent '%s' does not support ACP. Use an ACP-compatible agent like 'opencode'.", agentName)
	}

	// Prepare environment
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     "overseer",
		Rig:      rigName,
		TownRoot: m.townRoot,
	})
	for k, v := range envVars {
		os.Setenv(k, v)
	}
	os.Setenv("MS_TOWN_ROOT", m.townRoot)

	// Apply agent-specific environment variables from RuntimeConfig
	// This ensures variables like ANTHROPIC_API_KEY reach the agent process
	if rc.Env != nil {
		for k, v := range rc.Env {
			os.Setenv(k, v)
		}
	}

	overseerDir := m.overseerDir()
	if err := os.Chdir(overseerDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not cd to overseer directory: %v\n", err)
	}

	// Initialize ACP components
	proxy := acp.NewProxy()

	startupPrompt, err := m.buildACPStartupPrompt()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not render overseer prime context for ACP startup: %v\n", err)
	}
	proxy.SetStartupPrompt(startupPrompt)
	proxy.SetPIDFilePath(ACPPidFilePath(m.townRoot))
	proxy.SetTownRoot(m.townRoot)

	propeller := acp.NewPropeller(proxy, m.townRoot, m.SessionName())

	// Transition Point: Stop TMUX overseer if running, but only after ACP setup is ready.
	if running, _ := m.IsRunning(); running {
		fmt.Fprintf(os.Stderr, "Stopping tmux overseer to switch to ACP mode...\n")
		if err := m.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not stop tmux overseer: %v\n", err)
		}
	}

	// Write ACP PID and agent name after successful transition/stop
	if err := WriteACPPid(m.townRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write ACP PID file: %v\n", err)
	}
	if err := WriteACPAgent(m.townRoot, agentName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write ACP agent file: %v\n", err)
	}
	defer func() {
		if err := RemoveACPPid(m.townRoot); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove ACP PID file: %v\n", err)
		}
		if err := RemoveACPAgent(m.townRoot); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove ACP agent file: %v\n", err)
		}
	}()

	acpConfig := config.GetACPConfigFromRuntime(rc)
	var agentArgs []string
	if acpConfig != nil {
		// ACP mode: build args from ACP config
		// Handle different ACP invocation modes:
		//
		// 1. Native mode: Binary is already an ACP adapter (e.g., "claude-agent-acp")
		//    Config: { "mode": "native" } or { "mode": "native", "args": [...] }
		//    Result: claude-agent-acp [args...]
		//
		// 2. Subcommand mode: Agent has ACP as a subcommand (e.g., "opencode acp")
		//    Config: { "command": "acp", "args": ["--debug"] }
		//    Result: opencode acp --debug
		//
		// 3. Flag mode: Agent uses a flag to enable ACP (e.g., "gemini --experimental-acp")
		//    Config: { "args": ["--experimental-acp"] }
		//    Result: gemini --experimental-acp
		switch acpConfig.Mode {
		case config.ACPModeNative:
			// Native mode: the binary IS the ACP adapter
			// Just pass any additional args
			if len(acpConfig.Args) > 0 {
				agentArgs = append(agentArgs, acpConfig.Args...)
			}
		default:
			// Default (subcommand/flag) mode:
			// - If Command is set, it's a subcommand (prepend to args)
			// - If only Args is set, it's flag mode (use args directly)
			if acpConfig.Command != "" {
				agentArgs = []string{acpConfig.Command}
			}
			if len(acpConfig.Args) > 0 {
				agentArgs = append(agentArgs, acpConfig.Args...)
			}
		}
	}

	// Use rc.Command instead of agentName (alias) to ensure we run the correct binary.
	// If agentArgs is empty (no ACP config), we fall back to rc.Args for regular mode.
	execCmd := rc.Command
	if len(agentArgs) == 0 {
		agentArgs = rc.Args
	}

	if err := proxy.Start(ctx, execCmd, agentArgs, overseerDir); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	// Start background polling only after the agent process has successfully started.
	// The Propeller will wait for the ACP handshake to establish a SessionID
	// and verify the agent is not busy before attempting any prompt injections.
	propeller.Start(ctx)
	defer propeller.Stop()

	return proxy.Forward()
}

// Stop stops the overseer session.
func (m *Manager) Stop() error {
	t := tmux.NewTmux()
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

	// Kill the session and all its processes
	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if the overseer session is active in TMUX mode.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	return t.HasSession(m.SessionName())
}

// Status returns information about the overseer session.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := tmux.NewTmux()
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

// buildACPStartupPrompt composes the startup prompt used for ACP overseer sessions.
// It always includes the startup beacon and appends rendered overseer prime context
// when available.
func (m *Manager) buildACPStartupPrompt() (string, error) {
	beacon := session.FormatStartupBeacon(session.BeaconConfig{
		Recipient: "overseer",
		Sender:    "human",
		Topic:     "acp",
	})

	prime, err := GetOverseerPrime(m.townRoot)
	if err != nil {
		return beacon, err
	}
	if strings.TrimSpace(prime) == "" {
		return beacon, nil
	}

	return beacon + "\n\n" + prime, nil
}

// GetOverseerPrime returns the rendered overseer prime context as a raw string.
// This includes the formula from templates and a timestamp, suitable for
// ACP initialize responses where the full context needs to be provided
// as a single string payload.
func GetOverseerPrime(townRoot string) (string, error) {
	tmpl, err := templates.New()
	if err != nil {
		return "", fmt.Errorf("loading templates: %w", err)
	}

	townName, err := workspace.GetTownName(townRoot)
	if err != nil {
		townName = "unknown"
	}

	data := templates.RoleData{
		Role:          "overseer",
		TownRoot:      townRoot,
		TownName:      townName,
		WorkDir:       townRoot,
		OverseerSession:  session.OverseerSessionName(),
		SupervisorSession: session.SupervisorSessionName(),
	}

	content, err := tmpl.RenderRole("overseer", data)
	if err != nil {
		return "", fmt.Errorf("rendering overseer template: %w", err)
	}

	// Append timestamp
	timestamp := time.Now().UTC().Format(time.RFC3339)
	return fmt.Sprintf("[prime-rendered-at: %s]\n\n%s", timestamp, content), nil
}
