package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func captureMinecartStdoutErr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	runErr := fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy stdout: %v", err)
	}
	_ = r.Close()

	return buf.String(), runErr
}

func writeRoutingBdStub(t *testing.T, scriptBody string) {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := "#!/bin/sh\n" + scriptBody
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func chdirMinecartTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func makeRoutingTownWorkspace(t *testing.T) (string, string) {
	t.Helper()

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatalf("mkdir overseer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	expectedWD := townRoot
	if resolved, err := filepath.EvalSymlinks(townRoot); err == nil && resolved != "" {
		expectedWD = resolved
	}
	return townRoot, expectedWD
}

func TestRunMinecartList_UsesTownRootAndStripsBeadsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirMinecartTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")

	scriptBody := fmt.Sprintf(`
# Allow-stale version probe is exempt from BEADS_DIR check.
if [ "$*" = "--allow-stale version" ]; then
  exit 0
fi

if [ "$BEADS_DIR" != "%s/.beads" ]; then
  echo "expected hardened BEADS_DIR, got $BEADS_DIR" >&2
  exit 1
fi

case "$*" in
	  "list --label=gt:minecart --json --limit=0 --all --flat")
	    if [ "$PWD" != "%s" ]; then
	      echo "expected town root, got $PWD" >&2
	      exit 1
	    fi
	    echo '[{"id":"hq-cv-town","title":"Town minecart","status":"open","created_at":"2026-03-09T00:00:00Z","labels":["gt:minecart"]}]'
	    ;;
	  "list --json --limit=0 --all --flat")
	    echo '[]'
	    ;;
  "dep list hq-cv-town --direction=down --type=tracks --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[]'
    ;;
  "show hq-cv-town --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-town","title":"Town minecart","status":"open","issue_type":"minecart","dependencies":[]}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedWD, expectedWD, expectedWD, expectedWD)
	writeRoutingBdStub(t, scriptBody)

	oldJSON, oldAll, oldStatus, oldTree := minecartListJSON, minecartListAll, minecartListStatus, minecartListTree
	minecartListJSON = true
	minecartListAll = true
	minecartListStatus = ""
	minecartListTree = false
	t.Cleanup(func() {
		minecartListJSON = oldJSON
		minecartListAll = oldAll
		minecartListStatus = oldStatus
		minecartListTree = oldTree
	})

	out, err := captureMinecartStdoutErr(t, func() error {
		return runMinecartList(nil, nil)
	})
	if err != nil {
		t.Fatalf("runMinecartList: %v", err)
	}
	if !strings.Contains(out, `"id": "hq-cv-town"`) {
		t.Fatalf("expected minecart JSON output, got:\n%s", out)
	}
}

func TestRunMinecartStatus_UsesTownRootAndStripsBeadsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirMinecartTest(t, townRoot)
	t.Setenv("BEADS_DIR", "/wrong/.beads")

	scriptBody := fmt.Sprintf(`
# Allow-stale version probe is exempt from BEADS_DIR check.
if [ "$*" = "--allow-stale version" ]; then
  exit 0
fi

if [ "$BEADS_DIR" != "%s/.beads" ]; then
  echo "expected hardened BEADS_DIR, got $BEADS_DIR" >&2
  exit 1
fi

case "$*" in
  "show hq-cv-status --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-status","title":"Status minecart","status":"open","issue_type":"minecart","created_at":"2026-03-09T00:00:00Z","labels":[],"dependencies":[]}]'
    ;;
  "dep list hq-cv-status --direction=down --type=tracks --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedWD, expectedWD, expectedWD)
	writeRoutingBdStub(t, scriptBody)

	oldJSON := minecartStatusJSON
	minecartStatusJSON = false
	t.Cleanup(func() { minecartStatusJSON = oldJSON })

	out, err := captureMinecartStdoutErr(t, func() error {
		return runMinecartStatus(nil, []string{"hq-cv-status"})
	})
	if err != nil {
		t.Fatalf("runMinecartStatus: %v", err)
	}
	if !strings.Contains(out, "hq-cv-status") || !strings.Contains(out, "Progress:  0/0 completed") {
		t.Fatalf("unexpected status output:\n%s", out)
	}
}

