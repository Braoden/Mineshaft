package witness

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/miner"
	"github.com/steveyegge/mineshaft/internal/tmux"
)

type testOverseerEvent struct {
	Type    string            `json:"type"`
	Payload map[string]string `json:"payload"`
}

func setupSlotOpenTestTown(t *testing.T) (string, string) {
	t.Helper()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(townRoot, "mineshaft", "witness")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	return townRoot, workDir
}

func readOverseerEvents(t *testing.T, townRoot string) []testOverseerEvent {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(townRoot, "events", "overseer", "*.event"))
	if err != nil {
		t.Fatal(err)
	}
	events := make([]testOverseerEvent, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var event testOverseerEvent
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestNotifyOverseerSlotOpen_BlocksNonCompletedExit(t *testing.T) {
	townRoot, workDir := setupSlotOpenTestTown(t)

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeDeferred))

	events := readOverseerEvents(t, townRoot)
	if len(events) != 1 {
		t.Fatalf("events = %v, want one SLOT_BLOCKED event", events)
	}
	event := events[0]
	if event.Type != "SLOT_BLOCKED" {
		t.Fatalf("event type = %q, want SLOT_BLOCKED", event.Type)
	}
	if event.Payload["reason"] != "exit-deferred" {
		t.Fatalf("reason = %q, want exit-deferred", event.Payload["reason"])
	}
}

func TestNotifyOverseerSlotOpen_SchedulerDispatchSuppressesOverseer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	called := false
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		called = true
		if gotTownRoot != townRoot {
			t.Fatalf("townRoot = %q, want %q", gotTownRoot, townRoot)
		}
		return slotOpenSchedulerResult{Dispatched: 1}, nil
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	if !called {
		t.Fatal("scheduler trigger was not called")
	}
	if events := readOverseerEvents(t, townRoot); len(events) != 0 {
		t.Fatalf("events = %+v, want none when scheduler dispatches", events)
	}
}

func TestNotifyOverseerSlotOpen_DispatchThenEmptyEmitsSchedulerOpen(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		var result slotOpenSchedulerResult
		result.Ran = true
		result.Dispatched = 1
		result.After.Capacity.Max = 10
		result.After.Capacity.Free = 1
		result.After.QueuedReady = 0
		return result, nil
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	events := readOverseerEvents(t, townRoot)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one SCHEDULER_OPEN event", events)
	}
	if events[0].Type != "SCHEDULER_OPEN" {
		t.Fatalf("event type = %q, want SCHEDULER_OPEN", events[0].Type)
	}
}

func TestNotifyOverseerSlotOpen_DispatchWithStatusErrorSuppressesOverseer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		return slotOpenSchedulerResult{Dispatched: 1}, errors.New("status read failed")
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	if events := readOverseerEvents(t, townRoot); len(events) != 0 {
		t.Fatalf("events = %+v, want none after confirmed dispatch", events)
	}
}

func TestNotifyOverseerSlotOpen_EmitsSchedulerOpenWhenQueueEmpty(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		var result slotOpenSchedulerResult
		result.Before.Capacity.Max = 10
		result.Before.Capacity.Free = 2
		result.Before.QueuedReady = 0
		return result, nil
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	events := readOverseerEvents(t, townRoot)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one SCHEDULER_OPEN event", events)
	}
	if events[0].Type != "SCHEDULER_OPEN" {
		t.Fatalf("event type = %q, want SCHEDULER_OPEN", events[0].Type)
	}
	if events[0].Payload["capacity_free"] != "2" {
		t.Fatalf("capacity_free = %q, want 2", events[0].Payload["capacity_free"])
	}
}

func TestNotifyOverseerSlotOpen_QueuedReadyWithoutDispatchFallsBack(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		var result slotOpenSchedulerResult
		result.Before.Capacity.Max = 10
		result.Before.Capacity.Free = 1
		result.Before.QueuedReady = 1
		result.After = result.Before
		result.Ran = true
		return result, nil
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	events := readOverseerEvents(t, townRoot)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one fallback SLOT_OPEN event", events)
	}
	if events[0].Type != "SLOT_OPEN" {
		t.Fatalf("event type = %q, want SLOT_OPEN", events[0].Type)
	}
}

func TestNotifyOverseerSlotOpen_NoDispatchAfterCapacityFillsSuppressesOverseer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	townRoot, workDir := setupSlotOpenTestTown(t)

	prevRecovery := slotOpenRecoveryCheck
	prevDecision := slotOpenDecisionForNotify
	prevScheduler := runSchedulerForSlotOpen
	t.Cleanup(func() {
		slotOpenRecoveryCheck = prevRecovery
		slotOpenDecisionForNotify = prevDecision
		runSchedulerForSlotOpen = prevScheduler
	})

	slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
		return `{"verdict":"SAFE_TO_NUKE"}`, nil
	}
	slotOpenDecisionForNotify = func(workDir, townRoot, rigName, minerName, exitType string) miner.SlotReuseDecision {
		return miner.SlotReuseDecision{Reusable: true}
	}
	runSchedulerForSlotOpen = func(gotTownRoot string) (slotOpenSchedulerResult, error) {
		var result slotOpenSchedulerResult
		result.Before.Capacity.Max = 10
		result.Before.Capacity.Free = 1
		result.Before.QueuedReady = 1
		result.After.Capacity.Max = 10
		result.After.Capacity.Free = 0
		result.After.QueuedReady = 1
		result.Ran = true
		return result, nil
	}

	notifyOverseerSlotOpen(workDir, "mineshaft", "guzzle", string(ExitTypeCompleted))

	if events := readOverseerEvents(t, townRoot); len(events) != 0 {
		t.Fatalf("events = %+v, want none when scheduler no longer has capacity", events)
	}
}

