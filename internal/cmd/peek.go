package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

// Peek command flags
var peekLines int

func init() {
	rootCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&peekLines, "lines", "n", 100, "Number of lines to capture")
}

var peekCmd = &cobra.Command{
	Use:     "peek <rig/miner> [count]",
	GroupID: GroupComm,
	Short:   "View recent output from a miner or crew session",
	Long: `Capture and display recent terminal output from an agent session.

This is the ergonomic alias for 'gt session capture'. Use it to check
what an agent is currently doing or has recently output.

The nudge/peek pair provides the canonical interface for agent sessions:
  gt nudge - send messages TO a session (reliable delivery)
  gt peek  - read output FROM a session (capture-pane wrapper)

Supports miners, crew workers, and town-level agents:
  - Miners: rig/name format (e.g., greenplace/furiosa)
  - Crew: rig/crew/name format (e.g., beads/crew/dave)
  - Town-level: overseer, supervisor, boot (or hq/overseer, hq/supervisor, hq/boot)

Examples:
  gt peek greenplace/furiosa         # Miner: last 100 lines (default)
  gt peek greenplace/furiosa 50      # Miner: last 50 lines
  gt peek beads/crew/dave            # Crew: last 100 lines
  gt peek beads/crew/dave -n 200     # Crew: last 200 lines
  gt peek overseer                      # Overseer: last 100 lines
  gt peek supervisor -n 50               # Supervisor: last 50 lines`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runPeek,
}

func runPeek(cmd *cobra.Command, args []string) error {
	address := args[0]

	// Handle optional positional count argument
	lines := peekLines
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid line count: %s", args[1])
		}
		lines = n
	}

	// Handle town-level agents: overseer, supervisor, boot
	// These use session names like "hq-overseer", "hq-supervisor" but have no rig.
	townAgentSessions := map[string]string{
		"overseer":     "hq-overseer",
		"hq/overseer":  "hq-overseer",
		"supervisor":    "hq-supervisor",
		"hq/supervisor": "hq-supervisor",
		"boot":      "hq-boot",
		"hq/boot":   "hq-boot",
	}
	if sessionName, ok := townAgentSessions[address]; ok {
		_, err := workspace.FindFromCwdOrError()
		if err != nil {
			return fmt.Errorf("not in a Mineshaft workspace: %w", err)
		}
		t := tmux.NewTmux()
		output, err := t.CapturePane(sessionName, lines)
		if err != nil {
			return fmt.Errorf("capturing %s: %w", address, err)
		}
		fmt.Print(output)
		return nil
	}

	rigName, minerName, err := parseAddress(address)
	if err != nil {
		if !strings.Contains(address, "/") {
			return fmt.Errorf("not in a rig directory. Use full address format: gt peek <rig>/<miner>")
		}
		return err
	}

	mgr, _, err := getSessionManager(rigName)
	if err != nil {
		if !strings.Contains(address, "/") {
			return fmt.Errorf("not in a rig directory. Use full address format: gt peek <rig>/<miner>")
		}
		return err
	}

	var output string

	// Handle crew/ prefix for cross-rig crew workers
	// e.g., "beads/crew/dave" -> session name "gt-beads-crew-dave"
	if strings.HasPrefix(minerName, "crew/") {
		crewName := strings.TrimPrefix(minerName, "crew/")
		sessionID := session.CrewSessionName(session.PrefixFor(rigName), crewName)
		output, err = mgr.CaptureSession(sessionID, lines)
	} else {
		output, err = mgr.Capture(minerName, lines)
	}

	if err != nil {
		return fmt.Errorf("capturing output: %w", err)
	}

	fmt.Print(output)
	return nil
}
