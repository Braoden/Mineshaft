package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

func setupHandoffTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("ms", "mineshaft")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestHandoffStdinFlag(t *testing.T) {
	t.Run("errors when both stdin and message provided", func(t *testing.T) {
		// Save and restore flag state
		origMessage := handoffMessage
		origStdin := handoffStdin
		defer func() {
			handoffMessage = origMessage
			handoffStdin = origStdin
		}()

		handoffMessage = "some message"
		handoffStdin = true

		err := runHandoff(handoffCmd, nil)
		if err == nil {
			t.Fatal("expected error when both --stdin and --message are set")
		}
		if !strings.Contains(err.Error(), "cannot use --stdin with --message/-m") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestSessionWorkDir(t *testing.T) {
	setupHandoffTestRegistry(t)
	townRoot := "/home/test/ms"

	tests := []struct {
		name        string
		sessionName string
		wantDir     string
		wantErr     bool
	}{
		{
			name:        "overseer runs from overseer subdirectory",
			sessionName: "hq-overseer",
			wantDir:     townRoot + "/overseer",
			wantErr:     false,
		},
		{
			name:        "supervisor runs from supervisor subdirectory",
			sessionName: "hq-supervisor",
			wantDir:     townRoot + "/supervisor",
			wantErr:     false,
		},
		{
			name:        "crew runs from crew subdirectory",
			sessionName: "ms-crew-holden",
			wantDir:     townRoot + "/mineshaft/crew/holden",
			wantErr:     false,
		},
		{
			name:        "witness runs from witness directory",
			sessionName: "ms-witness",
			wantDir:     townRoot + "/mineshaft/witness",
			wantErr:     false,
		},
		{
			name:        "refinery runs from refinery/rig directory",
			sessionName: "ms-refinery",
			wantDir:     townRoot + "/mineshaft/refinery/rig",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, err := sessionWorkDir(tt.sessionName, townRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("sessionWorkDir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotDir != tt.wantDir {
				t.Errorf("sessionWorkDir() = %q, want %q", gotDir, tt.wantDir)
			}
		})
	}
}

func TestBuildRestartCommand_UsesRoleAgentsWhenNoAgentOverride(t *testing.T) {
	setupHandoffTestRegistry(t)

	origCwd, _ := os.Getwd()
	origGTAgent := os.Getenv("MS_AGENT")
	origTownRoot := os.Getenv("MS_TOWN_ROOT")
	origRoot := os.Getenv("MS_ROOT")

	// TempDir must be called BEFORE registering the Chdir cleanup so that
	// LIFO ordering restores the working directory before TempDir removal.
	// On Windows the directory cannot be deleted while the process CWD is
	// inside it.
	townRoot := t.TempDir()

	t.Cleanup(func() {
		_ = os.Chdir(origCwd)
		_ = os.Setenv("MS_AGENT", origGTAgent)
		_ = os.Setenv("MS_TOWN_ROOT", origTownRoot)
		_ = os.Setenv("MS_ROOT", origRoot)
	})
	rigPath := filepath.Join(townRoot, "mineshaft")
	witnessDir := filepath.Join(rigPath, "witness")

	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatalf("mkdir overseer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte(`{"name":"mineshaft"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(witnessDir, 0755); err != nil {
		t.Fatalf("mkdir witness dir: %v", err)
	}

	townSettings := config.NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.Agents = map[string]*config.RuntimeConfig{
		"claude-sonnet": {
			Command: "claude",
			Args:    []string{"--dangerously-skip-permissions", "--model", "sonnet"},
		},
	}
	townSettings.RoleAgents = map[string]string{
		"witness": "claude-sonnet",
	}
	if err := config.SaveTownSettings(config.TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), config.NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	if err := os.Setenv("MS_AGENT", ""); err != nil {
		t.Fatalf("Setenv MS_AGENT: %v", err)
	}
	if err := os.Setenv("MS_TOWN_ROOT", ""); err != nil {
		t.Fatalf("Setenv MS_TOWN_ROOT: %v", err)
	}
	if err := os.Setenv("MS_ROOT", ""); err != nil {
		t.Fatalf("Setenv MS_ROOT: %v", err)
	}
	if err := os.Chdir(witnessDir); err != nil {
		t.Fatalf("chdir witness dir: %v", err)
	}

	cmd, err := buildRestartCommand("ms-witness")
	if err != nil {
		t.Fatalf("buildRestartCommand: %v", err)
	}

	if !strings.Contains(cmd, "--model sonnet") {
		t.Errorf("expected role_agents witness model flag in restart command, got: %q", cmd)
	}
}

func TestBuildRestartCommand_MergesAgentPresetEnv(t *testing.T) {
	// Regression test: ensure agent preset Env block (config.json [agents.X.env])
	// is fully merged into the respawn command, not just NODE_OPTIONS.
	// Without this, custom env vars like ANTHROPIC_BASE_URL configured for
	// proxied Claude were silently dropped on handoff/respawn.
	setupHandoffTestRegistry(t)

	origCwd, _ := os.Getwd()
	origGTAgent := os.Getenv("MS_AGENT")
	origTownRoot := os.Getenv("MS_TOWN_ROOT")
	origRoot := os.Getenv("MS_ROOT")

	townRoot := t.TempDir()

	t.Cleanup(func() {
		_ = os.Chdir(origCwd)
		_ = os.Setenv("MS_AGENT", origGTAgent)
		_ = os.Setenv("MS_TOWN_ROOT", origTownRoot)
		_ = os.Setenv("MS_ROOT", origRoot)
	})
	rigPath := filepath.Join(townRoot, "mineshaft")
	witnessDir := filepath.Join(rigPath, "witness")

	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatalf("mkdir overseer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte(`{"name":"mineshaft"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(witnessDir, 0755); err != nil {
		t.Fatalf("mkdir witness dir: %v", err)
	}

	townSettings := config.NewTownSettings()
	townSettings.DefaultAgent = "claude-proxy"
	townSettings.Agents = map[string]*config.RuntimeConfig{
		"claude-proxy": {
			Command: "claude",
			Args:    []string{"--dangerously-skip-permissions"},
			Env: map[string]string{
				"ANTHROPIC_BASE_URL":       "http://localhost:8080",
				"CLAUDE_CODE_OAUTH_TOKEN":  "placeholder",
				"ANTHROPIC_CUSTOM_HEADERS": "Authorization: Bearer prx_test",
			},
		},
	}
	if err := config.SaveTownSettings(config.TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), config.NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	_ = os.Setenv("MS_AGENT", "claude-proxy")
	_ = os.Setenv("MS_TOWN_ROOT", "")
	_ = os.Setenv("MS_ROOT", "")
	if err := os.Chdir(witnessDir); err != nil {
		t.Fatalf("chdir witness dir: %v", err)
	}

	cmd, err := buildRestartCommand("ms-witness")
	if err != nil {
		t.Fatalf("buildRestartCommand: %v", err)
	}

	wantEnv := map[string]string{
		"ANTHROPIC_BASE_URL":       "http://localhost:8080",
		"CLAUDE_CODE_OAUTH_TOKEN":  "placeholder",
		"ANTHROPIC_CUSTOM_HEADERS": "Authorization: Bearer prx_test",
	}
	for k, v := range wantEnv {
		if !strings.Contains(cmd, k+"=") {
			t.Errorf("agent preset env %q not exported in restart command\ncmd: %s", k, cmd)
		}
		if !strings.Contains(cmd, v) {
			t.Errorf("agent preset env value for %q (%q) missing in restart command\ncmd: %s", k, v, cmd)
		}
	}
}

func TestBuildRestartCommandWithOpts_ContinuePrompt(t *testing.T) {
	setupHandoffTestRegistry(t)

	origCwd, _ := os.Getwd()
	origGTAgent := os.Getenv("MS_AGENT")
	origTownRoot := os.Getenv("MS_TOWN_ROOT")
	origRoot := os.Getenv("MS_ROOT")

	townRoot := t.TempDir()

	t.Cleanup(func() {
		_ = os.Chdir(origCwd)
		_ = os.Setenv("MS_AGENT", origGTAgent)
		_ = os.Setenv("MS_TOWN_ROOT", origTownRoot)
		_ = os.Setenv("MS_ROOT", origRoot)
	})
	rigPath := filepath.Join(townRoot, "mineshaft")
	crewDir := filepath.Join(rigPath, "crew", "bear")

	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatalf("mkdir overseer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "town.json"), []byte(`{"name":"mineshaft"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatalf("mkdir crew dir: %v", err)
	}

	townSettings := config.NewTownSettings()
	townSettings.DefaultAgent = "claude"
	if err := config.SaveTownSettings(config.TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), config.NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	_ = os.Setenv("MS_AGENT", "")
	_ = os.Setenv("MS_TOWN_ROOT", "")
	_ = os.Setenv("MS_ROOT", "")
	_ = os.Chdir(crewDir)

	t.Run("custom ContinuePrompt overrides default", func(t *testing.T) {
		cmd, err := buildRestartCommandWithOpts("ms-crew-bear", buildRestartCommandOpts{
			ContinueSession: true,
			ContinuePrompt:  "Context compacted. Continue your previous task.",
		})
		if err != nil {
			t.Fatalf("buildRestartCommandWithOpts: %v", err)
		}
		if !strings.Contains(cmd, "--continue") {
			t.Errorf("expected --continue flag in restart command, got: %q", cmd)
		}
		if !strings.Contains(cmd, "Context compacted") {
			t.Errorf("expected custom prompt in restart command, got: %q", cmd)
		}
	})

	t.Run("empty ContinuePrompt falls back to default", func(t *testing.T) {
		cmd, err := buildRestartCommandWithOpts("ms-crew-bear", buildRestartCommandOpts{
			ContinueSession: true,
		})
		if err != nil {
			t.Fatalf("buildRestartCommandWithOpts: %v", err)
		}
		if !strings.Contains(cmd, "--continue") {
			t.Errorf("expected --continue flag in restart command, got: %q", cmd)
		}
		if !strings.Contains(cmd, "Continue your previous task") {
			t.Errorf("expected default continuation message when ContinuePrompt is empty, got: %q", cmd)
		}
	})

	t.Run("ContinueSession false uses beacon", func(t *testing.T) {
		cmd, err := buildRestartCommandWithOpts("ms-crew-bear", buildRestartCommandOpts{
			ContinueSession: false,
		})
		if err != nil {
			t.Fatalf("buildRestartCommandWithOpts: %v", err)
		}
		if strings.Contains(cmd, "--continue") {
			t.Errorf("expected no --continue flag when ContinueSession is false, got: %q", cmd)
		}
	})
}

func TestDetectTownRootFromCwd_EnvFallback(t *testing.T) {
	// Save original env vars and restore after test
	origTownRoot := os.Getenv("MS_TOWN_ROOT")
	origRoot := os.Getenv("MS_ROOT")
	defer func() {
		os.Setenv("MS_TOWN_ROOT", origTownRoot)
		os.Setenv("MS_ROOT", origRoot)
	}()

	// Create a temp directory that looks like a valid town
	tmpTown := t.TempDir()
	overseerDir := filepath.Join(tmpTown, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatalf("creating overseer dir: %v", err)
	}
	townJSON := filepath.Join(overseerDir, "town.json")
	if err := os.WriteFile(townJSON, []byte(`{"name": "test-town"}`), 0644); err != nil {
		t.Fatalf("creating town.json: %v", err)
	}

	// Clear both env vars initially
	os.Setenv("MS_TOWN_ROOT", "")
	os.Setenv("MS_ROOT", "")

	t.Run("uses MS_TOWN_ROOT when cwd detection fails", func(t *testing.T) {
		// Set MS_TOWN_ROOT to our temp town
		os.Setenv("MS_TOWN_ROOT", tmpTown)
		os.Setenv("MS_ROOT", "")

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should use MS_TOWN_ROOT fallback)", result, tmpTown)
		}
	})

	t.Run("uses MS_ROOT when MS_TOWN_ROOT not set", func(t *testing.T) {
		// Set only MS_ROOT
		os.Setenv("MS_TOWN_ROOT", "")
		os.Setenv("MS_ROOT", tmpTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should use MS_ROOT fallback)", result, tmpTown)
		}
	})

	t.Run("prefers MS_TOWN_ROOT over MS_ROOT", func(t *testing.T) {
		// Create another temp town for MS_ROOT
		anotherTown := t.TempDir()
		anotherOverseer := filepath.Join(anotherTown, "overseer")
		os.MkdirAll(anotherOverseer, 0755)
		os.WriteFile(filepath.Join(anotherOverseer, "town.json"), []byte(`{"name": "other-town"}`), 0644)

		// Set both env vars
		os.Setenv("MS_TOWN_ROOT", tmpTown)
		os.Setenv("MS_ROOT", anotherTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should prefer MS_TOWN_ROOT)", result, tmpTown)
		}
	})

	t.Run("ignores invalid MS_TOWN_ROOT", func(t *testing.T) {
		// Set MS_TOWN_ROOT to non-existent path, MS_ROOT to valid
		os.Setenv("MS_TOWN_ROOT", "/nonexistent/path/to/town")
		os.Setenv("MS_ROOT", tmpTown)

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != tmpTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should skip invalid MS_TOWN_ROOT and use MS_ROOT)", result, tmpTown)
		}
	})

	t.Run("uses secondary marker when primary missing", func(t *testing.T) {
		// Create a temp town with only overseer/ directory (no town.json)
		secondaryTown := t.TempDir()
		overseerOnlyDir := filepath.Join(secondaryTown, workspace.SecondaryMarker)
		os.MkdirAll(overseerOnlyDir, 0755)

		os.Setenv("MS_TOWN_ROOT", secondaryTown)
		os.Setenv("MS_ROOT", "")

		// Save cwd, cd to a non-town directory, and restore after
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		defer os.Chdir(origCwd)

		result := detectTownRootFromCwd()
		if result != secondaryTown {
			t.Errorf("detectTownRootFromCwd() = %q, want %q (should accept secondary marker)", result, secondaryTown)
		}
	})
}

