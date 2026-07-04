//go:build integration

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cleanGTEnv returns os.Environ() with all GT_* variables removed.
// This ensures tests don't inherit stale role environment from CI or previous tests.
func cleanGTEnv() []string {
	var clean []string
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "GT_") {
			clean = append(clean, env)
		}
	}
	return clean
}

// resolveSymlinks resolves all symlinks in a path.
// On macOS, t.TempDir() returns /var/... but the OS resolves it to /private/var/...
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}
	return resolved
}

// TestRoleHomeE2E validates that gt role home returns correct paths
// for all role types after a full gt install.
func TestRoleHomeE2E(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "overseer",
			args:     []string{"role", "home", "overseer"},
			expected: filepath.Join(hqPath, "overseer"),
		},
		{
			name:     "supervisor",
			args:     []string{"role", "home", "supervisor"},
			expected: filepath.Join(hqPath, "supervisor"),
		},
		{
			name:     "witness",
			args:     []string{"role", "home", "witness", "--rig", rigName},
			expected: filepath.Join(hqPath, rigName, "witness"),
		},
		{
			name:     "refinery",
			args:     []string{"role", "home", "refinery", "--rig", rigName},
			expected: filepath.Join(hqPath, rigName, "refinery", "rig"),
		},
		{
			name:     "miner",
			args:     []string{"role", "home", "miner", "--rig", rigName, "--miner", "Toast"},
			expected: filepath.Join(hqPath, rigName, "miners", "Toast"),
		},
		{
			name:     "crew",
			args:     []string{"role", "home", "crew", "--rig", rigName, "--miner", "worker1"},
			expected: filepath.Join(hqPath, rigName, "crew", "worker1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, tt.args...)
			cmd.Dir = hqPath
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			// Use Output() to only capture stdout (warnings go to stderr)
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("gt %v failed: %v\nOutput: %s", tt.args, err, output)
			}

			got := strings.TrimSpace(string(output))
			if got != tt.expected {
				t.Errorf("gt %v = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

// TestRoleHomeMissingFlags validates that gt role home fails when required flags are missing.
func TestRoleHomeMissingFlags(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "witness without --rig",
			args: []string{"role", "home", "witness"},
		},
		{
			name: "refinery without --rig",
			args: []string{"role", "home", "refinery"},
		},
		{
			name: "miner without --rig",
			args: []string{"role", "home", "miner", "--miner", "Toast"},
		},
		{
			name: "miner without --miner",
			args: []string{"role", "home", "miner", "--rig", "testrig"},
		},
		{
			name: "crew without --rig",
			args: []string{"role", "home", "crew", "--miner", "worker1"},
		},
		{
			name: "crew without --miner",
			args: []string{"role", "home", "crew", "--rig", "testrig"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, tt.args...)
			cmd.Dir = hqPath
			// Use cleanGTEnv to ensure no stale GT_* vars affect the test
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Errorf("gt %v should have failed but succeeded with output: %s", tt.args, output)
			}
		})
	}
}


