package witness

import (
	"strings"
	"testing"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/rig"
)

func TestManagerStartForegroundDeprecated(t *testing.T) {
	mgr := NewManager(&rig.Rig{Name: "testrig", Path: t.TempDir()})
	err := mgr.Start(true, "", nil)
	if err == nil {
		t.Fatal("expected foreground mode deprecation error")
	}
	if !strings.Contains(err.Error(), "foreground mode is deprecated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildWitnessStartCommand_UsesRoleConfig(t *testing.T) {
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		StartCommand: "exec run --town {town} --rig {rig} --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "mineshaft", "/town", "", "", roleCfg, "")
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec env -u CLAUDECODE NODE_OPTIONS='' run --town /town --rig mineshaft --role witness"
	if got != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", got, want)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	t.Parallel()
	got, err := buildWitnessStartCommand("/town/rig", "mineshaft", "/town", "", "", nil, "")
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(got, "MS_ROLE=mineshaft/witness") {
		t.Errorf("expected MS_ROLE=mineshaft/witness in command, got %q", got)
	}
	if !strings.Contains(got, "BD_ACTOR=mineshaft/witness") {
		t.Errorf("expected BD_ACTOR=mineshaft/witness in command, got %q", got)
	}
}

// TestRoleConfigEnvVars_ExpandsQualifiedGTRole verifies that the TOML env vars
// expand MS_ROLE to a qualified value (e.g., "mineshaft/witness" not "witness").
func TestRoleConfigEnvVars_ExpandsQualifiedGTRole(t *testing.T) {
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		EnvVars: map[string]string{
			"MS_ROLE":  "{rig}/witness",
			"MS_SCOPE": "rig",
		},
	}

	got := roleConfigEnvVars(roleCfg, "/town", "mineshaft")
	if got["MS_ROLE"] != "mineshaft/witness" {
		t.Errorf("MS_ROLE = %q, want %q", got["MS_ROLE"], "mineshaft/witness")
	}
	if got["MS_SCOPE"] != "rig" {
		t.Errorf("MS_SCOPE = %q, want %q", got["MS_SCOPE"], "rig")
	}
}

// TestRoleConfigEnvVars_NilConfig verifies nil roleConfig returns nil.
func TestRoleConfigEnvVars_NilConfig(t *testing.T) {
	t.Parallel()
	got := roleConfigEnvVars(nil, "/town", "mineshaft")
	if got != nil {
		t.Errorf("expected nil for nil roleConfig, got %v", got)
	}
}

func TestBuildWitnessStartCommand_IncludesConfigDir(t *testing.T) {
	t.Parallel()
	got, err := buildWitnessStartCommand("/town/rig", "mineshaft", "/town", "", "", nil, "/home/user/.claude-accounts/work")
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(got, "CLAUDE_CONFIG_DIR=/home/user/.claude-accounts/work") {
		t.Errorf("expected CLAUDE_CONFIG_DIR in command, got %q", got)
	}
}

func TestBuildWitnessStartCommand_AgentOverrideWins(t *testing.T) {
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "mineshaft", "/town", "", "codex", roleCfg, "")
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}
	if strings.Contains(got, "exec run") {
		t.Fatalf("expected agent override to bypass role start_command, got %q", got)
	}
	if !strings.Contains(got, "MS_ROLE=mineshaft/witness") {
		t.Errorf("expected MS_ROLE=mineshaft/witness in command, got %q", got)
	}
}
