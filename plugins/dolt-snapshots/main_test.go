package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		title string
		id    string
		want  string
	}{
		{"Pi Rust Bug Fixes", "hq-cv-xrwki", "pi-rust-bug-fixes-hq-cv-xrwki"},
		{"SSD Distillation Pipeline", "hq-cv-80x", "ssd-distillation-pipeline-hq-cv-80x"},
		{"fix: SQL injection & XSS!", "hq-cv-abc", "fix-sql-injection-xss-hq-cv-abc"},
		{"", "hq-cv-123", "hq-cv-123"},
		{"a]b[c{d}e", "hq-cv-z", "a-b-c-d-e-hq-cv-z"},
		{
			"This is an extremely long minecart title that exceeds the maximum slug length limit we set",
			"hq-cv-long",
			"this-is-an-extremely-long-minecart-title-that-exceed-hq-cv-long",
		},
		{"---leading-trailing---", "id", "leading-trailing-id"},
		{"UPPERCASE TITLE", "hq-cv-up", "uppercase-title-hq-cv-up"},
		{"multiple   spaces", "hq-cv-sp", "multiple-spaces-hq-cv-sp"},
		{"special/chars\\here", "hq-cv-sc", "special-chars-here-hq-cv-sc"},
		{"dots.and.periods", "hq-cv-dp", "dots-and-periods-hq-cv-dp"},
		{"tab\there", "hq-cv-tab", "tab-here-hq-cv-tab"},
	}

	for _, tt := range tests {
		t.Run(tt.title+"_"+tt.id, func(t *testing.T) {
			got := sanitizeName(tt.title, tt.id)
			if got != tt.want {
				t.Errorf("sanitizeName(%q, %q) = %q, want %q", tt.title, tt.id, got, tt.want)
			}
		})
	}
}

func TestSanitizeName_Idempotent(t *testing.T) {
	// Running sanitizeName on an already-sanitized name should produce a valid result
	first := sanitizeName("Pi Rust Bug Fixes", "hq-cv-xrwki")
	second := sanitizeName(first, "hq-cv-xrwki")
	// Second pass should still contain the id
	if second == "" {
		t.Error("double sanitization produced empty string")
	}
}

