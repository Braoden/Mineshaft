package testutil

import (
	"os"
	"os/exec"
	"strings"
)

// CleanGTEnv returns os.Environ() with MS_* and BD_* variables removed, except
// MS_DOLT_PORT, MS_DOLT_HOST, and MS_TEST_EXTERNAL_DOLT which are preserved so
// subprocesses connect to and reuse the test Dolt server. BEADS_DOLT_PORT and
// BEADS_DOLT_SERVER_HOST (prefix BEADS_, not BD_) pass through implicitly since
// only BD_* is stripped.
//
// Use this when setting cmd.Env on bd/ms subprocess calls in tests.
// If you do NOT set cmd.Env, the process env (including MS_DOLT_PORT) is
// inherited automatically — no need for this function in that case.
func CleanGTEnv(extraEnv ...string) []string {
	var clean []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "MS_") &&
			!strings.HasPrefix(e, "MS_DOLT_PORT=") &&
			!strings.HasPrefix(e, "MS_DOLT_HOST=") &&
			!strings.HasPrefix(e, "MS_TEST_EXTERNAL_DOLT=") {
			continue
		}
		if strings.HasPrefix(e, "BD_") {
			continue
		}
		clean = append(clean, e)
	}
	return append(clean, extraEnv...)
}

// NewBDCommand creates an exec.Command for the bd CLI with MS_DOLT_PORT
// automatically propagated. The command inherits the full process environment
// (which includes MS_DOLT_PORT set by TestMain).
//
// Use this instead of bare exec.Command("bd", ...) in tests.
func NewBDCommand(args ...string) *exec.Cmd {
	return exec.Command("bd", args...)
}

// NewGTCommand creates an exec.Command for the ms CLI with MS_DOLT_PORT
// automatically propagated. The command inherits the full process environment
// (which includes MS_DOLT_PORT set by TestMain).
//
// Use this instead of bare exec.Command("ms", ...) in tests.
func NewGTCommand(args ...string) *exec.Cmd {
	return exec.Command("ms", args...)
}

// NewIsolatedBDCommand creates an exec.Command for the bd CLI with MS_*/BD_*
// env stripped except MS_DOLT_PORT and BEADS_DOLT_PORT. Use this when you need
// to isolate a subprocess from the parent Mineshaft workspace but still route
// to the test Dolt server.
func NewIsolatedBDCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("bd", args...)
	cmd.Env = CleanGTEnv()
	return cmd
}

// NewIsolatedGTCommand creates an exec.Command for the ms CLI with MS_*/BD_*
// env stripped except MS_DOLT_PORT and BEADS_DOLT_PORT. Use this when you need
// to isolate a subprocess from the parent Mineshaft workspace but still route
// to the test Dolt server.
func NewIsolatedGTCommand(args ...string) *exec.Cmd {
	cmd := exec.Command("ms", args...)
	cmd.Env = CleanGTEnv()
	return cmd
}