// makeTestGitRepo creates a minimal git repo in a temp dir and returns its path.
// The caller is responsible for cleanup via t.Cleanup or defer os.RemoveAll.
func makeTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
		// Disable background processes that hold file handles open after exit —
		// causes TempDir cleanup failures on Windows.
		{"git", "-C", dir, "config", "gc.auto", "0"},
		{"git", "-C", dir, "config", "core.fsmonitor", "false"},
		{"git", "-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			t.Fatalf("git setup %v: %v", args, err)
		}
	}
	return dir
}

// TestHandoffMinerEnvCheck verifies that the miner guard in runHandoff uses
// MS_ROLE as the authoritative check, so coordinators with a stale MS_MINER
// in their environment are not redirected to ms done (GH #1707).
func TestHandoffMinerEnvCheck(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		miner   string
		wantBlock bool
	}{
		{
			name:      "bare miner role is redirected",
			role:      "miner",
			miner:   "alpha",
			wantBlock: true,
		},
		{
			name:      "compound miner role is redirected",
			role:      "mineshaft/miners/Toast",
			miner:   "Toast",
			wantBlock: true,
		},
		{
			name:      "overseer with stale MS_MINER is NOT redirected",
			role:      "overseer",
			miner:   "alpha",
			wantBlock: false,
		},
		{
			name:      "compound witness with stale MS_MINER is NOT redirected",
			role:      "mineshaft/witness",
			miner:   "alpha",
			wantBlock: false,
		},
		{
			name:      "crew with stale MS_MINER is NOT redirected",
			role:      "crew",
			miner:   "alpha",
			wantBlock: false,
		},
		{
			name:      "compound crew with stale MS_MINER is NOT redirected",
			role:      "mineshaft/crew/den",
			miner:   "alpha",
			wantBlock: false,
		},
		{
			name:      "no MS_ROLE with MS_MINER set is redirected",
			role:      "",
			miner:   "alpha",
			wantBlock: true,
		},
		{
			name:      "no MS_ROLE and no MS_MINER is not redirected",
			role:      "",
			miner:   "",
			wantBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			gtLog := filepath.Join(t.TempDir(), "ms.log")
			_ = writeBDStub(t, binDir, "#!/bin/sh\nexit 0\n", "@echo off\r\nexit /b 0\r\n")
			gtStub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + gtLog + "\"\nprintf 'stub ms %s\\n' \"$*\"\nexit 0\n"
			if err := os.WriteFile(filepath.Join(binDir, "ms"), []byte(gtStub), 0755); err != nil {
				t.Fatalf("write ms stub: %v", err)
			}
			gtCmdStub := "@echo off\r\necho %* >> \"" + gtLog + "\"\r\necho stub ms %*\r\nexit /b 0\r\n"
			if err := os.WriteFile(filepath.Join(binDir, "ms.cmd"), []byte(gtCmdStub), 0644); err != nil {
				t.Fatalf("write ms.cmd stub: %v", err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			isolatedRoot := t.TempDir()
			t.Setenv("MS_TOWN_ROOT", isolatedRoot)
			t.Setenv("MS_ROOT", isolatedRoot)
			t.Chdir(isolatedRoot)
			t.Setenv("MS_ROLE", tt.role)
			t.Setenv("MS_MINER", tt.miner)
			// Ensure deterministic non-tmux execution so the non-miner
			// paths fail predictably instead of triggering real side effects.
			t.Setenv("TMUX", "")
			t.Setenv("TMUX_PANE", "")

			// Reset flags to avoid interference
			origMessage := handoffMessage
			origStdin := handoffStdin
			origAuto := handoffAuto
			defer func() {
				handoffMessage = origMessage
				handoffStdin = origStdin
				handoffAuto = origAuto
			}()
			handoffMessage = ""
			handoffStdin = false
			handoffAuto = false

			// The miner path tries to exec "ms done" which will fail in tests.
			// We capture stdout to detect the "Miner detected" message, which
			// confirms the miner guard triggered. Non-miner paths will fail
			// later (missing tmux, etc.) without printing the miner message.
			var blocked bool
			output := captureStdout(t, func() {
				defer func() {
					if r := recover(); r != nil {
						// Panic means we got past the guard — not blocked
					}
				}()
				runHandoff(handoffCmd, nil)
			})
			blocked = strings.Contains(output, "Miner detected")

			if blocked != tt.wantBlock {
				if tt.wantBlock {
					t.Errorf("expected miner redirect but was not redirected (MS_ROLE=%q MS_MINER=%q)", tt.role, tt.miner)
				} else {
					t.Errorf("unexpected miner redirect with MS_ROLE=%q MS_MINER=%q; output: %s", tt.role, tt.miner, output)
				}
			}
			gtLogBytes, _ := os.ReadFile(gtLog)
			stubRan := strings.Contains(string(gtLogBytes), "done --status DEFERRED")
			if stubRan != tt.wantBlock {
				t.Errorf("ms stub ran = %v, want %v; log: %s", stubRan, tt.wantBlock, gtLogBytes)
			}
		})
	}
}

