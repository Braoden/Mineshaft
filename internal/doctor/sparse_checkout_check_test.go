package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSparseCheckoutCheck(t *testing.T) {
	check := NewSparseCheckoutCheck()

	if check.Name() != "sparse-checkout" {
		t.Errorf("expected name 'sparse-checkout', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestSparseCheckoutCheck_NoRigSpecified(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""}

	result := check.Run(ctx)

	// No rig specified + no rigs found = StatusOK (nothing to check)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rigs found, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_TownWideMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two rigs with config.json so discoverRigPaths finds them
	rig1Dir := filepath.Join(tmpDir, "rig1")
	rig2Dir := filepath.Join(tmpDir, "rig2")

	// rig1: overseer/rig with legacy sparse checkout
	overseerRig1 := filepath.Join(rig1Dir, "overseer", "rig")
	initGitRepo(t, overseerRig1)
	configureLegacySparseCheckout(t, overseerRig1)
	if err := os.WriteFile(filepath.Join(rig1Dir, "config.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// rig2: overseer/rig with legacy sparse checkout
	overseerRig2 := filepath.Join(rig2Dir, "overseer", "rig")
	initGitRepo(t, overseerRig2)
	configureLegacySparseCheckout(t, overseerRig2)
	if err := os.WriteFile(filepath.Join(rig2Dir, "config.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""} // no --rig flag

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning in town-wide mode, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "2 repo(s) have legacy") {
		t.Errorf("expected message about 2 repos, got %q", result.Message)
	}
	if len(result.Details) != 2 {
		t.Errorf("expected 2 details, got %d: %v", len(result.Details), result.Details)
	}
}

func TestSparseCheckoutCheck_NoGitRepos(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// No git repos found = StatusOK (nothing to check)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no git repos, got %v", result.Status)
	}
}

// initGitRepo creates a minimal git repo with an initial commit.
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}

	// git init
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure user for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config email failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config name failed: %v\n%s", err, out)
	}

	// Create initial commit
	readmePath := filepath.Join(path, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// configureLegacySparseCheckout sets up legacy sparse checkout that should be removed.
func configureLegacySparseCheckout(t *testing.T, repoPath string) {
	t.Helper()

	// Enable sparse checkout
	cmd := exec.Command("git", "config", "core.sparseCheckout", "true")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %v\n%s", err, out)
	}

	// Write sparse-checkout file
	sparseFile := filepath.Join(repoPath, ".git", "info", "sparse-checkout")
	if err := os.MkdirAll(filepath.Dir(sparseFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sparseFile, []byte("/*\n!/.claude/\n!/CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSparseCheckoutCheck_NoSparseCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create overseer/rig as a git repo without sparse checkout
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// No sparse checkout = StatusOK (nothing to clean up)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no sparse checkout, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_LegacySparseCheckoutDetected(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create overseer/rig with legacy sparse checkout
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)
	configureLegacySparseCheckout(t, overseerRig)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for legacy sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 repo(s) have legacy") {
		t.Errorf("expected message about legacy sparse checkout, got %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "overseer/rig") {
		t.Errorf("expected details to contain overseer/rig, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_MultipleReposWithLegacySparseCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create multiple git repos with legacy sparse checkout
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)
	configureLegacySparseCheckout(t, overseerRig)

	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)
	configureLegacySparseCheckout(t, crewAgent)

	// Miner worktrees use nested layout: miners/<name>/<rigname>/
	miner := filepath.Join(rigDir, "miners", "pc1", "testrig")
	initGitRepo(t, miner)
	configureLegacySparseCheckout(t, miner)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for legacy sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "3 repo(s) have legacy") {
		t.Errorf("expected message about 3 repos, got %q", result.Message)
	}
	if len(result.Details) != 3 {
		t.Errorf("expected 3 details, got %d", len(result.Details))
	}
}

func TestSparseCheckoutCheck_MixedRepos(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create overseer/rig with legacy sparse checkout
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)
	configureLegacySparseCheckout(t, overseerRig)

	// Create crew/agent1 WITHOUT sparse checkout (clean)
	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for legacy sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 repo(s) have legacy") {
		t.Errorf("expected message about 1 legacy repo, got %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "overseer/rig") {
		t.Errorf("expected details to contain only overseer/rig, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_MinerNestedWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Miner worktrees use nested layout: miners/<name>/<rigname>/
	minerWorktree := filepath.Join(rigDir, "miners", "pc1", rigName)
	initGitRepo(t, minerWorktree)
	configureLegacySparseCheckout(t, minerWorktree)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for miner nested worktree, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 repo(s) have legacy") {
		t.Errorf("expected message about 1 legacy repo, got %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "miners/pc1/"+rigName) {
		t.Errorf("expected details to contain miners/pc1/%s, got %v", rigName, result.Details)
	}
}

func TestSparseCheckoutCheck_MinerLegacyFlatLayout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Legacy flat layout: miners/<name>/ is the worktree directly
	minerFlat := filepath.Join(rigDir, "miners", "pc1")
	initGitRepo(t, minerFlat)
	configureLegacySparseCheckout(t, minerFlat)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for miner flat layout, got %v", result.Status)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "miners/pc1") {
		t.Errorf("expected details to contain miners/pc1, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repos with legacy sparse checkout
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)
	configureLegacySparseCheckout(t, overseerRig)

	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)
	configureLegacySparseCheckout(t, crewAgent)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify sparse checkout is now disabled
	cmd := exec.Command("git", "config", "core.sparseCheckout")
	cmd.Dir = overseerRig
	output, _ := cmd.Output()
	if strings.TrimSpace(string(output)) == "true" {
		t.Error("expected sparse checkout to be disabled for overseer/rig")
	}

	cmd = exec.Command("git", "config", "core.sparseCheckout")
	cmd.Dir = crewAgent
	output, _ = cmd.Output()
	if strings.TrimSpace(string(output)) == "true" {
		t.Error("expected sparse checkout to be disabled for crew/agent1")
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_FixNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo without sparse checkout (already clean)
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Run check to populate state
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v", result.Status)
	}

	// Fix should be a no-op (no affected repos)
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Still OK
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after no-op fix, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_NonGitDirSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create non-git directories (should be skipped)
	if err := os.MkdirAll(filepath.Join(rigDir, "overseer", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, "crew", "agent1"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// Non-git dirs are skipped, so StatusOK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no git repos, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_FixRestoresFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo
	overseerRig := filepath.Join(rigDir, "overseer", "rig")
	initGitRepo(t, overseerRig)

	// Add and commit a .claude/settings.json file
	claudeDir := filepath.Join(overseerRig, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsFile := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsFile, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".claude/settings.json")
	cmd.Dir = overseerRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "Add .claude settings")
	cmd.Dir = overseerRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Configure legacy sparse checkout (this would hide .claude/)
	configureLegacySparseCheckout(t, overseerRig)

	// Apply sparse checkout to hide the file
	cmd = exec.Command("git", "read-tree", "-mu", "HEAD")
	cmd.Dir = overseerRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git read-tree failed: %v\n%s", err, out)
	}

	// Verify .claude is now hidden
	if _, err := os.Stat(settingsFile); !os.IsNotExist(err) {
		t.Fatal("expected .claude/settings.json to be hidden by sparse checkout")
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Apply fix
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v", result.Status)
	}

	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify .claude/settings.json is now restored
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		t.Error("expected .claude/settings.json to be restored after fix")
	}
}
