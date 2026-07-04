package supervisor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFeedStrandedStateFile(t *testing.T) {
	got := FeedStrandedStateFile("/tmp/town")
	want := filepath.Join("/tmp/town", "supervisor", "feed-stranded-state.json")
	if got != want {
		t.Errorf("FeedStrandedStateFile = %q, want %q", got, want)
	}
}

func TestLoadFeedStrandedState_FileNotExist(t *testing.T) {
	state, err := LoadFeedStrandedState(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Minecarts == nil {
		t.Fatal("expected initialized Minecarts map")
	}
	if len(state.Minecarts) != 0 {
		t.Errorf("expected empty Minecarts, got %d", len(state.Minecarts))
	}
}

func TestSaveThenLoadFeedStrandedState(t *testing.T) {
	tmpDir := t.TempDir()
	// Ensure supervisor dir exists
	os.MkdirAll(filepath.Join(tmpDir, "supervisor"), 0755)

	state := &FeedStrandedState{
		Minecarts: map[string]*MinecartFeedState{
			"hq-cv-test1": {
				MinecartID:     "hq-cv-test1",
				FeedCount:    2,
				LastFeedTime: time.Now().UTC().Add(-5 * time.Minute),
			},
		},
	}

	if err := SaveFeedStrandedState(tmpDir, state); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := LoadFeedStrandedState(tmpDir)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if len(loaded.Minecarts) != 1 {
		t.Fatalf("expected 1 minecart, got %d", len(loaded.Minecarts))
	}

	cs := loaded.Minecarts["hq-cv-test1"]
	if cs == nil {
		t.Fatal("missing hq-cv-test1")
	}
	if cs.FeedCount != 2 {
		t.Errorf("FeedCount = %d, want 2", cs.FeedCount)
	}
}

func TestMinecartFeedState_IsInCooldown(t *testing.T) {
	tests := []struct {
		name     string
		lastFeed time.Time
		cooldown time.Duration
		want     bool
	}{
		{
			name:     "zero time, not in cooldown",
			lastFeed: time.Time{},
			cooldown: 10 * time.Minute,
			want:     false,
		},
		{
			name:     "recent feed, in cooldown",
			lastFeed: time.Now().Add(-2 * time.Minute),
			cooldown: 10 * time.Minute,
			want:     true,
		},
		{
			name:     "old feed, not in cooldown",
			lastFeed: time.Now().Add(-20 * time.Minute),
			cooldown: 10 * time.Minute,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &MinecartFeedState{LastFeedTime: tt.lastFeed}
			if got := s.IsInCooldown(tt.cooldown); got != tt.want {
				t.Errorf("IsInCooldown() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinecartFeedState_CooldownRemaining(t *testing.T) {
	// Zero time = no cooldown
	s := &MinecartFeedState{}
	if got := s.CooldownRemaining(10 * time.Minute); got != 0 {
		t.Errorf("expected 0 remaining for zero time, got %v", got)
	}

	// Expired cooldown
	s.LastFeedTime = time.Now().Add(-20 * time.Minute)
	if got := s.CooldownRemaining(10 * time.Minute); got != 0 {
		t.Errorf("expected 0 remaining for expired cooldown, got %v", got)
	}

	// Active cooldown
	s.LastFeedTime = time.Now().Add(-2 * time.Minute)
	remaining := s.CooldownRemaining(10 * time.Minute)
	if remaining <= 0 || remaining > 10*time.Minute {
		t.Errorf("expected remaining between 0 and 10m, got %v", remaining)
	}
}

func TestMinecartFeedState_RecordFeed(t *testing.T) {
	s := &MinecartFeedState{MinecartID: "hq-cv-test"}

	if s.FeedCount != 0 {
		t.Errorf("initial FeedCount = %d, want 0", s.FeedCount)
	}

	s.RecordFeed()
	if s.FeedCount != 1 {
		t.Errorf("after RecordFeed, FeedCount = %d, want 1", s.FeedCount)
	}
	if s.LastFeedTime.IsZero() {
		t.Error("LastFeedTime should be set after RecordFeed")
	}

	s.RecordFeed()
	if s.FeedCount != 2 {
		t.Errorf("after second RecordFeed, FeedCount = %d, want 2", s.FeedCount)
	}
}

func TestGetMinecartState_CreatesNew(t *testing.T) {
	state := &FeedStrandedState{
		Minecarts: make(map[string]*MinecartFeedState),
	}

	cs := state.GetMinecartState("hq-cv-new")
	if cs == nil {
		t.Fatal("expected non-nil MinecartFeedState")
	}
	if cs.MinecartID != "hq-cv-new" {
		t.Errorf("MinecartID = %q, want %q", cs.MinecartID, "hq-cv-new")
	}
	if cs.FeedCount != 0 {
		t.Errorf("FeedCount = %d, want 0", cs.FeedCount)
	}
}

func TestGetMinecartState_ReturnsExisting(t *testing.T) {
	state := &FeedStrandedState{
		Minecarts: map[string]*MinecartFeedState{
			"hq-cv-exist": {MinecartID: "hq-cv-exist", FeedCount: 5},
		},
	}

	cs := state.GetMinecartState("hq-cv-exist")
	if cs.FeedCount != 5 {
		t.Errorf("FeedCount = %d, want 5", cs.FeedCount)
	}
}

func TestGetMinecartState_NilMap(t *testing.T) {
	state := &FeedStrandedState{}

	cs := state.GetMinecartState("hq-cv-test")
	if cs == nil {
		t.Fatal("expected non-nil MinecartFeedState even with nil map")
	}
	if state.Minecarts == nil {
		t.Fatal("Minecarts map should be initialized")
	}
}

func TestLoadFeedStrandedState_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "supervisor")
	os.MkdirAll(stateDir, 0755)

	// Write invalid JSON
	stateFile := filepath.Join(stateDir, "feed-stranded-state.json")
	os.WriteFile(stateFile, []byte("not json"), 0600)

	_, err := LoadFeedStrandedState(tmpDir)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
}

func TestSaveFeedStrandedState_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't pre-create supervisor dir — SaveFeedStrandedState should create it

	state := &FeedStrandedState{
		Minecarts: make(map[string]*MinecartFeedState),
	}

	if err := SaveFeedStrandedState(tmpDir, state); err != nil {
		t.Fatalf("save error: %v", err)
	}

	// Verify file was created
	stateFile := FeedStrandedStateFile(tmpDir)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("state file not created")
	}
}
