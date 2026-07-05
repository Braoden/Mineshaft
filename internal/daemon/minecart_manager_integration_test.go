//go:build integration

package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	beadsdk "github.com/steveyegge/beads"
)

// TestMinecartManager_FullLifecycle starts a real MinecartManager with a real beads
// store and mock ms, lets both goroutines tick (event poll + stranded scan),
// verifies log output, then stops and verifies clean shutdown.
//
// Exercises: S-08 (start guard), S-09 (context cancellation), S-10 (resolved paths).
func TestMinecartManager_FullLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Set up mock ms that returns stranded minecarts and logs all calls.
	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	callLogPath := filepath.Join(binDir, "ms-calls.log")
	strandedJSON := `[{"id":"cv-test1","title":"Test Minecart","ready_count":0,"ready_issues":[]}]`

	gtScript := fmt.Sprintf(`#!/bin/sh
echo "$@" >> "%s"
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  echo '%s'
  exit 0
fi
exit 0
`, callLogPath, strandedJSON)

	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	var mu = &sync.Mutex{}
	var logged []string
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	// Start with short scan interval so stranded scan fires quickly.
	m := NewMinecartManager(townRoot, logger, gtPath, 500*time.Millisecond, map[string]beadsdk.Storage{"hq": store}, nil, nil)

	// S-08: Start should succeed.
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// S-08: Second Start should be a no-op.
	if err := m.Start(); err != nil {
		t.Fatalf("double Start: %v", err)
	}

	// Wait for the seed poll to complete (first event poll tick at ~5s).
	// The first poll advances high-water marks without processing events.
	time.Sleep(6 * time.Second)

	// Create and close an issue AFTER seeding so the next poll detects it.
	now := time.Now().UTC()
	issue := &beadsdk.Issue{
		ID:        "ms-integ1",
		Title:     "Integration Test Issue",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := store.CloseIssue(ctx, issue.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	// Wait for the next event poll tick to detect the close event (~5s).
	time.Sleep(6 * time.Second)

	// Stop and verify bounded completion (S-09: context cancellation).
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
		// Success -- shutdown completed.
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not complete within 5s -- context cancellation may be broken")
	}

	// Verify event poll detected the close event.
	mu.Lock()
	logSnapshot := make([]string, len(logged))
	copy(logSnapshot, logged)
	mu.Unlock()

	foundClose := false
	foundScan := false
	foundDoubleStart := false
	for _, s := range logSnapshot {
		if strings.Contains(s, "close detected") && strings.Contains(s, "ms-integ1") {
			foundClose = true
		}
		if strings.Contains(s, "auto-closing") || strings.Contains(s, "minecart check") {
			foundScan = true
		}
		if strings.Contains(s, "already called") {
			foundDoubleStart = true
		}
	}

	if !foundClose {
		t.Errorf("event poll did not detect close event for ms-integ1; logs:\n%s", strings.Join(logSnapshot, "\n"))
	}
	if !foundScan {
		t.Errorf("stranded scan did not process the empty minecart; logs:\n%s", strings.Join(logSnapshot, "\n"))
	}
	if !foundDoubleStart {
		t.Errorf("double Start() guard did not fire; logs:\n%s", strings.Join(logSnapshot, "\n"))
	}

	// Verify ms was actually called (S-10: resolved path worked).
	if _, err := os.Stat(callLogPath); err != nil {
		t.Errorf("ms was never called (resolved path may be broken): %v", err)
	}
}

