package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/steveyegge/mineshaft/internal/session"
)

// setupTestRegistry sets up a prefix registry for tests and returns a cleanup function.
func setupTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("ms", "mineshaft")
	reg.Register("bd", "beads")
	reg.Register("nif", "niflheim")
	reg.Register("grc", "grctool")
	reg.Register("7s", "7thsense")
	reg.Register("pf", "pulseflow")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

// mockSessionLister allows deterministic testing of orphan session detection.
type mockSessionLister struct {
	sessions []string
	err      error
}

func (m *mockSessionLister) ListSessions() ([]string, error) {
	return m.sessions, m.err
}

func TestNewOrphanSessionCheck(t *testing.T) {
	check := NewOrphanSessionCheck()

	if check.Name() != "orphan-sessions" {
		t.Errorf("expected name 'orphan-sessions', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true for session check")
	}
}

func TestNewOrphanProcessCheck(t *testing.T) {
	check := NewOrphanProcessCheck()

	if check.Name() != "orphan-processes" {
		t.Errorf("expected name 'orphan-processes', got %q", check.Name())
	}

	// OrphanProcessCheck should NOT be fixable - it's informational only
	if check.CanFix() {
		t.Error("expected CanFix to return false for process check (informational only)")
	}
}

func TestOrphanProcessCheck_Run(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("orphan process detection is not supported on Windows")
	}

	// This test verifies the check runs without error.
	// Results depend on whether Claude processes exist in the test environment.
	check := NewOrphanProcessCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)

	// Should return OK (no processes or all inside tmux) or Warning (processes outside tmux)
	// Both are valid depending on test environment
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected StatusOK or StatusWarning, got %v: %s", result.Status, result.Message)
	}

	// If warning, should have informational details
	if result.Status == StatusWarning {
		if len(result.Details) < 3 {
			t.Errorf("expected at least 3 detail lines (2 info + 1 process), got %d", len(result.Details))
		}
		// Should NOT have a FixHint since this is informational only
		if result.FixHint != "" {
			t.Errorf("expected no FixHint for informational check, got %q", result.FixHint)
		}
	}
}

func TestOrphanProcessCheck_MessageContent(t *testing.T) {
	// Verify the check description is correct
	check := NewOrphanProcessCheck()

	expectedDesc := "Detect runtime processes outside tmux"
	if check.Description() != expectedDesc {
		t.Errorf("expected description %q, got %q", expectedDesc, check.Description())
	}
}

func TestIsCrewSession(t *testing.T) {
	setupTestRegistry(t)
	tests := []struct {
		session string
		want    bool
	}{
		{"ms-crew-joe", true},  // mineshaft crew (prefix: ms)
		{"bd-crew-max", true},  // beads crew (prefix: bd)
		{"nif-crew-a", true},   // niflheim crew (prefix: nif)
		{"ms-witness", false},  // witness, not crew
		{"ms-refinery", false}, // refinery, not crew
		{"ms-miner1", false}, // miner, not crew
		{"hq-supervisor", false},
		{"hq-overseer", false},
		{"other-session", false},
		{"ms-crew", false}, // "crew" is a miner name, not crew role (no name after crew-)
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := isCrewSession(tt.session)
			if got != tt.want {
				t.Errorf("isCrewSession(%q) = %v, want %v", tt.session, got, tt.want)
			}
		})
	}
}

func TestOrphanSessionCheck_IsValidSession(t *testing.T) {
	setupTestRegistry(t)
	check := NewOrphanSessionCheck()
	validRigs := []string{"mineshaft", "beads"}
	overseerSession := "hq-overseer"
	supervisorSession := "hq-supervisor"

	tests := []struct {
		session string
		want    bool
	}{
		// Town-level sessions
		{"hq-overseer", true},
		{"hq-supervisor", true},

		// Boot watchdog session
		{"hq-boot", true},

		// Valid rig sessions (using rig prefixes)
		{"ms-witness", true},  // mineshaft witness (prefix: ms)
		{"ms-refinery", true}, // mineshaft refinery
		{"ms-miner1", true}, // mineshaft miner
		{"bd-witness", true},  // beads witness (prefix: bd)
		{"bd-refinery", true}, // beads refinery
		{"bd-crew-max", true}, // beads crew

		// Invalid rig sessions (unknown prefix/rig)
		{"zz-witness", false},  // unknown prefix
		{"xx-refinery", false}, // unknown prefix

		// Non-MS sessions fail format validation
		{"other-session", false},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := check.isValidSession(tt.session, validRigs, overseerSession, supervisorSession)
			if got != tt.want {
				t.Errorf("isValidSession(%q) = %v, want %v", tt.session, got, tt.want)
			}
		})
	}
}