// TestRoleHomeCwdDetection validates gt role home without arguments detects role from cwd.
func TestRoleHomeCwdDetection(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create rig directory structure for cwd detection
	dirs := []string{
		filepath.Join(hqPath, rigName, "witness"),
		filepath.Join(hqPath, rigName, "refinery", "rig"),
		filepath.Join(hqPath, rigName, "miners", "Toast"),
		filepath.Join(hqPath, rigName, "crew", "worker1"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name     string
		cwd      string
		expected string
	}{
		{
			name:     "overseer from overseer dir",
			cwd:      filepath.Join(hqPath, "overseer"),
			expected: filepath.Join(hqPath, "overseer"),
		},
		{
			name:     "supervisor from supervisor dir",
			cwd:      filepath.Join(hqPath, "supervisor"),
			expected: filepath.Join(hqPath, "supervisor"),
		},
		{
			name:     "witness from witness dir",
			cwd:      filepath.Join(hqPath, rigName, "witness"),
			expected: filepath.Join(hqPath, rigName, "witness"),
		},
		{
			name:     "refinery from refinery/rig dir",
			cwd:      filepath.Join(hqPath, rigName, "refinery", "rig"),
			expected: filepath.Join(hqPath, rigName, "refinery", "rig"),
		},
		{
			name:     "miner from miners/Toast dir",
			cwd:      filepath.Join(hqPath, rigName, "miners", "Toast"),
			expected: filepath.Join(hqPath, rigName, "miners", "Toast"),
		},
		{
			name:     "crew from crew/worker1 dir",
			cwd:      filepath.Join(hqPath, rigName, "crew", "worker1"),
			expected: filepath.Join(hqPath, rigName, "crew", "worker1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "home")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role home failed: %v\nOutput: %s", err, output)
			}

			got := strings.TrimSpace(string(output))
			if got != tt.expected {
				t.Errorf("gt role home = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestRoleEnvCwdDetection validates gt role env without arguments detects role from cwd.
func TestRoleEnvCwdDetection(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create rig directory structure for cwd detection
	dirs := []string{
		filepath.Join(hqPath, rigName, "witness"),
		filepath.Join(hqPath, rigName, "refinery", "rig"),
		filepath.Join(hqPath, rigName, "miners", "Toast"),
		filepath.Join(hqPath, rigName, "crew", "worker1"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name string
		cwd  string
		want []string
	}{
		{
			name: "overseer from overseer dir",
			cwd:  filepath.Join(hqPath, "overseer"),
			want: []string{
				"export GT_ROLE=overseer",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, "overseer"),
			},
		},
		{
			name: "supervisor from supervisor dir",
			cwd:  filepath.Join(hqPath, "supervisor"),
			want: []string{
				"export GT_ROLE=supervisor",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, "supervisor"),
			},
		},
		{
			name: "witness from witness dir",
			cwd:  filepath.Join(hqPath, rigName, "witness"),
			want: []string{
				"export GT_ROLE=" + rigName + "/witness",
				"export GT_RIG=" + rigName,
				"export BD_ACTOR=" + rigName + "/witness",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, rigName, "witness"),
			},
		},
		{
			name: "refinery from refinery/rig dir",
			cwd:  filepath.Join(hqPath, rigName, "refinery", "rig"),
			want: []string{
				"export GT_ROLE=" + rigName + "/refinery",
				"export GT_RIG=" + rigName,
				"export BD_ACTOR=" + rigName + "/refinery",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, rigName, "refinery", "rig"),
			},
		},
		{
			name: "miner from miners/Toast dir",
			cwd:  filepath.Join(hqPath, rigName, "miners", "Toast"),
			want: []string{
				"export GT_ROLE=" + rigName + "/miners/Toast",
				"export GT_RIG=" + rigName,
				"export GT_MINER=Toast",
				"export BD_ACTOR=" + rigName + "/miners/Toast",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, rigName, "miners", "Toast"),
			},
		},
		{
			name: "crew from crew/worker1 dir",
			cwd:  filepath.Join(hqPath, rigName, "crew", "worker1"),
			want: []string{
				"export GT_ROLE=" + rigName + "/crew/worker1",
				"export GT_RIG=" + rigName,
				"export GT_CREW=worker1",
				"export BD_ACTOR=" + rigName + "/crew/worker1",
				"export GT_ROLE_HOME=" + filepath.Join(hqPath, rigName, "crew", "worker1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "env")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role env failed: %v\nOutput: %s", err, output)
			}

			got := string(output)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q\ngot: %s", w, got)
				}
			}
		})
	}
}