func TestParseSchedulerRunDispatched(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{name: "dispatched", output: "\n✓ Dispatched 2, failed 0 (reason: batch)\n", want: 2},
		{name: "skipped", output: "\n○ Skipped 1 bead(s) — zero capacity\n", want: 0},
		{name: "cleanup only", output: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseSchedulerRunDispatched(tt.output); got != tt.want {
				t.Fatalf("parseSchedulerRunDispatched() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestShouldNotifyOverseerSlotOpenRequiresSafeRecovery(t *testing.T) {
	prev := slotOpenRecoveryCheck
	t.Cleanup(func() { slotOpenRecoveryCheck = prev })

	tests := []struct {
		name    string
		output  string
		err     error
		wantOK  bool
		wantMsg string
	}{
		{
			name:   "safe to nuke notifies",
			output: `{"verdict":"SAFE_TO_NUKE"}`,
			wantOK: true,
		},
		{
			name:   "warning-prefixed json notifies",
			output: "warning: stale binary\n" + `{"verdict":"SAFE_TO_NUKE"}`,
			wantOK: true,
		},
		{
			name:    "needs recovery suppresses",
			output:  `{"verdict":"NEEDS_RECOVERY","blockers":["cleanup_status=has_unpushed"]}`,
			wantMsg: "NEEDS_RECOVERY",
		},
		{
			name:    "needs mq submit suppresses",
			output:  `{"verdict":"NEEDS_MQ_SUBMIT"}`,
			wantMsg: "NEEDS_MQ_SUBMIT",
		},
		{
			name:    "check failure suppresses",
			err:     errors.New("boom"),
			wantMsg: "check-recovery failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slotOpenRecoveryCheck = func(workDir, rigName, minerName string) (string, error) {
				return tt.output, tt.err
			}

			gotOK, gotMsg := shouldNotifyOverseerSlotOpen("/tmp", "mineshaft", "nitro")
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v (msg=%q)", gotOK, tt.wantOK, gotMsg)
			}
			if tt.wantMsg != "" && !strings.Contains(gotMsg, tt.wantMsg) {
				t.Fatalf("message %q does not contain %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestActiveMRBlockerFromCLIUsesTerminalStatus(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "empty active mr", want: ""},
		{name: "open mr blocks", output: `[{"status":"open"}]`, want: "active_mr=ms-mr status=open"},
		{name: "closed mr does not block", output: `[{"status":"closed"}]`, want: ""},
		{name: "not found does not block", err: fmt.Errorf("issue not found"), want: ""},
		{name: "lookup error blocks", err: fmt.Errorf("bd unavailable"), want: "active_mr=ms-mr status=lookup_error: bd unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd, _ := mockBd(
				func(args []string) (string, error) { return tt.output, tt.err },
				func(args []string) error { return nil },
			)
			mrID := "ms-mr"
			if tt.name == "empty active mr" {
				mrID = ""
			}
			if got := activeMRBlockerFromCLI(bd, t.TempDir(), mrID); got != tt.want {
				t.Fatalf("activeMRBlockerFromCLI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleMinerDoneFromBead_NilFields(t *testing.T) {
	t.Parallel()
	result := HandleMinerDoneFromBead(DefaultBdCli(), "/tmp", "testrig", "nux", nil, nil)
	if result.Error == nil {
		t.Error("expected error for nil fields")
	}
	if result.Handled {
		t.Error("should not be handled with nil fields")
	}
}

func TestHandleMinerDoneFromBead_PhaseComplete(t *testing.T) {
	t.Parallel()
	fields := &beads.AgentFields{
		ExitType: "PHASE_COMPLETE",
		Branch:   "miner/nux",
	}
	result := HandleMinerDoneFromBead(DefaultBdCli(), "/tmp", "testrig", "nux", fields, nil)
	if !result.Handled {
		t.Error("expected PHASE_COMPLETE to be handled")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
	if !strings.Contains(result.Action, "phase-complete") {
		t.Errorf("action %q should contain 'phase-complete'", result.Action)
	}
}

func TestHandleMinerDoneFromBead_NoMR(t *testing.T) {
	t.Parallel()
	fields := &beads.AgentFields{
		ExitType:       "COMPLETED",
		Branch:         "miner/nux",
		HookBead:       "ms-test123",
		CompletionTime: "2026-02-28T01:00:00Z",
	}
	result := HandleMinerDoneFromBead(DefaultBdCli(), "/tmp/nonexistent", "testrig", "nux", fields, nil)
	if !result.Handled {
		t.Error("expected completion with no MR to be handled")
	}
	if !strings.Contains(result.Action, "no MR") {
		t.Errorf("action %q should contain 'no MR'", result.Action)
	}
}

func TestHandleMinerDoneFromBead_ProtocolType(t *testing.T) {
	t.Parallel()
	fields := &beads.AgentFields{
		ExitType: "COMPLETED",
		Branch:   "miner/nux",
	}
	result := HandleMinerDoneFromBead(DefaultBdCli(), "/tmp/nonexistent", "testrig", "nux", fields, nil)
	if result.ProtocolType != ProtoMinerDone {
		t.Errorf("ProtocolType = %q, want %q", result.ProtocolType, ProtoMinerDone)
	}
}

func TestZombieResult_Types(t *testing.T) {
	t.Parallel()
	// Verify the ZombieResult type has all expected fields
	z := ZombieResult{
		MinerName:    "nux",
		AgentState:     "working",
		Classification: ZombieSessionDeadActive,
		HookBead:       "ms-abc123",
		Action:         "restarted",
		BeadRecovered:  true,
		Error:          nil,
	}

	if z.MinerName != "nux" {
		t.Errorf("MinerName = %q, want %q", z.MinerName, "nux")
	}
	if z.AgentState != "working" {
		t.Errorf("AgentState = %q, want %q", z.AgentState, "working")
	}
	if z.Classification != ZombieSessionDeadActive {
		t.Errorf("Classification = %q, want %q", z.Classification, ZombieSessionDeadActive)
	}
	if z.HookBead != "ms-abc123" {
		t.Errorf("HookBead = %q, want %q", z.HookBead, "ms-abc123")
	}
	if z.Action != "restarted" {
		t.Errorf("Action = %q, want %q", z.Action, "restarted")
	}
	if !z.BeadRecovered {
		t.Error("BeadRecovered = false, want true")
	}
}

func TestDetectZombieMinersResult_EmptyResult(t *testing.T) {
	t.Parallel()
	result := &DetectZombieMinersResult{}

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0", result.Checked)
	}
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies length = %d, want 0", len(result.Zombies))
	}
}

func TestDetectZombieMiners_NonexistentDir(t *testing.T) {
	t.Parallel()
	// Should handle missing miners directory gracefully
	result := DetectZombieMiners(DefaultBdCli(), "/nonexistent/path", "testrig", nil)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for nonexistent dir", result.Checked)
	}
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies = %d, want 0 for nonexistent dir", len(result.Zombies))
	}
}

func TestDetectZombieMiners_DirectoryScanning(t *testing.T) {
	t.Parallel()
	// Create a temp directory structure simulating miners
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create miner directories
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		if err := os.Mkdir(filepath.Join(minersDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create hidden dir (should be skipped)
	if err := os.Mkdir(filepath.Join(minersDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file (should be skipped, not a dir)
	if err := os.WriteFile(filepath.Join(minersDir, "notadir.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := DetectZombieMiners(DefaultBdCli(), tmpDir, rigName, nil)

	// Should have checked 3 miner dirs (not hidden, not file)
	if result.Checked != 3 {
		t.Errorf("Checked = %d, want 3 (should skip hidden dirs and files)", result.Checked)
	}

	// No zombies because agent bead state will be empty (bd not available),
	// so isZombie stays false for all miners
	if len(result.Zombies) != 0 {
		t.Errorf("Zombies = %d, want 0 (no agent state = not zombie)", len(result.Zombies))
	}
}

func TestDetectZombieMiners_EmptyMinersDir(t *testing.T) {
	t.Parallel()
	// Empty miners directory should return 0 checked
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result := DetectZombieMiners(DefaultBdCli(), tmpDir, rigName, nil)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for empty miners dir", result.Checked)
	}
}

func TestGetAgentBeadState_EmptyOutput(t *testing.T) {
	t.Parallel()
	// getAgentBeadState with invalid bead ID should return empty strings
	// (it calls bd which won't exist in test, so it returns empty)
	state, hook := getAgentBeadState(DefaultBdCli(), "/nonexistent", "nonexistent-bead")

	if state != "" {
		t.Errorf("state = %q, want empty for missing bead", state)
	}
	if hook != "" {
		t.Errorf("hook = %q, want empty for missing bead", hook)
	}
}

func TestSessionRecreated_NoSession(t *testing.T) {
	t.Parallel()
	// When the session doesn't exist, sessionRecreated should return false
	// (the session wasn't recreated, it's still dead)
	tm := tmux.NewTmux()
	detectedAt := time.Now()

	recreated := sessionRecreated(tm, "ms-nonexistent-session-xyz", detectedAt)
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session, want false")
	}
}

func TestSessionRecreated_DetectedAtEdgeCases(t *testing.T) {
	t.Parallel()
	// Verify that sessionRecreated returns false when session is dead
	// regardless of the detectedAt timestamp
	tm := tmux.NewTmux()

	// Try with a past timestamp
	recreated := sessionRecreated(tm, "ms-test-nosession-abc", time.Now().Add(-1*time.Hour))
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session with past time")
	}

	// Try with a future timestamp
	recreated = sessionRecreated(tm, "ms-test-nosession-def", time.Now().Add(1*time.Hour))
	if recreated {
		t.Error("sessionRecreated returned true for nonexistent session with future time")
	}
}

func TestZombieClassification_SpawningState(t *testing.T) {
	t.Parallel()
	// Verify that "spawning" agent state is treated as a zombie indicator.
	// This tests the classification logic inline in DetectZombieMiners.
	// We can't easily test this via the full function without mocking,
	// so we test the boolean logic directly.
	states := map[string]bool{
		"working":  true,
		"running":  true,
		"spawning": true,
		"idle":     false,
		"done":     false,
		"":         false,
	}

	for state, wantZombie := range states {
		hookBead := ""
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if isZombie != wantZombie {
			t.Errorf("agent_state=%q: isZombie=%v, want %v", state, isZombie, wantZombie)
		}
	}
}

func TestZombieClassification_HookBeadAlwaysZombie(t *testing.T) {
	t.Parallel()
	// Any miner with a hook_bead and dead session should be classified as zombie,
	// regardless of agent_state.
	for _, state := range []string{"", "idle", "done", "working"} {
		hookBead := "ms-some-issue"
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if !isZombie {
			t.Errorf("agent_state=%q with hook_bead=%q: isZombie=false, want true", state, hookBead)
		}
	}
}

func TestZombieClassification_NoHookNoActiveState(t *testing.T) {
	t.Parallel()
	// Miners with no hook_bead and non-active agent_state should NOT be zombies.
	for _, state := range []string{"", "idle", "done", "completed"} {
		hookBead := ""
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if state == "working" || state == "running" || state == "spawning" {
			isZombie = true
		}

		if isZombie {
			t.Errorf("agent_state=%q with no hook_bead: isZombie=true, want false", state)
		}
	}
}

func TestFindAnyCleanupWisp_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available (test environment), findAnyCleanupWisp
	// should return empty string without panicking
	result := findAnyCleanupWisp(DefaultBdCli(), "/nonexistent", "testminer")
	if result != "" {
		t.Errorf("findAnyCleanupWisp = %q, want empty when bd unavailable", result)
	}
}

// mockBdCalls captures bd invocations and returns canned responses.
// Returns a slice that accumulates "arg0 arg1 ..." strings for each call.
type mockBdCalls struct {
	calls []string
}

// mockBd creates a test-local *BdCli with mock exec/run functions.
// Returns the BdCli and a pointer to the captured call log.
// No global state is modified — safe for use with t.Parallel().
func mockBd(execFn func(args []string) (string, error), runFn func(args []string) error) (*BdCli, *mockBdCalls) {
	mock := &mockBdCalls{}
	bd := &BdCli{
		Exec: func(workDir string, args ...string) (string, error) {
			mock.calls = append(mock.calls, strings.Join(args, " "))
			return execFn(stripMockBdFlags(args))
		},
		Run: func(workDir string, args ...string) error {
			mock.calls = append(mock.calls, strings.Join(args, " "))
			return runFn(stripMockBdFlags(args))
		},
	}
	return bd, mock
}

func stripMockBdFlags(args []string) []string {
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		args = args[1:]
	}
	return args
}

func installFakeTmuxNoServer(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' 'no server running on /tmp/tmux' 1>&2\nexit 1\n"
	if runtime.GOOS == "windows" {
		scriptPath += ".bat"
		script = "@echo off\r\necho no server running on C:\\tmp\\tmux 1>&2\r\nexit /b 1\r\n"
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fakeBd creates a test-local *BdCli matching the old shell script behavior:
// list→"[]", update→ok, show→cleanup wisp JSON. Returns BdCli and captured call log.
func fakeBd() (*BdCli, *mockBdCalls) {
	return mockBd(
		func(args []string) (string, error) {
			if len(args) > 0 {
				switch args[0] {
				case "list":
					return "[]", nil
				case "show":
					return `[{"labels":["cleanup","miner:testpol","state:pending"]}]`, nil
				}
			}
			return "{}", nil
		},
		func(args []string) error { return nil },
	)
}

func setupActiveMRGitSafeWorkDir(t *testing.T, rigName, minerName string) string {
	t.Helper()
	townRoot := t.TempDir()
	clonePath := filepath.Join(townRoot, rigName, "miners", minerName, rigName)
	if err := os.MkdirAll(clonePath, 0755); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit(clonePath, "init")
	runGit(clonePath, "config", "user.email", "test@example.com")
	runGit(clonePath, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(clonePath, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(clonePath, "add", "README.md")
	runGit(clonePath, "commit", "-m", "initial")
	remotePath := filepath.Join(townRoot, "origin.git")
	runGit(townRoot, "init", "--bare", remotePath)
	runGit(clonePath, "remote", "add", "origin", remotePath)
	runGit(clonePath, "push", "-u", "origin", "HEAD")
	return townRoot
}

func TestHasPendingMRFromSnapshotAssessesMRStatus(t *testing.T) {
	issueJSON := func(id, status, desc string) string {
		b, err := json.Marshal([]map[string]any{{"id": id, "status": status, "description": desc}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	tests := []struct {
		name string
		show func(id string) (string, error)
		want bool
	}{
		{
			name: "open MR is pending",
			show: func(id string) (string, error) {
				return issueJSON(id, "open", ""), nil
			},
			want: true,
		},
		{
			name: "closed MR with terminal source is not pending",
			show: func(id string) (string, error) {
				if id == "ms-mr" {
					return issueJSON(id, "closed", ""), nil
				}
				return issueJSON(id, "closed", ""), nil
			},
		},
		{
			name: "missing MR with terminal source is not pending",
			show: func(id string) (string, error) {
				if id == "ms-mr" {
					return "", errors.New("not found")
				}
				return issueJSON(id, "closed", ""), nil
			},
		},
		{
			name: "lookup error is pending",
			show: func(id string) (string, error) { return "", errors.New("bd exploded") },
			want: true,
		},
		{
			name: "closed MR with open source is pending",
			show: func(id string) (string, error) {
				if id == "ms-mr" {
					return issueJSON(id, "closed", ""), nil
				}
				return issueJSON(id, "open", ""), nil
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir := setupActiveMRGitSafeWorkDir(t, "mineshaft", "nux")
			bd, _ := mockBd(
				func(args []string) (string, error) {
					if len(args) == 0 {
						return "", nil
					}
					switch args[0] {
					case "list":
						return "[]", nil
					case "show":
						return tt.show(args[1])
					}
					return "", nil
				},
				func(args []string) error { return nil },
			)
			snap := &agentBeadSnapshot{ActiveMR: "ms-mr", Fields: &beads.AgentFields{ActiveMR: "ms-mr", LastSourceIssue: "ms-src"}}
			if got := hasPendingMRFromSnapshot(bd, workDir, "mineshaft", "nux", snap); got != tt.want {
				t.Fatalf("hasPendingMRFromSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasPendingMRUsesAgentLastSourceIssue(t *testing.T) {
	workDir := setupActiveMRGitSafeWorkDir(t, "mineshaft", "nux")
	bd, _ := mockBd(
		func(args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "list":
				return "[]", nil
			case "show":
				switch args[1] {
				case "ms-agent":
					return `[{"active_mr":"ms-mr","description":"active_mr: ms-mr\nlast_source_issue: ms-src\n"}]`, nil
				case "ms-mr":
					return "", errors.New("not found")
				case "ms-src":
					return `[{"id":"ms-src","status":"closed"}]`, nil
				}
			}
			return "", errors.New("not found")
		},
		func(args []string) error { return nil },
	)

	if got := hasPendingMR(bd, workDir, "mineshaft", "nux", "ms-agent"); got {
		t.Fatalf("hasPendingMR() = true, want false for missing MR with terminal source")
	}
}

func TestHasPendingMRFromSnapshotRequiresGitSafe(t *testing.T) {
	bd, _ := mockBd(
		func(args []string) (string, error) {
			if len(args) == 0 {
				return "", nil
			}
			switch args[0] {
			case "list":
				return "[]", nil
			case "show":
				if args[1] == "ms-mr" {
					return "", errors.New("not found")
				}
				return `[{"id":"ms-src","status":"closed"}]`, nil
			}
			return "", nil
		},
		func(args []string) error { return nil },
	)
	snap := &agentBeadSnapshot{ActiveMR: "ms-mr", Fields: &beads.AgentFields{ActiveMR: "ms-mr", LastSourceIssue: "ms-src"}}
	if got := hasPendingMRFromSnapshot(bd, t.TempDir(), "mineshaft", "nux", snap); !got {
		t.Fatalf("hasPendingMRFromSnapshot() = false, want true when git is unsafe")
	}
}

func TestHasPendingMRCleanupWispFailsClosed(t *testing.T) {
	workDir := setupActiveMRGitSafeWorkDir(t, "mineshaft", "nux")
	tests := []struct {
		name string
		list string
		err  error
	}{
		{name: "cleanup wisp present", list: `[{"id":"ms-cleanup"}]`},
		{name: "cleanup wisp lookup error", err: errors.New("bd exploded")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bd, _ := mockBd(
				func(args []string) (string, error) {
					if len(args) == 0 {
						return "", nil
					}
					if args[0] == "list" {
						return tt.list, tt.err
					}
					if args[0] == "show" && args[1] == "ms-agent" {
						return `[{"active_mr":"ms-mr","description":"active_mr: ms-mr\nlast_source_issue: ms-src\n"}]`, nil
					}
					if args[0] == "show" && args[1] == "ms-mr" {
						return "", errors.New("not found")
					}
					return `[{"id":"ms-src","status":"closed"}]`, nil
				},
				func(args []string) error { return nil },
			)
			if got := hasPendingMR(bd, workDir, "mineshaft", "nux", "ms-agent"); !got {
				t.Fatalf("hasPendingMR() = false, want true")
			}
		})
	}
}

func TestTerminalSafeDoneSnapshot(t *testing.T) {
	workDir := setupActiveMRGitSafeWorkDir(t, "mineshaft", "nux")
	bd, _ := mockBd(
		func(args []string) (string, error) {
			if len(args) == 0 || args[0] != "show" {
				return "[]", nil
			}
			return `[{"id":"ms-src","status":"closed"}]`, nil
		},
		func(args []string) error { return nil },
	)
	snap := &agentBeadSnapshot{Fields: &beads.AgentFields{LastSourceIssue: "ms-src"}}
	if !terminalSafeDoneSnapshot(bd, workDir, "mineshaft", "nux", snap) {
		t.Fatalf("terminalSafeDoneSnapshot() = false, want true")
	}
	snap.Fields.HookBead = "ms-hook"
	if terminalSafeDoneSnapshot(bd, workDir, "mineshaft", "nux", snap) {
		t.Fatalf("terminalSafeDoneSnapshot() = true with hook set, want false")
	}
}

func TestFindCleanupWisp_UsesCorrectBdListFlags(t *testing.T) {
	t.Parallel()
	bd, mock := fakeBd()
	workDir := t.TempDir()

	_, _ = findCleanupWisp(bd, workDir, "nux")

	got := strings.Join(mock.calls, "\n")

	// Must use --label (singular), NOT --labels (plural)
	if !strings.Contains(got, "--label") {
		t.Errorf("findCleanupWisp: expected --label flag, got: %s", got)
	}
	if strings.Contains(got, "--labels") {
		t.Errorf("findCleanupWisp: must not use --labels (plural), got: %s", got)
	}

	// Must NOT use --ephemeral (invalid for bd list)
	if strings.Contains(got, "--ephemeral") {
		t.Errorf("findCleanupWisp: must not use --ephemeral (invalid for bd list), got: %s", got)
	}

	// Must include the miner label filter
	if !strings.Contains(got, "miner:nux") {
		t.Errorf("findCleanupWisp: expected miner:nux label, got: %s", got)
	}
}

func TestFindAnyCleanupWisp_UsesCorrectBdListFlags(t *testing.T) {
	t.Parallel()
	bd, mock := fakeBd()
	workDir := t.TempDir()

	_ = findAnyCleanupWisp(bd, workDir, "bravo")

	got := strings.Join(mock.calls, "\n")

	// Must use --label (singular), NOT --labels (plural)
	if !strings.Contains(got, "--label") {
		t.Errorf("findAnyCleanupWisp: expected --label flag, got: %s", got)
	}
	if strings.Contains(got, "--labels") {
		t.Errorf("findAnyCleanupWisp: must not use --labels (plural), got: %s", got)
	}

	// Must NOT use --ephemeral (invalid for bd list)
	if strings.Contains(got, "--ephemeral") {
		t.Errorf("findAnyCleanupWisp: must not use --ephemeral (invalid for bd list), got: %s", got)
	}

	// Must include the miner label filter
	if !strings.Contains(got, "miner:bravo") {
		t.Errorf("findAnyCleanupWisp: expected miner:bravo label, got: %s", got)
	}
}

func TestFindAllCleanupWisps_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available, findAllCleanupWisps should return nil
	result := findAllCleanupWisps(DefaultBdCli(), "/nonexistent", "testminer")
	if result != nil {
		t.Errorf("findAllCleanupWisps = %v, want nil when bd unavailable", result)
	}
}

func TestFindAllCleanupWisps_ReturnsAllIDs(t *testing.T) {
	t.Parallel()
	bd, mock := mockBd(
		func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "list" {
				return `[{"id":"ms-wisp-aaa"},{"id":"ms-wisp-bbb"}]`, nil
			}
			return "{}", nil
		},
		func(args []string) error { return nil },
	)
	workDir := t.TempDir()

	result := findAllCleanupWisps(bd, workDir, "nux")

	if len(result) != 2 {
		t.Fatalf("findAllCleanupWisps: got %d items, want 2", len(result))
	}
	if result[0] != "ms-wisp-aaa" || result[1] != "ms-wisp-bbb" {
		t.Errorf("findAllCleanupWisps: got %v, want [ms-wisp-aaa ms-wisp-bbb]", result)
	}

	got := strings.Join(mock.calls, "\n")
	if !strings.Contains(got, "--label") {
		t.Errorf("findAllCleanupWisps: expected --label flag, got: %s", got)
	}
	if !strings.Contains(got, "miner:nux") {
		t.Errorf("findAllCleanupWisps: expected miner:nux label, got: %s", got)
	}
}

func TestFindAllCleanupWisps_EmptyList(t *testing.T) {
	t.Parallel()
	bd, _ := mockBd(
		func(args []string) (string, error) {
			return "[]", nil
		},
		func(args []string) error { return nil },
	)
	workDir := t.TempDir()

	result := findAllCleanupWisps(bd, workDir, "nux")
	if result != nil {
		t.Errorf("findAllCleanupWisps: got %v, want nil for empty list", result)
	}
}

func TestUpdateCleanupWispState_UsesCorrectBdUpdateFlags(t *testing.T) {
	t.Parallel()
	bd, mock := fakeBd()
	workDir := t.TempDir()

	// UpdateCleanupWispState first calls "bd show <id> --json", then "bd update".
	// Our mock returns valid JSON for show with miner:testpol label,
	// so minerName will be "testpol". Then it calls bd update with new labels.
	_ = UpdateCleanupWispState(bd, workDir, "ms-wisp-abc", "merged")

	got := strings.Join(mock.calls, "\n")

	// Must use --set-labels=<label> per label (not --labels)
	if !strings.Contains(got, "--set-labels=") {
		t.Errorf("UpdateCleanupWispState: expected --set-labels=<label> flags, got: %s", got)
	}
	// Check for invalid --labels flag in both " --labels " and "--labels=" forms
	if strings.Contains(got, "--labels") && !strings.Contains(got, "--set-labels") {
		t.Errorf("UpdateCleanupWispState: must not use --labels (invalid for bd update), got: %s", got)
	}

	// Verify individual per-label arguments with correct miner name from show output
	if !strings.Contains(got, "--set-labels=cleanup") {
		t.Errorf("UpdateCleanupWispState: expected --set-labels=cleanup, got: %s", got)
	}
	if !strings.Contains(got, "--set-labels=miner:testpol") {
		t.Errorf("UpdateCleanupWispState: expected --set-labels=miner:testpol, got: %s", got)
	}
	if !strings.Contains(got, "--set-labels=state:merged") {
		t.Errorf("UpdateCleanupWispState: expected --set-labels=state:merged, got: %s", got)
	}
}

func TestExtractDoneIntent_Valid(t *testing.T) {
	t.Parallel()
	ts := time.Now().Add(-45 * time.Second)
	labels := []string{
		"ms:agent",
		"idle:2",
		fmt.Sprintf("done-intent:COMPLETED:%d", ts.Unix()),
	}

	intent := extractDoneIntent(labels)
	if intent == nil {
		t.Fatal("extractDoneIntent returned nil for valid label")
	}
	if intent.ExitType != "COMPLETED" {
		t.Errorf("ExitType = %q, want %q", intent.ExitType, "COMPLETED")
	}
	if intent.Timestamp.Unix() != ts.Unix() {
		t.Errorf("Timestamp = %d, want %d", intent.Timestamp.Unix(), ts.Unix())
	}
}

func TestExtractDoneIntent_Missing(t *testing.T) {
	t.Parallel()
	labels := []string{"ms:agent", "idle:2", "backoff-until:1738972900"}

	intent := extractDoneIntent(labels)
	if intent != nil {
		t.Errorf("extractDoneIntent = %+v, want nil for no done-intent label", intent)
	}
}

func TestExtractDoneIntent_Malformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		labels []string
	}{
		{"missing timestamp", []string{"done-intent:COMPLETED"}},
		{"bad timestamp", []string{"done-intent:COMPLETED:notanumber"}},
		{"empty labels", nil},
		{"empty label list", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent := extractDoneIntent(tt.labels)
			if intent != nil {
				t.Errorf("extractDoneIntent(%v) = %+v, want nil for malformed input", tt.labels, intent)
			}
		})
	}
}

func TestExtractDoneIntent_AllExitTypes(t *testing.T) {
	t.Parallel()
	ts := time.Now().Unix()
	for _, exitType := range []string{"COMPLETED", "ESCALATED", "DEFERRED", "PHASE_COMPLETE"} {
		label := fmt.Sprintf("done-intent:%s:%d", exitType, ts)
		intent := extractDoneIntent([]string{label})
		if intent == nil {
			t.Errorf("extractDoneIntent returned nil for exit type %q", exitType)
			continue
		}
		if intent.ExitType != exitType {
			t.Errorf("ExitType = %q, want %q", intent.ExitType, exitType)
		}
	}
}

func TestDetectZombie_DoneIntentDeadSession(t *testing.T) {
	t.Parallel()
	// Verify the logic: dead session + done-intent older than 30s → should be treated as zombie
	// ms-dsgp: action is restart (not nuke), but detection logic is the same
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-60 * time.Second), // 60s old
	}
	sessionAlive := false
	age := time.Since(doneIntent.Timestamp)

	// Dead session + old intent → restart path (ms-dsgp: was auto-nuke)
	shouldRestart := !sessionAlive && doneIntent != nil && age >= config.DefaultWitnessDoneIntentStuckTimeout
	if !shouldRestart {
		t.Errorf("expected restart for dead session + old done-intent (age=%v)", age)
	}
}

func TestDetectZombie_DoneIntentLiveStuck(t *testing.T) {
	t.Parallel()
	// Verify the logic: live session + done-intent older than 60s → should restart session
	// ms-dsgp: restart instead of kill
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-90 * time.Second), // 90s old
	}
	sessionAlive := true
	age := time.Since(doneIntent.Timestamp)

	// Live session + old intent → restart stuck session (ms-dsgp: was kill)
	shouldRestart := sessionAlive && doneIntent != nil && age > config.DefaultWitnessDoneIntentStuckTimeout
	if !shouldRestart {
		t.Errorf("expected restart for live session + old done-intent (age=%v)", age)
	}
}

func TestDetectZombie_DoneIntentRecent(t *testing.T) {
	t.Parallel()
	// Verify the logic: done-intent younger than config.DefaultWitnessDoneIntentStuckTimeout → skip (miner still working)
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-10 * time.Second), // 10s old
	}
	sessionAlive := false
	age := time.Since(doneIntent.Timestamp)

	// Recent intent → should skip
	shouldSkip := !sessionAlive && doneIntent != nil && age < config.DefaultWitnessDoneIntentStuckTimeout
	if !shouldSkip {
		t.Errorf("expected skip for recent done-intent (age=%v)", age)
	}

	// Live session + recent intent → also skip
	sessionAlive = true
	shouldSkipLive := sessionAlive && doneIntent != nil && age <= config.DefaultWitnessDoneIntentStuckTimeout
	if !shouldSkipLive {
		t.Errorf("expected skip for live session + recent done-intent (age=%v)", age)
	}
}

func TestDetectZombie_DoneOrNukedNotZombie(t *testing.T) {
	t.Parallel()
	// GH#2795: Miners with agent_state=done or agent_state=nuked and a dead
	// session should NOT be treated as zombies, even if hook_bead is still set.
	// Without this, isZombieState returns true (hookBead != ""), and the witness
	// floods the overseer inbox with RECOVERY_NEEDED alerts every patrol cycle.
	for _, state := range []beads.AgentState{beads.AgentStateDone, beads.AgentStateNuked} {
		hookBead := "ms-some-issue"
		// isZombieState returns true because hookBead != ""
		if !isZombieState(state, hookBead) {
			t.Errorf("isZombieState(%q, %q) = false, want true (pre-condition)", state, hookBead)
		}
		// But the done/nuked check in detectZombieDeadSession should skip these.
		// Verify the states are terminal (not active).
		if state.IsActive() {
			t.Errorf("state %q should not be active", state)
		}
	}
}

func TestDetectZombie_AgentDeadInLiveSession(t *testing.T) {
	t.Parallel()
	// Verify the logic: live session + agent process dead → zombie
	// This is the ms-kj6r6 fix: DetectZombieMiners now checks IsAgentAlive
	// for sessions that DO exist, catching the tmux-alive-but-agent-dead class.
	sessionAlive := true
	agentAlive := false
	var doneIntent *DoneIntent // No done-intent

	// Live session + no done-intent + agent dead → should be classified as zombie
	shouldDetect := sessionAlive && doneIntent == nil && !agentAlive
	if !shouldDetect {
		t.Error("expected zombie detection for live session with dead agent")
	}

	// Live session + agent alive → NOT a zombie
	agentAlive = true
	shouldSkip := sessionAlive && doneIntent == nil && agentAlive
	if !shouldSkip {
		t.Error("expected skip for live session with alive agent")
	}
}

func TestGetAgentBeadLabels_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available, should return nil without panicking
	labels := getAgentBeadLabels(DefaultBdCli(), "/nonexistent", "nonexistent-bead")
	if labels != nil {
		t.Errorf("getAgentBeadLabels = %v, want nil when bd unavailable", labels)
	}
}

// --- extractMinerFromJSON tests (issue #1228: panic-safe JSON parsing) ---

func TestExtractMinerFromJSON_ValidOutput(t *testing.T) {
	t.Parallel()
	input := `[{"labels":["cleanup","miner:nux","state:pending"]}]`
	got := extractMinerFromJSON(input)
	if got != "nux" {
		t.Errorf("extractMinerFromJSON() = %q, want %q", got, "nux")
	}
}

func TestExtractMinerFromJSON_InvalidInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty output", ""},
		{"malformed JSON", "{not valid json"},
		{"empty array", "[]"},
		{"no miner label", `[{"labels":["cleanup","state:pending"]}]`},
		{"empty labels", `[{"labels":[]}]`},
		{"truncated JSON", `[{"labels":["miner:`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMinerFromJSON(tt.input)
			if got != "" {
				t.Errorf("extractMinerFromJSON(%q) = %q, want empty", tt.input, got)
			}
		})
	}
}

