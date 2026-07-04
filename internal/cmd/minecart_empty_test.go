package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mockBdForMinecartTest creates a fake bd binary tailored for minecart empty-check
// tests. The script handles show, dep, close, and list subcommands.
// closeLogPath is the file where close commands are logged for verification.
func mockBdForMinecartTest(t *testing.T, minecartID, minecartTitle string) (binDir, townRoot, closeLogPath string) {
	t.Helper()

	binDir = t.TempDir()
	townRoot = t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	closeLogPath = filepath.Join(binDir, "bd-close.log")

	bdPath := filepath.Join(binDir, "bd")
	if runtime.GOOS == "windows" {
		t.Skip("skipping minecart empty test on Windows")
	}

	// Shell script that handles the bd subcommands needed by
	// checkSingleMinecart and findStrandedMinecarts.
	script := `#!/bin/sh
CLOSE_LOG="` + closeLogPath + `"
MINECART_ID="` + minecartID + `"
MINECART_TITLE="` + minecartTitle + `"

# Find the actual subcommand (skip global flags like --allow-stale)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;; # skip flags
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  show)
    # Return minecart JSON
    echo '[{"id":"'"$MINECART_ID"'","title":"'"$MINECART_TITLE"'","status":"open","issue_type":"minecart"}]'
    exit 0
    ;;
  sql)
    # bdDepListRawIDs uses bd sql for dep queries — return empty
    echo '[]'
    exit 0
    ;;
  dep)
    # Return empty tracked issues
    echo '[]'
    exit 0
    ;;
  close)
    # Log the close command for verification
    echo "$@" >> "$CLOSE_LOG"
    exit 0
    ;;
  list)
    # Return one open minecart
    echo '[{"id":"'"$MINECART_ID"'","title":"'"$MINECART_TITLE"'"}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	// Prepend mock bd to PATH
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return binDir, townRoot, closeLogPath
}

func TestCheckSingleMinecart_EmptyMinecartDoesNotAutoClose(t *testing.T) {
	// When getTrackedIssues returns empty (cross-rig resolution failure or truly
	// no tracked issues), closeMinecartIfComplete must NOT auto-close. A 0/0 result
	// means "could not resolve", not "all done". (GH#hq-439)
	_, townBeads, closeLogPath := mockBdForMinecartTest(t, "hq-empty1", "Empty test minecart")

	err := checkSingleMinecart(townBeads, "hq-empty1", false)
	if err != nil {
		t.Fatalf("checkSingleMinecart() error: %v", err)
	}

	// Verify bd close was NOT called — 0 tracked issues = no auto-close.
	_, err = os.ReadFile(closeLogPath)
	if err == nil {
		t.Error("minecart with 0 tracked issues should NOT be auto-closed, but close log exists")
	}
}

func TestCheckSingleMinecart_EmptyMinecartDryRun(t *testing.T) {
	_, townBeads, closeLogPath := mockBdForMinecartTest(t, "hq-empty2", "Dry run minecart")

	err := checkSingleMinecart(townBeads, "hq-empty2", true)
	if err != nil {
		t.Fatalf("checkSingleMinecart() dry-run error: %v", err)
	}

	// In dry-run mode, bd close should NOT be called
	_, err = os.ReadFile(closeLogPath)
	if err == nil {
		t.Error("dry-run should not call bd close, but close log exists")
	}
}

func TestFindStrandedMinecarts_EmptyMinecartFlagged(t *testing.T) {
	_, townBeads, _ := mockBdForMinecartTest(t, "hq-empty3", "Stranded empty minecart")

	stranded, err := findStrandedMinecarts(townBeads)
	if err != nil {
		t.Fatalf("findStrandedMinecarts() error: %v", err)
	}

	if len(stranded) != 1 {
		t.Fatalf("expected 1 stranded minecart, got %d", len(stranded))
	}

	s := stranded[0]
	if s.ID != "hq-empty3" {
		t.Errorf("stranded minecart ID = %q, want %q", s.ID, "hq-empty3")
	}
	if s.ReadyCount != 0 {
		t.Errorf("stranded ReadyCount = %d, want 0", s.ReadyCount)
	}
	if len(s.ReadyIssues) != 0 {
		t.Errorf("stranded ReadyIssues = %v, want empty", s.ReadyIssues)
	}
}

// TestFindStrandedMinecarts_MixedMinecarts verifies that findStrandedMinecarts
// correctly returns both empty (cleanup) and feedable (has ready issues)
// minecarts, and that the JSON output shape is correct for each type.
func TestFindStrandedMinecarts_MixedMinecarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping minecart test on Windows")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	// Routes needed so isSlingableBead can resolve gt- prefix to a rig
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(`{"prefix":"gt-","path":"excavation/overseer/rig"}`+"\n"), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	bdPath := filepath.Join(binDir, "bd")

	// Mock bd that returns two minecarts: one empty, one with a ready issue.
	// Uses positional arg parsing to dispatch on minecart ID for dep commands.
	script := `#!/bin/sh
# Collect positional args (skip flags)
i=0
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) eval "pos$i=\"$arg\""; i=$((i+1)) ;;
  esac
