package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/mineshaft/internal/config"
)

func TestGetPrefixForRig(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "bd-", "path": "beads/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		rig      string
		expected string
	}{
		{"mineshaft", "ms"},
		{"beads", "bd"},
		{"unknown", "ms"}, // default
		{"", "ms"},        // empty rig -> default
	}

	for _, tc := range tests {
		t.Run(tc.rig, func(t *testing.T) {
			result := GetPrefixForRig(tmpDir, tc.rig)
			if result != tc.expected {
				t.Errorf("GetPrefixForRig(%q, %q) = %q, want %q", tmpDir, tc.rig, result, tc.expected)
			}
		})
	}
}

func TestGetPrefixForRig_NoRoutesFile(t *testing.T) {
	tmpDir := t.TempDir()
	// No routes.jsonl file

	result := GetPrefixForRig(tmpDir, "anything")
	if result != "ms" {
		t.Errorf("Expected default 'ms' when no routes file, got %q", result)
	}
}

func TestGetPrefixForRig_RigsConfigFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Write rigs.json with a non-ms prefix
	rigsPath := filepath.Join(tmpDir, "overseer", "rigs.json")
	if err := os.MkdirAll(filepath.Dir(rigsPath), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"project_ideas": {
				BeadsConfig: &config.BeadsConfig{Prefix: "pi"},
			},
		},
	}
	if err := config.SaveRigsConfig(rigsPath, cfg); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}

	result := GetPrefixForRig(tmpDir, "project_ideas")
	if result != "pi" {
		t.Errorf("Expected prefix from rigs config, got %q", result)
	}
}

func TestExtractPrefix(t *testing.T) {
	tests := []struct {
		beadID   string
		expected string
	}{
		{"ap-qtsup.16", "ap-"},
		{"hq-cv-abc", "hq-"},
		{"ms-mol-xyz", "ms-"},
		{"bd-123", "bd-"},
		{"", ""},
		{"nohyphen", ""},
		{"-startswithhyphen", ""}, // Leading hyphen = invalid prefix
		{"-", ""},                 // Just hyphen = invalid
		{"a-", "a-"},              // Trailing hyphen is valid
	}

	for _, tc := range tests {
		t.Run(tc.beadID, func(t *testing.T) {
			result := ExtractPrefix(tc.beadID)
			if result != tc.expected {
				t.Errorf("ExtractPrefix(%q) = %q, want %q", tc.beadID, result, tc.expected)
			}
		})
	}
}