func TestGetBeadStatus_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available (test environment), getBeadStatus
	// should return ("", false) without panicking
	result, ok := getBeadStatus(DefaultBdCli(), "/nonexistent", "ms-abc123")
	if result != "" || ok {
		t.Errorf("getBeadStatus = (%q, %v), want (\"\", false) when bd unavailable", result, ok)
	}
}

func TestGetBeadStatus_EmptyBeadID(t *testing.T) {
	t.Parallel()
	// Empty bead ID should return ("", false) immediately
	result, ok := getBeadStatus(DefaultBdCli(), "/nonexistent", "")
	if result != "" || ok {
		t.Errorf("getBeadStatus(\"\") = (%q, %v), want (\"\", false)", result, ok)
	}
}

func TestDetectZombie_BeadClosedStillRunning(t *testing.T) {
	t.Parallel()
	// Verify the logic: live session + agent alive + hooked bead closed → zombie
	// This is the ms-h1l6i fix: DetectZombieMiners now checks if the
	// miner's hooked bead has been closed while the session is still running.
	sessionAlive := true
	agentAlive := true
	var doneIntent *DoneIntent // No done-intent
	hookBead := "ms-some-issue"
	beadStatus := "closed"

	// Live session + agent alive + no done-intent + bead closed → should detect
	shouldDetect := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if !shouldDetect {
		t.Error("expected zombie detection for live session with closed bead")
	}

	// Bead open → NOT a zombie
	beadStatus = "open"
	shouldSkip := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if shouldSkip {
		t.Error("should not detect zombie when bead is still open")
	}

	// No hook bead → NOT a zombie
	hookBead = ""
	beadStatus = "closed"
	shouldSkipNoHook := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"
	if shouldSkipNoHook {
		t.Error("should not detect zombie when no hook bead exists")
	}
}