done

case "$pos0" in
  list)
    echo '[{"id":"hq-empty-mix","title":"Empty minecart"},{"id":"hq-feed-mix","title":"Feedable minecart"}]'
    exit 0
    ;;
  sql)
    # bdDepListRawIDs: SELECT depends_on_id FROM dependencies WHERE issue_id = '<id>' AND type = 'tracks'
    case "$*" in
      *"issue_id = 'hq-empty-mix'"*)
        echo '[]'
        ;;
      *"issue_id = 'hq-feed-mix'"*)
        echo '[{"depends_on_id":"gt-ready1"}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    exit 0
    ;;
  dep)
    # pos2 is the minecart ID (dep list <minecart-id> ...)
    case "$pos2" in
      hq-empty-mix)
        echo '[]'
        ;;
      hq-feed-mix)
        echo '[{"id":"gt-ready1","title":"Ready issue","status":"open","issue_type":"task","assignee":"","dependency_type":"tracks"}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    exit 0
    ;;
  show)
    # Return issue details for any show query
    echo '[{"id":"gt-ready1","title":"Ready issue","status":"open","issue_type":"task","assignee":"","blocked_by":[],"blocked_by_count":0,"dependencies":[]}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Pass townRoot (not .beads) — matches getTownBeadsDir() which returns the workspace root.
	stranded, err := findStrandedMinecarts(townRoot)
	if err != nil {
		t.Fatalf("findStrandedMinecarts() error: %v", err)
	}

	if len(stranded) != 2 {
		t.Fatalf("expected 2 stranded minecarts, got %d", len(stranded))
	}

	// Build a map for easier assertions
	byID := map[string]strandedMinecartInfo{}
	for _, s := range stranded {
		byID[s.ID] = s
	}

	// Verify empty minecart
	empty, ok := byID["hq-empty-mix"]
	if !ok {
		t.Fatal("missing empty minecart hq-empty-mix in stranded results")
	}
	if empty.ReadyCount != 0 {
		t.Errorf("empty minecart ReadyCount = %d, want 0", empty.ReadyCount)
	}
	if empty.TrackedCount != 0 {
		t.Errorf("empty minecart TrackedCount = %d, want 0", empty.TrackedCount)
	}
	if len(empty.ReadyIssues) != 0 {
		t.Errorf("empty minecart ReadyIssues = %v, want empty", empty.ReadyIssues)
	}

	// Verify feedable minecart
	feedable, ok := byID["hq-feed-mix"]
	if !ok {
		t.Fatal("missing feedable minecart hq-feed-mix in stranded results")
	}
	if feedable.ReadyCount != 1 {
		t.Errorf("feedable minecart ReadyCount = %d, want 1", feedable.ReadyCount)
	}
	if feedable.TrackedCount != 1 {
		t.Errorf("feedable minecart TrackedCount = %d, want 1", feedable.TrackedCount)
	}
	if len(feedable.ReadyIssues) != 1 || feedable.ReadyIssues[0] != "gt-ready1" {
		t.Errorf("feedable minecart ReadyIssues = %v, want [gt-ready1]", feedable.ReadyIssues)
	}

	// Verify JSON encoding shape — empty slice encodes as [] not null
	jsonBytes, err := json.Marshal(stranded)
	if err != nil {
		t.Fatalf("json.Marshal(stranded): %v", err)
	}
	jsonStr := string(jsonBytes)
	if strings.Contains(jsonStr, `"ready_issues":null`) {
		t.Error("JSON output contains ready_issues:null — should be [] for empty minecarts")
	}
	// Verify tracked_count appears in JSON
	if !strings.Contains(jsonStr, `"tracked_count"`) {
		t.Error("JSON output missing tracked_count field")
	}
}