// TestRoleListE2E validates gt role list shows all roles.
func TestRoleListE2E(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	cmd = exec.Command(gtBinary, "role", "list")
	cmd.Dir = hqPath
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt role list failed: %v\nOutput: %s", err, output)
	}

	got := string(output)

	// Check header
	if !strings.Contains(got, "Available roles:") {
		t.Errorf("output missing 'Available roles:' header\ngot: %s", got)
	}

	// Check all roles are listed
	roles := []string{"overseer", "supervisor", "witness", "refinery", "miner", "crew"}
	for _, role := range roles {
		if !strings.Contains(got, role) {
			t.Errorf("output missing role %q\ngot: %s", role, got)
		}
	}
}

// TestRoleShowE2E validates gt role show displays correct role info.
func TestRoleShowE2E(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create rig directory structure for cwd detection
	dirs := []string{
		filepath.Join(hqPath, rigName, "witness"),
		filepath.Join(hqPath, rigName, "refinery", "rig"),
		filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
		filepath.Join(hqPath, rigName, "crew", "worker1", "rig"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name       string
		cwd        string
		wantRole   string
		wantSource string
		wantHome   string
		wantRig    string
		wantWorker string
	}{
		{
			name:       "overseer from overseer dir",
			cwd:        filepath.Join(hqPath, "overseer"),
			wantRole:   "overseer",
			wantSource: "cwd",
			wantHome:   filepath.Join(hqPath, "overseer"),
		},
		{
			name:       "supervisor from supervisor dir",
			cwd:        filepath.Join(hqPath, "supervisor"),
			wantRole:   "supervisor",
			wantSource: "cwd",
			wantHome:   filepath.Join(hqPath, "supervisor"),
		},
		{
			name:       "witness from witness dir",
			cwd:        filepath.Join(hqPath, rigName, "witness"),
			wantRole:   "witness",
			wantSource: "cwd",
			wantHome:   filepath.Join(hqPath, rigName, "witness"),
			wantRig:    rigName,
		},
		{
			name:       "miner from miners/Toast/rig dir",
			cwd:        filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
			wantRole:   "miner",
			wantSource: "cwd",
			wantHome:   filepath.Join(hqPath, rigName, "miners", "Toast"),
			wantRig:    rigName,
			wantWorker: "Toast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "show")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role show failed: %v\nOutput: %s", err, output)
			}

			got := string(output)

			if !strings.Contains(got, tt.wantRole) {
				t.Errorf("output missing role %q\ngot: %s", tt.wantRole, got)
			}

			if !strings.Contains(got, "Source: "+tt.wantSource) {
				t.Errorf("output missing 'Source: %s'\ngot: %s", tt.wantSource, got)
			}

			if !strings.Contains(got, "Home: "+tt.wantHome) {
				t.Errorf("output missing 'Home: %s'\ngot: %s", tt.wantHome, got)
			}

			if tt.wantRig != "" {
				if !strings.Contains(got, "Rig: "+tt.wantRig) {
					t.Errorf("output missing 'Rig: %s'\ngot: %s", tt.wantRig, got)
				}
			}

			if tt.wantWorker != "" {
				if !strings.Contains(got, "Worker: "+tt.wantWorker) {
					t.Errorf("output missing 'Worker: %s'\ngot: %s", tt.wantWorker, got)
				}
			}
		})
	}
}

// TestRoleShowMismatch validates gt role show displays mismatch warning.
func TestRoleShowMismatch(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Run from overseer dir but set GT_ROLE to supervisor
	cmd = exec.Command(gtBinary, "role", "show")
	cmd.Dir = filepath.Join(hqPath, "overseer")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir, "GT_ROLE=supervisor")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt role show failed: %v\nOutput: %s", err, output)
	}

	got := string(output)

	// GT_ROLE takes precedence, so role should be supervisor
	if !strings.Contains(got, "supervisor") {
		t.Errorf("should show 'supervisor' from GT_ROLE, got: %s", got)
	}

	// Source should be env
	if !strings.Contains(got, "Source: env") {
		t.Errorf("source should be 'env', got: %s", got)
	}

	// Should show mismatch warning
	if !strings.Contains(got, "ROLE MISMATCH") {
		t.Errorf("should show ROLE MISMATCH warning\ngot: %s", got)
	}

	// Should show both the env value and cwd suggestion
	if !strings.Contains(got, "GT_ROLE=supervisor") {
		t.Errorf("should show GT_ROLE value\ngot: %s", got)
	}

	if !strings.Contains(got, "cwd suggests: overseer") {
		t.Errorf("should show cwd suggestion\ngot: %s", got)
	}
}