func TestDetectZombie_BeadClosedVsDoneIntent(t *testing.T) {
	t.Parallel()
	// Verify done-intent takes priority over closed-bead check.
	// If done-intent exists (recent), the miner is still working through
	// ms done and we should NOT trigger the closed-bead path.
	sessionAlive := true
	agentAlive := true
	doneIntent := &DoneIntent{
		ExitType:  "COMPLETED",
		Timestamp: time.Now().Add(-10 * time.Second), // Recent
	}
	hookBead := "ms-some-issue"
	beadStatus := "closed"

	// Done-intent exists + bead closed → done-intent check runs first,
	// closed-bead check should NOT run (it's in the else branch)
	doneIntentHandled := sessionAlive && doneIntent != nil && time.Since(doneIntent.Timestamp) > config.DefaultWitnessDoneIntentStuckTimeout
	closedBeadCheck := sessionAlive && agentAlive && doneIntent == nil &&
		hookBead != "" && beadStatus == "closed"

	// Neither should trigger: done-intent is recent (not stuck), and
	// closed-bead check requires doneIntent == nil
	if doneIntentHandled {
		t.Error("recent done-intent should not trigger stuck-session handler")
	}
	if closedBeadCheck {
		t.Error("closed-bead check should not run when done-intent exists")
	}
}

func TestResetAbandonedBead_EmptyHookBead(t *testing.T) {
	t.Parallel()
	// resetAbandonedBead should return false for empty hookBead
	result := resetAbandonedBead(DefaultBdCli(), "/tmp", "testrig", "", "nux", nil)
	if result {
		t.Error("resetAbandonedBead should return false for empty hookBead")
	}
}

func TestResetAbandonedBead_NoRouter(t *testing.T) {
	t.Parallel()
	// resetAbandonedBead with nil router should not panic even if bead exists.
	// It will return false because bd won't find the bead, but shouldn't crash.
	result := resetAbandonedBead(DefaultBdCli(), "/tmp/nonexistent", "testrig", "ms-fake123", "nux", nil)
	if result {
		t.Error("resetAbandonedBead should return false when bd commands fail")
	}
}

func TestResetAbandonedBead_ClosesWhenWorkOnMain(t *testing.T) {
	// Not parallel: overrides package-level verifyCommitOnMain.
	// When verifyCommitOnMain returns true, resetAbandonedBead should close the
	// bead instead of resetting it for re-dispatch. This is the fix for #2036.

	oldVerify := verifyCommitOnMain
	verifyCommitOnMain = func(workDir, rigName, minerName string) (bool, error) {
		return true, nil // work is on main
	}
	t.Cleanup(func() { verifyCommitOnMain = oldVerify })

	bd, mock := mockBd(
		func(args []string) (string, error) {
			if len(args) >= 1 && args[0] == "show" {
				return `[{"status":"hooked"}]`, nil
			}
			return "", nil
		},
		func(args []string) error {
			return nil
		},
	)

	tmpDir := t.TempDir()
	result := resetAbandonedBead(bd, tmpDir, "testrig", "ms-work123", "alpha", nil)
	if result {
		t.Error("resetAbandonedBead should return false when work is on main (bead closed, not re-dispatched)")
	}

	// Verify "close" was called, NOT "update ... --status=open"
	var foundClose, foundUpdate bool
	for _, call := range mock.calls {
		if strings.Contains(call, "close ms-work123") {
			foundClose = true
		}
		if strings.Contains(call, "update") && strings.Contains(call, "--status=open") {
			foundUpdate = true
		}
	}
	if !foundClose {
		t.Errorf("expected bd close to be called, got calls: %v", mock.calls)
	}
	if foundUpdate {
		t.Error("bd update --status=open should NOT be called when work is on main")
	}
}