func TestGetRigPathForPrefix(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ap-", "path": "ai_platform/overseer/rig"}
{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		prefix   string
		expected string
	}{
		{"ap-", filepath.Join(tmpDir, "ai_platform/overseer/rig")},
		{"ms-", filepath.Join(tmpDir, "mineshaft/overseer/rig")},
		{"hq-", tmpDir},  // Town-level beads return townRoot
		{"unknown-", ""}, // Unknown prefix returns empty
		{"", ""},         // Empty prefix returns empty
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			result := GetRigPathForPrefix(tmpDir, tc.prefix)
			if result != tc.expected {
				t.Errorf("GetRigPathForPrefix(%q, %q) = %q, want %q", tmpDir, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestGetRigPathForPrefix_NoRoutesFile(t *testing.T) {
	tmpDir := t.TempDir()
	// No routes.jsonl file

	result := GetRigPathForPrefix(tmpDir, "ap-")
	if result != "" {
		t.Errorf("Expected empty string when no routes file, got %q", result)
	}
}

func TestResolveHookDir(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ap-", "path": "ai_platform/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		beadID      string
		hookWorkDir string
		expected    string
	}{
		{
			name:        "prefix resolution takes precedence over hookWorkDir",
			beadID:      "ap-test",
			hookWorkDir: "/custom/path",
			expected:    filepath.Join(tmpDir, "ai_platform/overseer/rig"),
		},
		{
			name:        "resolves rig path from prefix",
			beadID:      "ap-test",
			hookWorkDir: "",
			expected:    filepath.Join(tmpDir, "ai_platform/overseer/rig"),
		},
		{
			name:        "town-level bead returns townRoot",
			beadID:      "hq-test",
			hookWorkDir: "",
			expected:    tmpDir,
		},
		{
			name:        "unknown prefix uses hookWorkDir as fallback",
			beadID:      "xx-unknown",
			hookWorkDir: "/fallback/path",
			expected:    "/fallback/path",
		},
		{
			name:        "unknown prefix without hookWorkDir falls back to townRoot",
			beadID:      "xx-unknown",
			hookWorkDir: "",
			expected:    tmpDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveHookDir(tmpDir, tc.beadID, tc.hookWorkDir)
			if result != tc.expected {
				t.Errorf("ResolveHookDir(%q, %q, %q) = %q, want %q",
					tmpDir, tc.beadID, tc.hookWorkDir, result, tc.expected)
			}
		})
	}
}

func TestResolveBeadsDirForID(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig beads directory for ms- prefix
	rigBeadsDir := filepath.Join(tmpDir, "mineshaft/overseer/rig/.beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		{
			name:     "town-level bead returns currentBeadsDir",
			beadID:   "hq-test123",
			expected: beadsDir,
		},
		{
			name:     "rig-prefixed bead resolves to rig beadsDir",
			beadID:   "ms-abc",
			expected: rigBeadsDir,
		},
		{
			name:     "unknown prefix returns currentBeadsDir",
			beadID:   "xx-unknown",
			expected: beadsDir,
		},
		{
			name:     "empty bead ID returns currentBeadsDir",
			beadID:   "",
			expected: beadsDir,
		},
		{
			name:     "no hyphen returns currentBeadsDir",
			beadID:   "nohyphen",
			expected: beadsDir,
		},
		{
			name:     "wisp bead (hq-wisp-xxx) resolves to town beads",
			beadID:   "hq-wisp-abc123",
			expected: beadsDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveBeadsDirForID(beadsDir, tc.beadID)
			if result != tc.expected {
				t.Errorf("ResolveBeadsDirForID(%q, %q) = %q, want %q",
					beadsDir, tc.beadID, result, tc.expected)
			}
		})
	}
}

func TestResolveBeadsDirForID_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// No routes.jsonl — should always return currentBeadsDir
	result := ResolveBeadsDirForID(beadsDir, "ms-abc")
	if result != beadsDir {
		t.Errorf("expected %q, got %q", beadsDir, result)
	}
}