// TestRoleShowNoMismatchAtTownRoot validates that town root + GT_ROLE does not
// trigger a mismatch warning, since town root returns RoleUnknown (not RoleOverseer).
// Regression test for https://github.com/steveyegge/excavation/issues/1496
func TestRoleShowNoMismatchAtTownRoot(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Run from town root with GT_ROLE=overseer — should NOT show mismatch
	cmd = exec.Command(gtBinary, "role", "show")
	cmd.Dir = hqPath
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir, "GT_ROLE=overseer")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt role show failed: %v\nOutput: %s", err, output)
	}

	got := string(output)

	// GT_ROLE takes precedence, so role should be overseer
	if !strings.Contains(got, "overseer") {
		t.Errorf("should show 'overseer' from GT_ROLE, got: %s", got)
	}

	// Source should be env
	if !strings.Contains(got, "Source: env") {
		t.Errorf("source should be 'env', got: %s", got)
	}

	// Should NOT show mismatch warning — town root is neutral (RoleUnknown)
	if strings.Contains(got, "ROLE MISMATCH") {
		t.Errorf("should NOT show ROLE MISMATCH at town root\ngot: %s", got)
	}
}

// TestRoleDetectE2E validates gt role detect uses cwd and ignores GT_ROLE.
func TestRoleDetectE2E(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create rig directory structure for cwd detection
	dirs := []string{
		filepath.Join(hqPath, rigName, "witness"),
		filepath.Join(hqPath, rigName, "refinery", "rig"),
		filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
		filepath.Join(hqPath, rigName, "crew", "worker1", "rig"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name        string
		cwd         string
		wantRole    string
		wantRig     string
		wantWorker  string
	}{
		{
			name:     "town root returns unknown",
			cwd:      hqPath,
			wantRole: "unknown",
		},
		{
			name:     "overseer from overseer dir",
			cwd:      filepath.Join(hqPath, "overseer"),
			wantRole: "overseer",
		},
		{
			name:     "supervisor from supervisor dir",
			cwd:      filepath.Join(hqPath, "supervisor"),
			wantRole: "supervisor",
		},
		{
			name:     "witness from witness dir",
			cwd:      filepath.Join(hqPath, rigName, "witness"),
			wantRole: "witness",
			wantRig:  rigName,
		},
		{
			name:     "refinery from refinery/rig dir",
			cwd:      filepath.Join(hqPath, rigName, "refinery", "rig"),
			wantRole: "refinery",
			wantRig:  rigName,
		},
		{
			name:       "miner from miners/Toast/rig dir",
			cwd:        filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
			wantRole:   "miner",
			wantRig:    rigName,
			wantWorker: "Toast",
		},
		{
			name:       "crew from crew/worker1/rig dir",
			cwd:        filepath.Join(hqPath, rigName, "crew", "worker1", "rig"),
			wantRole:   "crew",
			wantRig:    rigName,
			wantWorker: "worker1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "detect")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role detect failed: %v\nOutput: %s", err, output)
			}

			got := string(output)

			// Check role is detected
			if !strings.Contains(got, tt.wantRole) {
				t.Errorf("output missing role %q\ngot: %s", tt.wantRole, got)
			}

			// Check "(from cwd)" marker
			if !strings.Contains(got, "(from cwd)") {
				t.Errorf("output missing '(from cwd)' marker\ngot: %s", got)
			}

			// Check rig if expected
			if tt.wantRig != "" {
				if !strings.Contains(got, "Rig: "+tt.wantRig) {
					t.Errorf("output missing 'Rig: %s'\ngot: %s", tt.wantRig, got)
				}
			}

			// Check worker if expected
			if tt.wantWorker != "" {
				if !strings.Contains(got, "Worker: "+tt.wantWorker) {
					t.Errorf("output missing 'Worker: %s'\ngot: %s", tt.wantWorker, got)
				}
			}
		})
	}
}