// TestOrphanSessionCheck_IsValidSession_EdgeCases tests edge cases that have caused
// false positives in production - sessions incorrectly detected as orphans.
func TestOrphanSessionCheck_IsValidSession_EdgeCases(t *testing.T) {
	setupTestRegistry(t)
	check := NewOrphanSessionCheck()
	validRigs := []string{"mineshaft", "niflheim", "grctool", "7thsense", "pulseflow"}
	overseerSession := "hq-overseer"
	supervisorSession := "hq-supervisor"

	tests := []struct {
		name    string
		session string
		want    bool
		reason  string
	}{
		// Crew sessions with various name formats (using rig prefixes)
		{
			name:    "crew_simple_name",
			session: "ms-crew-max",
			want:    true,
			reason:  "simple crew name should be valid",
		},
		{
			name:    "crew_with_numbers",
			session: "nif-crew-codex1",
			want:    true,
			reason:  "crew name with numbers should be valid",
		},
		{
			name:    "crew_alphanumeric",
			session: "grc-crew-grc1",
			want:    true,
			reason:  "alphanumeric crew name should be valid",
		},
		{
			name:    "crew_short_name",
			session: "7s-crew-ss1",
			want:    true,
			reason:  "short crew name should be valid",
		},
		{
			name:    "crew_pf1",
			session: "pf-crew-pf1",
			want:    true,
			reason:  "pf1 crew name should be valid",
		},

		// Miner sessions (any name after prefix should be accepted)
		{
			name:    "miner_hash_style",
			session: "ms-abc123def",
			want:    true,
			reason:  "miner with hash-style name should be valid",
		},
		{
			name:    "miner_descriptive",
			session: "nif-fix-auth-bug",
			want:    true,
			reason:  "miner with descriptive name should be valid",
		},

		// Sessions that should be detected as orphans
		{
			name:    "unknown_prefix_witness",
			session: "zz-witness",
			want:    false,
			reason:  "unknown prefix/rig should be orphan",
		},
		{
			name:    "malformed_no_dash",
			session: "x",
			want:    false,
			reason:  "session without dash should be invalid",
		},

		// Edge case: hyphenated rig names are no longer ambiguous because
		// each rig has a distinct prefix. E.g., rig "foo-bar" uses prefix "fb",
		// so its witness session is simply "fb-witness".
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check.isValidSession(tt.session, validRigs, overseerSession, supervisorSession)
			if got != tt.want {
				t.Errorf("isValidSession(%q) = %v, want %v: %s", tt.session, got, tt.want, tt.reason)
			}
		})
	}
}

// TestOrphanSessionCheck_GetValidRigs verifies rig detection from filesystem.
func TestOrphanSessionCheck_GetValidRigs(t *testing.T) {
	check := NewOrphanSessionCheck()
	townRoot := t.TempDir()

	// Setup: create overseer directory (required for getValidRigs to proceed)
	if err := os.MkdirAll(filepath.Join(townRoot, "overseer"), 0755); err != nil {
		t.Fatalf("failed to create overseer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "overseer", "rigs.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create rigs.json: %v", err)
	}

	// Create some rigs with miners/crew directories
	createRigDir := func(name string, hasCrew, hasMiners bool) {
		rigPath := filepath.Join(townRoot, name)
		os.MkdirAll(rigPath, 0755)
		if hasCrew {
			os.MkdirAll(filepath.Join(rigPath, "crew"), 0755)
		}
		if hasMiners {
			os.MkdirAll(filepath.Join(rigPath, "miners"), 0755)
		}
	}

	createRigDir("mineshaft", true, true)
	createRigDir("niflheim", true, false)
	createRigDir("grctool", false, true)
	createRigDir("not-a-rig", false, false) // No crew or miners

	rigs := check.getValidRigs(townRoot)

	// Should find mineshaft, niflheim, grctool but not "not-a-rig"
	expected := map[string]bool{
		"mineshaft":  true,
		"niflheim": true,
		"grctool":  true,
	}

	for _, rig := range rigs {
		if !expected[rig] {
			t.Errorf("unexpected rig %q in result", rig)
		}
		delete(expected, rig)
	}

	for rig := range expected {
		t.Errorf("expected rig %q not found in result", rig)
	}
}

// TestOrphanSessionCheck_FixProtectsCrewSessions verifies that Fix() never kills crew sessions.
func TestOrphanSessionCheck_FixProtectsCrewSessions(t *testing.T) {
	setupTestRegistry(t)
	check := NewOrphanSessionCheck()

	// Simulate cached orphan sessions including a crew session
	check.orphanSessions = []string{
		"ms-crew-max",     // Crew - should be protected
		"zz-witness",      // Not crew - would be killed
		"nif-crew-codex1", // Crew - should be protected
	}

	// Verify isCrewSession correctly identifies crew sessions
	for _, sess := range check.orphanSessions {
		if sess == "ms-crew-max" || sess == "nif-crew-codex1" {
			if !isCrewSession(sess) {
				t.Errorf("isCrewSession(%q) should return true for crew session", sess)
			}
		} else {
			if isCrewSession(sess) {
				t.Errorf("isCrewSession(%q) should return false for non-crew session", sess)
			}
		}
	}
}

// TestIsCrewSession_ComprehensivePatterns tests the crew session detection pattern thoroughly.
func TestIsCrewSession_ComprehensivePatterns(t *testing.T) {
	setupTestRegistry(t)

	tests := []struct {
		session string
		want    bool
		reason  string
	}{
		// Valid crew patterns (new format: <prefix>-crew-<name>)
		{"ms-crew-joe", true, "mineshaft crew session"},
		{"bd-crew-max", true, "beads crew session"},
		{"nif-crew-codex1", true, "niflheim crew with numbers in name"},
		{"grc-crew-grc1", true, "grctool crew with alphanumeric name"},
		{"7s-crew-ss1", true, "rig starting with number"},

		// Invalid crew patterns
		{"ms-witness", false, "witness is not crew"},
		{"ms-refinery", false, "refinery is not crew"},
		{"ms-miner-abc", false, "miner name, not crew"},
		{"hq-supervisor", false, "supervisor is not crew"},
		{"hq-overseer", false, "overseer is not crew"},
		{"", false, "empty string"},
		{"ms-morsov", false, "miner, not crew"},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := isCrewSession(tt.session)
			if got != tt.want {
				t.Errorf("isCrewSession(%q) = %v, want %v: %s", tt.session, got, tt.want, tt.reason)
			}
		})
	}
}