// TestFindStrandedMinecarts_StuckMinecart verifies that a minecart with tracked
// issues but none ready (stuck) is included in the stranded list with
// TrackedCount > 0 and ReadyCount == 0, preventing accidental auto-close.
func TestFindStrandedMinecarts_StuckMinecart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping minecart test on Windows")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(`{"prefix":"gt-","path":"excavation/overseer/rig"}`+"\n"), 0644); err != nil {
		t.Fatalf("write routes: %v", err)
	}

	bdPath := filepath.Join(binDir, "bd")

	// Mock bd: minecart has tracked issues but all are blocked — none are ready.
	script := `#!/bin/sh
i=0
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) eval "pos$i=\"$arg\""; i=$((i+1)) ;;
  esac
done

case "$pos0" in
  list)
    echo '[{"id":"hq-stuck1","title":"Stuck minecart"}]'
    exit 0
    ;;
  sql)
    # bdDepListRawIDs: return tracked bead IDs for hq-stuck1
    echo '[{"depends_on_id":"gt-busy1"},{"depends_on_id":"gt-busy2"}]'
    exit 0
    ;;
  dep)
    # All tracked issues are open but blocked — none are ready
    echo '[{"id":"gt-busy1","title":"Blocked issue 1","status":"open","issue_type":"task","assignee":"","dependency_type":"tracks"},{"id":"gt-busy2","title":"Blocked issue 2","status":"open","issue_type":"task","assignee":"","dependency_type":"tracks"}]'
    exit 0
    ;;
  show)
    # Both issues have blockers so isReadyIssue returns false
    echo '[{"id":"gt-busy1","title":"Blocked issue 1","status":"open","issue_type":"task","assignee":"","blocked_by":["gt-blocker1"],"blocked_by_count":1,"dependencies":[]},{"id":"gt-busy2","title":"Blocked issue 2","status":"open","issue_type":"task","assignee":"","blocked_by":["gt-blocker1"],"blocked_by_count":1,"dependencies":[]}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stranded, err := findStrandedMinecarts(townRoot)
	if err != nil {
		t.Fatalf("findStrandedMinecarts() error: %v", err)
	}

	if len(stranded) != 1 {
		t.Fatalf("expected 1 stranded minecart (stuck), got %d", len(stranded))
	}

	s := stranded[0]
	if s.ID != "hq-stuck1" {
		t.Errorf("stranded minecart ID = %q, want %q", s.ID, "hq-stuck1")
	}
	if s.TrackedCount != 2 {
		t.Errorf("stuck minecart TrackedCount = %d, want 2", s.TrackedCount)
	}
	if s.ReadyCount != 0 {
		t.Errorf("stuck minecart ReadyCount = %d, want 0", s.ReadyCount)
	}
}