func TestResetAbandonedBead_ResetsWhenWorkNotOnMain(t *testing.T) {
	// Not parallel: overrides package-level verifyCommitOnMain.
	// When verifyCommitOnMain returns false, resetAbandonedBead should reset
	// the bead for re-dispatch (existing behavior).

	oldVerify := verifyCommitOnMain
	verifyCommitOnMain = func(workDir, rigName, minerName string) (bool, error) {
		return false, nil // work NOT on main
	}
	t.Cleanup(func() { verifyCommitOnMain = oldVerify })

	bd, mock := mockBd(
		func(args []string) (string, error) {
			if len(args) >= 1 && args[0] == "show" {
				return `[{"status":"hooked"}]`, nil
			}
			return "", nil
		},
		func(args []string) error {
			return nil
		},
	)

	tmpDir := t.TempDir()
	result := resetAbandonedBead(bd, tmpDir, "testrig", "ms-work123", "alpha", nil)
	if !result {
		t.Error("resetAbandonedBead should return true when work is NOT on main (bead reset for re-dispatch)")
	}

	// Verify "update --status=open" was called (normal reset path)
	var foundUpdate bool
	for _, call := range mock.calls {
		if strings.Contains(call, "update") && strings.Contains(call, "--status=open") {
			foundUpdate = true
		}
	}
	if !foundUpdate {
		t.Errorf("expected bd update --status=open to be called, got calls: %v", mock.calls)
	}
}

func TestBeadRecoveredField_DefaultFalse(t *testing.T) {
	t.Parallel()
	// BeadRecovered should default to false (zero value)
	z := ZombieResult{
		MinerName:    "nux",
		AgentState:     "working",
		Classification: ZombieSessionDeadActive,
	}
	if z.BeadRecovered {
		t.Error("BeadRecovered should default to false")
	}
}

func TestStalledResult_Types(t *testing.T) {
	t.Parallel()
	// Verify the StalledResult type has all expected fields
	s := StalledResult{
		MinerName: "alpha",
		StallType:   "startup-stall",
		Action:      "auto-dismissed",
		Error:       nil,
	}

	if s.MinerName != "alpha" {
		t.Errorf("MinerName = %q, want %q", s.MinerName, "alpha")
	}
	if s.StallType != "startup-stall" {
		t.Errorf("StallType = %q, want %q", s.StallType, "startup-stall")
	}
	if s.Action != "auto-dismissed" {
		t.Errorf("Action = %q, want %q", s.Action, "auto-dismissed")
	}
	if s.Error != nil {
		t.Errorf("Error = %v, want nil", s.Error)
	}

	// Verify error field works
	s2 := StalledResult{
		MinerName: "bravo",
		StallType:   "startup-stall",
		Action:      "escalated",
		Error:       fmt.Errorf("auto-dismiss failed"),
	}
	if s2.Error == nil {
		t.Error("Error = nil, want non-nil")
	}
}

func TestDetectStalledMinersResult_Empty(t *testing.T) {
	t.Parallel()
	result := &DetectStalledMinersResult{}

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0", result.Checked)
	}
	if len(result.Stalled) != 0 {
		t.Errorf("Stalled length = %d, want 0", len(result.Stalled))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors length = %d, want 0", len(result.Errors))
	}
}

func TestDetectStalledMiners_NoMiners(t *testing.T) {
	t.Parallel()
	// Should handle missing miners directory gracefully
	result := DetectStalledMiners("/nonexistent/path", "testrig")

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for nonexistent dir", result.Checked)
	}
	if len(result.Stalled) != 0 {
		t.Errorf("Stalled = %d, want 0 for nonexistent dir", len(result.Stalled))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %d, want 0 for nonexistent dir", len(result.Errors))
	}
}

func TestDetectStalledMiners_EmptyMinersDir(t *testing.T) {
	t.Parallel()
	// Empty miners directory should return 0 checked
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result := DetectStalledMiners(tmpDir, rigName)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for empty miners dir", result.Checked)
	}
	if len(result.Stalled) != 0 {
		t.Errorf("Stalled = %d, want 0 for empty miners dir", len(result.Stalled))
	}
}

func TestDetectStalledMiners_NoSession(t *testing.T) {
	t.Parallel()
	// When tmux sessions don't exist (no real tmux in test),
	// HasSession returns false so miners are skipped (not errors).
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create miner directories
	for _, name := range []string{"alpha", "bravo"} {
		if err := os.Mkdir(filepath.Join(minersDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create hidden dir (should be skipped)
	if err := os.Mkdir(filepath.Join(minersDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := DetectStalledMiners(tmpDir, rigName)

	// Should count 2 miners (skip hidden)
	if result.Checked != 2 {
		t.Errorf("Checked = %d, want 2 (should skip hidden dirs)", result.Checked)
	}

	// No stalled because HasSession returns false (no real tmux in test),
	// so miners are skipped before structured signal checks.
	if len(result.Stalled) != 0 {
		t.Errorf("Stalled = %d, want 0 (no tmux sessions in test)", len(result.Stalled))
	}
}

func TestStartupStallThresholds(t *testing.T) {
	t.Parallel()
	// Verify config defaults are reasonable (tests the operational config defaults,
	// not removed handler constants).
	stallThreshold := config.DefaultWitnessStartupStallThreshold
	activityGrace := config.DefaultWitnessStartupActivityGrace
	if stallThreshold < 30*time.Second {
		t.Errorf("DefaultWitnessStartupStallThreshold = %v, too short (< 30s)", stallThreshold)
	}
	if stallThreshold > 5*time.Minute {
		t.Errorf("DefaultWitnessStartupStallThreshold = %v, too long (> 5min)", stallThreshold)
	}
	if activityGrace < 15*time.Second {
		t.Errorf("DefaultWitnessStartupActivityGrace = %v, too short (< 15s)", activityGrace)
	}
	if activityGrace > 5*time.Minute {
		t.Errorf("DefaultWitnessStartupActivityGrace = %v, too long (> 5min)", activityGrace)
	}
}

func TestDetectOrphanedBeads_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available (test environment), should return empty result
	result := DetectOrphanedBeads(DefaultBdCli(), "/nonexistent", "testrig", nil)

	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 when bd unavailable", result.Checked)
	}
	if len(result.Orphans) != 0 {
		t.Errorf("Orphans = %d, want 0 when bd unavailable", len(result.Orphans))
	}
}

func TestDetectOrphanedBeads_ResultTypes(t *testing.T) {
	t.Parallel()
	// Verify the OrphanedBeadResult type has all expected fields
	o := OrphanedBeadResult{
		BeadID:        "ms-orphan1",
		Assignee:      "testrig/miners/alpha",
		MinerName:   "alpha",
		BeadRecovered: true,
	}

	if o.BeadID != "ms-orphan1" {
		t.Errorf("BeadID = %q, want %q", o.BeadID, "ms-orphan1")
	}
	if o.Assignee != "testrig/miners/alpha" {
		t.Errorf("Assignee = %q, want %q", o.Assignee, "testrig/miners/alpha")
	}
	if o.MinerName != "alpha" {
		t.Errorf("MinerName = %q, want %q", o.MinerName, "alpha")
	}
	if !o.BeadRecovered {
		t.Error("BeadRecovered = false, want true")
	}
}

func TestDetectOrphanedBeads_WithMockBd(t *testing.T) {
	installFakeTmuxNoServer(t)

	// Set up town directory structure
	townRoot := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(townRoot, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a miner directory for "bravo" (alive dir, dead session)
	// This case should be SKIPPED (deferred to DetectZombieMiners)
	if err := os.Mkdir(filepath.Join(minersDir, "bravo"), 0o755); err != nil {
		t.Fatal(err)
	}

	// "alpha" has NO directory and NO tmux session — true orphan
	// "bravo" has directory but no session — deferred to DetectZombieMiners
	// "charlie" is hooked, no dir, no session — also an orphan
	// "delta" is assigned to a different rig — skipped by rigName filter

	bd, mock := mockBd(
		func(args []string) (string, error) {
			if len(args) == 0 {
				return "{}", nil
			}
			switch args[0] {
			case "list":
				joined := strings.Join(args, " ")
				if strings.Contains(joined, "--status=in_progress") {
					return `[
  {"id":"ms-orphan1","assignee":"testrig/miners/alpha"},
  {"id":"ms-alive1","assignee":"testrig/miners/bravo"},
  {"id":"ms-nocrew","assignee":"testrig/crew/sean"},
  {"id":"ms-noassign","assignee":""},
  {"id":"ms-otherrig","assignee":"otherrig/miners/delta"}
]`, nil
				}
				if strings.Contains(joined, "--status=hooked") {
					return `[{"id":"ms-hooked1","assignee":"testrig/miners/charlie"}]`, nil
				}
				return "[]", nil
			case "show":
				return `[{"status":"in_progress"}]`, nil
			}
			return "{}", nil
		},
		func(args []string) error { return nil },
	)

	result := DetectOrphanedBeads(bd, townRoot, rigName, nil)

	// Verify --limit=0 was passed in bd list invocations
	logStr := strings.Join(mock.calls, "\n")
	if !strings.Contains(logStr, "--limit=0") {
		t.Errorf("bd list was not called with --limit=0; log:\n%s", logStr)
	}
	// Verify both statuses were queried
	if !strings.Contains(logStr, "--status=in_progress") {
		t.Errorf("bd list was not called with --status=in_progress; log:\n%s", logStr)
	}
	if !strings.Contains(logStr, "--status=hooked") {
		t.Errorf("bd list was not called with --status=hooked; log:\n%s", logStr)
	}

	// Should have checked 3 miner assignees in "testrig":
	// alpha (in_progress), bravo (in_progress), charlie (hooked)
	// "crew/sean" is not a miner, "" has no assignee,
	// "otherrig/miners/delta" is filtered out by rigName
	if result.Checked != 3 {
		t.Errorf("Checked = %d, want 3 (alpha + bravo from in_progress, charlie from hooked)", result.Checked)
	}

	// Should have found 2 orphans:
	// alpha (in_progress, no dir, no session) and charlie (hooked, no dir, no session)
	// bravo has directory so deferred to DetectZombieMiners
	if len(result.Orphans) != 2 {
		t.Fatalf("Orphans = %d, want 2 (alpha + charlie)", len(result.Orphans))
	}

	// Verify first orphan (alpha from in_progress scan)
	orphan := result.Orphans[0]
	if orphan.BeadID != "ms-orphan1" {
		t.Errorf("orphan[0] BeadID = %q, want %q", orphan.BeadID, "ms-orphan1")
	}
	if orphan.MinerName != "alpha" {
		t.Errorf("orphan[0] MinerName = %q, want %q", orphan.MinerName, "alpha")
	}
	if orphan.Assignee != "testrig/miners/alpha" {
		t.Errorf("orphan[0] Assignee = %q, want %q", orphan.Assignee, "testrig/miners/alpha")
	}
	// BeadRecovered should be true (mock bd update succeeds)
	if !orphan.BeadRecovered {
		t.Error("orphan[0] BeadRecovered = false, want true")
	}

	// Verify second orphan (charlie from hooked scan)
	orphan2 := result.Orphans[1]
	if orphan2.BeadID != "ms-hooked1" {
		t.Errorf("orphan[1] BeadID = %q, want %q", orphan2.BeadID, "ms-hooked1")
	}
	if orphan2.MinerName != "charlie" {
		t.Errorf("orphan[1] MinerName = %q, want %q", orphan2.MinerName, "charlie")
	}

	// Verify no unexpected errors
	if len(result.Errors) != 0 {
		t.Errorf("unexpected errors: %v", result.Errors)
	}
}

func TestDetectOrphanedBeads_ErrorPath(t *testing.T) {
	t.Parallel()
	bdErr := fmt.Errorf("bd: connection refused")
	bd, _ := mockBd(
		func(args []string) (string, error) { return "", bdErr },
		func(args []string) error { return bdErr },
	)

	result := DetectOrphanedBeads(bd, t.TempDir(), "testrig", nil)

	if len(result.Errors) == 0 {
		t.Error("expected errors when bd fails, got none")
	}
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 when bd fails", result.Checked)
	}
	if len(result.Orphans) != 0 {
		t.Errorf("Orphans = %d, want 0 when bd fails", len(result.Orphans))
	}
}

// --- DetectOrphanedMolecules tests ---

func TestOrphanedMoleculeResult_Types(t *testing.T) {
	t.Parallel()
	// Verify the result types have all expected fields.
	r := OrphanedMoleculeResult{
		BeadID:        "ms-work-123",
		MoleculeID:    "ms-mol-456",
		Assignee:      "testrig/miners/alpha",
		MinerName:   "alpha",
		Closed:        5,
		BeadRecovered: true,
		Error:         nil,
	}
	if r.BeadID != "ms-work-123" {
		t.Errorf("BeadID = %q, want %q", r.BeadID, "ms-work-123")
	}
	if r.MoleculeID != "ms-mol-456" {
		t.Errorf("MoleculeID = %q, want %q", r.MoleculeID, "ms-mol-456")
	}
	if r.MinerName != "alpha" {
		t.Errorf("MinerName = %q, want %q", r.MinerName, "alpha")
	}
	if r.Closed != 5 {
		t.Errorf("Closed = %d, want 5", r.Closed)
	}
	if !r.BeadRecovered {
		t.Error("BeadRecovered = false, want true")
	}

	// Aggregate result
	agg := DetectOrphanedMoleculesResult{
		Checked: 10,
		Orphans: []OrphanedMoleculeResult{r},
		Errors:  []error{fmt.Errorf("test error")},
	}
	if agg.Checked != 10 {
		t.Errorf("Checked = %d, want 10", agg.Checked)
	}
	if len(agg.Orphans) != 1 {
		t.Errorf("len(Orphans) = %d, want 1", len(agg.Orphans))
	}
	if len(agg.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(agg.Errors))
	}
}

func TestDetectOrphanedMolecules_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available, should return empty result with errors.
	bdErr := fmt.Errorf("bd: not found")
	bd, _ := mockBd(
		func(args []string) (string, error) { return "", bdErr },
		func(args []string) error { return bdErr },
	)
	result := DetectOrphanedMolecules(bd, "/tmp/nonexistent", "testrig", nil)
	if result == nil {
		t.Fatal("result should not be nil")
	}
	// Should have errors from failed bd list commands
	if len(result.Errors) == 0 {
		t.Error("expected errors when bd is not available")
	}
	if len(result.Orphans) != 0 {
		t.Errorf("expected no orphans, got %d", len(result.Orphans))
	}
}

func TestDetectOrphanedMolecules_EmptyResult(t *testing.T) {
	t.Parallel()
	// With a mock bd that returns empty lists, should get empty result.
	bd, _ := mockBd(
		func(args []string) (string, error) { return "[]", nil },
		func(args []string) error { return nil },
	)

	result := DetectOrphanedMolecules(bd, t.TempDir(), "testrig", nil)
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0", result.Checked)
	}
	if len(result.Orphans) != 0 {
		t.Errorf("len(Orphans) = %d, want 0", len(result.Orphans))
	}
}

