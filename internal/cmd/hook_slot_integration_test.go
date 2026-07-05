//go:build integration

// Package cmd contains integration tests for hook slot verification.
//
// Run with: go test -tags=integration ./internal/cmd -run TestHookSlot -v
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/testutil"
)

// hookTestCounter generates unique prefixes for each hook test to isolate
// Dolt databases on the shared server.
var hookTestCounter atomic.Int32

// setupHookTestTown creates a minimal Mineshaft with a miner for testing hooks.
// Uses requireDoltServer for ephemeral port and unique prefixes per test to
// isolate Dolt databases.
// Returns townRoot, the path to the miner's worktree, and the beads prefix.
func setupHookTestTown(t *testing.T) (townRoot, minerDir, rigPrefix string) {
	t.Helper()
	requireDoltServer(t)

	n := hookTestCounter.Add(1)
	rigPrefix = fmt.Sprintf("ht%d", n)

	townRoot = t.TempDir()

	// Create a real town marker so workspace discovery resolves the outer town
	// root instead of stopping at the nested rig overseer directory.
	townOverseerDir := filepath.Join(townRoot, "overseer")
	if err := os.MkdirAll(townOverseerDir, 0755); err != nil {
		t.Fatalf("mkdir town overseer: %v", err)
	}
	if err := config.SaveTownConfig(filepath.Join(townOverseerDir, "town.json"), &config.TownConfig{
		Type:      "town",
		Version:   config.CurrentTownVersion,
		Name:      "hook-test-town",
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("write town config: %v", err)
	}

	// Create town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Create routes.jsonl
	routes := []beads.Route{
		{Prefix: "hq-", Path: "."},                           // Town-level beads
		{Prefix: rigPrefix + "-", Path: "mineshaft/overseer/rig"}, // Mineshaft rig
	}
	if err := beads.WriteRoutes(townBeadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	// Create mineshaft rig structure
	gasRigPath := filepath.Join(townRoot, "mineshaft", "overseer", "rig")
	if err := os.MkdirAll(gasRigPath, 0755); err != nil {
		t.Fatalf("mkdir mineshaft: %v", err)
	}

	// Create mineshaft .beads directory with its own config
	gasBeadsDir := filepath.Join(gasRigPath, ".beads")
	if err := os.MkdirAll(gasBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir mineshaft .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gasBeadsDir, "config.yaml"), []byte("prefix: "+rigPrefix+"\n"), 0644); err != nil {
		t.Fatalf("write mineshaft config: %v", err)
	}

	// Create miner worktree with redirect
	minerDir = filepath.Join(townRoot, "mineshaft", "miners", "toast")
	if err := os.MkdirAll(minerDir, 0755); err != nil {
		t.Fatalf("mkdir miners: %v", err)
	}
	gitDir := filepath.Join(minerDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("mkdir miner .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("write miner .git HEAD: %v", err)
	}

	// Create redirect file for miner -> overseer/rig/.beads
	minerBeadsDir := filepath.Join(minerDir, ".beads")
	if err := os.MkdirAll(minerBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir miner .beads: %v", err)
	}
	redirectContent := "../../overseer/rig/.beads"
	if err := os.WriteFile(filepath.Join(minerBeadsDir, "redirect"), []byte(redirectContent), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	return townRoot, minerDir, rigPrefix
}

// initBeadsDB initializes the beads database by running bd init on the test server.
func initBeadsDB(t *testing.T, dir string) {
	t.Helper()
	testutil.RequireDoltContainer(t)

	cmd := exec.Command("bd", "init", "--server", "--server-port", testutil.DoltContainerPort())
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\n%s", err, output)
	}
}

// TestHookSlot_BasicHook verifies that a bead can be hooked to an agent.
func TestHookSlot_BasicHook(t *testing.T) {
	// Skip if bd is not available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot // Not used directly but shows test context

	// Initialize beads in the rig
	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)

	// Create a test bead
	issue, err := b.Create(beads.CreateOptions{
		Title:    "Test task for hooking",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}
	t.Logf("Created bead: %s", issue.ID)

	// Hook the bead to the miner
	agentID := "mineshaft/miners/toast"
	status := beads.StatusHooked
	if err := b.Update(issue.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook bead: %v", err)
	}

	// Verify the bead is hooked
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked beads: %v", err)
	}

	if len(hookedBeads) != 1 {
		t.Errorf("expected 1 hooked bead, got %d", len(hookedBeads))
	}

	if len(hookedBeads) > 0 && hookedBeads[0].ID != issue.ID {
		t.Errorf("hooked bead ID = %s, want %s", hookedBeads[0].ID, issue.ID)
	}
}

// TestHookSlot_Singleton verifies that only one bead can be hooked per agent.
func TestHookSlot_Singleton(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot

	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	agentID := "mineshaft/miners/toast"
	status := beads.StatusHooked

	// Create and hook first bead
	issue1, err := b.Create(beads.CreateOptions{
		Title:    "First task",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create first bead: %v", err)
	}

	if err := b.Update(issue1.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook first bead: %v", err)
	}

	// Create second bead
	issue2, err := b.Create(beads.CreateOptions{
		Title:    "Second task",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create second bead: %v", err)
	}

	// Hook second bead to same agent
	if err := b.Update(issue2.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook second bead: %v", err)
	}

	// Query hooked beads - both should be hooked (bd allows multiple)
	// The singleton constraint is enforced by gt hook, not bd itself
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked beads: %v", err)
	}

	t.Logf("Found %d hooked beads for agent %s", len(hookedBeads), agentID)
	for _, h := range hookedBeads {
		t.Logf("  - %s: %s", h.ID, h.Title)
	}

	// The test documents actual behavior: bd allows multiple hooked beads
	// The gt hook command enforces singleton behavior
	if len(hookedBeads) != 2 {
		t.Errorf("expected 2 hooked beads (bd allows multiple), got %d", len(hookedBeads))
	}
}

// TestHookSlot_Unhook verifies that a bead can be unhooked by changing status.
func TestHookSlot_Unhook(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot

	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	agentID := "mineshaft/miners/toast"

	// Create and hook a bead
	issue, err := b.Create(beads.CreateOptions{
		Title:    "Task to unhook",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}

	status := beads.StatusHooked
	if err := b.Update(issue.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook bead: %v", err)
	}

	// Unhook by setting status back to open
	openStatus := "open"
	if err := b.Update(issue.ID, beads.UpdateOptions{
		Status: &openStatus,
	}); err != nil {
		t.Fatalf("unhook bead: %v", err)
	}

	// Verify no hooked beads remain
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked beads: %v", err)
	}

	if len(hookedBeads) != 0 {
		t.Errorf("expected 0 hooked beads after unhook, got %d", len(hookedBeads))
	}
}

// TestHookSlot_DifferentAgents verifies that different agents can have different hooks.
func TestHookSlot_DifferentAgents(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)

	// Create second miner directory
	miner2Dir := filepath.Join(townRoot, "mineshaft", "miners", "nux")
	if err := os.MkdirAll(miner2Dir, 0755); err != nil {
		t.Fatalf("mkdir miner2: %v", err)
	}

	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	agent1 := "mineshaft/miners/toast"
	agent2 := "mineshaft/miners/nux"
	status := beads.StatusHooked

	// Create and hook bead to first agent
	issue1, err := b.Create(beads.CreateOptions{
		Title:    "Toast's task",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead 1: %v", err)
	}

	if err := b.Update(issue1.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agent1,
	}); err != nil {
		t.Fatalf("hook bead to agent1: %v", err)
	}

	// Create and hook bead to second agent
	issue2, err := b.Create(beads.CreateOptions{
		Title:    "Nux's task",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead 2: %v", err)
	}

	if err := b.Update(issue2.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agent2,
	}); err != nil {
		t.Fatalf("hook bead to agent2: %v", err)
	}

	// Verify each agent has exactly one hook
	agent1Hooks, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agent1,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list agent1 hooks: %v", err)
	}

	agent2Hooks, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agent2,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list agent2 hooks: %v", err)
	}

	if len(agent1Hooks) != 1 {
		t.Errorf("agent1 should have 1 hook, got %d", len(agent1Hooks))
	}
	if len(agent2Hooks) != 1 {
		t.Errorf("agent2 should have 1 hook, got %d", len(agent2Hooks))
	}

	// Verify correct assignment
	if len(agent1Hooks) > 0 && agent1Hooks[0].ID != issue1.ID {
		t.Errorf("agent1 hook ID = %s, want %s", agent1Hooks[0].ID, issue1.ID)
	}
	if len(agent2Hooks) > 0 && agent2Hooks[0].ID != issue2.ID {
		t.Errorf("agent2 hook ID = %s, want %s", agent2Hooks[0].ID, issue2.ID)
	}
}