func TestResolveBeadsDirForID_UsesTownRoutesFromWorktreeBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	townBeadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	worktreeBeadsDir := filepath.Join(tmpDir, "mineshaft", "miners", "chrome", "mineshaft", ".beads")
	if err := os.MkdirAll(worktreeBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigBeadsDir := filepath.Join(tmpDir, "mineshaft", "overseer", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "overseer"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "overseer", "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := ResolveBeadsDirForID(worktreeBeadsDir, "ms-abc")
	if result != rigBeadsDir {
		t.Fatalf("ResolveBeadsDirForID(%q, %q) = %q, want %q", worktreeBeadsDir, "ms-abc", result, rigBeadsDir)
	}
	result = ResolveBeadsDirForID(worktreeBeadsDir, "hq-wisp-abc")
	if result != townBeadsDir {
		t.Fatalf("ResolveBeadsDirForID(%q, %q) = %q, want %q", worktreeBeadsDir, "hq-wisp-abc", result, townBeadsDir)
	}
}

func TestGetRigNameForPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "bd-", "path": "beads/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		prefix   string
		expected string
	}{
		{"ms-", "mineshaft"},
		{"bd-", "beads"},
		{"hq-", ""},      // Town-level, no specific rig
		{"unknown-", ""}, // Not in routes
		{"", ""},         // Empty prefix
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			result := GetRigNameForPrefix(tmpDir, tc.prefix)
			if result != tc.expected {
				t.Errorf("GetRigNameForPrefix(%q, %q) = %q, want %q", tmpDir, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestGetRigDirForName(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ga-", "path": "gantry"}
{"prefix": "al-", "path": "algoanki/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		rigName  string
		expected string
	}{
		{"gantry", filepath.Join(tmpDir, "gantry")},
		{"algoanki", filepath.Join(tmpDir, "algoanki/overseer/rig")},
		{"unknown", ""}, // Not in routes
		{"", ""},        // Empty rig name
	}

	for _, tc := range tests {
		t.Run(tc.rigName, func(t *testing.T) {
			result := GetRigDirForName(tmpDir, tc.rigName)
			if result != tc.expected {
				t.Errorf("GetRigDirForName(%q, %q) = %q, want %q", tmpDir, tc.rigName, result, tc.expected)
			}
		})
	}
}

func TestGetRigDirForName_TownLevelNotReturned(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}
	// Town-level rig (path=".") should not be returned — it has no rig dir.
	result := GetRigDirForName(tmpDir, "hq")
	if result != "" {
		t.Errorf("GetRigDirForName for town-level path = %q, want empty string", result)
	}
}

func TestResolveRepoAliasBeadsDir(t *testing.T) {
	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	rigRoot := filepath.Join(townRoot, "mineshaft")
	canonicalRig := filepath.Join(rigRoot, "overseer", "rig")
	canonicalBeads := filepath.Join(canonicalRig, ".beads")
	decoyBeads := filepath.Join(rigRoot, ".beads")
	escapeRig := filepath.Join(townRoot, "escape", "overseer", "rig")
	escapeBeads := filepath.Join(escapeRig, ".beads")
	escapeTarget := filepath.Join(townRoot, "..", "outside", ".beads")

	for _, dir := range []string{townBeads, canonicalBeads, decoyBeads, escapeBeads, escapeTarget} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(
		`{"prefix":"hq-","path":"."}`+"\n"+
			`{"prefix":"ms-","path":"mineshaft/overseer/rig"}`+"\n"+
			`{"prefix":"es-","path":"escape/overseer/rig"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalBeads, "metadata.json"), []byte(`{"dolt_database":"mineshaft"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoyBeads, "metadata.json"), []byte(`{"dolt_database":"hq"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(escapeBeads, "redirect"), []byte(filepath.Join("..", "..", "..", "..", "outside", ".beads")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		repo     string
		expected string
		ok       bool
	}{
		{"rig alias uses canonical route", "mineshaft", canonicalBeads, true},
		{"hq alias uses town beads", "hq", townBeads, true},
		{"town alias uses town beads", "town", townBeads, true},
		{"path-like remains unresolved", "mineshaft/overseer/rig", "", false},
		{"unknown bare repo remains unresolved", "unknown", "", false},
		{"redirect outside town rejected", "escape", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ResolveRepoAliasBeadsDir(townRoot, tc.repo)
			if ok != tc.ok {
				t.Fatalf("ResolveRepoAliasBeadsDir ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.expected {
				t.Fatalf("ResolveRepoAliasBeadsDir dir = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestRewriteBDCreateRepoAlias(t *testing.T) {
	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	canonicalBeads := filepath.Join(townRoot, "mineshaft", "overseer", "rig", ".beads")
	decoyBeads := filepath.Join(townRoot, "mineshaft", ".beads")
	for _, dir := range []string{townBeads, canonicalBeads, decoyBeads} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte(
		`{"prefix":"hq-","path":"."}`+"\n"+
			`{"prefix":"ms-","path":"mineshaft/overseer/rig"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		argv    []string
		want    []string
		wantDir string
	}{
		{
			name:    "global flags before create",
			argv:    []string{"bd", "--allow-stale", "create", "--repo", "mineshaft", "--title", "x"},
			want:    []string{"bd", "--allow-stale", "create", "--title", "x"},
			wantDir: canonicalBeads,
		},
		{
			name:    "equals form",
			argv:    []string{"bd", "--json", "create", "--repo=mineshaft"},
			want:    []string{"bd", "--json", "create"},
			wantDir: canonicalBeads,
		},
		{
			name:    "unknown bare repo unchanged",
			argv:    []string{"bd", "create", "--repo", "unknown", "--title", "x"},
			want:    []string{"bd", "create", "--repo", "unknown", "--title", "x"},
			wantDir: "",
		},
		{
			name:    "path-like repo unchanged",
			argv:    []string{"bd", "create", "--repo", "mineshaft/overseer/rig", "--title", "x"},
			want:    []string{"bd", "create", "--repo", "mineshaft/overseer/rig", "--title", "x"},
			wantDir: "",
		},
		{
			name:    "duplicate repo unchanged",
			argv:    []string{"bd", "create", "--repo", "mineshaft", "--repo", "/tmp/other"},
			want:    []string{"bd", "create", "--repo", "mineshaft", "--repo", "/tmp/other"},
			wantDir: "",
		},
		{
			name:    "repo after sentinel unchanged",
			argv:    []string{"bd", "create", "--", "--repo", "mineshaft"},
			want:    []string{"bd", "create", "--", "--repo", "mineshaft"},
			wantDir: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotDir := RewriteBDCreateRepoAlias(townRoot, tc.argv)
			if gotDir != tc.wantDir {
				t.Fatalf("beads dir = %q, want %q", gotDir, tc.wantDir)
			}
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("argv = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCheckPrefixAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "bd-", "path": "beads/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		prefix  string
		newPath string
		wantErr bool
	}{
		{
			name:    "new prefix is available",
			prefix:  "cr-",
			newPath: "crucible",
			wantErr: false,
		},
		{
			name:    "same rig re-registering same prefix",
			prefix:  "ms-",
			newPath: "mineshaft",
			wantErr: false,
		},
		{
			name:    "same rig different path variant",
			prefix:  "ms-",
			newPath: "mineshaft/overseer/rig",
			wantErr: false,
		},
		{
			name:    "collision with different rig",
			prefix:  "ms-",
			newPath: "getresearch",
			wantErr: true,
		},
		{
			name:    "collision with beads prefix",
			prefix:  "bd-",
			newPath: "boardgame",
			wantErr: true,
		},
		{
			name:    "town-level prefix not blocked",
			prefix:  "hq-",
			newPath: "headquarters",
			wantErr: true, // "." rig name conflicts with "headquarters"
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPrefixAvailable(tmpDir, tc.prefix, tc.newPath)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckPrefixAvailable(%q, %q) error = %v, wantErr %v",
					tc.prefix, tc.newPath, err, tc.wantErr)
			}
		})
	}
}

func TestCheckPrefixAvailable_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	// No .beads directory — all prefixes should be available
	err := CheckPrefixAvailable(tmpDir, "ms-", "mineshaft")
	if err != nil {
		t.Errorf("expected no error with no routes file, got: %v", err)
	}
}

func TestAgentBeadIDsWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		expected string
	}{
		{"MinerBeadIDWithPrefix bd beads obsidian",
			func() string { return MinerBeadIDWithPrefix("bd", "beads", "obsidian") },
			"bd-beads-miner-obsidian"},
		{"MinerBeadIDWithPrefix ms mineshaft Toast",
			func() string { return MinerBeadIDWithPrefix("ms", "mineshaft", "Toast") },
			"ms-mineshaft-miner-Toast"},
		{"WitnessBeadIDWithPrefix bd beads",
			func() string { return WitnessBeadIDWithPrefix("bd", "beads") },
			"bd-beads-witness"},
		{"RefineryBeadIDWithPrefix bd beads",
			func() string { return RefineryBeadIDWithPrefix("bd", "beads") },
			"bd-beads-refinery"},
		{"CrewBeadIDWithPrefix bd beads max",
			func() string { return CrewBeadIDWithPrefix("bd", "beads", "max") },
			"bd-beads-crew-max"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.fn()
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestValidateRigPrefix verifies the post-creation prefix guard (ms-gpy).
func TestValidateRigPrefix(t *testing.T) {
	// Set up a town root with routes.jsonl.
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	routesContent := `{"prefix": "ms-", "path": "mineshaft/overseer/rig"}
{"prefix": "bd-", "path": "beads/overseer/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		rigName string
		beadID  string
		wantErr bool
	}{
		{
			name:    "same-rig bead: no error",
			rigName: "mineshaft",
			beadID:  "ms-wisp-abc",
			wantErr: false,
		},
		{
			name:    "cross-rig: hq- bead on mineshaft rig returns error",
			rigName: "mineshaft",
			beadID:  "hq-wisp-xyz",
			wantErr: true,
		},
		{
			name:    "bd- bead on beads rig: no error",
			rigName: "beads",
			beadID:  "bd-wisp-123",
			wantErr: false,
		},
		{
			name:    "empty bead ID: no error (can't determine prefix)",
			rigName: "mineshaft",
			beadID:  "",
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRigPrefix(tmpDir, tc.rigName, tc.beadID)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateRigPrefix(%q, %q) error = %v, wantErr %v", tc.rigName, tc.beadID, err, tc.wantErr)
			}
		})
	}
}
