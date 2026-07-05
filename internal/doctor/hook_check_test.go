package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewHookAttachmentValidCheck(t *testing.T) {
	check := NewHookAttachmentValidCheck()

	if check.Name() != "hook-attachment-valid" {
		t.Errorf("expected name 'hook-attachment-valid', got %q", check.Name())
	}

	if check.Description() != "Verify attached molecules exist and are not closed" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestHookAttachmentValidCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewHookAttachmentValidCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestHookAttachmentValidCheck_EmptyBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookAttachmentValidCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Empty beads dir means no pinned beads, should be OK
	// Note: This may error if bd CLI is not available, but should still handle gracefully
	if result.Status != StatusOK && result.Status != StatusError {
		t.Errorf("expected StatusOK or graceful error, got %v", result.Status)
	}
}

func TestHookAttachmentValidCheck_FormatInvalid(t *testing.T) {
	check := NewHookAttachmentValidCheck()

	tests := []struct {
		inv      invalidAttachment
		expected string
	}{
		{
			inv: invalidAttachment{
				pinnedBeadID: "hq-123",
				moleculeID:   "ms-456",
				reason:       "not_found",
			},
			expected: "hq-123: attached molecule ms-456 not found",
		},
		{
			inv: invalidAttachment{
				pinnedBeadID: "hq-123",
				moleculeID:   "ms-789",
				reason:       "closed",
			},
			expected: "hq-123: attached molecule ms-789 is closed",
		},
	}

	for _, tt := range tests {
		result := check.formatInvalid(tt.inv)
		if result != tt.expected {
			t.Errorf("formatInvalid() = %q, want %q", result, tt.expected)
		}
	}
}

func TestHookAttachmentValidCheck_FindRigBeadsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create town-level .beads (should be excluded)
	townBeads := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads
	rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookAttachmentValidCheck()
	dirs := check.findRigBeadsDirs(tmpDir)

	// Should find the rig-level beads but not town-level
	found := false
	for _, dir := range dirs {
		if dir == townBeads {
			t.Error("findRigBeadsDirs should not include town-level .beads")
		}
		if dir == rigBeads {
			found = true
		}
	}

	if !found && len(dirs) > 0 {
		t.Logf("Found dirs: %v", dirs)
	}
}

// Tests for HookSingletonCheck

func TestNewHookSingletonCheck(t *testing.T) {
	check := NewHookSingletonCheck()

	if check.Name() != "hook-singleton" {
		t.Errorf("expected name 'hook-singleton', got %q", check.Name())
	}

	if check.Description() != "Ensure each agent has at most one handoff bead" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestHookSingletonCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewHookSingletonCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestHookSingletonCheck_EmptyBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewHookSingletonCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Empty beads dir means no pinned beads, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when empty beads dir, got %v", result.Status)
	}
}

func TestHookSingletonCheck_FormatDuplicate(t *testing.T) {
	check := NewHookSingletonCheck()

	tests := []struct {
		dup      duplicateHandoff
		expected string
	}{
		{
			dup: duplicateHandoff{
				title:   "Overseer Handoff",
				beadIDs: []string{"hq-123", "hq-456"},
			},
			expected: `"Overseer Handoff" has 2 beads: hq-123, hq-456`,
		},
		{
			dup: duplicateHandoff{
				title:   "Witness Handoff",
				beadIDs: []string{"ms-1", "ms-2", "ms-3"},
			},
			expected: `"Witness Handoff" has 3 beads: ms-1, ms-2, ms-3`,
		},
	}

	for _, tt := range tests {
		result := check.formatDuplicate(tt.dup)
		if result != tt.expected {
			t.Errorf("formatDuplicate() = %q, want %q", result, tt.expected)
		}
	}
}

// Tests for OrphanedAttachmentsCheck

func TestNewOrphanedAttachmentsCheck(t *testing.T) {
	check := NewOrphanedAttachmentsCheck()

	if check.Name() != "orphaned-attachments" {
		t.Errorf("expected name 'orphaned-attachments', got %q", check.Name())
	}

	if check.Description() != "Detect handoff beads for non-existent agents" {
		t.Errorf("unexpected description: %q", check.Description())
	}

	// This check is not auto-fixable (uses BaseCheck, not FixableCheck)
	if check.CanFix() {
		t.Error("expected CanFix to return false")
	}
}

func TestOrphanedAttachmentsCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewOrphanedAttachmentsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// No beads dir means nothing to check, should be OK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no beads dir, got %v", result.Status)
	}
}

func TestOrphanedAttachmentsCheck_FormatOrphan(t *testing.T) {
	check := NewOrphanedAttachmentsCheck()

	tests := []struct {
		orph     orphanedHandoff
		expected string
	}{
		{
			orph: orphanedHandoff{
				beadID: "hq-123",
				agent:  "mineshaft/nux",
			},
			expected: `hq-123: agent "mineshaft/nux" no longer exists`,
		},
		{
			orph: orphanedHandoff{
				beadID: "ms-456",
				agent:  "mineshaft/crew/joe",
			},
			expected: `ms-456: agent "mineshaft/crew/joe" no longer exists`,
		},
	}

	for _, tt := range tests {
		result := check.formatOrphan(tt.orph)
		if result != tt.expected {
			t.Errorf("formatOrphan() = %q, want %q", result, tt.expected)
		}
	}
}

func TestOrphanedAttachmentsCheck_AgentExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some agent directories
	minerDir := filepath.Join(tmpDir, "mineshaft", "miners", "nux")
	if err := os.MkdirAll(minerDir, 0755); err != nil {
		t.Fatal(err)
	}

	crewDir := filepath.Join(tmpDir, "mineshaft", "crew", "joe")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}

	overseerDir := filepath.Join(tmpDir, "overseer")
	if err := os.MkdirAll(overseerDir, 0755); err != nil {
		t.Fatal(err)
	}

	witnessDir := filepath.Join(tmpDir, "mineshaft", "witness")
	if err := os.MkdirAll(witnessDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewOrphanedAttachmentsCheck()

	tests := []struct {
		agent    string
		expected bool
	}{
		// Existing agents
		{"mineshaft/nux", true},
		{"mineshaft/crew/joe", true},
		{"overseer", true},
		{"mineshaft-witness", true},

		// Non-existent agents
		{"mineshaft/deleted", false},
		{"mineshaft/crew/gone", false},
		{"otherrig-witness", false},
	}

	for _, tt := range tests {
		result := check.agentExists(tt.agent, tmpDir)
		if result != tt.expected {
			t.Errorf("agentExists(%q) = %v, want %v", tt.agent, result, tt.expected)
		}
	}
}