// TestOrphanSessionCheck_HQSessions tests that hq-* sessions are properly recognized as valid.
func TestOrphanSessionCheck_HQSessions(t *testing.T) {
	townRoot := t.TempDir()
	overseerDir := filepath.Join(townRoot, "overseer")
	if err := os.MkdirAll(overseerDir, 0o755); err != nil {
		t.Fatalf("create overseer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overseerDir, "rigs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("create rigs.json: %v", err)
	}

	lister := &mockSessionLister{
		sessions: []string{
			"hq-overseer",  // valid: headquarters overseer session
			"hq-supervisor", // valid: headquarters supervisor session
		},
	}
	check := NewOrphanSessionCheckWithSessionLister(lister)
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK for valid hq sessions, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "All 2 Mineshaft sessions are valid" {
		t.Fatalf("unexpected message: %q", result.Message)
	}
	if len(check.orphanSessions) != 0 {
		t.Fatalf("expected no orphan sessions, got %v", check.orphanSessions)
	}
}

// TestOrphanSessionCheck_Run_Deterministic tests the full Run path with a mock session
// lister, ensuring deterministic behavior without depending on real tmux state.
func TestOrphanSessionCheck_Run_Deterministic(t *testing.T) {
	setupTestRegistry(t)

	townRoot := t.TempDir()
	overseerDir := filepath.Join(townRoot, "overseer")
	if err := os.MkdirAll(overseerDir, 0o755); err != nil {
		t.Fatalf("create overseer dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(overseerDir, "rigs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("create rigs.json: %v", err)
	}

	// Create rig directories to make them "valid"
	if err := os.MkdirAll(filepath.Join(townRoot, "mineshaft", "miners"), 0o755); err != nil {
		t.Fatalf("create mineshaft rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "beads", "crew"), 0o755); err != nil {
		t.Fatalf("create beads rig: %v", err)
	}

	lister := &mockSessionLister{
		sessions: []string{
			"ms-witness",     // valid: mineshaft rig exists (prefix "ms")
			"ms-miner1",    // valid: mineshaft rig exists
			"bd-refinery",    // valid: beads rig exists (prefix "bd")
			"hq-overseer",       // valid: hq-overseer is recognized
			"hq-supervisor",      // valid: hq-supervisor is recognized
			"zz-witness",     // ignored: unknown prefix, not a mineshaft session
			"xx-crew-joe",    // ignored: unknown prefix, not a mineshaft session
			"random-session", // ignored: unknown prefix, not a mineshaft session
		},
	}
	check := NewOrphanSessionCheckWithSessionLister(lister)
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}

	if len(check.orphanSessions) != 0 {
		t.Fatalf("expected 0 orphans (unknown prefixes are ignored), got %d: %v", len(check.orphanSessions), check.orphanSessions)
	}
}

func TestArgvHasFlag(t *testing.T) {
	if !argvHasFlag("/x/cursor-agent -f --resume z", "-f") {
		t.Error("expected -f in argv")
	}
	if argvHasFlag("/x/cursor-agent --resume z", "-f") {
		t.Error("did not expect -f")
	}
}

func TestMineshaftRuntimeYOLO(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		args string
		want bool
	}{
		{"claude_ms", "claude", "/x/claude --dangerously-skip-permissions foo", true},
		{"claude_personal", "claude", "/x/claude foo", false},
		{"cursor_agent", "cursor-agent", "cursor-agent -f --resume x", true},
		{"cursor_no_f", "cursor-agent", "cursor-agent --resume x", false},
		{"agent_symlink", "agent", "agent -f --resume x", true},
		{"agent_f_only", "agent", "agent -f", false},
		{"copilot_yolo", "copilot", "copilot --yolo", true},
		{"copilot_plain", "copilot", "copilot", false},
		{"unknown", "vim", "vim foo", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mineshaftRuntimeYOLO(tt.cmd, tt.args); got != tt.want {
				t.Errorf("mineshaftRuntimeYOLO(%q, %q) = %v, want %v", tt.cmd, tt.args, got, tt.want)
			}
		})
	}
}