// TestHookSlot_HookPersistence verifies that hooks persist across beads object recreation.
func TestHookSlot_HookPersistence(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot

	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	agentID := "mineshaft/miners/toast"
	status := beads.StatusHooked

	// Create first beads instance and hook a bead
	b1 := beads.New(rigDir)
	issue, err := b1.Create(beads.CreateOptions{
		Title:    "Persistent task",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}

	if err := b1.Update(issue.ID, beads.UpdateOptions{
		Status:   &status,
		Assignee: &agentID,
	}); err != nil {
		t.Fatalf("hook bead: %v", err)
	}

	// Create new beads instance (simulates session restart)
	b2 := beads.New(rigDir)

	// Verify hook persists
	hookedBeads, err := b2.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		t.Fatalf("list hooked beads with new instance: %v", err)
	}

	if len(hookedBeads) != 1 {
		t.Errorf("expected hook to persist, got %d hooked beads", len(hookedBeads))
	}

	if len(hookedBeads) > 0 && hookedBeads[0].ID != issue.ID {
		t.Errorf("persisted hook ID = %s, want %s", hookedBeads[0].ID, issue.ID)
	}
}

// TestHookSlot_StatusTransitions tests valid status transitions for hooked beads.
func TestHookSlot_StatusTransitions(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed, skipping test")
	}

	townRoot, minerDir, rigPrefix := setupHookTestTown(t)
	_ = townRoot

	rigDir := filepath.Join(minerDir, "..", "..", "overseer", "rig")
	initBeadsDBWithPrefix(t, rigDir, rigPrefix)

	b := beads.New(rigDir)
	agentID := "mineshaft/miners/toast"

	// Create a bead
	issue, err := b.Create(beads.CreateOptions{
		Title:    "Status transition test",
		Labels:   []string{"gt:task"},
		Priority: 2,
	})
	if err != nil {
		t.Fatalf("create bead: %v", err)
	}

	// Test transitions: open -> hooked -> open -> hooked -> closed
	transitions := []struct {
		name   string
		status string
	}{
		{"hook", beads.StatusHooked},
		{"unhook", "open"},
		{"rehook", beads.StatusHooked},
	}

	for _, trans := range transitions {
		t.Run(trans.name, func(t *testing.T) {
			status := trans.status
			opts := beads.UpdateOptions{Status: &status}
			if trans.status == beads.StatusHooked {
				opts.Assignee = &agentID
			}

			if err := b.Update(issue.ID, opts); err != nil {
				t.Errorf("transition to %s failed: %v", trans.status, err)
			}

			// Verify status
			updated, err := b.Show(issue.ID)
			if err != nil {
				t.Errorf("show after %s: %v", trans.name, err)
				return
			}
			if updated.Status != trans.status {
				t.Errorf("status after %s = %s, want %s", trans.name, updated.Status, trans.status)
			}
		})
	}

	// Finally close the bead
	if err := b.Close(issue.ID); err != nil {
		t.Errorf("close hooked bead: %v", err)
	}

	// Verify it's closed
	closed, err := b.Show(issue.ID)
	if err != nil {
		t.Fatalf("show closed bead: %v", err)
	}
	if closed.Status != "closed" {
		t.Errorf("final status = %s, want closed", closed.Status)
	}
}