func TestWarnHandoffGitStatus(t *testing.T) {
	origCwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origCwd) })

	t.Run("no warning on clean repo", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStderr(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output for clean repo, got: %q", output)
		}
	})

	t.Run("warns on untracked file", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStderr(t, func() {
			warnHandoffGitStatus()
		})
		if !strings.Contains(output, "uncommitted work") {
			t.Errorf("expected warning about uncommitted work, got: %q", output)
		}
		if !strings.Contains(output, "untracked") {
			t.Errorf("expected 'untracked' in output, got: %q", output)
		}
	})

	t.Run("warns on modified tracked file", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		// Create and commit a file
		fpath := filepath.Join(dir, "tracked.txt")
		os.WriteFile(fpath, []byte("original"), 0644)
		exec.Command("git", "-C", dir, "add", ".").Run()
		exec.Command("git", "-C", dir, "commit", "-m", "add file").Run()
		// Now modify it
		os.WriteFile(fpath, []byte("modified"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStderr(t, func() {
			warnHandoffGitStatus()
		})
		if !strings.Contains(output, "uncommitted work") {
			t.Errorf("expected warning about uncommitted work, got: %q", output)
		}
		if !strings.Contains(output, "modified") {
			t.Errorf("expected 'modified' in output, got: %q", output)
		}
	})

	t.Run("no warning for .beads-only changes", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		// Only .beads/ untracked files — should be clean (excluded)
		os.MkdirAll(filepath.Join(dir, ".beads"), 0755)
		os.WriteFile(filepath.Join(dir, ".beads", "somefile.db"), []byte("db"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		output := captureStderr(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output for .beads-only changes, got: %q", output)
		}
	})

	t.Run("no warning outside git repo", func(t *testing.T) {
		os.Chdir(os.TempDir())
		output := captureStderr(t, func() {
			warnHandoffGitStatus()
		})
		if output != "" {
			t.Errorf("expected no output outside git repo, got: %q", output)
		}
	})

	t.Run("no-git-check flag suppresses warning", func(t *testing.T) {
		dir := makeTestGitRepo(t)
		os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0644)
		os.Chdir(dir)
		t.Cleanup(func() { os.Chdir(origCwd) })
		// Simulate --no-git-check by setting the flag
		origFlag := handoffNoGitCheck
		handoffNoGitCheck = true
		defer func() { handoffNoGitCheck = origFlag }()
		output := captureStderr(t, func() {
			if !handoffNoGitCheck {
				warnHandoffGitStatus()
			}
		})
		if output != "" {
			t.Errorf("expected no output with --no-git-check, got: %q", output)
		}
	})
}

