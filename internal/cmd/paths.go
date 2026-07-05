package cmd

import (
	"os"
	"path/filepath"
)

// gtDataDir returns the directory used for MS's runtime data files
// (logs, telemetry, cost records, etc.).
//
// Resolution order:
//  1. $MS_HOME/.ms  — when MS_HOME is set, data is kept alongside the MS
//     workspace rather than in the user's home directory.
//  2. ~/.ms         — default location when MS_HOME is not set.
func gtDataDir() string {
	if h := os.Getenv("MS_HOME"); h != "" {
		return filepath.Join(h, ".ms")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".ms")
	}
	return filepath.Join(home, ".ms")
}
