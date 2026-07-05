package cmd

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/daemon"
	"github.com/steveyegge/mineshaft/internal/doltserver"
	"github.com/steveyegge/mineshaft/internal/overseer"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/style"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var overseerCmd = &cobra.Command{
	Use:     "overseer",
	Aliases: []string{"may"},
	GroupID: GroupAgents,
	Short:   "Manage the Overseer (Chief of Staff for cross-rig coordination)",
	RunE:    requireSubcommand,
	Long: `Manage the Overseer - the Boss's Chief of Staff.

The Overseer is the global coordinator for Mineshaft:
  - Receives escalations from Witnesses and Supervisor
  - Coordinates work across multiple rigs
  - Handles human communication when needed
  - Routes strategic decisions and cross-project issues

The Overseer is the primary interface between the human Boss and the
automated agents. When in doubt, escalate to the Overseer.

Role shortcuts: "overseer" in mail/nudge addresses resolves to this agent.`,
}

var (
	overseerAgentOverride string
	overseerStatusRunning bool
)

var overseerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Overseer session",
	Long: `Start the Overseer tmux session.

Creates a new detached tmux session for the Overseer and launches Claude.
The session runs in the workspace root directory.`,
	RunE: runOverseerStart,
}

var overseerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Overseer session",
	Long: `Stop the Overseer tmux session.

Attempts graceful shutdown first (Ctrl-C), then kills the tmux session.`,
	RunE: runOverseerStop,
}

var overseerAttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"at"},
	Short:   "Attach to the Overseer session",
	Long: `Attach to the running Overseer tmux session.

Attaches the current terminal to the Overseer's tmux session.
Detach with Ctrl-B D.`,
	RunE: runOverseerAttach,
}

var overseerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check Overseer session status",
	Long:  `Check if the Overseer tmux session is currently running.`,
	RunE:  runOverseerStatus,
}

var overseerRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Overseer session",
	Long: `Restart the Overseer tmux session.

Stops the current session (if running) and starts a fresh one.`,
	RunE: runOverseerRestart,
}

var overseerAcpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run Overseer in headless mode (Agent Control Protocol)",
	Long: `Run the Overseer in headless mode with stdin/stdout connected.

This command initializes a headless session without tmux, designed for
IDE integration via the Agent Control Protocol. It bypasses all tmux
logic and runs directly in the current terminal.

Environment variable overrides:
  MS_RIG          - Override rig name
  MS_TOWN_ROOT    - Override town root directory
  MS_ROLE         - Override role (default: overseer)

The agent reads prompts from stdin and outputs to stdout. This enables
programmatic control by IDEs or other tools that need direct agent access.

While an ACP session is active, automatic cleanup of miner workspaces
is vetoed to allow the Overseer to review worker diffs before they vanish.`,
	RunE: runOverseerAcp,
}

var acpRigOverride string
var acpTownRootOverride string

func init() {
	overseerCmd.AddCommand(overseerStartCmd)
	overseerCmd.AddCommand(overseerStopCmd)
	overseerCmd.AddCommand(overseerAttachCmd)
	overseerCmd.AddCommand(overseerStatusCmd)
	overseerCmd.AddCommand(overseerRestartCmd)
	overseerCmd.AddCommand(overseerAcpCmd)

	overseerStatusCmd.Flags().BoolVar(&overseerStatusRunning, "running", false, "Output only true/false for running status")

	overseerStartCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")
	overseerAttachCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")
	overseerRestartCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run the Overseer with (overrides town default)")

	overseerAcpCmd.Flags().StringVar(&acpRigOverride, "rig", "", "Rig name (overrides MS_RIG env)")
	overseerAcpCmd.Flags().StringVar(&acpTownRootOverride, "town", "", "Town root directory (overrides MS_TOWN_ROOT env)")
	overseerAcpCmd.Flags().StringVar(&overseerAgentOverride, "agent", "", "Agent alias to run (overrides town default)")

	rootCmd.AddCommand(overseerCmd)
}