func TestHandoffProcessNames(t *testing.T) {
	t.Run("same-agent restart preserves MS_PROCESS_NAMES from env", func(t *testing.T) {
		setupHandoffTestRegistry(t)

		tmpTown := t.TempDir()
		overseerDir := filepath.Join(tmpTown, "overseer")
		os.MkdirAll(overseerDir, 0755)
		os.WriteFile(filepath.Join(overseerDir, "town.json"), []byte(`{"name":"test"}`), 0644)

		t.Setenv("MS_ROOT", tmpTown)
		t.Setenv("MS_AGENT", "claude")
		t.Setenv("MS_PROCESS_NAMES", "node,claude")
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		t.Cleanup(func() { os.Chdir(origCwd) })

		// Same-agent restart should preserve existing process names from env
		cmd, err := buildRestartCommand("ms-crew-propane")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(cmd, "MS_PROCESS_NAMES") || !strings.Contains(cmd, "node,claude") {
			t.Errorf("expected MS_PROCESS_NAMES=node,claude preserved from env, got: %q", cmd)
		}
	})

	t.Run("first boot without MS_PROCESS_NAMES computes from config", func(t *testing.T) {
		setupHandoffTestRegistry(t)

		tmpTown := t.TempDir()
		overseerDir := filepath.Join(tmpTown, "overseer")
		os.MkdirAll(overseerDir, 0755)
		os.WriteFile(filepath.Join(overseerDir, "town.json"), []byte(`{"name":"test"}`), 0644)

		t.Setenv("MS_ROOT", tmpTown)
		t.Setenv("MS_AGENT", "claude")
		// Explicitly clear MS_PROCESS_NAMES to simulate first boot
		t.Setenv("MS_PROCESS_NAMES", "")
		origCwd, _ := os.Getwd()
		os.Chdir(os.TempDir())
		t.Cleanup(func() { os.Chdir(origCwd) })

		// No MS_PROCESS_NAMES in env — should compute from agent config
		cmd, err := buildRestartCommand("ms-crew-propane")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Claude's default process names are "node,claude"
		if !strings.Contains(cmd, "MS_PROCESS_NAMES") || !strings.Contains(cmd, "node,claude") {
			t.Errorf("expected MS_PROCESS_NAMES=node,claude computed from config, got: %q", cmd)
		}
	})
}