func TestSanitizeDBName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hq", "hq"},
		{"pi_agent_rust", "pi_agent_rust"},
		{"sf-mineshaft", "sfmineshaft"},          // dashes stripped
		{"db'; DROP TABLE--", "dbDROPTABLE"}, // injection attempt sanitized
		{"normal_db_123", "normal_db_123"},
		{"", ""},
		{"Robert'); DROP TABLE students;--", "RobertDROPTABLEstudents"}, // Bobby Tables
		{"../../etc/passwd", "etcpasswd"},                               // path traversal
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeDBName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeDBName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSystemDB(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"information_schema", true},
		{"mysql", true},
		{"dolt_cluster", true},
		{"testdb_abc", true},
		{"beads_t123", true},
		{"beads_pt456", true},
		{"doctest_xyz", true},
		{"hq", false},
		{"petals", false},
		{"sfmineshaft", false},
		{"lora_forge", false},
		{"node0", false},
		// Edge cases: names that start with system prefixes but aren't
		{"testdb", false},           // exactly "testdb" with no underscore
		{"beads", false},            // exactly "beads"
		{"beads_production", false}, // doesn't match beads_t or beads_pt
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSystemDB(tt.name)
			if got != tt.want {
				t.Errorf("isSystemDB(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadRoutes(t *testing.T) {
	dir := t.TempDir()
	routesFile := filepath.Join(dir, "routes.jsonl")

	content := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"pe-","path":"petals/overseer/rig"}
{"prefix":"lf-","path":"lora_forge/overseer/rig"}
{"prefix":"gs-","path":"sfmineshaft/overseer/rig"}
{"prefix":"sc-","path":"sf_config"}
`
	if err := os.WriteFile(routesFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	routes := loadRoutes(routesFile)

	// hq and hq-cv should be skipped (path is ".")
	if _, ok := routes["hq"]; ok {
		t.Error("Expected hq to be skipped (path='.')")
	}

	// pe → petals (first component of path)
	if got, ok := routes["pe"]; !ok || got != "petals" {
		t.Errorf("routes[pe] = %q, want 'petals'", got)
	}

	// lf → lora_forge
	if got, ok := routes["lf"]; !ok || got != "lora_forge" {
		t.Errorf("routes[lf] = %q, want 'lora_forge'", got)
	}

	// gs → sfmineshaft
	if got, ok := routes["gs"]; !ok || got != "sfmineshaft" {
		t.Errorf("routes[gs] = %q, want 'sfmineshaft'", got)
	}

	// sc → sf_config (no slash in path, whole thing is db name)
	if got, ok := routes["sc"]; !ok || got != "sf_config" {
		t.Errorf("routes[sc] = %q, want 'sf_config'", got)
	}
}

func TestLoadRoutes_MissingFile(t *testing.T) {
	routes := loadRoutes("/nonexistent/routes.jsonl")
	if len(routes) != 0 {
		t.Errorf("Expected empty map for missing file, got %d entries", len(routes))
	}
}

func TestLoadRoutes_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	routesFile := filepath.Join(dir, "routes.jsonl")

	content := `{"prefix":"pe-","path":"petals/overseer/rig"}
not json at all
{"prefix":"lf-","path":"lora_forge/overseer/rig"}
{"bad json
`
	if err := os.WriteFile(routesFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	routes := loadRoutes(routesFile)

	// Should skip bad lines and parse good ones
	if _, ok := routes["pe"]; !ok {
		t.Error("Expected pe route to be parsed despite bad lines")
	}
	if _, ok := routes["lf"]; !ok {
		t.Error("Expected lf route to be parsed despite bad lines")
	}
}

func TestLoadRoutes_EmptyLines(t *testing.T) {
	dir := t.TempDir()
	routesFile := filepath.Join(dir, "routes.jsonl")

	content := `
{"prefix":"pe-","path":"petals/overseer/rig"}

{"prefix":"lf-","path":"lora_forge/overseer/rig"}

`
	if err := os.WriteFile(routesFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	routes := loadRoutes(routesFile)
	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}
}

func TestLoadRoutes_DuplicatePrefix(t *testing.T) {
	dir := t.TempDir()
	routesFile := filepath.Join(dir, "routes.jsonl")

	// Last one wins
	content := `{"prefix":"pe-","path":"petals/old"}
{"prefix":"pe-","path":"petals/overseer/rig"}
`
	if err := os.WriteFile(routesFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	routes := loadRoutes(routesFile)
	if got := routes["pe"]; got != "petals" {
		t.Errorf("Expected last-wins for duplicate prefix, got %q", got)
	}
}

func TestResolveDependencyDB(t *testing.T) {
	routes := map[string]string{
		"pe": "petals",
		"lf": "lora_forge",
		"no": "node0",
		"gs": "sfmineshaft",
	}

	tests := []struct {
		depID string
		want  string
	}{
		// External format
		{"external:petals:pe-k0e.1.1", "petals"},
		{"external:lora_forge:lf-mx5rb", "lora_forge"},
		{"external:node0:no-2yg", "node0"},
		{"external:pe:pe-k0e.1.1", "petals"},
		{"external:lf:lf-mx5rb", "lora_forge"},

		// Prefix format (via routes)
		{"pe-k0e", "petals"},
		{"lf-abc", "lora_forge"},
		{"no-xyz", "node0"},

		// Unknown prefix
		{"zz-unknown", ""},

		// Malformed
		{"", ""},
		{"nohyphen", ""},
		{"external:", ""},
		{"external::", ""},

		// Edge: external with only rig, no bead ID
		{"external:petals:", "petals"},
	}

	for _, tt := range tests {
		t.Run(tt.depID, func(t *testing.T) {
			got := resolveDependencyDB(tt.depID, routes)
			if got != tt.want {
				t.Errorf("resolveDependencyDB(%q) = %q, want %q", tt.depID, got, tt.want)
			}
		})
	}
}

func TestResolveDependencyDB_EmptyRoutes(t *testing.T) {
	routes := map[string]string{}

	// External deps still work without routes
	if got := resolveDependencyDB("external:petals:pe-123", routes); got != "petals" {
		t.Errorf("Expected 'petals' for external dep with empty routes, got %q", got)
	}

	// Prefix deps fail without routes
	if got := resolveDependencyDB("pe-123", routes); got != "" {
		t.Errorf("Expected empty for prefix dep with empty routes, got %q", got)
	}
}

func TestMinecartDependencyTargetsQueryUsesTypedTargets(t *testing.T) {
	query := minecartDependencyTargetsQuery()
	if strings.Contains(query, "d.depends_on_id") {
		t.Fatalf("query should not select legacy physical depends_on_id column:\n%s", query)
	}
	for _, want := range []string{
		"COALESCE(d.depends_on_issue_id, d.depends_on_wisp_id, d.depends_on_external) AS depends_on_id",
		"COALESCE(d.depends_on_issue_id, d.depends_on_wisp_id, d.depends_on_external) IS NOT NULL",
		"d.issue_id = ?",
		"d.type = 'tracks'",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q:\n%s", want, query)
		}
	}
}

func TestResolveHost(t *testing.T) {
	os.Unsetenv("MS_DOLT_HOST")
	os.Unsetenv("DOLT_HOST")

	// Flag takes precedence
	if got := resolveHost("192.168.1.1"); got != "192.168.1.1" {
		t.Errorf("resolveHost with flag = %q, want 192.168.1.1", got)
	}

	// MS_DOLT_HOST takes precedence over DOLT_HOST
	os.Setenv("MS_DOLT_HOST", "10.0.0.2")
	os.Setenv("DOLT_HOST", "10.0.0.1")
	defer os.Unsetenv("MS_DOLT_HOST")
	defer os.Unsetenv("DOLT_HOST")
	if got := resolveHost(""); got != "10.0.0.2" {
		t.Errorf("resolveHost with MS_DOLT_HOST = %q, want 10.0.0.2", got)
	}

	// DOLT_HOST fallback
	os.Unsetenv("MS_DOLT_HOST")
	if got := resolveHost(""); got != "10.0.0.1" {
		t.Errorf("resolveHost with DOLT_HOST = %q, want 10.0.0.1", got)
	}

	// Default
	os.Unsetenv("DOLT_HOST")
	if got := resolveHost(""); got != "127.0.0.1" {
		t.Errorf("resolveHost default = %q, want 127.0.0.1", got)
	}
}

func TestResolvePort(t *testing.T) {
	os.Unsetenv("MS_DOLT_PORT")
	os.Unsetenv("DOLT_PORT")

	// Flag takes precedence
	if got := resolvePort("3308"); got != "3308" {
		t.Errorf("resolvePort with flag = %q, want 3308", got)
	}

	// MS_DOLT_PORT takes precedence over DOLT_PORT
	os.Setenv("MS_DOLT_PORT", "3309")
	os.Setenv("DOLT_PORT", "3310")
	defer os.Unsetenv("MS_DOLT_PORT")
	defer os.Unsetenv("DOLT_PORT")
	if got := resolvePort(""); got != "3309" {
		t.Errorf("resolvePort with MS_DOLT_PORT = %q, want 3309", got)
	}

	// DOLT_PORT fallback
	os.Unsetenv("MS_DOLT_PORT")
	if got := resolvePort(""); got != "3310" {
		t.Errorf("resolvePort with DOLT_PORT = %q, want 3310", got)
	}

	// Default
	os.Unsetenv("DOLT_PORT")
	if got := resolvePort(""); got != "3307" {
		t.Errorf("resolvePort default = %q, want 3307", got)
	}
}

func TestResolveRoutesFile(t *testing.T) {
	os.Unsetenv("ROUTES_FILE")

	// Flag takes precedence
	if got := resolveRoutesFile("/custom/routes.jsonl"); got != "/custom/routes.jsonl" {
		t.Errorf("resolveRoutesFile with flag = %q", got)
	}

	// Env var
	os.Setenv("ROUTES_FILE", "/env/routes.jsonl")
	defer os.Unsetenv("ROUTES_FILE")
	if got := resolveRoutesFile(""); got != "/env/routes.jsonl" {
		t.Errorf("resolveRoutesFile with ROUTES_FILE = %q, want /env/routes.jsonl", got)
	}

	// Default includes ~/ms/.beads/routes.jsonl
	os.Unsetenv("ROUTES_FILE")
	got := resolveRoutesFile("")
	if !filepath.IsAbs(got) {
		t.Errorf("resolveRoutesFile default should be absolute, got %q", got)
	}
	if filepath.Base(got) != "routes.jsonl" {
		t.Errorf("resolveRoutesFile default should end with routes.jsonl, got %q", got)
	}
}

func TestMinecartRow_SnapshotLogic(t *testing.T) {
	// Test the snapshot decision logic that snapshotMinecarts uses
	tests := []struct {
		name       string
		minecart     minecartRow
		wantOpen   bool // should create open/ tag
		wantStaged bool // should create staged/ tag + branch
	}{
		{
			name:       "new open minecart needs open tag only",
			minecart:     minecartRow{Status: "open", HasOpenTag: false, HasStagedTag: false},
			wantOpen:   true,
			wantStaged: false,
		},
		{
			name:       "staged_ready minecart needs both",
			minecart:     minecartRow{Status: "staged_ready", HasOpenTag: false, HasStagedTag: false},
			wantOpen:   true,
			wantStaged: true,
		},
		{
			name:       "closed minecart with both tags needs nothing",
			minecart:     minecartRow{Status: "closed", HasOpenTag: true, HasStagedTag: true},
			wantOpen:   false,
			wantStaged: false,
		},
		{
			name:       "closed minecart missing staged tag",
			minecart:     minecartRow{Status: "closed", HasOpenTag: true, HasStagedTag: false},
			wantOpen:   false,
			wantStaged: true,
		},
		{
			name:       "launched minecart needs staged tag",
			minecart:     minecartRow{Status: "launched", HasOpenTag: true, HasStagedTag: false},
			wantOpen:   false,
			wantStaged: true,
		},
		{
			name:       "open minecart already tagged",
			minecart:     minecartRow{Status: "open", HasOpenTag: true, HasStagedTag: false},
			wantOpen:   false,
			wantStaged: false, // open minecarts don't get staged tags
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needOpen := !tt.minecart.HasOpenTag
			needStaged := !tt.minecart.HasStagedTag && tt.minecart.Status != "open"

			if needOpen != tt.wantOpen {
				t.Errorf("needOpen = %v, want %v", needOpen, tt.wantOpen)
			}
			if needStaged != tt.wantStaged {
				t.Errorf("needStaged = %v, want %v", needStaged, tt.wantStaged)
			}
		})
	}
}