// getOverseerManager returns a overseer manager for the current workspace.
func getOverseerManager() (*overseer.Manager, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Mineshaft workspace: %w", err)
	}
	return overseer.NewManager(townRoot), nil
}

// getOverseerSessionName returns the Overseer session name.
func getOverseerSessionName() string {
	return overseer.SessionName()
}

func runOverseerStart(cmd *cobra.Command, args []string) error {
	mgr, err := getOverseerManager()
	if err != nil {
		return err
	}

	fmt.Println("Starting Overseer session...")
	if err := mgr.Start(overseerAgentOverride); err != nil {
		if err == overseer.ErrAlreadyRunning {
			return fmt.Errorf("Overseer session already running. Attach with: ms overseer attach")
		}
		return err
	}

	fmt.Printf("%s Overseer session started. Attach with: %s\n",
		style.Bold.Render("✓"),
		style.Dim.Render("ms overseer attach"))

	return nil
}

func runOverseerStop(cmd *cobra.Command, args []string) error {
	mgr, err := getOverseerManager()
	if err != nil {
		return err
	}

	fmt.Println("Stopping Overseer session...")
	if err := mgr.Stop(); err != nil {
		if err == overseer.ErrNotRunning {
			return fmt.Errorf("Overseer session is not running")
		}
		return err
	}

	fmt.Printf("%s Overseer session stopped.\n", style.Bold.Render("✓"))
	return nil
}

func runOverseerAttach(cmd *cobra.Command, args []string) error {
	mgr, err := getOverseerManager()
	if err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	// Check if ACP is active and gracefully shut it down before switching to tmux.
	// Only 'ms overseer attach' is allowed to transition from ACP to tmux mode.
	if overseer.IsACPActive(townRoot) {
		fmt.Fprintf(os.Stderr, "ACP Overseer is active. Switching to tmux mode...\n")
		if err := gracefullyShutdownACP(townRoot); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not gracefully shutdown ACP: %v\n", err)
		}
	}

	// Ensure daemon and dolt are running before attaching.
	if err := ensureOverseerInfra(townRoot); err != nil {
		return err
	}

	t := tmux.NewTmux()
	sessionID := mgr.SessionName()

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		// Auto-start if not running
		fmt.Println("Overseer session not running, starting...")
		if err := mgr.Start(overseerAgentOverride); err != nil {
			return err
		}
	} else {
		// Session exists - check if runtime is still running (hq-95xfq, ms-7zl)
		// If runtime exited or sitting at shell, restart with proper context.
		// Use IsAgentAlive (checks descendant processes) instead of IsAgentRunning
		// (pane command only), since overseer launches via bash wrapper.
		if !t.IsAgentAlive(sessionID) {
			// Runtime has exited, restart it with proper context
			fmt.Println("Runtime exited, restarting with context...")

			paneID, err := t.GetPaneID(sessionID)
			if err != nil {
				return fmt.Errorf("getting pane ID: %w", err)
			}

			// Build startup beacon for context (like ms handoff does)
			beacon := session.FormatStartupBeacon(session.BeaconConfig{
				Recipient: "overseer",
				Sender:    "human",
				Topic:     "attach",
			})

			// Build startup command with beacon
			startupCmd, err := config.BuildAgentStartupCommandWithAgentOverride("overseer", "", townRoot, "", beacon, overseerAgentOverride)
			if err != nil {
				return fmt.Errorf("building startup command: %w", err)
			}

			// Resolve CLAUDE_CONFIG_DIR and prepend it so the respawned process
			// uses the correct account (mirrors what StartTMUX does).
			accountsPath := constants.OverseerAccountsPath(townRoot)
			claudeConfigDir, _, _ := config.ResolveAccountConfigDir(accountsPath, "")
			if claudeConfigDir == "" {
				claudeConfigDir = os.Getenv("CLAUDE_CONFIG_DIR")
			}
			if claudeConfigDir != "" {
				startupCmd = config.PrependEnv(startupCmd, map[string]string{"CLAUDE_CONFIG_DIR": claudeConfigDir})
				_ = t.SetEnvironment(sessionID, "CLAUDE_CONFIG_DIR", claudeConfigDir)
			}

			// Set remain-on-exit so the pane survives process death during respawn.
			// Without this, killing processes causes tmux to destroy the pane.
			if err := t.SetRemainOnExit(paneID, true); err != nil {
				style.PrintWarning("could not set remain-on-exit: %v", err)
			}

			// Kill all processes in the pane before respawning to prevent orphan leaks
			// RespawnPane's -k flag only sends SIGHUP which Claude/Node may ignore
			if err := t.KillPaneProcesses(paneID); err != nil {
				// Non-fatal but log the warning
				style.PrintWarning("could not kill pane processes: %v", err)
			}

			// Note: respawn-pane automatically resets remain-on-exit to off
			if err := t.RespawnPane(paneID, startupCmd); err != nil {
				return fmt.Errorf("restarting runtime: %w", err)
			}

			fmt.Printf("%s Overseer restarted with context\n", style.Bold.Render("✓"))
		}
	}

	// Use shared attach helper (smart: links if inside tmux, attaches if outside)
	return attachToTmuxSession(sessionID)
}