// TestMinecartManager_LoggingFlow verifies the end-to-end log chain when a close
// event triggers minecart tracking lookups and feeding decisions. This exercises
// both MinecartManager event detection and CheckMinecartsForIssue operations
// flowing through the same logger.
//
// Expected log chain for a close event with minecart tracking:
//  1. "Minecart: close detected: <issue>"
//  2. "Minecart: <issue> tracked by 1 minecart(s): [<minecart>]"
//  3. "Minecart: checking minecart <minecart>"
//  4. "Minecart: minecart <minecart>: feeding next ready issue <issue2> to <rig>"
//     OR "Minecart: minecart <minecart>: no ready issues to feed"
func TestMinecartManager_LoggingFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// Create minecart
	minecart := &beadsdk.Issue{
		ID:        "hq-cv-logtest",
		Title:     "Logging Test Minecart",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, minecart, "test"); err != nil {
		t.Fatalf("CreateIssue minecart: %v", err)
	}

	// Create tracked issue 1 (will be closed to trigger event)
	task1 := &beadsdk.Issue{
		ID:        "ms-logtask1",
		Title:     "Task 1 (close me)",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, task1, "test"); err != nil {
		t.Fatalf("CreateIssue task1: %v", err)
	}

	// Create tracked issue 2 (stays open, should be fed)
	task2 := &beadsdk.Issue{
		ID:        "ms-logtask2",
		Title:     "Task 2 (ready to feed)",
		Status:    beadsdk.StatusOpen,
		Priority:  3,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, task2, "test"); err != nil {
		t.Fatalf("CreateIssue task2: %v", err)
	}

	// Add tracks dependencies: minecart tracks both tasks
	for _, taskID := range []string{task1.ID, task2.ID} {
		dep := &beadsdk.Dependency{
			IssueID:     minecart.ID,
			DependsOnID: taskID,
			Type:        beadsdk.DependencyType("tracks"),
			CreatedAt:   now,
			CreatedBy:   "test",
		}
		if err := store.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("AddDependency %s: %v", taskID, err)
		}
	}

	// Close task1 to generate a close event
	if err := store.CloseIssue(ctx, task1.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	// Set up mock ms and routes
	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// routes.jsonl: "ms-" prefix maps to rig "ms"
	routes := `{"prefix":"ms-","path":"ms/.beads"}` + "\n"
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	slingLogPath := filepath.Join(binDir, "sling.log")
	gtScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  echo '[]'
  exit 0
fi
if [ "$1" = "minecart" ] && [ "$2" = "check" ]; then
  exit 0
fi
if [ "$1" = "sling" ]; then
  echo "$@" >> "%s"
  exit 0
fi
exit 0
`, slingLogPath)

	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	// Thread-safe logger
	var mu sync.Mutex
	var logged []string
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	// Start manager with short scan interval; event poll is 5s (fixed).
	stores := map[string]beadsdk.Storage{"hq": store}
	m := NewMinecartManager(townRoot, logger, gtPath, 1*time.Hour, stores, nil, nil)
	// Skip seeding so pollStoresSnapshot processes events immediately.
	m.seeded.Store(true)
	// Drive one poll manually instead of waiting for the 5s ticker.
	m.pollStoresSnapshot(stores)

	mu.Lock()
	snapshot := make([]string, len(logged))
	copy(snapshot, logged)
	mu.Unlock()

	// Verify the complete log chain.
	// 1. MinecartManager detects the close event
	assertLogContains(t, snapshot, "close detected", task1.ID)
	// 2. CheckMinecartsForIssue reports minecart tracking
	assertLogContains(t, snapshot, "tracked by", minecart.ID)
	// 3. CheckMinecartsForIssue runs minecart check
	assertLogContains(t, snapshot, "checking minecart", minecart.ID)
	// 4. CheckMinecartsForIssue feeds next ready issue (task2 is open+unassigned)
	assertLogContains(t, snapshot, "feeding next ready issue", task2.ID)

	// Verify no format string errors (e.g., %!s(MISSING), %!(EXTRA)
	for _, line := range snapshot {
		if strings.Contains(line, "%!") {
			t.Errorf("malformed log line (format string error): %q", line)
		}
	}

	// Verify sling was actually called for task2
	data, err := os.ReadFile(slingLogPath)
	if err != nil {
		t.Errorf("sling was never called (expected dispatch of %s): %v", task2.ID, err)
	} else if !strings.Contains(string(data), task2.ID) {
		t.Errorf("sling log does not contain %s: %q", task2.ID, string(data))
	}
}

// TestMinecartManager_LoggingFlow_NoReadyIssues verifies the log chain when
// all tracked issues are closed and there's nothing to feed.
func TestMinecartManager_LoggingFlow_NoReadyIssues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	minecart := &beadsdk.Issue{
		ID:        "hq-cv-nofeed",
		Title:     "No Feed Minecart",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	task := &beadsdk.Issue{
		ID:        "ms-nofeed1",
		Title:     "Only Task (will close)",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, minecart, "test"); err != nil {
		t.Fatalf("CreateIssue minecart: %v", err)
	}
	if err := store.CreateIssue(ctx, task, "test"); err != nil {
		t.Fatalf("CreateIssue task: %v", err)
	}
	dep := &beadsdk.Dependency{
		IssueID:     minecart.ID,
		DependsOnID: task.ID,
		Type:        beadsdk.DependencyType("tracks"),
		CreatedAt:   now,
		CreatedBy:   "test",
	}
	if err := store.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if err := store.CloseIssue(ctx, task.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	// Mock ms
	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	gtScript := `#!/bin/sh
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  echo '[]'
  exit 0
fi
exit 0
`
	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	var mu sync.Mutex
	var logged []string
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	stores := map[string]beadsdk.Storage{"hq": store}
	m := NewMinecartManager(townRoot, logger, gtPath, 1*time.Hour, stores, nil, nil)
	// Skip seeding so pollStoresSnapshot processes events immediately.
	m.seeded.Store(true)
	m.pollStoresSnapshot(stores)

	mu.Lock()
	snapshot := make([]string, len(logged))
	copy(snapshot, logged)
	mu.Unlock()

	// Verify chain ends with "no ready issues"
	assertLogContains(t, snapshot, "close detected", task.ID)
	assertLogContains(t, snapshot, "tracked by", minecart.ID)
	assertLogContains(t, snapshot, "checking minecart", minecart.ID)
	assertLogContains(t, snapshot, "no ready issues to feed", "")

	// Verify no format string errors
	for _, line := range snapshot {
		if strings.Contains(line, "%!") {
			t.Errorf("malformed log line (format string error): %q", line)
		}
	}
}

// TestMinecartManager_MultipleTrackingMinecarts verifies that when a single issue
// is tracked by two different minecarts, closing the issue triggers minecart checks
// for BOTH. This exercises the getTrackingMinecarts path returning >1 result and
// the CheckMinecartsForIssue loop iterating all tracking minecarts.
func TestMinecartManager_MultipleTrackingMinecarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// Two minecarts, both tracking the same task.
	minecartA := &beadsdk.Issue{
		ID: "hq-cv-multi-a", Title: "Minecart A",
		Status: beadsdk.StatusOpen, Priority: 2,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}
	minecartB := &beadsdk.Issue{
		ID: "hq-cv-multi-b", Title: "Minecart B",
		Status: beadsdk.StatusOpen, Priority: 2,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}
	task := &beadsdk.Issue{
		ID: "ms-multi1", Title: "Shared Task",
		Status: beadsdk.StatusOpen, Priority: 2,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}

	for _, issue := range []*beadsdk.Issue{minecartA, minecartB, task} {
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("CreateIssue %s: %v", issue.ID, err)
		}
	}

	// Both minecarts track the same task.
	for _, minecartID := range []string{minecartA.ID, minecartB.ID} {
		dep := &beadsdk.Dependency{
			IssueID:     minecartID,
			DependsOnID: task.ID,
			Type:        beadsdk.DependencyType("tracks"),
			CreatedAt:   now,
			CreatedBy:   "test",
		}
		if err := store.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("AddDependency %s -> %s: %v", minecartID, task.ID, err)
		}
	}

	// Close the shared task to generate a close event.
	if err := store.CloseIssue(ctx, task.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	// Mock ms
	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	checkLogPath := filepath.Join(binDir, "check.log")
	gtScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  echo '[]'
  exit 0
fi
if [ "$1" = "minecart" ] && [ "$2" = "check" ]; then
  echo "$3" >> "%s"
  exit 0
fi
exit 0
`, checkLogPath)

	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	var mu sync.Mutex
	var logged []string
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	stores := map[string]beadsdk.Storage{"hq": store}
	m := NewMinecartManager(townRoot, logger, gtPath, 1*time.Hour, stores, nil, nil)
	// Skip seeding so pollStoresSnapshot processes events immediately.
	m.seeded.Store(true)
	m.pollStoresSnapshot(stores)

	mu.Lock()
	snapshot := make([]string, len(logged))
	copy(snapshot, logged)
	mu.Unlock()

	// Verify: close detected once for the task.
	assertLogContains(t, snapshot, "close detected", task.ID)

	// Verify: "tracked by 2 minecart(s)".
	assertLogContains(t, snapshot, "tracked by 2 minecart(s)")

	// Verify: both minecarts were checked.
	assertLogContains(t, snapshot, "checking minecart", minecartA.ID)
	assertLogContains(t, snapshot, "checking minecart", minecartB.ID)

	// Verify ms minecart check was called for both (via mock log file).
	data, err := os.ReadFile(checkLogPath)
	if err != nil {
		t.Fatalf("ms minecart check was never called: %v", err)
	}
	checkLog := string(data)
	if !strings.Contains(checkLog, minecartA.ID) {
		t.Errorf("ms minecart check not called for %s; log: %q", minecartA.ID, checkLog)
	}
	if !strings.Contains(checkLog, minecartB.ID) {
		t.Errorf("ms minecart check not called for %s; log: %q", minecartB.ID, checkLog)
	}
}

