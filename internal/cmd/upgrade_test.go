package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateCLAUDEMD(t *testing.T) {
	content := generateCLAUDEMD()

	// Must contain the Mineshaft header
	if content == "" {
		t.Fatal("generateCLAUDEMD returned empty string")
	}
	if content[0:10] != "# Mineshaft" {
		t.Errorf("expected content to start with '# Mineshaft', got: %q", content[:10])
	}

	// Must contain identity anchoring instructions
	if !contains(content, "Do NOT adopt an identity") {
		t.Error("CLAUDE.md should contain identity anchoring warning")
	}
	if !contains(content, "MS_ROLE") {
		t.Error("CLAUDE.md should reference MS_ROLE environment variable")
	}
}


func TestUpgradeCLAUDEMD_CreatesMissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with a "town root" that has no CLAUDE.md
	upgradeDryRun = false
	upgradeVerbose = false

	result := upgradeCLAUDEMD(tmpDir)

	// 2 changes: CLAUDE.md created + AGENTS.md symlink created
	if runtime.GOOS == "windows" {
		// On Windows, symlink creation requires elevated privileges.
		// Only CLAUDE.md is created; AGENTS.md symlink may fail silently.
		if result.changed < 1 {
			t.Errorf("expected at least 1 change for new CLAUDE.md, got %d", result.changed)
		}
	} else {
		if result.changed != 2 {
			t.Errorf("expected 2 changes for new CLAUDE.md + AGENTS.md, got %d", result.changed)
		}
	}

	// Verify file was created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("CLAUDE.md not created: %v", err)
	}

	expected := generateCLAUDEMD()
	if string(data) != expected {
		t.Error("CLAUDE.md content doesn't match expected template")
	}

	// Verify AGENTS.md symlink was created
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	if runtime.GOOS != "windows" {
		target, err := os.Readlink(agentsPath)
		if err != nil {
			t.Fatalf("AGENTS.md symlink not created: %v", err)
		}
		if target != "CLAUDE.md" {
			t.Errorf("AGENTS.md symlink target = %q, want %q", target, "CLAUDE.md")
		}
	}
}

func TestUpgradeCLAUDEMD_UpToDate(t *testing.T) {
	tmpDir := t.TempDir()

	// Write the expected content
	expected := generateCLAUDEMD()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte(expected), 0644); err != nil {
		t.Fatal(err)
	}

	upgradeDryRun = false
	upgradeVerbose = false

	result := upgradeCLAUDEMD(tmpDir)

	if result.changed != 0 {
		t.Errorf("expected 0 changes for up-to-date CLAUDE.md, got %d", result.changed)
	}
}

func TestUpgradeCLAUDEMD_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	upgradeDryRun = true
	upgradeVerbose = false

	result := upgradeCLAUDEMD(tmpDir)

	if result.changed != 1 {
		t.Errorf("expected 1 change in dry-run mode, got %d", result.changed)
	}

	// Verify file was NOT created
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Error("dry-run should not create CLAUDE.md")
	}

	// Reset
	upgradeDryRun = false
}

func TestUpgradeDaemonConfig_CreatesMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create overseer directory (required by DaemonPatrolConfigPath)
	overseerDir := filepath.Join(tmpDir, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatal(err)
	}

	upgradeDryRun = false
	upgradeVerbose = false

	result := upgradeDaemonConfig(tmpDir)

	if result.changed != 1 {
		t.Errorf("expected 1 change for new daemon.json, got %d", result.changed)
	}

	// Verify file exists
	daemonPath := filepath.Join(overseerDir, "daemon.json")
	if _, err := os.Stat(daemonPath); err != nil {
		t.Errorf("daemon.json not created: %v", err)
	}
}

func TestUpgradeDaemonConfig_ExistingValid(t *testing.T) {
	tmpDir := t.TempDir()
	overseerDir := filepath.Join(tmpDir, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid daemon.json
	daemonPath := filepath.Join(overseerDir, "daemon.json")
	content := `{
		"type": "daemon-patrol-config",
		"version": 1,
		"heartbeat": {"enabled": true, "interval": "3m"},
		"patrols": {}
	}`
	if err := os.WriteFile(daemonPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	upgradeDryRun = false
	upgradeVerbose = false

	result := upgradeDaemonConfig(tmpDir)

	if result.changed != 0 {
		t.Errorf("expected 0 changes for existing daemon.json, got %d", result.changed)
	}
}

func TestUpgradeCommandRegistered(t *testing.T) {
	// Verify the upgrade command is registered in rootCmd
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "upgrade" {
			found = true
			break
		}
	}
	if !found {
		t.Error("upgrade command not registered with rootCmd")
	}
}

func TestUpgradeBeadsExempt(t *testing.T) {
	if !beadsExemptCommands["upgrade"] {
		t.Error("upgrade should be in beadsExemptCommands")
	}
}

func TestUpgradeBranchCheckExempt(t *testing.T) {
	if !branchCheckExemptCommands["upgrade"] {
		t.Error("upgrade should be in branchCheckExemptCommands")
	}
}

func TestFormulaBeadsParents(t *testing.T) {
	tmpDir := t.TempDir()
	overseerDir := filepath.Join(tmpDir, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Rig "tracked" has a redirect to overseer/rig/.beads (tracked beads).
	trackedBeads := filepath.Join(tmpDir, "tracked", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}
	trackedCanonical := filepath.Join(tmpDir, "tracked", "overseer", "rig", ".beads")
	if err := os.MkdirAll(trackedCanonical, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trackedBeads, "redirect"), []byte("overseer/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rig "local" has its own .beads (no redirect).
	localBeads := filepath.Join(tmpDir, "local", ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}

	rigsJSON := `{"version": 1, "rigs": {"tracked": {"git_url": "x"}, "local": {"git_url": "x"}, "missing": {"git_url": "x"}}}`
	if err := os.WriteFile(filepath.Join(overseerDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	parents := formulaBeadsParents(tmpDir)

	want := []string{
		tmpDir,
		filepath.Join(tmpDir, "local"),
		filepath.Join(tmpDir, "tracked", "overseer", "rig"),
	}
	if len(parents) != len(want) {
		t.Fatalf("parents = %v, want %v", parents, want)
	}
	for i := range want {
		if parents[i] != want[i] {
			t.Errorf("parents[%d] = %q, want %q", i, parents[i], want[i])
		}
	}
}

func TestFormulaBeadsParents_NoRigsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	parents := formulaBeadsParents(tmpDir)
	if len(parents) != 1 || parents[0] != tmpDir {
		t.Errorf("parents = %v, want just town root", parents)
	}
}

// contains is already declared in mq_test.go in this package,
// so we reuse it here.
