package tmux

import (
	"strings"
	"testing"
)

// TestSanitizedEnvStripsNestingMarkers verifies the mi-z9r fix: tmux client
// subprocesses must not inherit TMUX or PSMUX_SESSION, or the multiplexer's
// nesting guard refuses new-session (psmux exits 0, so sling fails later at
// send-keys with a misleading "no server running").
func TestSanitizedEnvStripsNestingMarkers(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/ms-town,123,0")
	t.Setenv("PSMUX_SESSION", "Mineshaft-overseer")
	t.Setenv("TMUX_PANE", "%5")

	env := sanitizedEnv()

	foundPane := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "TMUX=") || strings.HasPrefix(kv, "PSMUX_SESSION=") {
			t.Errorf("sanitizedEnv leaked nesting marker %q", kv)
		}
		if kv == "TMUX_PANE=%5" {
			foundPane = true
		}
	}
	if !foundPane {
		t.Error("sanitizedEnv must keep TMUX_PANE (tmux resolves the current pane from it)")
	}
}
