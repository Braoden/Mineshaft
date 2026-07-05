//go:build windows

package tmux

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// Regression test for the overseer zombie-kill loop: MS_PROCESS_NAMES is set
// with .exe on Windows (e.g. "claude.exe"), but snapshot process names are
// compared with .exe stripped. Both sides must normalize or a healthy agent
// is reported dead and killed every 3 heartbeats.
func TestHasDescendantWithNamesWindows_ExeSuffix(t *testing.T) {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	self := strconv.Itoa(os.Getpid())
	for _, name := range []string{"powershell", "powershell.exe", "POWERSHELL.EXE"} {
		if !hasDescendantWithNamesWindows(self, []string{name}, 0) {
			t.Errorf("descendant %q not found, want match", name)
		}
	}
	if hasDescendantWithNamesWindows(self, []string{"no-such-proc"}, 0) {
		t.Error("matched nonexistent process name")
	}
}