// TestCollectGitState verifies that collectGitState returns deterministic
// workspace state from a git repo without shelling out to ms/bd. (GH#1996)
func TestCollectGitState(t *testing.T) {
	t.Run("returns_state_from_git_repo", func(t *testing.T) {
		// Create a temp git repo
		tmpDir := t.TempDir()
		cmds := [][]string{
			{"git", "init"},
			{"git", "config", "user.email", "test@test.com"},
			{"git", "config", "user.name", "Test"},
		}
		for _, args := range cmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = tmpDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %s", args, out)
			}
		}

		// Create a file and commit
		if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("hello"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		for _, args := range [][]string{
			{"git", "add", "file.txt"},
			{"git", "commit", "-m", "initial commit"},
		} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = tmpDir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v failed: %s", args, out)
			}
		}

		// Modify a file to create uncommitted changes
		if err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("modified"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}

		// Run collectGitState from the temp repo
		t.Chdir(tmpDir)

		state := collectGitState()

		if state == "" {
			t.Fatal("collectGitState() returned empty string for a git repo with changes")
		}
		if !strings.Contains(state, "## Workspace State") {
			t.Errorf("expected '## Workspace State' header, got: %s", state)
		}
		if !strings.Contains(state, "Modified") {
			t.Errorf("expected 'Modified' in state, got: %s", state)
		}
		if !strings.Contains(state, "initial commit") {
			t.Errorf("expected recent commit in state, got: %s", state)
		}
	})

	t.Run("returns_empty_outside_git_repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		state := collectGitState()
		if state != "" {
			t.Errorf("expected empty string outside git repo, got: %s", state)
		}
	})
}