// TestRoleDetectIgnoresGTRole validates gt role detect ignores GT_ROLE env var.
func TestRoleDetectIgnoresGTRole(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	// Run from overseer dir but set GT_ROLE to supervisor
	cmd = exec.Command(gtBinary, "role", "detect")
	cmd.Dir = filepath.Join(hqPath, "overseer")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir, "GT_ROLE=supervisor")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gt role detect failed: %v\nOutput: %s", err, output)
	}

	got := string(output)

	// Should detect overseer from cwd, not supervisor from env
	if !strings.Contains(got, "overseer") {
		t.Errorf("should detect 'overseer' from cwd, got: %s", got)
	}

	// Should show mismatch warning
	if !strings.Contains(got, "Mismatch") {
		t.Errorf("should show mismatch warning when GT_ROLE disagrees\ngot: %s", got)
	}

	if !strings.Contains(got, "GT_ROLE=supervisor") {
		t.Errorf("should show conflicting GT_ROLE value\ngot: %s", got)
	}
}

// TestRoleDetectInvalidPaths validates detection behavior for incomplete/invalid paths.
func TestRoleDetectInvalidPaths(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create incomplete directory structures
	dirs := []string{
		filepath.Join(hqPath, rigName),                        // rig root
		filepath.Join(hqPath, rigName, "miners"),            // miners without name
		filepath.Join(hqPath, rigName, "crew"),                // crew without name
		filepath.Join(hqPath, rigName, "refinery"),            // refinery without /rig
		filepath.Join(hqPath, rigName, "witness"),             // witness (valid - no /rig needed)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name     string
		cwd      string
		wantRole string
	}{
		{
			name:     "rig root returns unknown",
			cwd:      filepath.Join(hqPath, rigName),
			wantRole: "unknown",
		},
		{
			name:     "miners without name returns unknown",
			cwd:      filepath.Join(hqPath, rigName, "miners"),
			wantRole: "unknown",
		},
		{
			name:     "crew without name returns unknown",
			cwd:      filepath.Join(hqPath, rigName, "crew"),
			wantRole: "unknown",
		},
		{
			name:     "refinery without /rig still detects refinery",
			cwd:      filepath.Join(hqPath, rigName, "refinery"),
			wantRole: "refinery",
		},
		{
			name:     "witness without /rig detects witness",
			cwd:      filepath.Join(hqPath, rigName, "witness"),
			wantRole: "witness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "detect")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role detect failed: %v\nOutput: %s", err, output)
			}

			got := string(output)
			if !strings.Contains(got, tt.wantRole) {
				t.Errorf("expected role %q\ngot: %s", tt.wantRole, got)
			}
		})
	}
}

