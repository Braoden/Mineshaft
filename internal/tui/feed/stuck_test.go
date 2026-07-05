package feed

import (
	"fmt"
	"testing"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
)

// mockHealthSource is a test double for HealthDataSource
type mockHealthSource struct {
	agents     map[string]*beads.Issue
	sessions   map[string]bool
	listErr    error
	sessionErr error // if set, IsSessionAlive returns this error
}

func newMockHealthSource() *mockHealthSource {
	return &mockHealthSource{
		agents:   make(map[string]*beads.Issue),
		sessions: make(map[string]bool),
	}
}

func (m *mockHealthSource) ListAgentBeads() (map[string]*beads.Issue, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.agents, nil
}

func (m *mockHealthSource) IsSessionAlive(sessionName string) (bool, error) {
	if m.sessionErr != nil {
		return false, m.sessionErr
	}
	return m.sessions[sessionName], nil
}

// TestAgentStateString tests the String() method for all AgentState values
func TestAgentStateString(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "gupp"},
		{StateStalled, "stalled"},
		{StateWorking, "working"},
		{StateIdle, "idle"},
		{StateZombie, "zombie"},
		{AgentState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("AgentState(%d).String() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestAgentStatePriority tests that priorities are ordered correctly
func TestAgentStatePriority(t *testing.T) {
	if StateGUPPViolation.Priority() >= StateStalled.Priority() {
		t.Error("GUPP violation should have higher priority than stalled")
	}
	if StateStalled.Priority() >= StateWorking.Priority() {
		t.Error("Stalled should have higher priority than working")
	}
	if StateWorking.Priority() >= StateIdle.Priority() {
		t.Error("Working should have higher priority than idle")
	}
	if StateIdle.Priority() >= StateZombie.Priority() {
		t.Error("Idle should have higher priority than zombie")
	}
}

// TestAgentStateNeedsAttention tests which states require user attention
func TestAgentStateNeedsAttention(t *testing.T) {
	needsAttention := []AgentState{
		StateGUPPViolation,
		StateStalled,
		StateZombie,
	}
	noAttention := []AgentState{
		StateWorking,
		StateIdle,
	}

	for _, state := range needsAttention {
		if !state.NeedsAttention() {
			t.Errorf("%s.NeedsAttention() = false, want true", state)
		}
	}
	for _, state := range noAttention {
		if state.NeedsAttention() {
			t.Errorf("%s.NeedsAttention() = true, want false", state)
		}
	}
}

// TestAgentStateSymbol tests the display symbols
func TestAgentStateSymbol(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "🔥"},
		{StateStalled, "⚠"},
		{StateWorking, "●"},
		{StateIdle, "○"},
		{StateZombie, "💀"},
		{AgentState(99), "?"},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := tt.state.Symbol(); got != tt.expected {
				t.Errorf("AgentState(%d).Symbol() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestAgentStateLabel tests the display labels
func TestAgentStateLabel(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected string
	}{
		{StateGUPPViolation, "GUPP!"},
		{StateStalled, "STALL"},
		{StateWorking, "work"},
		{StateIdle, "idle"},
		{StateZombie, "dead"},
		{AgentState(99), "???"},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if got := tt.state.Label(); got != tt.expected {
				t.Errorf("AgentState(%d).Label() = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// TestIsGUPPViolation tests the GUPP violation detection
func TestIsGUPPViolation(t *testing.T) {
	tests := []struct {
		name          string
		hasHookedWork bool
		minutes       int
		expected      bool
	}{
		{"no work, no time", false, 0, false},
		{"no work, long time", false, 60, false},
		{"has work, short time", true, 10, false},
		{"has work, at threshold", true, 30, true},
		{"has work, over threshold", true, 45, true},
		{"has work, just under threshold", true, 29, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGUPPViolation(tt.hasHookedWork, tt.minutes); got != tt.expected {
				t.Errorf("IsGUPPViolation(%v, %d) = %v, want %v",
					tt.hasHookedWork, tt.minutes, got, tt.expected)
			}
		})
	}
}

// TestProblemAgentDurationDisplay tests the human-readable duration formatting
func TestProblemAgentDurationDisplay(t *testing.T) {
	tests := []struct {
		minutes  int
		expected string
	}{
		{0, "<1m"},
		{1, "1m"},
		{5, "5m"},
		{59, "59m"},
		{60, "1h"},
		{61, "1h1m"},
		{90, "1h30m"},
		{120, "2h"},
		{125, "2h5m"},
		{180, "3h"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			agent := &ProblemAgent{IdleMinutes: tt.minutes}
			if got := agent.DurationDisplay(); got != tt.expected {
				t.Errorf("ProblemAgent{IdleMinutes: %d}.DurationDisplay() = %q, want %q",
					tt.minutes, got, tt.expected)
			}
		})
	}
}

// TestProblemAgentNeedsAttention tests the NeedsAttention delegation
func TestProblemAgentNeedsAttention(t *testing.T) {
	tests := []struct {
		state    AgentState
		expected bool
	}{
		{StateGUPPViolation, true},
		{StateStalled, true},
		{StateZombie, true},
		{StateWorking, false},
		{StateIdle, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			agent := &ProblemAgent{State: tt.state}
			if got := agent.NeedsAttention(); got != tt.expected {
				t.Errorf("ProblemAgent{State: %s}.NeedsAttention() = %v, want %v",
					tt.state, got, tt.expected)
			}
		})
	}
}

// TestThresholdConstants verifies the threshold constants are reasonable
func TestThresholdConstants(t *testing.T) {
	if GUPPViolationMinutes != 30 {
		t.Errorf("GUPPViolationMinutes = %d, want 30", GUPPViolationMinutes)
	}
	if StalledThresholdMinutes != 15 {
		t.Errorf("StalledThresholdMinutes = %d, want 15", StalledThresholdMinutes)
	}
	if GUPPViolationMinutes <= StalledThresholdMinutes {
		t.Error("GUPP violation threshold should be longer than stalled threshold")
	}
}

// TestCheckAll_GUPPViolation tests that agents with hook + >30min stale are detected as GUPP
func TestCheckAll_GUPPViolation(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Toast"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Toast",
		HookBead:  "ms-abc12",
		UpdatedAt: time.Now().Add(-45 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Toast"] = true // session alive

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State != StateGUPPViolation {
		t.Errorf("expected StateGUPPViolation, got %s", agents[0].State)
	}
	if !agents[0].HasHookedWork {
		t.Error("expected HasHookedWork to be true")
	}
	if agents[0].Name != "Toast" {
		t.Errorf("expected name 'Toast', got %q", agents[0].Name)
	}
}

// TestCheckAll_Stalled tests that agents with hook + >15min stale are detected as stalled
func TestCheckAll_Stalled(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Pearl"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Pearl",
		HookBead:  "ms-def34",
		UpdatedAt: time.Now().Add(-20 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Pearl"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State != StateStalled {
		t.Errorf("expected StateStalled, got %s", agents[0].State)
	}
}

// TestCheckAll_Working tests that agents with hook + recent update are working
func TestCheckAll_Working(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Max"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Max",
		HookBead:  "ms-xyz89",
		UpdatedAt: time.Now().Add(-2 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Max"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State != StateWorking {
		t.Errorf("expected StateWorking, got %s", agents[0].State)
	}
}

// TestCheckAll_Idle tests that agents with no hook are idle
func TestCheckAll_Idle(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Joe"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Joe",
		HookBead:  "", // no hooked work
		UpdatedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Joe"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State != StateIdle {
		t.Errorf("expected StateIdle, got %s", agents[0].State)
	}
}

// TestCheckAll_Zombie tests that agents with dead sessions are zombies
func TestCheckAll_Zombie(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Dead"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Dead",
		HookBead:  "ms-work1",
		UpdatedAt: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
	}
	// session NOT alive (not in mock.sessions)

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State != StateZombie {
		t.Errorf("expected StateZombie, got %s", agents[0].State)
	}
}

// TestCheckAll_MultipleAgents tests sorting with multiple agents in different states
func TestCheckAll_MultipleAgents(t *testing.T) {
	mock := newMockHealthSource()
	now := time.Now()

	// GUPP violation agent
	mock.agents["ms-mineshaft-miner-Stuck"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Stuck",
		HookBead:  "ms-work1",
		UpdatedAt: now.Add(-40 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Stuck"] = true

	// Working agent
	mock.agents["ms-mineshaft-miner-Happy"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Happy",
		HookBead:  "ms-work2",
		UpdatedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Happy"] = true

	// Idle agent
	mock.agents["ms-mineshaft-miner-Lazy"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Lazy",
		HookBead:  "",
		UpdatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-Lazy"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	// Should be sorted: GUPP first, then Working, then Idle
	if agents[0].State != StateGUPPViolation {
		t.Errorf("first agent should be GUPP violation, got %s", agents[0].State)
	}
	if agents[1].State != StateWorking {
		t.Errorf("second agent should be Working, got %s", agents[1].State)
	}
	if agents[2].State != StateIdle {
		t.Errorf("third agent should be Idle, got %s", agents[2].State)
	}
}

// TestCheckAll_Empty tests with no agent beads
func TestCheckAll_Empty(t *testing.T) {
	mock := newMockHealthSource()

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

// TestCheckAll_ListError tests error handling when ListAgentBeads fails
func TestCheckAll_ListError(t *testing.T) {
	mock := newMockHealthSource()
	mock.listErr = beads.ErrNotInstalled

	detector := NewStuckDetectorWithSource(mock)
	_, err := detector.CheckAll()
	if err == nil {
		t.Error("expected error from CheckAll")
	}
}

// TestCheckAll_TownLevelAgent tests detection of town-level agents (overseer, supervisor)
func TestCheckAll_TownLevelAgent(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["hq-overseer"] = &beads.Issue{
		ID:        "hq-overseer",
		HookBead:  "",
		UpdatedAt: time.Now().Add(-3 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["hq-overseer"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Role != "overseer" {
		t.Errorf("expected role 'overseer', got %q", agents[0].Role)
	}
	if agents[0].SessionID != "hq-overseer" {
		t.Errorf("expected session 'hq-overseer', got %q", agents[0].SessionID)
	}
	if agents[0].State != StateIdle {
		t.Errorf("expected StateIdle, got %s", agents[0].State)
	}
}

// TestCheckAll_RigSingleton tests detection of rig-level singletons (witness, refinery)
func TestCheckAll_RigSingleton(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-witness"] = &beads.Issue{
		ID:        "ms-mineshaft-witness",
		HookBead:  "",
		UpdatedAt: time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-witness"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Role != "witness" {
		t.Errorf("expected role 'witness', got %q", agents[0].Role)
	}
	if agents[0].Rig != "mineshaft" {
		t.Errorf("expected rig 'mineshaft', got %q", agents[0].Rig)
	}
	if agents[0].SessionID != "ms-witness" {
		t.Errorf("expected session 'ms-witness', got %q", agents[0].SessionID)
	}
}

// TestCheckAll_CrewAgent tests detection of crew agents
func TestCheckAll_CrewAgent(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-crew-joe"] = &beads.Issue{
		ID:        "ms-mineshaft-crew-joe",
		HookBead:  "ms-task1",
		UpdatedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	mock.sessions["ms-crew-joe"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].Role != "crew" {
		t.Errorf("expected role 'crew', got %q", agents[0].Role)
	}
	if agents[0].SessionID != "ms-crew-joe" {
		t.Errorf("expected session 'ms-crew-joe', got %q", agents[0].SessionID)
	}
	if agents[0].State != StateWorking {
		t.Errorf("expected StateWorking, got %s", agents[0].State)
	}
}

// TestDeriveSessionName tests the session name derivation for all agent types
func TestDeriveSessionName(t *testing.T) {
	tests := []struct {
		name     string
		rig      string
		role     string
		agentNm  string
		expected string
	}{
		{"overseer", "", "overseer", "", "hq-overseer"},
		{"supervisor", "", "supervisor", "", "hq-supervisor"},
		{"witness", "mineshaft", "witness", "", "ms-witness"},
		{"refinery", "mineshaft", "refinery", "", "ms-refinery"},
		{"crew", "mineshaft", "crew", "joe", "ms-crew-joe"},
		{"miner", "mineshaft", "miner", "Toast", "ms-Toast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSessionName(tt.rig, tt.role, tt.agentNm)
			if got != tt.expected {
				t.Errorf("deriveSessionName(%q, %q, %q) = %q, want %q",
					tt.rig, tt.role, tt.agentNm, got, tt.expected)
			}
		})
	}
}

// TestCheckAll_InvalidBeadID tests that invalid bead IDs are skipped
func TestCheckAll_InvalidBeadID(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["invalid-id"] = &beads.Issue{
		ID:        "invalid-id",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	// Invalid bead ID should be skipped (ParseAgentBeadID returns ok=false for single-char prefix)
	// "invalid-id" has prefix "invalid" which is > 3 chars, so ParseAgentBeadID will return false
	if len(agents) != 0 {
		t.Errorf("expected 0 agents for invalid bead ID, got %d", len(agents))
	}
}

// TestCheckAll_SessionError tests that IsSessionAlive errors don't cause false zombies
func TestCheckAll_SessionError(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Alpha"] = &beads.Issue{
		ID:        "ms-mineshaft-miner-Alpha",
		HookBead:  "ms-work1",
		UpdatedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	// Session error (e.g., tmux socket contention) - should NOT mark as zombie
	mock.sessionErr = fmt.Errorf("tmux: socket not found")

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].State == StateZombie {
		t.Errorf("agent should NOT be zombie when IsSessionAlive returns error, got %s", agents[0].State)
	}
	// Should be Working (has hook, 5 min idle < 15 min stalled threshold)
	if agents[0].State != StateWorking {
		t.Errorf("expected StateWorking, got %s", agents[0].State)
	}
}

// TestCheckAll_RalphcatNotStalled tests that a ralphcat with 45min idle is NOT stalled
// (would be stalled for a normal miner at the 15min threshold)
func TestCheckAll_RalphcatNotStalled(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Ralph"] = &beads.Issue{
		ID:       "ms-mineshaft-miner-Ralph",
		HookBead: "ms-abc12",
		// 45 minutes idle — stalled for normal miner, but fine for ralphcat
		UpdatedAt: time.Now().Add(-45 * time.Minute).Format(time.RFC3339),
		// Description contains mode: ralph (agent fields)
		Description: "Miner Ralph\n\nrole_type: miner\nrig: mineshaft\nagent_state: working\nhook_bead: ms-abc12\ncleanup_status: null\nactive_mr: null\nnotification_level: null\nmode: ralph",
	}
	mock.sessions["ms-Ralph"] = true // session alive

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	// At 45 min, normal miner would be GUPP (>=30min). Ralphcat threshold is 240min.
	// At 45 min idle, ralphcat should be Working (< 120min stalled threshold).
	if agents[0].State != StateWorking {
		t.Errorf("expected StateWorking for ralphcat at 45min idle, got %s", agents[0].State)
	}
}

// TestCheckAll_RalphcatStalled tests that a ralphcat IS stalled after 2+ hours
func TestCheckAll_RalphcatStalled(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Ralph2"] = &beads.Issue{
		ID:          "ms-mineshaft-miner-Ralph2",
		HookBead:    "ms-def34",
		UpdatedAt:   time.Now().Add(-150 * time.Minute).Format(time.RFC3339), // 2.5 hours
		Description: "Miner Ralph2\n\nrole_type: miner\nrig: mineshaft\nagent_state: working\nhook_bead: ms-def34\ncleanup_status: null\nactive_mr: null\nnotification_level: null\nmode: ralph",
	}
	mock.sessions["ms-Ralph2"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	// 150 min > 120 min stalled threshold for ralphcat
	if agents[0].State != StateStalled {
		t.Errorf("expected StateStalled for ralphcat at 150min idle, got %s", agents[0].State)
	}
}

// TestCheckAll_RalphcatGUPP tests that a ralphcat with 5h idle IS in GUPP violation
func TestCheckAll_RalphcatGUPP(t *testing.T) {
	mock := newMockHealthSource()
	mock.agents["ms-mineshaft-miner-Ralph3"] = &beads.Issue{
		ID:          "ms-mineshaft-miner-Ralph3",
		HookBead:    "ms-ghi56",
		UpdatedAt:   time.Now().Add(-300 * time.Minute).Format(time.RFC3339), // 5 hours
		Description: "Miner Ralph3\n\nrole_type: miner\nrig: mineshaft\nagent_state: working\nhook_bead: ms-ghi56\ncleanup_status: null\nactive_mr: null\nnotification_level: null\nmode: ralph",
	}
	mock.sessions["ms-Ralph3"] = true

	detector := NewStuckDetectorWithSource(mock)
	agents, err := detector.CheckAll()
	if err != nil {
		t.Fatalf("CheckAll: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	// 300 min > 240 min GUPP threshold for ralphcat
	if agents[0].State != StateGUPPViolation {
		t.Errorf("expected StateGUPPViolation for ralphcat at 300min idle, got %s", agents[0].State)
	}
}

// TestIsRalphMode tests the ralph mode detection from agent bead description
func TestIsRalphMode(t *testing.T) {
	tests := []struct {
		name     string
		issue    *beads.Issue
		expected bool
	}{
		{
			name:     "nil issue",
			issue:    nil,
			expected: false,
		},
		{
			name:     "empty description",
			issue:    &beads.Issue{Description: ""},
			expected: false,
		},
		{
			name:     "no mode field",
			issue:    &beads.Issue{Description: "role_type: miner\nrig: mineshaft"},
			expected: false,
		},
		{
			name:     "mode ralph",
			issue:    &beads.Issue{Description: "role_type: miner\nmode: ralph"},
			expected: true,
		},
		{
			name:     "mode other",
			issue:    &beads.Issue{Description: "role_type: miner\nmode: other"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRalphMode(tt.issue); got != tt.expected {
				t.Errorf("isRalphMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestNudgeTarget tests the nudge target format for all agent types
func TestNudgeTarget(t *testing.T) {
	tests := []struct {
		name     string
		agent    *ProblemAgent
		expected string
	}{
		{
			name:     "overseer",
			agent:    &ProblemAgent{Role: "overseer", Name: "overseer", Rig: ""},
			expected: "overseer",
		},
		{
			name:     "supervisor",
			agent:    &ProblemAgent{Role: "supervisor", Name: "supervisor", Rig: ""},
			expected: "supervisor",
		},
		{
			name:     "witness",
			agent:    &ProblemAgent{Role: "witness", Name: "witness", Rig: "mineshaft"},
			expected: "mineshaft/witness",
		},
		{
			name:     "refinery",
			agent:    &ProblemAgent{Role: "refinery", Name: "refinery", Rig: "mineshaft"},
			expected: "mineshaft/refinery",
		},
		{
			name:     "crew",
			agent:    &ProblemAgent{Role: "crew", Name: "joe", Rig: "mineshaft"},
			expected: "mineshaft/crew/joe",
		},
		{
			name:     "miner",
			agent:    &ProblemAgent{Role: "miner", Name: "Toast", Rig: "mineshaft"},
			expected: "mineshaft/Toast",
		},
		{
			name:     "unknown role falls back to session ID",
			agent:    &ProblemAgent{Role: "custom", Name: "x", Rig: "r", SessionID: "ms-r-custom-x"},
			expected: "ms-r-custom-x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nudgeTarget(tt.agent)
			if got != tt.expected {
				t.Errorf("nudgeTarget() = %q, want %q", got, tt.expected)
			}
		})
	}
}
