package miner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTouchAndReadSessionHeartbeat(t *testing.T) {
	townRoot := t.TempDir()

	// No heartbeat initially
	hb := ReadSessionHeartbeat(townRoot, "ms-test-session")
	if hb != nil {
		t.Fatal("expected nil heartbeat before touch")
	}

	// Touch heartbeat
	TouchSessionHeartbeat(townRoot, "ms-test-session")

	// Read it back
	hb = ReadSessionHeartbeat(townRoot, "ms-test-session")
	if hb == nil {
		t.Fatal("expected non-nil heartbeat after touch")
	}

	if time.Since(hb.Timestamp) > 5*time.Second {
		t.Errorf("heartbeat timestamp too old: %v", hb.Timestamp)
	}

	// v2: TouchSessionHeartbeat writes state="working" by default (ms-3vr5)
	if hb.State != HeartbeatWorking {
		t.Errorf("heartbeat state = %q, want %q", hb.State, HeartbeatWorking)
	}
}

func TestTouchSessionHeartbeatWithState(t *testing.T) {
	townRoot := t.TempDir()

	TouchSessionHeartbeatWithState(townRoot, "ms-test-state", HeartbeatExiting, "ms done", "ms-abc123")

	hb := ReadSessionHeartbeat(townRoot, "ms-test-state")
	if hb == nil {
		t.Fatal("expected non-nil heartbeat after touch with state")
	}

	if hb.State != HeartbeatExiting {
		t.Errorf("state = %q, want %q", hb.State, HeartbeatExiting)
	}
	if hb.Context != "ms done" {
		t.Errorf("context = %q, want %q", hb.Context, "ms done")
	}
	if hb.Bead != "ms-abc123" {
		t.Errorf("bead = %q, want %q", hb.Bead, "ms-abc123")
	}
}