// TestMinecartCreate_UsesTrackingHelper verifies minecart create delegates tracking
// to the in-process helper instead of shelling out to `bd dep add`.
func TestMinecartCreate_UsesTrackingHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirMinecartTest(t, townRoot)

	// Write sentinel files to skip EnsureCustomTypes/Statuses (they call bd
	// config set/get which isn't relevant to routing).
	beadsDir := filepath.Join(townRoot, ".beads")
	typesList := "agent,role,rig,minecart,slot,queue,event,message,molecule,gate,merge-request"
	_ = os.WriteFile(filepath.Join(beadsDir, ".gt-types-configured"), []byte(typesList), 0644)
	_ = os.WriteFile(filepath.Join(beadsDir, ".gt-statuses-configured"), []byte("staged_ready,staged_warnings"), 0644)

	var helperTownRoot, helperMinecartID, helperIssueID string
	oldAddTracking := addTrackingRelationFn
	addTrackingRelationFn = func(townRoot, minecartID, issueID string) error {
		helperTownRoot = townRoot
		helperMinecartID = minecartID
		helperIssueID = issueID
		return nil
	}
	t.Cleanup(func() { addTrackingRelationFn = oldAddTracking })

	scriptBody := `
case "$1" in
  create)
    echo '[{"id":"hq-cv-test"}]'
    ;;
  init|config)
    exit 0
    ;;
  *)
    echo '[]'
    ;;
esac
`
	writeRoutingBdStub(t, scriptBody)

	// Override the entropy source for deterministic minecart IDs.
	oldEntropy := minecartIDEntropy
	minecartIDEntropy = strings.NewReader("abcde")
	t.Cleanup(func() { minecartIDEntropy = oldEntropy })

	_, err := captureMinecartStdoutErr(t, func() error {
		return runMinecartCreate(nil, []string{"test-minecart", "mo-2sh.1"})
	})
	if err != nil {
		t.Fatalf("runMinecartCreate: %v", err)
	}

	if helperTownRoot != expectedWD {
		t.Errorf("tracking helper townRoot = %q, want %q", helperTownRoot, expectedWD)
	}
	if helperMinecartID != "hq-cv-pqrst" {
		t.Errorf("tracking helper minecartID = %q, want %q", helperMinecartID, "hq-cv-pqrst")
	}
	if helperIssueID != "mo-2sh.1" {
		t.Errorf("tracking helper issueID = %q, want %q", helperIssueID, "mo-2sh.1")
	}
}

func TestMinecartAdd_UsesTrackingHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, expectedWD := makeRoutingTownWorkspace(t)
	chdirMinecartTest(t, townRoot)

	var helperTownRoot, helperMinecartID string
	var helperIssues []string
	oldAddTracking := addTrackingRelationFn
	addTrackingRelationFn = func(townRoot, minecartID, issueID string) error {
		helperTownRoot = townRoot
		helperMinecartID = minecartID
		helperIssues = append(helperIssues, issueID)
		return nil
	}
	t.Cleanup(func() { addTrackingRelationFn = oldAddTracking })

	scriptBody := fmt.Sprintf(`
case "$*" in
  "show hq-cv-test --json")
    if [ "$PWD" != "%s" ]; then
      echo "expected town root, got $PWD" >&2
      exit 1
    fi
    echo '[{"id":"hq-cv-test","title":"Test Minecart","status":"open","issue_type":"minecart"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`, expectedWD)
	writeRoutingBdStub(t, scriptBody)

	_, err := captureMinecartStdoutErr(t, func() error {
		return runMinecartAdd(nil, []string{"hq-cv-test", "ag-95s.1", "ag-95s.2"})
	})
	if err != nil {
		t.Fatalf("runMinecartAdd: %v", err)
	}

	if helperTownRoot != expectedWD {
		t.Errorf("tracking helper townRoot = %q, want %q", helperTownRoot, expectedWD)
	}
	if helperMinecartID != "hq-cv-test" {
		t.Errorf("tracking helper minecartID = %q, want %q", helperMinecartID, "hq-cv-test")
	}
	if got := strings.Join(helperIssues, ","); got != "ag-95s.1,ag-95s.2" {
		t.Errorf("tracking helper issues = %q, want %q", got, "ag-95s.1,ag-95s.2")
	}
}