// TestMinecartManager_ParkedRig_SkipsFeedOnEventPoll verifies that the event poll
// path (CheckMinecartsForIssue → feedNextReadyIssue) skips dispatching issues to
// parked rigs. The minecart is detected and checked, but the ready issue is not
// slung because the target rig is parked.
func TestMinecartManager_ParkedRig_SkipsFeedOnEventPoll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// Minecart tracks two issues: task1 (will close) and task2 (stays open, ready to feed).
	minecart := &beadsdk.Issue{
		ID: "hq-cv-parked", Title: "Parked Rig Minecart",
		Status: beadsdk.StatusOpen, Priority: 2,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}
	task1 := &beadsdk.Issue{
		ID: "ms-parked1", Title: "Task 1 (close me)",
		Status: beadsdk.StatusOpen, Priority: 2,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}
	task2 := &beadsdk.Issue{
		ID: "ms-parked2", Title: "Task 2 (ready but rig parked)",
		Status: beadsdk.StatusOpen, Priority: 3,
		IssueType: beadsdk.TypeTask, CreatedAt: now, UpdatedAt: now,
	}

	for _, issue := range []*beadsdk.Issue{minecart, task1, task2} {
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("CreateIssue %s: %v", issue.ID, err)
		}
	}
	for _, taskID := range []string{task1.ID, task2.ID} {
		dep := &beadsdk.Dependency{
			IssueID: minecart.ID, DependsOnID: taskID,
			Type: beadsdk.DependencyType("tracks"), CreatedAt: now, CreatedBy: "test",
		}
		if err := store.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("AddDependency %s: %v", taskID, err)
		}
	}
	if err := store.CloseIssue(ctx, task1.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue: %v", err)
	}

	// Mock ms with routes for "ms-" prefix → rig "ms"
	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	routes := `{"prefix":"ms-","path":"ms/.beads"}` + "\n"
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	slingLogPath := filepath.Join(binDir, "sling.log")
	gtScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  echo '[]'
  exit 0