func TestSessionHeartbeat_EffectiveState(t *testing.T) {
	tests := []struct {
		name  string
		state HeartbeatState
		want  HeartbeatState
	}{
		{"empty (v1 compat)", "", HeartbeatWorking},
		{"working", HeartbeatWorking, HeartbeatWorking},
		{"idle", HeartbeatIdle, HeartbeatIdle},
		{"exiting", HeartbeatExiting, HeartbeatExiting},
		{"stuck", HeartbeatStuck, HeartbeatStuck},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hb := &SessionHeartbeat{State: tt.state}
			if got := hb.EffectiveState(); got != tt.want {
				t.Errorf("EffectiveState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionHeartbeat_IsV2(t *testing.T) {
	// v1 heartbeat (no state)
	v1 := &SessionHeartbeat{Timestamp: time.Now()}
	if v1.IsV2() {
		t.Error("expected IsV2()=false for v1 heartbeat")
	}

	// v2 heartbeat (has state)
	v2 := &SessionHeartbeat{Timestamp: time.Now(), State: HeartbeatWorking}
	if !v2.IsV2() {
		t.Error("expected IsV2()=true for v2 heartbeat")
	}
}

func TestIsSessionHeartbeatStale_NoFile(t *testing.T) {
	townRoot := t.TempDir()

	stale, exists := IsSessionHeartbeatStale(townRoot, "nonexistent")
	if exists {
		t.Error("expected exists=false for missing heartbeat")
	}
	if stale {
		t.Error("expected stale=false for missing heartbeat")
	}
}

func TestIsSessionHeartbeatStale_Fresh(t *testing.T) {
	townRoot := t.TempDir()

	TouchSessionHeartbeat(townRoot, "ms-test-fresh")

	stale, exists := IsSessionHeartbeatStale(townRoot, "ms-test-fresh")
	if !exists {
		t.Error("expected exists=true for fresh heartbeat")
	}
	if stale {
		t.Error("expected stale=false for fresh heartbeat")
	}
}

func TestIsSessionHeartbeatStale_Old(t *testing.T) {
	townRoot := t.TempDir()

	// Write a heartbeat with an old timestamp
	dir := filepath.Join(townRoot, ".runtime", "heartbeats")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-10 * time.Minute).UTC()
	data := []byte(`{"timestamp":"` + oldTime.Format(time.RFC3339Nano) + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "ms-test-stale.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	stale, exists := IsSessionHeartbeatStale(townRoot, "ms-test-stale")
	if !exists {
		t.Error("expected exists=true for old heartbeat")
	}
	if !stale {
		t.Error("expected stale=true for 10-minute-old heartbeat")
	}
}

func TestRemoveSessionHeartbeat(t *testing.T) {
	townRoot := t.TempDir()

	TouchSessionHeartbeat(townRoot, "ms-test-remove")

	// Verify it exists
	hb := ReadSessionHeartbeat(townRoot, "ms-test-remove")
	if hb == nil {
		t.Fatal("expected heartbeat to exist before removal")
	}

	// Remove it
	RemoveSessionHeartbeat(townRoot, "ms-test-remove")

	// Verify it's gone
	hb = ReadSessionHeartbeat(townRoot, "ms-test-remove")
	if hb != nil {
		t.Error("expected nil heartbeat after removal")
	}
}

func TestRemoveSessionHeartbeat_NoopOnMissing(t *testing.T) {
	townRoot := t.TempDir()
	// Should not panic or error on missing file
	RemoveSessionHeartbeat(townRoot, "nonexistent")
}

func TestIsSessionProcessDead_HeartbeatFresh(t *testing.T) {
	townRoot := t.TempDir()
	sessionName := "ms-test-hb-alive"

	// Touch a fresh heartbeat — isSessionProcessDead should return false
	TouchSessionHeartbeat(townRoot, sessionName)

	dead := isSessionProcessDead(nil, sessionName, townRoot)
	if dead {
		t.Error("expected alive (dead=false) for session with fresh heartbeat")
	}
}

func TestIsSessionProcessDead_HeartbeatStale(t *testing.T) {
	townRoot := t.TempDir()
	sessionName := "ms-test-hb-dead"

	// Write a stale heartbeat
	dir := filepath.Join(townRoot, ".runtime", "heartbeats")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * time.Minute).UTC()
	data := []byte(`{"timestamp":"` + oldTime.Format(time.RFC3339Nano) + `"}`)
	if err := os.WriteFile(filepath.Join(dir, sessionName+".json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	dead := isSessionProcessDead(nil, sessionName, townRoot)
	if !dead {
		t.Error("expected dead=true for session with stale heartbeat")
	}
}

func TestIsSessionProcessDead_EmptyTownRoot(t *testing.T) {
	// With empty townRoot, heartbeat check is skipped entirely.
	// This tests backward compatibility when townRoot isn't available.
	// We can't test the full PID fallback without a real tmux session,
	// but we verify no panic with empty townRoot.
	sessionName := "ms-test-no-townroot"

	// Empty townRoot skips heartbeat, falls through to PID check.
	// Can't test PID path without tmux, but verify heartbeat path is skipped.
	stale, exists := IsSessionHeartbeatStale("", sessionName)
	if exists {
		t.Error("expected exists=false with empty townRoot")
	}
	if stale {
		t.Error("expected stale=false with empty townRoot")
	}
}

func TestReadSessionHeartbeat_V1BackwardsCompat(t *testing.T) {
	townRoot := t.TempDir()

	// Write a v1 heartbeat (timestamp only, no state field)
	dir := filepath.Join(townRoot, ".runtime", "heartbeats")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	ts := time.Now().UTC()
	data := []byte(`{"timestamp":"` + ts.Format(time.RFC3339Nano) + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "ms-test-v1.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	hb := ReadSessionHeartbeat(townRoot, "ms-test-v1")
	if hb == nil {
		t.Fatal("expected non-nil heartbeat for v1 format")
	}

	// State should be empty (v1)
	if hb.State != "" {
		t.Errorf("v1 heartbeat state = %q, want empty", hb.State)
	}

	// IsV2 should return false
	if hb.IsV2() {
		t.Error("expected IsV2()=false for v1 heartbeat")
	}

	// EffectiveState should default to working
	if hb.EffectiveState() != HeartbeatWorking {
		t.Errorf("v1 EffectiveState() = %q, want %q", hb.EffectiveState(), HeartbeatWorking)
	}
}

func TestReadSessionHeartbeat_V2AllStates(t *testing.T) {
	townRoot := t.TempDir()

	dir := filepath.Join(townRoot, ".runtime", "heartbeats")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	states := []HeartbeatState{HeartbeatWorking, HeartbeatIdle, HeartbeatExiting, HeartbeatStuck}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			session := "ms-test-v2-" + string(state)
			hb := SessionHeartbeat{
				Timestamp: time.Now().UTC(),
				State:     state,
				Context:   "test context",
				Bead:      "ms-test-bead",
			}
			data, err := json.Marshal(hb)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, session+".json"), data, 0644); err != nil {
				t.Fatal(err)
			}

			read := ReadSessionHeartbeat(townRoot, session)
			if read == nil {
				t.Fatal("expected non-nil heartbeat")
			}
			if read.State != state {
				t.Errorf("state = %q, want %q", read.State, state)
			}
			if !read.IsV2() {
				t.Error("expected IsV2()=true")
			}
			if read.EffectiveState() != state {
				t.Errorf("EffectiveState() = %q, want %q", read.EffectiveState(), state)
			}
			if read.Context != "test context" {
				t.Errorf("context = %q, want %q", read.Context, "test context")
			}
			if read.Bead != "ms-test-bead" {
				t.Errorf("bead = %q, want %q", read.Bead, "ms-test-bead")
			}
		})
	}
}
