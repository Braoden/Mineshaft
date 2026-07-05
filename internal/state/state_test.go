// ABOUTME: Tests for global state management.
// ABOUTME: Verifies enable/disable toggle and XDG path resolution.

package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "mineshaft")

	os.Unsetenv("XDG_STATE_HOME")
	if got := StateDir(); got != expected {
		t.Errorf("StateDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_STATE_HOME", "/custom/state")
	defer os.Unsetenv("XDG_STATE_HOME")
	if got := filepath.ToSlash(StateDir()); got != "/custom/state/mineshaft" {
		t.Errorf("StateDir() with XDG = %q, want /custom/state/mineshaft", got)
	}
}

func TestConfigDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "mineshaft")

	os.Unsetenv("XDG_CONFIG_HOME")
	if got := ConfigDir(); got != expected {
		t.Errorf("ConfigDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")
	if got := filepath.ToSlash(ConfigDir()); got != "/custom/config/mineshaft" {
		t.Errorf("ConfigDir() with XDG = %q, want /custom/config/mineshaft", got)
	}
}

func TestCacheDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cache", "mineshaft")

	os.Unsetenv("XDG_CACHE_HOME")
	if got := CacheDir(); got != expected {
		t.Errorf("CacheDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_CACHE_HOME", "/custom/cache")
	defer os.Unsetenv("XDG_CACHE_HOME")
	if got := filepath.ToSlash(CacheDir()); got != "/custom/cache/mineshaft" {
		t.Errorf("CacheDir() with XDG = %q, want /custom/cache/mineshaft", got)
	}
}

func TestIsEnabled_EnvOverride(t *testing.T) {
	os.Setenv("MINESHAFT_DISABLED", "1")
	defer os.Unsetenv("MINESHAFT_DISABLED")
	if IsEnabled() {
		t.Error("IsEnabled() should return false when MINESHAFT_DISABLED=1")
	}

	os.Unsetenv("MINESHAFT_DISABLED")
	os.Setenv("MINESHAFT_ENABLED", "1")
	defer os.Unsetenv("MINESHAFT_ENABLED")
	if !IsEnabled() {
		t.Error("IsEnabled() should return true when MINESHAFT_ENABLED=1")
	}
}

func TestIsEnabled_DisabledOverridesEnabled(t *testing.T) {
	os.Setenv("MINESHAFT_DISABLED", "1")
	os.Setenv("MINESHAFT_ENABLED", "1")
	defer os.Unsetenv("MINESHAFT_DISABLED")
	defer os.Unsetenv("MINESHAFT_ENABLED")

	if IsEnabled() {
		t.Error("MINESHAFT_DISABLED should take precedence over MINESHAFT_ENABLED")
	}
}

func TestEnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Unsetenv("XDG_STATE_HOME")
	os.Unsetenv("MINESHAFT_DISABLED")
	os.Unsetenv("MINESHAFT_ENABLED")

	if err := Enable("1.0.0"); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	if !IsEnabled() {
		t.Error("IsEnabled() should return true after Enable()")
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if s.Version != "1.0.0" {
		t.Errorf("State.Version = %q, want %q", s.Version, "1.0.0")
	}
	if s.MachineID == "" {
		t.Error("State.MachineID should not be empty")
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable() failed: %v", err)
	}

	if IsEnabled() {
		t.Error("IsEnabled() should return false after Disable()")
	}
}

func TestGenerateMachineID(t *testing.T) {
	id1 := generateMachineID()
	id2 := generateMachineID()

	if len(id1) != 8 {
		t.Errorf("generateMachineID() length = %d, want 8", len(id1))
	}
	if id1 == id2 {
		t.Error("generateMachineID() should generate unique IDs")
	}
}