fi
if [ "$1" = "minecart" ] && [ "$2" = "check" ]; then
  exit 0
fi
if [ "$1" = "sling" ]; then
  echo "$@" >> "%s"
  exit 0
fi
exit 0
`, slingLogPath)
	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	var mu sync.Mutex
	var logged []string
	logger := func(format string, args ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	// isRigParked returns true for "ms" rig
	parked := func(rig string) bool { return rig == "ms" }
	stores := map[string]beadsdk.Storage{"hq": store}
	m := NewMinecartManager(townRoot, logger, gtPath, 1*time.Hour, stores, nil, parked)
	// Skip seeding so pollStoresSnapshot processes events immediately.
	m.seeded.Store(true)
	m.pollStoresSnapshot(stores)

	mu.Lock()
	snapshot := make([]string, len(logged))
	copy(snapshot, logged)
	mu.Unlock()

	// Close event should be detected and minecart checked.
	assertLogContains(t, snapshot, "close detected", task1.ID)
	assertLogContains(t, snapshot, "tracked by", minecart.ID)
	assertLogContains(t, snapshot, "checking minecart", minecart.ID)

	// Feed should be skipped because rig is parked.
	assertLogContains(t, snapshot, "parked", "ms-parked2")

	// Sling should NOT have been called.
	if _, err := os.Stat(slingLogPath); err == nil {
		data, _ := os.ReadFile(slingLogPath)
		t.Errorf("sling was called for parked rig: %s", data)
	}

	// Should NOT contain "feeding next ready issue" log.
	for _, s := range snapshot {
		if strings.Contains(s, "feeding next ready issue") {
			t.Errorf("expected no feeding for parked rig, got: %s", s)
		}
	}
}

// assertLogContains checks that at least one log line contains all specified substrings.
func assertLogContains(t *testing.T, logs []string, substrings ...string) {
	t.Helper()
	for _, line := range logs {
		allMatch := true
		for _, sub := range substrings {
			if sub != "" && !strings.Contains(line, sub) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return
		}
	}
	t.Errorf("no log line contains all of %v; logs:\n%s", substrings, strings.Join(logs, "\n"))
}

// TestMinecartManager_ShutdownKillsHangingSubprocess verifies that Stop()
// completes within bounded time even when a ms subprocess is hanging.
// This is the critical S-09 test: without CommandContext + process group kill,
// the wg.Wait() in Stop() would block indefinitely.
func TestMinecartManager_ShutdownKillsHangingSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (process groups)")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Mock ms that hangs on stranded (simulates a stuck subprocess).
	gtScript := `#!/bin/sh
if [ "$1" = "minecart" ] && [ "$2" = "stranded" ]; then
  sleep 999
  exit 0
fi
exit 0
`
	gtPath := filepath.Join(binDir, "ms")
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("write mock ms: %v", err)
	}

	var logged []string
	logger := func(format string, args ...interface{}) {
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	// Short scan interval so the hanging ms fires immediately.
	m := NewMinecartManager(townRoot, logger, gtPath, 100*time.Millisecond, nil, nil, nil)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let the stranded scan fire and start hanging on `sleep 999`.
	time.Sleep(500 * time.Millisecond)

	// Stop must complete within bounded time despite the hanging subprocess.
	// Before S-09 (exec.CommandContext), this would block forever.
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("Stop completed cleanly (subprocess was killed)")
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() blocked for >5s -- hanging subprocess was NOT killed by context cancellation")
	}
}
