package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMinecartTracksBeadExactMatch verifies that minecartTracksBead finds a bead
// when the dep query returns the raw beadID.
func TestMinecartTracksBeadExactMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd sql to return a tracked dep with raw beadID
	bdScript := `#!/bin/sh
echo '[{"depends_on_id":"ms-abc123"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !minecartTracksBead(beadsDir, "hq-cv-test1", "ms-abc123") {
		t.Error("minecartTracksBead should return true for exact match")
	}
}

// TestMinecartTracksBeadExternalRef verifies that minecartTracksBead finds a bead
// when the dep query returns an external-formatted reference.
func TestMinecartTracksBeadExternalRef(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd sql to return a tracked dep with external:prefix:beadID format
	bdScript := `#!/bin/sh
echo '[{"depends_on_id":"external:ms-abc:ms-abc123"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !minecartTracksBead(beadsDir, "hq-cv-test2", "ms-abc123") {
		t.Error("minecartTracksBead should return true for external ref match")
	}
}

// TestMinecartTracksBeadNoMatch verifies that minecartTracksBead returns false
// when the minecart tracks a different bead.
func TestMinecartTracksBeadNoMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd sql to return a tracked dep with a different beadID
	bdScript := `#!/bin/sh
echo '[{"depends_on_id":"ms-other456"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if minecartTracksBead(beadsDir, "hq-cv-test3", "ms-abc123") {
		t.Error("minecartTracksBead should return false when bead not tracked")
	}
}

// TestMinecartTracksBeadEmptyDeps verifies that minecartTracksBead returns false
// when the minecart has no tracked deps.
func TestMinecartTracksBeadEmptyDeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd sql to return empty array
	bdScript := `#!/bin/sh
echo '[]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if minecartTracksBead(beadsDir, "hq-cv-test4", "ms-abc123") {
		t.Error("minecartTracksBead should return false for empty deps")
	}
}

// TestMinecartTracksBeadMultipleDeps verifies that minecartTracksBead finds the
// target bead among multiple tracked deps.
func TestMinecartTracksBeadMultipleDeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	beadsDir := t.TempDir()

	// Stub bd sql to return multiple tracked deps, one of which matches
	bdScript := `#!/bin/sh
echo '[{"depends_on_id":"ms-other1"},{"depends_on_id":"external:ms-abc:ms-abc123"},{"depends_on_id":"ms-other2"}]'
`
	bdPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+":"+origPath)

	if !minecartTracksBead(beadsDir, "hq-cv-test5", "ms-abc123") {
		t.Error("minecartTracksBead should return true when bead found among multiple deps")
	}
}

// TestBdDepListRawIDsValidation verifies that bdDepListRawIDs rejects
// invalid bead IDs to prevent SQL injection.
func TestBdDepListRawIDsValidation(t *testing.T) {
	_, err := bdDepListRawIDs("/tmp", "'; DROP TABLE deps; --", "down", "tracks")
	if err == nil {
		t.Error("bdDepListRawIDs should reject SQL injection attempts")
	}

	_, err = bdDepListRawIDs("/tmp", "valid-id", "down", "'; DROP TABLE deps; --")
	if err == nil {
		t.Error("bdDepListRawIDs should reject SQL injection in depType")
	}
}

func TestBdDepListRawIDsUsesAutoCommitEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "bd.log")
	writeBDStub(t, binDir, `#!/usr/bin/env sh
{
	printf 'args:'
	for arg in "$@"; do
		printf '[%s]' "$arg"
	done
	printf '\nBD_READONLY=%s\n' "${BD_READONLY-}"
	printf 'BD_DOLT_AUTO_COMMIT=%s\n' "${BD_DOLT_AUTO_COMMIT-}"
} >> "$BD_STUB_LOG"
printf '[{"depends_on_id":"external:ag:ag-95s.1"}]\n'
`, "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_STUB_LOG", logPath)
	t.Setenv("BD_READONLY", "true")
	t.Setenv("BD_DOLT_AUTO_COMMIT", "off")

	workDir := t.TempDir()
	beadsDir := filepath.Join(workDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"dolt_database":"hq"}`), 0644); err != nil {
		t.Fatal(err)
	}

	ids, err := bdDepListRawIDs(workDir, "hq-cv-test", "down", "tracks")
	if err != nil {
		t.Fatalf("bdDepListRawIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != "ag-95s.1" {
		t.Fatalf("ids = %v, want [ag-95s.1]", ids)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	for _, want := range []string{
		"args:[sql][SELECT COALESCE",
		"\nBD_READONLY=\n",
		"BD_DOLT_AUTO_COMMIT=on",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("bd stub log missing %q:\n%s", want, log)
		}
	}
}

func TestSQLExternalDepTargetClauseEscapesUnderscore(t *testing.T) {
	got := sqlExternalDepTargetClause("ms-a_b")
	want := "depends_on_external LIKE '%:ms-a!_b' ESCAPE '!'"
	if got != want {
		t.Fatalf("sqlExternalDepTargetClause() = %q, want %q", got, want)
	}
}