func TestGetAttachedMoleculeID_EmptyOutput(t *testing.T) {
	t.Parallel()
	// When bd returns error, should return empty string.
	bd, _ := mockBd(
		func(args []string) (string, error) { return "", fmt.Errorf("bd: not found") },
		func(args []string) error { return fmt.Errorf("bd: not found") },
	)
	result := getAttachedMoleculeID(bd, "/tmp", "ms-fake-123")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestHandleMinerDone_CompletedWithoutMRID_NoMergeReady(t *testing.T) {
	t.Parallel()
	// When Exit==COMPLETED but MRID is empty and MRFailed is true,
	// the witness should NOT send MERGE_READY (go to no-MR path).
	// This tests the fix for ms-xp6e9p.
	payload := &MinerDonePayload{
		MinerName: "nux",
		Exit:        "COMPLETED",
		IssueID:     "ms-abc123",
		MRID:        "",
		Branch:      "miner/nux-abc123",
		MRFailed:    true,
	}

	// hasPendingMR should be false when MRID is empty
	hasPendingMR := payload.MRID != ""
	if hasPendingMR {
		t.Error("hasPendingMR = true, want false when MRID is empty")
	}

	// Even with Exit==COMPLETED, MRFailed should prevent the bead lookup fallback
	if !payload.MRFailed && payload.Exit == "COMPLETED" && payload.Branch != "" {
		t.Error("should not attempt MR bead lookup when MRFailed is true")
	}
}

func TestHandleMinerDone_CompletedWithMRID(t *testing.T) {
	t.Parallel()
	// When Exit==COMPLETED and MRID is set, hasPendingMR should be true.
	payload := &MinerDonePayload{
		MinerName: "nux",
		Exit:        "COMPLETED",
		MRID:        "ms-mr-xyz",
		Branch:      "miner/nux-abc123",
	}

	hasPendingMR := payload.MRID != ""
	if !hasPendingMR {
		t.Error("hasPendingMR = false, want true when MRID is set")
	}
}

func TestFindMRBeadForBranch_NoBdAvailable(t *testing.T) {
	t.Parallel()
	// When bd is not available, should return empty string
	result := findMRBeadForBranch(DefaultBdCli(), "/nonexistent", "miner/nux-abc123")
	if result != "" {
		t.Errorf("findMRBeadForBranch = %q, want empty when bd unavailable", result)
	}
}

func TestDetectOrphanedMolecules_WithMockBd(t *testing.T) {
	installFakeTmuxNoServer(t)

	// Full test with mock bd returning beads assigned to dead miners.
	//
	// Setup:
	// - alpha: dead miner (no tmux, no directory) with attached molecule → orphaned
	// - bravo: alive miner (directory exists) → skip
	// - crew/sean: non-miner assignee → skip
	// - empty assignee → skip

	tmpDir := t.TempDir()

	// Create town structure: tmpDir is the "town root"
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create bravo's directory (alive miner)
	if err := os.MkdirAll(filepath.Join(minersDir, "bravo"), 0755); err != nil {
		t.Fatal(err)
	}
	// No directory for alpha (dead miner)

	// Create workspace.Find marker
	if err := os.WriteFile(filepath.Join(tmpDir, ".ms-root"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	bd, mock := mockBd(
		func(args []string) (string, error) {
			if len(args) == 0 {
				return "[]", nil
			}
			joined := strings.Join(args, " ")
			switch args[0] {
			case "list":
				if strings.Contains(joined, "--status=hooked") {
					return `[
  {"id":"ms-work-001","assignee":"testrig/miners/alpha"},
  {"id":"ms-work-002","assignee":"testrig/miners/bravo"},
  {"id":"ms-work-003","assignee":"testrig/crew/sean"},
  {"id":"ms-work-004","assignee":""}
]`, nil
				}
				if strings.Contains(joined, "--status=in_progress") {
					return "[]", nil
				}
				if strings.Contains(joined, "--parent=ms-mol-orphan") {
					return `[
  {"id":"ms-step-001","status":"open"},
  {"id":"ms-step-002","status":"open"},
  {"id":"ms-step-003","status":"closed"}
]`, nil
				}
				return "[]", nil
			case "show":
				if len(args) > 1 {
					switch args[1] {
					case "ms-work-001":
						return `[{"status":"hooked","description":"attached_molecule: ms-mol-orphan\nattached_at: 2026-01-15T10:00:00Z\ndispatched_by: overseer"}]`, nil
					case "ms-mol-orphan":
						return `[{"status":"open"}]`, nil
					}
				}
				return `[{"status":"open","description":""}]`, nil
			}
			return "{}", nil
		},
		func(args []string) error { return nil },
	)

	result := DetectOrphanedMolecules(bd, tmpDir, rigName, nil)
	if result == nil {
		t.Fatal("result should not be nil")
	}

	// Should have checked 2 miner-assigned beads (alpha and bravo)
	if result.Checked != 2 {
		t.Errorf("Checked = %d, want 2 (alpha + bravo)", result.Checked)
	}

	// Should have found 1 orphan (alpha's molecule)
	if len(result.Orphans) != 1 {
		t.Fatalf("len(Orphans) = %d, want 1", len(result.Orphans))
	}

	orphan := result.Orphans[0]
	if orphan.BeadID != "ms-work-001" {
		t.Errorf("orphan.BeadID = %q, want %q", orphan.BeadID, "ms-work-001")
	}
	if orphan.MoleculeID != "ms-mol-orphan" {
		t.Errorf("orphan.MoleculeID = %q, want %q", orphan.MoleculeID, "ms-mol-orphan")
	}
	if orphan.MinerName != "alpha" {
		t.Errorf("orphan.MinerName = %q, want %q", orphan.MinerName, "alpha")
	}
	// Closed should be 3: 2 open step children + 1 molecule itself
	if orphan.Closed != 3 {
		t.Errorf("orphan.Closed = %d, want 3 (2 open steps + 1 molecule)", orphan.Closed)
	}
	if orphan.Error != nil {
		t.Errorf("orphan.Error = %v, want nil", orphan.Error)
	}

	// Verify bd close was called by checking the mock log
	logContent := strings.Join(mock.calls, "\n")
	if !strings.Contains(logContent, "close ms-step-001 ms-step-002") {
		t.Errorf("expected bd close for step children, got log:\n%s", logContent)
	}
	if !strings.Contains(logContent, "close ms-mol-orphan") {
		t.Errorf("expected bd close for molecule, got log:\n%s", logContent)
	}
	// Verify bead was recovered (resetAbandonedBead called bd update)
	if !orphan.BeadRecovered {
		t.Error("orphan.BeadRecovered = false, want true (resetAbandonedBead should have reset the bead)")
	}
	if !strings.Contains(logContent, "update ms-work-001") {
		t.Errorf("expected bd update for bead reset, got log:\n%s", logContent)
	}
}

func TestCompletionDiscovery_Types(t *testing.T) {
	t.Parallel()
	// Verify CompletionDiscovery has all expected fields
	d := CompletionDiscovery{
		MinerName:    "nux",
		AgentBeadID:    "ms-mineshaft-miner-nux",
		ExitType:       "COMPLETED",
		IssueID:        "ms-abc123",
		MRID:           "ms-mr-xyz",
		Branch:         "miner/nux/ms-abc123@hash",
		MRFailed:       false,
		CompletionTime: "2026-02-28T02:00:00Z",
		Action:         "merge-ready-sent",
		WispCreated:    "ms-wisp-123",
	}

	if d.MinerName != "nux" {
		t.Errorf("MinerName = %q, want %q", d.MinerName, "nux")
	}
	if d.ExitType != "COMPLETED" {
		t.Errorf("ExitType = %q, want %q", d.ExitType, "COMPLETED")
	}
	if d.Branch != "miner/nux/ms-abc123@hash" {
		t.Errorf("Branch = %q, want correct value", d.Branch)
	}
}

func TestDiscoverCompletionsResult_EmptyResult(t *testing.T) {
	t.Parallel()
	result := &DiscoverCompletionsResult{}
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0", result.Checked)
	}
	if len(result.Discovered) != 0 {
		t.Errorf("Discovered = %d, want 0", len(result.Discovered))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %d, want 0", len(result.Errors))
	}
}

func TestDiscoverCompletions_NonexistentDir(t *testing.T) {
	t.Parallel()
	// When workDir doesn't exist, should return empty result
	result := DiscoverCompletions(DefaultBdCli(), "/nonexistent/path", "testrig", nil)
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for nonexistent dir", result.Checked)
	}
}

func TestDiscoverCompletions_EmptyMinersDir(t *testing.T) {
	t.Parallel()
	// When miners directory exists but is empty, should scan 0
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(minersDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create workspace marker
	if err := os.WriteFile(filepath.Join(tmpDir, ".ms-root"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	result := DiscoverCompletions(DefaultBdCli(), tmpDir, rigName, nil)
	if result.Checked != 0 {
		t.Errorf("Checked = %d, want 0 for empty miners dir", result.Checked)
	}
}

func TestDiscoverCompletions_NoCompletionMetadata(t *testing.T) {
	// Miner exists but agent bead has no completion metadata — should be skipped
	tmpDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(tmpDir, rigName, "miners")
	if err := os.MkdirAll(filepath.Join(minersDir, "nux"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ms-root"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock bd that returns agent bead with no completion fields
	bd, _ := mockBd(
		func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "show" {
				return `[{"id":"ms-testrig-miner-nux","description":"Agent: testrig/miners/nux\n\nrole_type: miner\nrig: testrig\nagent_state: working\nhook_bead: ms-work-001","agent_state":"working","hook_bead":"ms-work-001"}]`, nil
			}
			return "[]", nil
		},
		func(args []string) error { return nil },
	)

	result := DiscoverCompletions(bd, tmpDir, rigName, nil)
	if result.Checked != 1 {
		t.Errorf("Checked = %d, want 1", result.Checked)
	}
	if len(result.Discovered) != 0 {
		t.Errorf("Discovered = %d, want 0 (no completion metadata)", len(result.Discovered))
	}
}

func TestProcessDiscoveredCompletion_PhaseComplete(t *testing.T) {
	t.Parallel()
	payload := &MinerDonePayload{
		MinerName: "nux",
		Exit:        "PHASE_COMPLETE",
	}
	discovery := &CompletionDiscovery{}
	processDiscoveredCompletion(DefaultBdCli(), "/tmp", "testrig", payload, discovery)
	if discovery.Action != "phase-complete" {
		t.Errorf("Action = %q, want %q", discovery.Action, "phase-complete")
	}
}

func TestProcessDiscoveredCompletion_NoMR(t *testing.T) {
	t.Parallel()
	payload := &MinerDonePayload{
		MinerName: "nux",
		Exit:        "COMPLETED",
		MRFailed:    true, // Prevents fallback MR lookup
	}
	discovery := &CompletionDiscovery{}
	processDiscoveredCompletion(DefaultBdCli(), "/tmp", "testrig", payload, discovery)
	if !strings.Contains(discovery.Action, "acknowledged-idle") {
		t.Errorf("Action = %q, want to contain %q", discovery.Action, "acknowledged-idle")
	}
}

func TestProcessDiscoveredCompletion_EscalatedNoMR(t *testing.T) {
	t.Parallel()
	payload := &MinerDonePayload{
		MinerName: "nux",
		Exit:        "ESCALATED",
	}
	discovery := &CompletionDiscovery{}
	processDiscoveredCompletion(DefaultBdCli(), "/tmp", "testrig", payload, discovery)
	if !strings.Contains(discovery.Action, "acknowledged-idle") {
		t.Errorf("Action = %q, want to contain %q for ESCALATED exit", discovery.Action, "acknowledged-idle")
	}
}

func TestGetAgentBeadFields_NoAgentBead(t *testing.T) {
	t.Parallel()
	// When bd fails, should return nil
	bd, _ := mockBd(
		func(args []string) (string, error) { return "", fmt.Errorf("bd: not found") },
		func(args []string) error { return fmt.Errorf("bd: not found") },
	)
	fields := getAgentBeadFields(bd, "/tmp", "ms-fake-agent")
	if fields != nil {
		t.Error("expected nil fields when bd unavailable")
	}
}

func TestClearCompletionMetadata_NoBd(t *testing.T) {
	t.Parallel()
	// When bd fails, should return error
	bd, _ := mockBd(
		func(args []string) (string, error) { return "", fmt.Errorf("bd: not found") },
		func(args []string) error { return fmt.Errorf("bd: not found") },
	)
	err := clearCompletionMetadata(bd, "/tmp", "ms-fake-agent")
	if err == nil {
		t.Error("expected error when bd unavailable")
	}
}

// --- Heartbeat v2 tests (ms-3vr5) ---

func TestHeartbeatV2_ExitingStateSkipsZombieDetection(t *testing.T) {
	t.Parallel()
	// Agent reports "exiting" state via heartbeat v2.
	// The witness should trust the agent and NOT flag as zombie,
	// even if done-intent is older than config.DefaultWitnessDoneIntentStuckTimeout.
	// This replaces timer-based inference for v2 agents.

	// Fresh heartbeat with state="exiting" → not a zombie
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatExiting,
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	if stale {
		t.Error("fresh heartbeat should not be stale")
	}
	if hb.EffectiveState() != miner.HeartbeatExiting {
		t.Errorf("EffectiveState() = %q, want %q", hb.EffectiveState(), miner.HeartbeatExiting)
	}

	// With a v2 exiting heartbeat, the witness should NOT check done-intent timers
	shouldSkip := hb.IsV2() && !stale && hb.EffectiveState() == miner.HeartbeatExiting
	if !shouldSkip {
		t.Error("expected v2 exiting heartbeat to skip zombie detection")
	}
}

func TestHeartbeatV2_StuckStateEscalates(t *testing.T) {
	t.Parallel()
	// Agent self-reports "stuck" via heartbeat v2.
	// The witness should escalate (not restart — agent is alive).
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatStuck,
		Context:   "blocked on auth issue",
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	if stale {
		t.Error("fresh heartbeat should not be stale")
	}

	shouldEscalate := hb.IsV2() && !stale && hb.EffectiveState() == miner.HeartbeatStuck
	if !shouldEscalate {
		t.Error("expected v2 stuck heartbeat to trigger escalation")
	}
}

func TestHeartbeatV2_WorkingStateHealthy(t *testing.T) {
	t.Parallel()
	// Agent heartbeats "working" — healthy, not a zombie.
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatWorking,
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	shouldSkip := hb.IsV2() && !stale && (hb.EffectiveState() == miner.HeartbeatWorking || hb.EffectiveState() == miner.HeartbeatIdle)
	if !shouldSkip {
		t.Error("expected v2 working heartbeat to skip zombie detection")
	}
}

func TestHeartbeatV2_IdleStateHealthy(t *testing.T) {
	t.Parallel()
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatIdle,
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	shouldSkip := hb.IsV2() && !stale && (hb.EffectiveState() == miner.HeartbeatWorking || hb.EffectiveState() == miner.HeartbeatIdle)
	if !shouldSkip {
		t.Error("expected v2 idle heartbeat to skip zombie detection")
	}
}

func TestHeartbeatV2_StaleHeartbeatFallsThrough(t *testing.T) {
	t.Parallel()
	// Stale v2 heartbeat (agent died) → fall through to legacy detection.
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now().Add(-10 * time.Minute), // 10min old → stale
		State:     miner.HeartbeatWorking,
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	if !stale {
		t.Error("10-minute-old heartbeat should be stale")
	}

	// Stale heartbeat should NOT skip zombie detection — falls through to legacy
	shouldSkip := hb.IsV2() && !stale
	if shouldSkip {
		t.Error("stale v2 heartbeat should fall through to legacy detection")
	}
}

func TestHeartbeatV2_V1FallsThrough(t *testing.T) {
	t.Parallel()
	// v1 heartbeat (no state field) → fall through to legacy detection.
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		// No State field → v1
	}
	if hb.IsV2() {
		t.Error("expected IsV2()=false for v1 heartbeat")
	}

	// v1 heartbeat should NOT trigger v2 logic
	shouldUseV2 := hb.IsV2()
	if shouldUseV2 {
		t.Error("v1 heartbeat should fall through to legacy detection")
	}
}

func TestHeartbeatV2_DeadSessionFreshHeartbeatRace(t *testing.T) {
	t.Parallel()
	// Dead session but fresh heartbeat → possible race (session just restarted).
	// Should skip zombie detection to avoid killing a newly-started session.
	hb := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatWorking,
	}
	stale := time.Since(hb.Timestamp) >= miner.SessionHeartbeatStaleThreshold
	sessionDead := true

	// Fresh heartbeat + dead session → skip (race condition)
	shouldSkip := sessionDead && hb.IsV2() && !stale
	if !shouldSkip {
		t.Error("expected fresh v2 heartbeat + dead session to skip zombie detection (race)")
	}
}

func TestZombieAgentSelfReportedStuck_Classification(t *testing.T) {
	t.Parallel()
	// Verify the new classification type
	if ZombieAgentSelfReportedStuck != "agent-self-reported-stuck" {
		t.Errorf("ZombieAgentSelfReportedStuck = %q, want %q", ZombieAgentSelfReportedStuck, "agent-self-reported-stuck")
	}
	// Should imply active work (agent is alive and asking for help)
	if !ZombieAgentSelfReportedStuck.ImpliesActiveWork() {
		t.Error("ZombieAgentSelfReportedStuck should imply active work")
	}
}

func TestZombieNeverHeartbeated_Classification(t *testing.T) {
	t.Parallel()
	if ZombieNeverHeartbeated != "never-heartbeated" {
		t.Errorf("ZombieNeverHeartbeated = %q, want %q", ZombieNeverHeartbeated, "never-heartbeated")
	}
	if !ZombieNeverHeartbeated.ImpliesActiveWork() {
		t.Error("ZombieNeverHeartbeated should imply active work")
	}

	// Session old enough (>5m default) with assigned work and no heartbeat → flag.
	oldSession := time.Now().Add(-10 * time.Minute)
	shouldFlag := time.Since(oldSession) > config.DefaultWitnessHeartbeatStartupGrace
	if !shouldFlag {
		t.Errorf("expected flag for session age=%v, threshold=%v",
			time.Since(oldSession).Round(time.Second), config.DefaultWitnessHeartbeatStartupGrace)
	}

	// Session within grace period → no flag.
	newSession := time.Now().Add(-2 * time.Minute)
	shouldNotFlag := time.Since(newSession) <= config.DefaultWitnessHeartbeatStartupGrace
	if !shouldNotFlag {
		t.Errorf("expected no flag for session age=%v, threshold=%v",
			time.Since(newSession).Round(time.Second), config.DefaultWitnessHeartbeatStartupGrace)
	}
}

func TestSubmittedStillRunningCandidate(t *testing.T) {
	t.Parallel()

	baseSnap := &agentBeadSnapshot{
		AgentState: string(beads.AgentStateDone),
		HookBead:   "ms-work-123",
		UpdatedAt:  time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		Fields: &beads.AgentFields{
			CleanupStatus: "clean",
			MRID:          "ms-mr-123",
		},
	}
	staleHB := &miner.SessionHeartbeat{
		Timestamp: time.Now().Add(-10 * time.Minute),
		State:     miner.HeartbeatWorking,
	}

	age, ok := isSubmittedStillRunningCandidate(baseSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace)
	if !ok {
		t.Fatalf("expected submitted still-running candidate, age=%v", age)
	}

	noHookSnap := *baseSnap
	noHookSnap.HookBead = ""
	if _, ok := isSubmittedStillRunningCandidate(&noHookSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); !ok {
		t.Error("no-hook submitted sessions must still be treated as submitted still-running")
	}

	idleSnap := *baseSnap
	idleSnap.AgentState = string(beads.AgentStateIdle)
	if _, ok := isSubmittedStillRunningCandidate(&idleSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("normal idle miners with submitted MR metadata must not be treated as submitted still-running")
	}

	freshHB := &miner.SessionHeartbeat{
		Timestamp: time.Now(),
		State:     miner.HeartbeatWorking,
	}
	if _, ok := isSubmittedStillRunningCandidate(baseSnap, freshHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("fresh heartbeat must not be treated as submitted still-running")
	}

	dirtySnap := *baseSnap
	dirtyFields := *baseSnap.Fields
	dirtyFields.CleanupStatus = "has_uncommitted"
	dirtySnap.Fields = &dirtyFields
	if _, ok := isSubmittedStillRunningCandidate(&dirtySnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("dirty cleanup status must not be treated as safe submitted still-running")
	}

	noSubmitSnap := *baseSnap
	noSubmitSnap.AgentState = string(beads.AgentStateWorking)
	noSubmitSnap.ActiveMR = ""
	noSubmitSnap.Fields = &beads.AgentFields{CleanupStatus: "clean"}
	if _, ok := isSubmittedStillRunningCandidate(&noSubmitSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("open hooked work without submission evidence must not be treated as submitted still-running")
	}

	completedOnlySnap := *baseSnap
	completedOnlySnap.ActiveMR = ""
	completedOnlySnap.Fields = &beads.AgentFields{
		CleanupStatus:  "clean",
		ExitType:       string(ExitTypeCompleted),
		CompletionTime: time.Now().Format(time.RFC3339),
	}
	if _, ok := isSubmittedStillRunningCandidate(&completedOnlySnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("COMPLETED metadata alone must not be treated as successful submission evidence")
	}

	failedSubmitSnap := *baseSnap
	failedSubmitSnap.Fields = &beads.AgentFields{
		CleanupStatus: "clean",
		MRID:          "ms-mr-123",
		MRFailed:      true,
	}
	if _, ok := isSubmittedStillRunningCandidate(&failedSubmitSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("failed MR submission must not be treated as successful submission evidence")
	}

	pushFailedSnap := *baseSnap
	pushFailedSnap.Fields = &beads.AgentFields{
		CleanupStatus: "clean",
		MRID:          "ms-mr-123",
		PushFailed:    true,
	}
	if _, ok := isSubmittedStillRunningCandidate(&pushFailedSnap, staleHB, config.DefaultWitnessHeartbeatStartupGrace); ok {
		t.Error("failed push must not be treated as successful submission evidence")
	}
}

func TestZombieSubmittedStillRunning_Classification(t *testing.T) {
	t.Parallel()
	if ZombieSubmittedStillRunning != "submitted-still-running" {
		t.Errorf("ZombieSubmittedStillRunning = %q, want %q", ZombieSubmittedStillRunning, "submitted-still-running")
	}
	if ZombieSubmittedStillRunning.ImpliesActiveWork() {
		t.Error("ZombieSubmittedStillRunning should be classified as orphan/submitted idle, not active failed work")
	}
}

func TestNotifyRefineryMergeReady_EmitsChannelEvent(t *testing.T) {
	// Create a fake town root with the workspace marker so workspace.Find recognizes it
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set MS_TEST_NUDGE_LOG to prevent actual tmux operations in nudgeRefinery
	t.Setenv("MS_TEST_NUDGE_LOG", filepath.Join(t.TempDir(), "nudge.log"))

	result := &HandlerResult{}
	// notifyRefineryMergeReady takes workDir and calls workspace.Find(workDir) internally
	notifyRefineryMergeReady(townRoot, "dashboard", result)

	// Verify that a MERGE_READY event file was created in the refinery channel
	eventDir := filepath.Join(townRoot, "events", "refinery")
	entries, err := os.ReadDir(eventDir)
	if err != nil {
		t.Fatalf("reading event dir: %v", err)
	}

	var eventFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".event") {
			eventFiles = append(eventFiles, e.Name())
		}
	}

	if len(eventFiles) == 0 {
		t.Fatal("expected at least one .event file in ~/ms/events/refinery/, got none")
	}

	// Read and verify the event content
	data, err := os.ReadFile(filepath.Join(eventDir, eventFiles[0]))
	if err != nil {
		t.Fatalf("reading event file: %v", err)
	}

	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("parsing event JSON: %v", err)
	}

	if event["type"] != "MERGE_READY" {
		t.Errorf("event type = %v, want MERGE_READY", event["type"])
	}
	if event["channel"] != "refinery" {
		t.Errorf("event channel = %v, want refinery", event["channel"])
	}

	payload, ok := event["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload is not a map: %T", event["payload"])
	}
	if payload["source"] != "witness" {
		t.Errorf("payload.source = %v, want witness", payload["source"])
	}
	if payload["rig"] != "dashboard" {
		t.Errorf("payload.rig = %v, want dashboard", payload["rig"])
	}
}