// TestRoleEnvIncompleteEnvVars validates gt role env fills gaps from cwd with warning.
func TestRoleEnvIncompleteEnvVars(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create rig directory structure for cwd detection
	dirs := []string{
		filepath.Join(hqPath, rigName, "witness"),
		filepath.Join(hqPath, rigName, "refinery", "rig"),
		filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
		filepath.Join(hqPath, rigName, "crew", "worker1", "rig"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name       string
		cwd        string
		envVars    []string
		wantExport []string // Expected exports in stdout
		wantStderr string   // Expected warning in stderr
	}{
		{
			name: "GT_ROLE=witness without GT_RIG, filled from cwd",
			cwd:  filepath.Join(hqPath, rigName, "witness"),
			envVars: []string{"GT_ROLE=witness"},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/witness",
				"export GT_RIG=" + rigName,
				"export BD_ACTOR=" + rigName + "/witness",
			},
			wantStderr: "env vars incomplete",
		},
		{
			name: "GT_ROLE=refinery without GT_RIG, filled from cwd",
			cwd:  filepath.Join(hqPath, rigName, "refinery", "rig"),
			envVars: []string{"GT_ROLE=refinery"},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/refinery",
				"export GT_RIG=" + rigName,
				"export BD_ACTOR=" + rigName + "/refinery",
			},
			wantStderr: "env vars incomplete",
		},
		{
			name: "GT_ROLE=miner without GT_RIG or GT_MINER, filled from cwd",
			cwd:  filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
			envVars: []string{"GT_ROLE=miner"},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/miners/Toast",
				"export GT_RIG=" + rigName,
				"export GT_MINER=Toast",
				"export BD_ACTOR=" + rigName + "/miners/Toast",
			},
			wantStderr: "env vars incomplete",
		},
		{
			name: "GT_ROLE=miner with GT_RIG but no GT_MINER, filled from cwd",
			cwd:  filepath.Join(hqPath, rigName, "miners", "Toast", "rig"),
			envVars: []string{"GT_ROLE=miner", "GT_RIG=" + rigName},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/miners/Toast",
				"export GT_RIG=" + rigName,
				"export GT_MINER=Toast",
				"export BD_ACTOR=" + rigName + "/miners/Toast",
			},
			wantStderr: "env vars incomplete",
		},
		{
			name: "GT_ROLE=crew without GT_RIG or GT_CREW, filled from cwd",
			cwd:  filepath.Join(hqPath, rigName, "crew", "worker1", "rig"),
			envVars: []string{"GT_ROLE=crew"},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/crew/worker1",
				"export GT_RIG=" + rigName,
				"export GT_CREW=worker1",
				"export BD_ACTOR=" + rigName + "/crew/worker1",
			},
			wantStderr: "env vars incomplete",
		},
		{
			name: "Complete env vars - no warning",
			cwd:  filepath.Join(hqPath, rigName, "witness"),
			envVars: []string{"GT_ROLE=witness", "GT_RIG=" + rigName},
			wantExport: []string{
				"export GT_ROLE=" + rigName + "/witness",
				"export GT_RIG=" + rigName,
				"export BD_ACTOR=" + rigName + "/witness",
			},
			wantStderr: "", // No warning expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "env")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
			cmd.Env = append(cmd.Env, tt.envVars...)

			// Use CombinedOutput to see stderr for debugging, but separate stdout/stderr
			stdout, _ := cmd.Output() // Only stdout
			// Re-run to get stderr
			cmd2 := exec.Command(gtBinary, "role", "env")
			cmd2.Dir = tt.cwd
			cmd2.Env = append(cleanGTEnv(), "HOME="+tmpDir)
			cmd2.Env = append(cmd2.Env, tt.envVars...)
			combined, _ := cmd2.CombinedOutput()
			stderr := strings.TrimPrefix(string(combined), string(stdout))

			// Check expected exports in stdout
			gotStdout := string(stdout)
			for _, w := range tt.wantExport {
				if !strings.Contains(gotStdout, w) {
					t.Errorf("stdout missing %q\ngot: %s", w, gotStdout)
				}
			}

			// Check expected warning in stderr
			if tt.wantStderr != "" {
				if !strings.Contains(stderr, tt.wantStderr) {
					t.Errorf("stderr should contain %q\ngot: %s\ncombined: %s", tt.wantStderr, stderr, combined)
				}
			} else {
				if strings.Contains(stderr, "incomplete") {
					t.Errorf("stderr should not contain 'incomplete' warning\ngot: %s", stderr)
				}
			}
		})
	}
}

