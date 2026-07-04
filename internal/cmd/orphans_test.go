package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFindOrphanMinerBranches verifies that miner worktrees with unmerged
// branches are detected and reported (GH #1024).
func TestFindOrphanMinerBranches(t *testing.T) {
	// Create a fake rig with a miner worktree that has unmerged commits.
	rigDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(rigDir, "miners")

	// Create a bare "origin" repo to serve as a remote
	originDir := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(originDir, 0755); err != nil {
		t.Fatal(err)
	}
	run(t, originDir, "git", "init", "--bare")

	// Create the miner worktree with an initial commit on main (legacy flat layout)
	minerName := "alpha"
	worktreePath := filepath.Join(minersDir, minerName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	run(t, worktreePath, "git", "init")
	run(t, worktreePath, "git", "remote", "add", "origin", originDir)

	// Create initial commit on main
	writeFile(t, filepath.Join(worktreePath, "README.md"), "# test\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "initial commit")
	run(t, worktreePath, "git", "branch", "-M", "main")
	run(t, worktreePath, "git", "push", "-u", "origin", "main")

	// Create a miner branch with an extra commit
	run(t, worktreePath, "git", "checkout", "-b", "miner/alpha-work")
	writeFile(t, filepath.Join(worktreePath, "feature.go"), "package feature\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "feat: add feature")

	// Scan for orphan branches
	branches, skipped, err := findOrphanMinerBranches(rigDir, rigName, "main")
	if err != nil {
		t.Fatalf("findOrphanMinerBranches: %v", err)
	}
	if len(skipped) > 0 {
		t.Errorf("unexpected skipped miners: %v", skipped)
	}

	if len(branches) != 1 {
		t.Fatalf("expected 1 orphan branch, got %d", len(branches))
	}

	b := branches[0]
	if b.Miner != minerName {
		t.Errorf("miner = %q, want %q", b.Miner, minerName)
	}
	if b.Branch != "miner/alpha-work" {
		t.Errorf("branch = %q, want %q", b.Branch, "miner/alpha-work")
	}
	if b.AheadCount != 1 {
		t.Errorf("ahead count = %d, want 1", b.AheadCount)
	}
	if b.LatestSubject != "feat: add feature" {
		t.Errorf("latest subject = %q, want %q", b.LatestSubject, "feat: add feature")
	}
	if b.HasUncommitted {
		t.Error("expected no uncommitted changes")
	}
	if b.WorktreePath != worktreePath {
		t.Errorf("worktree path = %q, want %q", b.WorktreePath, worktreePath)
	}
}

// TestFindOrphanMinerBranches_NewStructure verifies that the new-structure
// layout (miners/<name>/<rigname>/) is correctly detected.
func TestFindOrphanMinerBranches_NewStructure(t *testing.T) {
	rigDir := t.TempDir()
	rigName := "myrig"
	minersDir := filepath.Join(rigDir, "miners")

	// Create a bare "origin" repo
	originDir := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(originDir, 0755); err != nil {
		t.Fatal(err)
	}
	run(t, originDir, "git", "init", "--bare")

	// New structure: miners/<name>/<rigname>/
	minerName := "charlie"
	worktreePath := filepath.Join(minersDir, minerName, rigName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	run(t, worktreePath, "git", "init")
	run(t, worktreePath, "git", "remote", "add", "origin", originDir)

	writeFile(t, filepath.Join(worktreePath, "README.md"), "# test\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "initial commit")
	run(t, worktreePath, "git", "branch", "-M", "main")
	run(t, worktreePath, "git", "push", "-u", "origin", "main")

	run(t, worktreePath, "git", "checkout", "-b", "miner/charlie-work")
	writeFile(t, filepath.Join(worktreePath, "new.go"), "package new\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "feat: new structure work")

	branches, skipped, err := findOrphanMinerBranches(rigDir, rigName, "main")
	if err != nil {
		t.Fatalf("findOrphanMinerBranches: %v", err)
	}
	if len(skipped) > 0 {
		t.Errorf("unexpected skipped miners: %v", skipped)
	}

	if len(branches) != 1 {
		t.Fatalf("expected 1 orphan branch, got %d", len(branches))
	}

	b := branches[0]
	if b.Miner != minerName {
		t.Errorf("miner = %q, want %q", b.Miner, minerName)
	}
	if b.WorktreePath != worktreePath {
		t.Errorf("worktree path = %q, want %q", b.WorktreePath, worktreePath)
	}
	if b.AheadCount != 1 {
		t.Errorf("ahead count = %d, want 1", b.AheadCount)
	}
}

// TestFindOrphanMinerBranches_CustomDefaultBranch verifies that a non-main
// default branch is respected.
func TestFindOrphanMinerBranches_CustomDefaultBranch(t *testing.T) {
	rigDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(rigDir, "miners")

	originDir := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(originDir, 0755); err != nil {
		t.Fatal(err)
	}
	run(t, originDir, "git", "init", "--bare")

	minerName := "delta"
	worktreePath := filepath.Join(minersDir, minerName)
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	run(t, worktreePath, "git", "init")
	run(t, worktreePath, "git", "remote", "add", "origin", originDir)
	writeFile(t, filepath.Join(worktreePath, "README.md"), "# test\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "initial commit")
	// Use "develop" as the default branch
	run(t, worktreePath, "git", "branch", "-M", "develop")
	run(t, worktreePath, "git", "push", "-u", "origin", "develop")

	run(t, worktreePath, "git", "checkout", "-b", "feature/delta-work")
	writeFile(t, filepath.Join(worktreePath, "feature.go"), "package feature\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "feat: custom branch work")

	// Scan with defaultBranch="develop"
	branches, _, err := findOrphanMinerBranches(rigDir, rigName, "develop")
	if err != nil {
		t.Fatalf("findOrphanMinerBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 orphan branch, got %d", len(branches))
	}

	// Scan with defaultBranch="main" should fail to count (no main branch),
	// and the miner should be skipped
	branches2, skipped, err := findOrphanMinerBranches(rigDir, rigName, "main")
	if err != nil {
		t.Fatalf("findOrphanMinerBranches: %v", err)
	}
	if len(branches2) != 0 {
		t.Errorf("expected 0 branches when scanning with wrong default branch, got %d", len(branches2))
	}
	if len(skipped) != 1 {
		t.Errorf("expected 1 skipped miner (wrong base branch), got %d", len(skipped))
	}
}

// TestFindOrphanMinerBranches_OnMain verifies that miners on main are not
// reported as orphans.
func TestFindOrphanMinerBranches_OnMain(t *testing.T) {
	rigDir := t.TempDir()
	rigName := "testrig"
	minersDir := filepath.Join(rigDir, "miners")
	worktreePath := filepath.Join(minersDir, "bravo")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	run(t, worktreePath, "git", "init")
	writeFile(t, filepath.Join(worktreePath, "README.md"), "# test\n")
	run(t, worktreePath, "git", "add", ".")
	run(t, worktreePath, "git", "commit", "-m", "initial commit")
	run(t, worktreePath, "git", "branch", "-M", "main")

	branches, _, err := findOrphanMinerBranches(rigDir, rigName, "main")
	if err != nil {
		t.Fatalf("findOrphanMinerBranches: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 orphan branches for miner on main, got %d", len(branches))
	}
}

// TestFindOrphanMinerBranches_NoMinersDir verifies graceful handling when
// there is no miners directory.
func TestFindOrphanMinerBranches_NoMinersDir(t *testing.T) {
	rigDir := t.TempDir()
	branches, skipped, err := findOrphanMinerBranches(rigDir, "testrig", "main")
	if err != nil {
		t.Fatalf("expected nil error for missing miners dir, got: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected 0 branches, got %d", len(branches))
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(skipped))
	}
}

// --- helpers ---

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