// TestRecordHandoffTime verifies that recordHandoffTime creates the
// timestamp file in .runtime/ with a recent modification time.
func TestRecordHandoffTime(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	recordHandoffTime()

	tsPath := filepath.Join(tmpDir, constants.DirRuntime, constants.FileLastHandoffTS)
	info, err := os.Stat(tsPath)
	if err != nil {
		t.Fatalf("expected last_handoff_ts file to exist: %v", err)
	}
	if time.Since(info.ModTime()) > 5*time.Second {
		t.Errorf("expected recent modification time, got %v ago", time.Since(info.ModTime()))
	}
}

// TestEnforceHandoffCooldown verifies the cooldown logic:
// - No cooldown when no previous handoff recorded
// - Cooldown triggers when last handoff was recent
// - No cooldown when enough time has passed
func TestEnforceHandoffCooldown(t *testing.T) {
	t.Run("no cooldown without previous handoff", func(t *testing.T) {
		t.Setenv("MS_ROLE", "")
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		start := time.Now()
		enforceHandoffCooldown()
		elapsed := time.Since(start)

		// Should return almost immediately (no file to check)
		if elapsed > 1*time.Second {
			t.Errorf("expected no cooldown, but waited %v", elapsed)
		}
	})

	t.Run("no cooldown when last handoff is old", func(t *testing.T) {
		t.Setenv("MS_ROLE", "")
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		// Create a last_handoff_ts file with old mtime
		runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
		os.MkdirAll(runtimeDir, 0755)
		tsPath := filepath.Join(runtimeDir, constants.FileLastHandoffTS)
		os.WriteFile(tsPath, []byte("1000000000"), 0644)
		// Set mtime to well in the past
		oldTime := time.Now().Add(-10 * time.Minute)
		os.Chtimes(tsPath, oldTime, oldTime)

		start := time.Now()
		enforceHandoffCooldown()
		elapsed := time.Since(start)

		if elapsed > 1*time.Second {
			t.Errorf("expected no cooldown for old handoff, but waited %v", elapsed)
		}
	})

	t.Run("cooldown triggers for recent handoff", func(t *testing.T) {
		// Use a non-exempt role so cooldown applies
		t.Setenv("MS_ROLE", "mineshaft/witness")
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		// Create a last_handoff_ts file with very recent mtime
		runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
		os.MkdirAll(runtimeDir, 0755)
		tsPath := filepath.Join(runtimeDir, constants.FileLastHandoffTS)
		os.WriteFile(tsPath, []byte("now"), 0644)
		// Set mtime to (MinHandoffCooldown - 1s) ago so remaining is ~1s
		recentTime := time.Now().Add(-(constants.MinHandoffCooldown - 1*time.Second))
		os.Chtimes(tsPath, recentTime, recentTime)

		start := time.Now()
		enforceHandoffCooldown()
		elapsed := time.Since(start)

		// Should have waited approximately 1 second (the remaining cooldown)
		if elapsed < 500*time.Millisecond {
			t.Errorf("expected cooldown sleep of ~1s, but only waited %v", elapsed)
		}
		if elapsed > 3*time.Second {
			t.Errorf("expected cooldown sleep of ~1s, but waited %v", elapsed)
		}
	})

	t.Run("no cooldown for crew role", func(t *testing.T) {
		t.Setenv("MS_ROLE", "mineshaft/crew/max")
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		// Create a recent handoff file that would normally trigger cooldown
		runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
		os.MkdirAll(runtimeDir, 0755)
		tsPath := filepath.Join(runtimeDir, constants.FileLastHandoffTS)
		os.WriteFile(tsPath, []byte("now"), 0644)

		start := time.Now()
		enforceHandoffCooldown()
		elapsed := time.Since(start)

		if elapsed > 1*time.Second {
			t.Errorf("crew should be exempt from cooldown, but waited %v", elapsed)
		}
	})

	t.Run("no cooldown for overseer role", func(t *testing.T) {
		t.Setenv("MS_ROLE", "overseer")
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		// Create a recent handoff file that would normally trigger cooldown
		runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
		os.MkdirAll(runtimeDir, 0755)
		tsPath := filepath.Join(runtimeDir, constants.FileLastHandoffTS)
		os.WriteFile(tsPath, []byte("now"), 0644)

		start := time.Now()
		enforceHandoffCooldown()
		elapsed := time.Since(start)

		if elapsed > 1*time.Second {
			t.Errorf("overseer should be exempt from cooldown, but waited %v", elapsed)
		}
	})
}