// TestRoleEnvCwdMismatchFromIncompleteDir validates warnings when in incomplete directories.
func TestRoleEnvCwdMismatchFromIncompleteDir(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create incomplete directory structures (missing /rig)
	dirs := []string{
		filepath.Join(hqPath, rigName, "refinery"),            // refinery without /rig
		filepath.Join(hqPath, rigName, "miners", "Toast"),   // miner without /rig
		filepath.Join(hqPath, rigName, "crew", "worker1"),     // crew without /rig
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name       string
		cwd        string
		envVars    []string
		wantStderr string // Expected warning about cwd mismatch
	}{
		{
			name: "refinery without /rig shows cwd mismatch",
			cwd:  filepath.Join(hqPath, rigName, "refinery"),
			envVars: []string{"GT_ROLE=refinery", "GT_RIG=" + rigName},
			wantStderr: "cwd",
		},
		{
			name: "miner without /rig is valid (home is miners/name)",
			cwd:  filepath.Join(hqPath, rigName, "miners", "Toast"),
			envVars: []string{"GT_ROLE=miner", "GT_RIG=" + rigName, "GT_MINER=Toast"},
			wantStderr: "", // No mismatch: miner home is miners/<name>, not miners/<name>/rig
		},
		{
			name: "crew without /rig is valid (home is crew/name)",
			cwd:  filepath.Join(hqPath, rigName, "crew", "worker1"),
			envVars: []string{"GT_ROLE=crew", "GT_RIG=" + rigName, "GT_CREW=worker1"},
			wantStderr: "", // No mismatch: crew home is crew/<name>, not crew/<name>/rig
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "env")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
			cmd.Env = append(cmd.Env, tt.envVars...)

			combined, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gt role env failed: %v\nOutput: %s", err, combined)
			}

			// Check for cwd mismatch warning
			if tt.wantStderr != "" {
				if !strings.Contains(string(combined), tt.wantStderr) {
					t.Errorf("output should contain %q warning\ngot: %s", tt.wantStderr, combined)
				}
			} else {
				if strings.Contains(string(combined), "mismatch") || strings.Contains(string(combined), "WARNING") {
					t.Errorf("output should NOT contain mismatch warning\ngot: %s", combined)
				}
			}
		})
	}
}

// TestRoleHomeInvalidPaths validates that commands fail gracefully for incomplete paths.
func TestRoleHomeInvalidPaths(t *testing.T) {
	tmpDir := resolveSymlinks(t, t.TempDir())
	hqPath := filepath.Join(tmpDir, "test-hq")
	gtBinary := buildGT(t)

	cmd := exec.Command(gtBinary, "install", hqPath, "--no-beads")
	cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gt install failed: %v\nOutput: %s", err, output)
	}

	rigName := "testrig"

	// Create incomplete directory structures
	dirs := []string{
		filepath.Join(hqPath, rigName),
		filepath.Join(hqPath, rigName, "miners"),
		filepath.Join(hqPath, rigName, "crew"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	tests := []struct {
		name      string
		cwd       string
		shouldErr bool
	}{
		{
			name:      "rig root fails",
			cwd:       filepath.Join(hqPath, rigName),
			shouldErr: true,
		},
		{
			name:      "miners without name fails",
			cwd:       filepath.Join(hqPath, rigName, "miners"),
			shouldErr: true,
		},
		{
			name:      "crew without name fails",
			cwd:       filepath.Join(hqPath, rigName, "crew"),
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(gtBinary, "role", "home")
			cmd.Dir = tt.cwd
			cmd.Env = append(cleanGTEnv(), "HOME="+tmpDir)

			_, err := cmd.CombinedOutput()
			if tt.shouldErr && err == nil {
				t.Errorf("expected error but command succeeded")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected success but got error: %v", err)
			}
		})
	}
}