// gracefullyShutdownACP removes the PID file to signal the ACP proxy to exit,
// then waits for the process to terminate.
func gracefullyShutdownACP(townRoot string) error {
	// Get the PID before removing the file
	pid, err := overseer.GetACPPid(townRoot)
	if err != nil {
		// PID file doesn't exist or is invalid, nothing to shut down
		return nil
	}

	// Remove the PID file - this signals the ACP proxy to shut down gracefully
	if err := overseer.RemoveACPPid(townRoot); err != nil {
		return fmt.Errorf("removing ACP PID file: %w", err)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil // Process doesn't exist
	}

	// Wait for the process to exit (with timeout)
	fmt.Fprintf(os.Stderr, "Waiting for ACP session to shut down")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(os.Stderr, ".")
		// Check if process is still alive
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process has exited
			fmt.Fprintf(os.Stderr, " done\n")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Process didn't exit gracefully, force kill
	fmt.Fprintf(os.Stderr, " forcing shutdown\n")
	_ = process.Kill()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func runOverseerStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	mgr := overseer.NewManager(townRoot)
	status, err := mgr.CombinedStatus()
	if err != nil {
		return err
	}

	if overseerStatusRunning {
		fmt.Println(status.Active)
		return nil
	}

	if !status.Active {
		fmt.Printf("%s Overseer session is %s\n",
			style.Dim.Render("○"),
			"not running")
		fmt.Printf("\nStart with: %s\n", style.Dim.Render("ms overseer start"))
		return nil
	}

	if status.Tmux != nil {
		attachedStatus := "detached"
		if status.Tmux.Attached {
			attachedStatus = "attached"
		}
		fmt.Printf("%s Overseer (tmux) is %s\n",
			style.Bold.Render("●"),
			style.Bold.Render("running"))
		fmt.Printf("  Status: %s\n", attachedStatus)
		fmt.Printf("  Created: %s\n", status.Tmux.Created)
	}

	if status.ACPPid != 0 {
		fmt.Printf("%s Overseer (ACP) is %s\n",
			style.Bold.Render("●"),
			style.Bold.Render("running (headless)"))
		fmt.Printf("  PID: %d\n", status.ACPPid)
	}

	if status.Tmux != nil {
		fmt.Printf("\nAttach with: %s\n", style.Dim.Render("ms overseer attach"))
	} else if status.ACPPid != 0 {
		fmt.Printf("\nAttach with: %s\n", style.Dim.Render("ms overseer acp"))
	}

	return nil
}

