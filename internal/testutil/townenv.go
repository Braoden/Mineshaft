package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/mineshaft/internal/workspace"
)

// RequireTownEnv skips the test if the process is not running inside a Gas
// Town workspace. It checks workspace.FindFromCwd and, when a workspace is
// found, verifies that overseer/rigs.json exists (a proxy for a fully
// initialized town).
//
// Returns the workspace root path for use by the caller.
//
// Use this guard for integration tests that shell out to ms/bd or otherwise
// depend on a live Mineshaft directory tree being present. Tests that create
// their own temporary town structure (via t.TempDir) do NOT need this guard.
func RequireTownEnv(t *testing.T) string {
	t.Helper()

	root, err := workspace.FindFromCwd()
	if err != nil {
		t.Skipf("skipping: not in a Mineshaft workspace (%v)", err)
	}
	if root == "" {
		t.Skip("skipping: not in a Mineshaft workspace")
	}

	if _, err := os.Stat(filepath.Join(root, "overseer", "rigs.json")); os.IsNotExist(err) {
		t.Skip("skipping: overseer/rigs.json not found — not a fully initialized town")
	}

	return root
}
