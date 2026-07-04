package overseer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/excavation/internal/config"
	"github.com/steveyegge/excavation/internal/workspace"
)

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-town")
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", m.townRoot, "/tmp/test-town")
	}
}

func TestManager_overseerDir(t *testing.T) {
	m := NewManager("/tmp/test-town")
	got := m.overseerDir()
	want := filepath.Join("/tmp/test-town", "overseer")
	if got != want {
		t.Errorf("overseerDir() = %q, want %q", got, want)
	}
}

func TestSessionName_ReturnsConsistentValue(t *testing.T) {
	name := SessionName()
	if name == "" {
		t.Error("SessionName() returned empty string")
	}
	// Verify idempotent
	if SessionName() != name {
		t.Error("SessionName() returned different values on subsequent calls")
	}
}

func TestManager_SessionName_MatchesPackageFunc(t *testing.T) {
	m := NewManager("/tmp/test-town")
	if m.SessionName() != SessionName() {
		t.Errorf("Manager.SessionName() = %q, SessionName() = %q — should match",
			m.SessionName(), SessionName())
	}
}

func TestManager_Errors(t *testing.T) {
	if ErrNotRunning.Error() != "overseer not running" {
		t.Errorf("ErrNotRunning = %q", ErrNotRunning)
	}
	if ErrAlreadyRunning.Error() != "overseer already running" {
		t.Errorf("ErrAlreadyRunning = %q", ErrAlreadyRunning)
	}
}

func TestGetOverseerPrime(t *testing.T) {
	// Create a temporary directory with town.json
	tmpDir, err := os.MkdirTemp("", "overseer-prime-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create overseer directory
	overseerDir := filepath.Join(tmpDir, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatalf("failed to create overseer dir: %v", err)
	}

	// Create a minimal town.json
	townConfig := &config.TownConfig{
		Name: "test-town",
	}
	townConfigPath := filepath.Join(tmpDir, workspace.PrimaryMarker)
	if err := config.SaveTownConfig(townConfigPath, townConfig); err != nil {
		t.Fatalf("failed to save town config: %v", err)
	}

	// Test GetOverseerPrime
	content, err := GetOverseerPrime(tmpDir)
	if err != nil {
		t.Fatalf("GetOverseerPrime failed: %v", err)
	}

	// Verify content has expected elements
	if !strings.Contains(content, "[prime-rendered-at:") {
		t.Error("GetOverseerPrime should contain timestamp marker")
	}
	if !strings.Contains(content, "# Overseer Context") {
		t.Error("GetOverseerPrime should render overseer template")
	}
	if !strings.Contains(content, tmpDir) {
		t.Error("GetOverseerPrime should contain town root path")
	}
}

func TestGetOverseerPrime_InvalidTownRoot(t *testing.T) {
	// Test with non-existent directory - should still return content
	// (town name defaults to "unknown" on error)
	content, err := GetOverseerPrime("/nonexistent/path")
	if err != nil {
		t.Fatalf("GetOverseerPrime should not fail with invalid town root: %v", err)
	}

	// Should still have the template content
	if !strings.Contains(content, "# Overseer Context") {
		t.Error("GetOverseerPrime should render overseer template even with invalid town root")
	}
}
