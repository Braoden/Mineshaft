package doctor

import (
	"testing"
)

func TestStalledMinerCheck_Properties(t *testing.T) {
	check := NewStalledMinerCheck()

	if check.Name() != "stalled-miners" {
		t.Errorf("Name() = %q, want %q", check.Name(), "stalled-miners")
	}

	if check.Description() == "" {
		t.Error("Description() should not be empty")
	}

	if !check.CanFix() {
		t.Error("CanFix() should be true — stalled miners can have branches pushed")
	}

	if check.Category() != CategoryCleanup {
		t.Errorf("Category() = %q, want %q", check.Category(), CategoryCleanup)
	}
}

func TestStalledMinerCheck_EmptyTownRoot(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := &CheckContext{TownRoot: tmpDir}

	check := NewStalledMinerCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK for empty town root", result.Status)
	}
}

func TestStalledMinerCheck_NoMiners(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: "testrig"}

	check := NewStalledMinerCheck()
	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK when no miners dir exists", result.Status)
	}
}

func TestStalledMinerCheck_FixNoStalled(t *testing.T) {
	check := NewStalledMinerCheck()
	// Fix with no stalled miners should be a no-op
	if err := check.Fix(&CheckContext{TownRoot: t.TempDir()}); err != nil {
		t.Errorf("Fix() with no stalled miners returned error: %v", err)
	}
}

func TestStalledMinerCheck_ResolveClonePath_NoDir(t *testing.T) {
	check := NewStalledMinerCheck()
	path := check.resolveClonePath(t.TempDir(), "testrig", "furiosa")
	if path != "" {
		t.Errorf("resolveClonePath() = %q, want empty for nonexistent", path)
	}
}