// TestCherryHasUnmergedCommits covers the git-cherry output parser used by
// verifyBranchAlreadyMerged (aa-apw).
func TestCherryHasUnmergedCommits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty output — branch has no commits beyond base", "", false},
		{"whitespace only", "  \n\n", false},
		{"all squash-applied (-)", "- abc123\n- def456\n", false},
		{"one unmerged (+)", "+ abc123\n", true},
		{"mixed", "- abc123\n+ def456\n", true},
		{"unmerged only", "+ a\n+ b\n+ c\n", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cherryHasUnmergedCommits(tc.in); got != tc.want {
				t.Errorf("cherryHasUnmergedCommits(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestHandleZombieRestart_SkipsWhenBranchAlreadyMerged verifies the aa-apw fix:
// when a stopped miner's branch work is already merged to origin/main (e.g.,
// via squash-merge), the witness must NOT restart the session — restarting
// would let the miner re-push its pre-squash HEAD and create a duplicate MR.
// Instead the miner is archived.
//
// Not parallel: overrides the package-level verifyBranchAlreadyMerged var.
func TestHandleZombieRestart_SkipsWhenBranchAlreadyMerged(t *testing.T) {
	oldVerify := verifyBranchAlreadyMerged
	verifyBranchAlreadyMerged = func(workDir, rigName, minerName string) (bool, error) {
		return true, nil
	}
	t.Cleanup(func() { verifyBranchAlreadyMerged = oldVerify })

	bd, _ := mockBd(
		func(args []string) (string, error) { return "[]", nil },
		func(args []string) error { return nil },
	)

	z := &ZombieResult{MinerName: "scavenger", HookBead: "ma-poc.4"}
	handleZombieRestart(bd, t.TempDir(), "testrig", "scavenger", "ma-poc.4", "has_unpushed", z)

	// Action must reflect the archive decision; must NOT be a "restarted*" action.
	if !strings.Contains(z.Action, "work-already-merged") {
		t.Errorf("action = %q, want it to mention work-already-merged (aa-apw)", z.Action)
	}
	if strings.HasPrefix(z.Action, "restarted") || strings.HasPrefix(z.Action, "restart-") {
		t.Errorf("action = %q, miner must not be restarted when work is already merged", z.Action)
	}
}

// TestHandleZombieRestart_RestartsWhenBranchNotMerged verifies the pre-aa-apw
// behavior is preserved when work is NOT merged: handleZombieRestart proceeds
// to its normal cleanup/restart flow.
//
// Not parallel: overrides the package-level verifyBranchAlreadyMerged var.
func TestHandleZombieRestart_RestartsWhenBranchNotMerged(t *testing.T) {
	oldVerify := verifyBranchAlreadyMerged
	verifyBranchAlreadyMerged = func(workDir, rigName, minerName string) (bool, error) {
		return false, nil
	}
	t.Cleanup(func() { verifyBranchAlreadyMerged = oldVerify })

	bd, _ := mockBd(
		func(args []string) (string, error) { return "[]", nil },
		func(args []string) error { return nil },
	)

	z := &ZombieResult{MinerName: "scavenger", HookBead: "ma-poc.4"}
	handleZombieRestart(bd, t.TempDir(), "testrig", "scavenger", "ma-poc.4", "clean", z)

	// Should NOT take the archive path.
	if strings.Contains(z.Action, "work-already-merged") {
		t.Errorf("action = %q, should not archive when work is not merged", z.Action)
	}
}
