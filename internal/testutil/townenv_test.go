package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequireTownEnv_ReturnsRoot(t *testing.T) {
	root := RequireTownEnv(t)

	// If we got here (didn't skip), root must be non-empty.
	if root == "" {
		t.Fatal("RequireTownEnv returned empty root")
	}

	// The returned root must contain overseer/rigs.json (the check we just added).
	rigsPath := filepath.Join(root, "overseer", "rigs.json")
	if _, err := os.Stat(rigsPath); err != nil {
		t.Errorf("overseer/rigs.json not found at %s: %v", rigsPath, err)
	}
}