func runOverseerRestart(cmd *cobra.Command, args []string) error {
	mgr, err := getOverseerManager()
	if err != nil {
		return err
	}

	// Stop if running (ignore not-running error)
	if err := mgr.Stop(); err != nil && err != overseer.ErrNotRunning {
		return fmt.Errorf("stopping session: %w", err)
	}

	// Start fresh
	return runOverseerStart(cmd, args)
}

// ensureOverseerInfra checks that daemon and dolt are running before attaching
// to the Overseer session. Warns and auto-starts each if absent.
// Returns an error if Dolt fails to start — a missing Dolt server is fatal
// for the Overseer (it cannot operate without database access).
// Daemon failures are non-fatal (warned but do not block).
func ensureOverseerInfra(townRoot string) error {
	// Load daemon.json env vars (e.g., MS_DOLT_PORT) so Dolt uses the right port.
	if patrolCfg := daemon.LoadPatrolConfig(townRoot); patrolCfg != nil {
		for k, v := range patrolCfg.Env {
			os.Setenv(k, v)
		}
	}

	// Daemon (non-fatal)
	daemonRunning, _, _ := daemon.IsRunning(townRoot)
	if !daemonRunning {
		style.PrintWarning("daemon is not running, starting...")
		if err := ensureDaemon(townRoot); err != nil {
			style.PrintWarning("daemon start failed: %v", err)
		} else {
			fmt.Printf("  %s Daemon started\n", style.Bold.Render("✓"))
		}
	}

	// Dolt (fatal on failure — Overseer requires database access)
	doltCfg := doltserver.DefaultConfig(townRoot)
	if !doltCfg.IsRemote() {
		if _, err := os.Stat(doltCfg.DataDir); err == nil {
			doltRunning, _, _ := doltserver.IsRunning(townRoot)
			if !doltRunning {
				style.PrintWarning("Dolt server is not running, starting...")
				if err := doltserver.Start(townRoot); err != nil {
					// Enrich port-conflict errors with a concrete free-port suggestion.
					msg := fmt.Sprintf("Dolt server start failed: %v", err)
					if pid, dataDir := doltserver.PortHolder(doltCfg.Port); pid > 0 {
						if dataDir != "" {
							msg += fmt.Sprintf("\n  port %d held by dolt PID %d serving %s", doltCfg.Port, pid, dataDir)
						} else {
							msg += fmt.Sprintf("\n  port %d held by PID %d", doltCfg.Port, pid)
						}
					}
					if freePort := doltserver.FindFreePort(doltCfg.Port + 1); freePort > 0 {
						msg += fmt.Sprintf("\n\nConfigure a free port for this town, then retry:\n  ms config set dolt.port %d && ms overseer at", freePort)
					}
					return fmt.Errorf("%s", msg)
				}
				fmt.Printf("  %s Dolt server started (port %d)\n", style.Bold.Render("✓"), doltCfg.Port)
			}
		}
	}
	return nil
}

// runOverseerAcp runs the Overseer in headless mode for IDE integration.
// It bypasses tmux and execs the agent directly with stdin/stdout connected.
// A PID file is created to signal that automatic cleanup should be vetoed,
// allowing the Overseer to review worker diffs before cleanup.
func runOverseerAcp(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	townRoot := acpTownRootOverride
	if townRoot == "" {
		townRoot = os.Getenv("MS_TOWN_ROOT")
	}
	if townRoot == "" {
		var err error
		townRoot, err = workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Mineshaft workspace: %w", err)
		}
	}

	if err := ensureOverseerInfra(townRoot); err != nil {
		return err
	}

	rigName := acpRigOverride
	if rigName == "" {
		rigName = os.Getenv("MS_RIG")
	}

	mgr := overseer.NewManager(townRoot)
	return mgr.StartACP(ctx, overseerAgentOverride, rigName)
}
