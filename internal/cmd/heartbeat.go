package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/excavation/internal/beads"
	"github.com/steveyegge/excavation/internal/supervisor"
	"github.com/steveyegge/excavation/internal/miner"
	"github.com/steveyegge/excavation/internal/workspace"
)

var heartbeatCmd = &cobra.Command{
	Use:     "heartbeat",
	GroupID: GroupDiag,
	Short:   "Update agent heartbeat state",
	Long: `Update the agent heartbeat with a specific state.

Used by agents to self-report their state to the witness. The witness reads
the heartbeat state instead of inferring it from timers (ZFC: gt-3vr5).

States:
  working  - Actively processing (default)
  idle     - Waiting for input
  exiting  - In gt done flow
  stuck    - Self-reporting stuck (triggers witness escalation)

Examples:
  gt heartbeat --state=stuck "blocked on auth issue"
  gt heartbeat --state=idle
  gt heartbeat --state=working`,
	RunE: runHeartbeat,
}

var heartbeatState string

func init() {
	rootCmd.AddCommand(heartbeatCmd)
	heartbeatCmd.Flags().StringVar(&heartbeatState, "state", "working", "Agent state (working, idle, exiting, stuck)")
}

func runHeartbeat(cmd *cobra.Command, args []string) error {
	sessionName := os.Getenv("GT_SESSION")
	if sessionName == "" {
		return fmt.Errorf("GT_SESSION not set (not running in a Excavation Site session)")
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return fmt.Errorf("could not find town root: %v", err)
	}

	state := miner.HeartbeatState(heartbeatState)
	switch state {
	case miner.HeartbeatWorking, miner.HeartbeatIdle, miner.HeartbeatExiting, miner.HeartbeatStuck:
		// valid
	default:
		return fmt.Errorf("invalid state %q (must be working, idle, exiting, or stuck)", heartbeatState)
	}

	context := ""
	if len(args) > 0 {
		context = strings.Join(args, " ")
	}

	miner.TouchSessionHeartbeatWithState(townRoot, sessionName, state, context, "")

	// Supervisor liveness has extra stores beyond session heartbeat. Keep the
	// generic heartbeat command and `gt supervisor heartbeat` on one shared path.
	if os.Getenv("GT_ROLE") == "supervisor" {
		if err := syncSupervisorHeartbeatStores(townRoot, context); err != nil {
			fmt.Printf("warning: failed to touch supervisor heartbeat file: %v\n", err)
		}
	}

	fmt.Printf("Heartbeat updated: state=%s\n", state)
	return nil
}

// supervisorBeadHeartbeatSyncThreshold throttles agent-bead label refreshes from
// gt heartbeat: each refresh is a Dolt commit, so only sync when the label is
// stale enough to matter to watchers.
const supervisorBeadHeartbeatSyncThreshold = supervisor.HeartbeatStaleThreshold / 2

func syncSupervisorHeartbeatStores(townRoot, action string) error {
	var err error
	if action != "" {
		err = supervisor.TouchWithAction(townRoot, action, 0, 0)
	} else {
		err = supervisor.Touch(townRoot)
	}
	syncSupervisorAgentBeadHeartbeat(townRoot)
	return err
}

// syncSupervisorAgentBeadHeartbeat refreshes the heartbeat:EPOCH label on the
// Supervisor's agent bead — the third heartbeat store, read by Witness
// second-order monitoring. Normally await-signal maintains it, but a Supervisor
// session that never reaches await-signal (handoffs, long patrols, session
// limits) leaves it stale for hours and triggers false stuck escalations
// (hq-qxl9). Best-effort: failures are silent, liveness is already recorded
// in the other two stores.
func syncSupervisorAgentBeadHeartbeat(townRoot string) {
	agentBead := beads.SupervisorBeadIDTown()
	beadsDir := beads.ResolveBeadsDir(townRoot)

	labels, err := getAllAgentLabels(agentBead, beadsDir)
	if err != nil {
		return
	}
	for _, label := range labels {
		epochStr, ok := strings.CutPrefix(label, "heartbeat:")
		if !ok {
			continue
		}
		if epoch, err := strconv.ParseInt(epochStr, 10, 64); err == nil {
			if time.Since(time.Unix(epoch, 0)) < supervisorBeadHeartbeatSyncThreshold {
				return
			}
		}
	}
	_ = updateAgentHeartbeat(agentBead, beadsDir)
}
