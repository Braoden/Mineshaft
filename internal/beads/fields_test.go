package beads

import (
	"strings"
	"testing"
)

// --- parseIntField (not covered in beads_test.go) ---

func TestParseIntField(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"0", 0, false},
		{"-1", -1, false},
		{"abc", 0, true},
		{"", 0, true},
		{"3.14", 3, false}, // Sscanf reads the integer part
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseIntField(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIntField(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseIntField(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- AttachmentFields Mode round-trip ---

func TestAttachmentFieldsModeRoundTrip(t *testing.T) {
	original := &AttachmentFields{
		AttachedMolecule: "gt-wisp-123",
		AttachedAt:       "2026-02-18T12:00:00Z",
		Mode:             "ralph",
	}

	formatted := FormatAttachmentFields(original)
	if !strings.Contains(formatted, "mode: ralph") {
		t.Errorf("FormatAttachmentFields missing mode field, got:\n%s", formatted)
	}

	issue := &Issue{Description: formatted}
	parsed := ParseAttachmentFields(issue)
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.Mode != "ralph" {
		t.Errorf("Mode: got %q, want %q", parsed.Mode, "ralph")
	}
	if parsed.AttachedMolecule != "gt-wisp-123" {
		t.Errorf("AttachedMolecule: got %q, want %q", parsed.AttachedMolecule, "gt-wisp-123")
	}
}

func TestSetAttachmentFieldsPreservesMode(t *testing.T) {
	issue := &Issue{
		Description: "mode: ralph\nattached_molecule: gt-wisp-old\nSome other content",
	}
	fields := &AttachmentFields{
		AttachedMolecule: "gt-wisp-new",
		Mode:             "ralph",
	}
	newDesc := SetAttachmentFields(issue, fields)
	if !strings.Contains(newDesc, "mode: ralph") {
		t.Errorf("SetAttachmentFields lost mode field, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "attached_molecule: gt-wisp-new") {
		t.Errorf("SetAttachmentFields lost attached_molecule, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "Some other content") {
		t.Errorf("SetAttachmentFields lost non-attachment content, got:\n%s", newDesc)
	}
}

func TestAttachmentFormulaVarsRoundTrip(t *testing.T) {
	fields := &AttachmentFields{
		AttachedFormula: "mol-miner-work",
		FormulaVars:     "feature=Bug to fix\nissue=gt-abc123\nbase_branch=main",
	}

	formatted := FormatAttachmentFields(fields)
	if !strings.Contains(formatted, `formula_vars: ["feature=Bug to fix","issue=gt-abc123","base_branch=main"]`) {
		t.Fatalf("formula_vars should use single-line JSON array, got:\n%s", formatted)
	}
	if strings.Contains(formatted, "\nissue=gt-abc123") {
		t.Fatalf("formula_vars leaked continuation lines:\n%s", formatted)
	}

	parsed := ParseAttachmentFields(&Issue{Description: formatted})
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	want := "feature=Bug to fix\nissue=gt-abc123\nbase_branch=main"
	if parsed.FormulaVars != want {
		t.Fatalf("FormulaVars = %q, want %q", parsed.FormulaVars, want)
	}
}

func TestParseAttachmentFieldsDoesNotConsumeAdjacentKeyValueLines(t *testing.T) {
	desc := "formula_vars: feature=Bug to fix\nissue=gt-abc123\nbase_branch=main\nexample=value\nmode: ralph"
	fields := ParseAttachmentFields(&Issue{Description: desc})
	if fields == nil {
		t.Fatal("ParseAttachmentFields returned nil")
	}
	want := "feature=Bug to fix"
	if fields.FormulaVars != want {
		t.Fatalf("FormulaVars = %q, want %q", fields.FormulaVars, want)
	}
	for _, adjacent := range []string{"issue=gt-abc123", "base_branch=main", "example=value"} {
		if strings.Contains(fields.FormulaVars, adjacent) {
			t.Fatalf("adjacent key=value line should not be parsed as formula var: %q", fields.FormulaVars)
		}
	}
	if fields.Mode != "ralph" {
		t.Fatalf("Mode = %q, want ralph", fields.Mode)
	}
}

func TestSetAttachmentFieldsPreservesAdjacentKeyValueLines(t *testing.T) {
	issue := &Issue{Description: "formula_vars: old=1\nissue=old\nbase_branch=old\nexample=value\n\nBody"}
	fields := &AttachmentFields{FormulaVars: "feature=New\nissue=gt-new"}

	newDesc := SetAttachmentFields(issue, fields)
	if !strings.Contains(newDesc, `formula_vars: ["feature=New","issue=gt-new"]`) {
		t.Fatalf("new formula_vars missing, got:\n%s", newDesc)
	}
	for _, adjacent := range []string{"issue=old", "base_branch=old", "example=value"} {
		if !strings.Contains(newDesc, adjacent) {
			t.Fatalf("adjacent key=value line %q should be preserved, got:\n%s", adjacent, newDesc)
		}
	}
	if !strings.Contains(newDesc, "Body") {
		t.Fatalf("prose should be preserved, got:\n%s", newDesc)
	}
}

// --- AgentFields Mode round-trip ---

func TestAgentFieldsModeRoundTrip(t *testing.T) {
	original := &AgentFields{
		RoleType:   "miner",
		Rig:        "excavation",
		AgentState: "working",
		HookBead:   "gt-abc",
		Mode:       "ralph",
	}

	formatted := FormatAgentDescription("Miner Test", original)
	if !strings.Contains(formatted, "mode: ralph") {
		t.Errorf("FormatAgentDescription missing mode field, got:\n%s", formatted)
	}

	parsed := ParseAgentFields(formatted)
	if parsed.Mode != "ralph" {
		t.Errorf("Mode: got %q, want %q", parsed.Mode, "ralph")
	}
	if parsed.RoleType != "miner" {
		t.Errorf("RoleType: got %q, want %q", parsed.RoleType, "miner")
	}
}

func TestAgentFieldsModeOmittedWhenEmpty(t *testing.T) {
	fields := &AgentFields{
		RoleType:   "miner",
		Rig:        "excavation",
		AgentState: "working",
		// Mode intentionally empty
	}

	formatted := FormatAgentDescription("Miner Test", fields)
	if strings.Contains(formatted, "mode:") {
		t.Errorf("FormatAgentDescription should not include mode when empty, got:\n%s", formatted)
	}
}

// --- Minecart fields in AttachmentFields (gt-7b6wf fix) ---

func TestParseAttachmentFieldsMinecart(t *testing.T) {
	tests := []struct {
		name              string
		desc              string
		wantMinecartID      string
		wantMergeStrategy string
	}{
		{
			name:              "minecart_id and merge_strategy",
			desc:              "attached_molecule: gt-wisp-abc\nminecart_id: hq-cv-xyz\nmerge_strategy: direct",
			wantMinecartID:      "hq-cv-xyz",
			wantMergeStrategy: "direct",
		},
		{
			name:              "hyphenated keys",
			desc:              "minecart-id: hq-cv-123\nmerge-strategy: local",
			wantMinecartID:      "hq-cv-123",
			wantMergeStrategy: "local",
		},
		{
			name:              "minecart key alias",
			desc:              "minecart: hq-cv-456",
			wantMinecartID:      "hq-cv-456",
			wantMergeStrategy: "",
		},
		{
			name:              "only merge_strategy (no minecart_id)",
			desc:              "merge_strategy: mr",
			wantMinecartID:      "",
			wantMergeStrategy: "mr",
		},
		{
			name:              "no minecart fields",
			desc:              "attached_molecule: gt-wisp-abc\ndispatched_by: overseer/",
			wantMinecartID:      "",
			wantMergeStrategy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &Issue{Description: tt.desc}
			fields := ParseAttachmentFields(issue)
			if fields == nil {
				if tt.wantMinecartID != "" || tt.wantMergeStrategy != "" {
					t.Fatal("ParseAttachmentFields() = nil, want non-nil")
				}
				return
			}
			if fields.MinecartID != tt.wantMinecartID {
				t.Errorf("MinecartID = %q, want %q", fields.MinecartID, tt.wantMinecartID)
			}
			if fields.MergeStrategy != tt.wantMergeStrategy {
				t.Errorf("MergeStrategy = %q, want %q", fields.MergeStrategy, tt.wantMergeStrategy)
			}
		})
	}
}

func TestFormatAttachmentFieldsMinecart(t *testing.T) {
	fields := &AttachmentFields{
		AttachedMolecule: "gt-wisp-abc",
		MinecartID:         "hq-cv-xyz",
		MergeStrategy:    "direct",
		MinecartOwned:      true,
	}
	got := FormatAttachmentFields(fields)
	if !strings.Contains(got, "minecart_id: hq-cv-xyz") {
		t.Errorf("FormatAttachmentFields missing minecart_id, got:\n%s", got)
	}
	if !strings.Contains(got, "merge_strategy: direct") {
		t.Errorf("FormatAttachmentFields missing merge_strategy, got:\n%s", got)
	}
	if !strings.Contains(got, "minecart_owned: true") {
		t.Errorf("FormatAttachmentFields missing minecart_owned, got:\n%s", got)
	}
}

func TestMinecartFieldsRoundTrip(t *testing.T) {
	original := &AttachmentFields{
		AttachedMolecule: "gt-wisp-abc",
		DispatchedBy:     "overseer/",
		MinecartID:         "hq-cv-xyz",
		MergeStrategy:    "direct",
		MinecartOwned:      true,
	}
	formatted := FormatAttachmentFields(original)
	parsed := ParseAttachmentFields(&Issue{Description: formatted})
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.MinecartID != original.MinecartID {
		t.Errorf("MinecartID: got %q, want %q", parsed.MinecartID, original.MinecartID)
	}
	if parsed.MergeStrategy != original.MergeStrategy {
		t.Errorf("MergeStrategy: got %q, want %q", parsed.MergeStrategy, original.MergeStrategy)
	}
	if parsed.AttachedMolecule != original.AttachedMolecule {
		t.Errorf("AttachedMolecule: got %q, want %q", parsed.AttachedMolecule, original.AttachedMolecule)
	}
	if parsed.MinecartOwned != original.MinecartOwned {
		t.Errorf("MinecartOwned: got %v, want %v", parsed.MinecartOwned, original.MinecartOwned)
	}
}

func TestMinecartOwnedFalseNotFormatted(t *testing.T) {
	fields := &AttachmentFields{
		MinecartID:    "hq-cv-xyz",
		MinecartOwned: false,
	}
	got := FormatAttachmentFields(fields)
	if strings.Contains(got, "minecart_owned") {
		t.Errorf("FormatAttachmentFields should not include minecart_owned when false, got:\n%s", got)
	}
}

func TestSetAttachmentFieldsPreservesMinecart(t *testing.T) {
	issue := &Issue{
		Description: "minecart_id: hq-cv-old\nmerge_strategy: direct\nminecart_owned: true\nattached_molecule: gt-wisp-old\nSome other content",
	}
	fields := &AttachmentFields{
		AttachedMolecule: "gt-wisp-new",
		MinecartID:         "hq-cv-new",
		MergeStrategy:    "local",
		MinecartOwned:      true,
	}
	newDesc := SetAttachmentFields(issue, fields)
	if !strings.Contains(newDesc, "minecart_id: hq-cv-new") {
		t.Errorf("SetAttachmentFields lost minecart_id field, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "merge_strategy: local") {
		t.Errorf("SetAttachmentFields lost merge_strategy field, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "minecart_owned: true") {
		t.Errorf("SetAttachmentFields lost minecart_owned field, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "Some other content") {
		t.Errorf("SetAttachmentFields lost non-attachment content, got:\n%s", newDesc)
	}
}

// --- FormatMinecartFields / SetMinecartFields ---

func TestFormatMinecartFields(t *testing.T) {
	tests := []struct {
		name   string
		fields *MinecartFields
		want   string
	}{
		{
			name:   "nil fields",
			fields: nil,
			want:   "",
		},
		{
			name:   "empty fields",
			fields: &MinecartFields{},
			want:   "",
		},
		{
			name:   "all fields",
			fields: &MinecartFields{Owner: "overseer/", Notify: "witness/", Merge: "direct", Molecule: "gt-wisp-abc"},
			want:   "Owner: overseer/\nNotify: witness/\nMerge: direct\nMolecule: gt-wisp-abc",
		},
		{
			name:   "only merge",
			fields: &MinecartFields{Merge: "mr"},
			want:   "Merge: mr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMinecartFields(tt.fields)
			if got != tt.want {
				t.Errorf("FormatMinecartFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetMinecartFields(t *testing.T) {
	tests := []struct {
		name   string
		issue  *Issue
		fields *MinecartFields
		want   string
	}{
		{
			name:   "nil issue",
			issue:  nil,
			fields: &MinecartFields{Owner: "overseer/", Merge: "direct"},
			want:   "Owner: overseer/\nMerge: direct",
		},
		{
			name:   "preserves prose",
			issue:  &Issue{Description: "Minecart tracking 3 issues"},
			fields: &MinecartFields{Owner: "overseer/", Merge: "mr"},
			want:   "Minecart tracking 3 issues\nOwner: overseer/\nMerge: mr",
		},
		{
			name:   "replaces existing fields",
			issue:  &Issue{Description: "Minecart tracking 3 issues\nOwner: old/\nMerge: local"},
			fields: &MinecartFields{Owner: "overseer/", Merge: "direct"},
			want:   "Minecart tracking 3 issues\nOwner: overseer/\nMerge: direct",
		},
		{
			name:   "empty fields removes field lines",
			issue:  &Issue{Description: "Minecart tracking 3 issues\nOwner: overseer/\nMerge: direct"},
			fields: &MinecartFields{},
			want:   "Minecart tracking 3 issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetMinecartFields(tt.issue, tt.fields)
			if got != tt.want {
				t.Errorf("SetMinecartFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMinecartFieldsParseFormatRoundTrip(t *testing.T) {
	original := &MinecartFields{
		Owner:                "overseer/",
		Notify:               "witness/",
		Merge:                "direct",
		Molecule:             "gt-wisp-abc",
		CompletionNotifiedAt: "2026-05-25T02:30:00Z",
	}
	formatted := FormatMinecartFields(original)
	parsed := ParseMinecartFields(&Issue{Description: formatted})
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.Owner != original.Owner {
		t.Errorf("Owner: got %q, want %q", parsed.Owner, original.Owner)
	}
	if parsed.Notify != original.Notify {
		t.Errorf("Notify: got %q, want %q", parsed.Notify, original.Notify)
	}
	if parsed.Merge != original.Merge {
		t.Errorf("Merge: got %q, want %q", parsed.Merge, original.Merge)
	}
	if parsed.Molecule != original.Molecule {
		t.Errorf("Molecule: got %q, want %q", parsed.Molecule, original.Molecule)
	}
	if parsed.CompletionNotifiedAt != original.CompletionNotifiedAt {
		t.Errorf("CompletionNotifiedAt: got %q, want %q", parsed.CompletionNotifiedAt, original.CompletionNotifiedAt)
	}
}

func TestSetMinecartFieldsWithMixedContent(t *testing.T) {
	issue := &Issue{Description: "Minecart tracking 3 issues\nOwner: old/\nSome prose line\nMerge: local\nAnother line"}
	fields := &MinecartFields{Owner: "new/", Merge: "direct", Molecule: "gt-mol-xyz"}
	got := SetMinecartFields(issue, fields)

	// Should preserve non-minecart prose
	if !strings.Contains(got, "Some prose line") {
		t.Errorf("lost prose line, got:\n%s", got)
	}
	if !strings.Contains(got, "Another line") {
		t.Errorf("lost another line, got:\n%s", got)
	}
	// Should have new fields
	if !strings.Contains(got, "Owner: new/") {
		t.Errorf("missing new Owner, got:\n%s", got)
	}
	if !strings.Contains(got, "Merge: direct") {
		t.Errorf("missing Merge, got:\n%s", got)
	}
	if !strings.Contains(got, "Molecule: gt-mol-xyz") {
		t.Errorf("missing Molecule, got:\n%s", got)
	}
	// Should NOT have old fields
	if strings.Contains(got, "Owner: old/") {
		t.Errorf("still has old Owner, got:\n%s", got)
	}
	if strings.Contains(got, "Merge: local") {
		t.Errorf("still has old Merge, got:\n%s", got)
	}
}

// --- ParseAgentFields (not covered in beads_test.go) ---

func TestParseAgentFields_AllFields(t *testing.T) {
	desc := "role_type: miner\nrig: excavation\nagent_state: working\nhook_bead: gt-abc\ncleanup_status: clean\nactive_mr: gt-mr1\nlast_source_issue: gt-src\nnotification_level: verbose"
	got := ParseAgentFields(desc)
	if got.RoleType != "miner" {
		t.Errorf("RoleType = %q, want %q", got.RoleType, "miner")
	}
	if got.Rig != "excavation" {
		t.Errorf("Rig = %q, want %q", got.Rig, "excavation")
	}
	if got.AgentState != "working" {
		t.Errorf("AgentState = %q, want %q", got.AgentState, "working")
	}
	if got.HookBead != "gt-abc" {
		t.Errorf("HookBead = %q, want %q", got.HookBead, "gt-abc")
	}
	if got.CleanupStatus != "clean" {
		t.Errorf("CleanupStatus = %q, want %q", got.CleanupStatus, "clean")
	}
	if got.ActiveMR != "gt-mr1" {
		t.Errorf("ActiveMR = %q, want %q", got.ActiveMR, "gt-mr1")
	}
	if got.LastSourceIssue != "gt-src" {
		t.Errorf("LastSourceIssue = %q, want %q", got.LastSourceIssue, "gt-src")
	}
	if got.NotificationLevel != "verbose" {
		t.Errorf("NotificationLevel = %q, want %q", got.NotificationLevel, "verbose")
	}
}

// --- Completion metadata fields (gt-x7t9) ---

func TestAgentFieldsCompletionMetadataRoundTrip(t *testing.T) {
	original := &AgentFields{
		RoleType:        "miner",
		Rig:             "excavation",
		AgentState:      "done",
		HookBead:        "gt-abc",
		ExitType:        "COMPLETED",
		MRID:            "gt-mr-xyz",
		Branch:          "miner/nux/gt-abc@hash",
		LastSourceIssue: "gt-abc",
		MRFailed:        false,
		CompletionTime:  "2026-02-28T01:00:00Z",
	}

	formatted := FormatAgentDescription("Miner nux", original)

	// Verify all completion fields are present
	if !strings.Contains(formatted, "exit_type: COMPLETED") {
		t.Errorf("missing exit_type in formatted output:\n%s", formatted)
	}
	if !strings.Contains(formatted, "mr_id: gt-mr-xyz") {
		t.Errorf("missing mr_id in formatted output:\n%s", formatted)
	}
	if !strings.Contains(formatted, "branch: miner/nux/gt-abc@hash") {
		t.Errorf("missing branch in formatted output:\n%s", formatted)
	}
	if !strings.Contains(formatted, "last_source_issue: gt-abc") {
		t.Errorf("missing last_source_issue in formatted output:\n%s", formatted)
	}
	if !strings.Contains(formatted, "completion_time: 2026-02-28T01:00:00Z") {
		t.Errorf("missing completion_time in formatted output:\n%s", formatted)
	}
	// mr_failed=false should NOT appear
	if strings.Contains(formatted, "mr_failed") {
		t.Errorf("mr_failed should not appear when false:\n%s", formatted)
	}

	// Parse and verify round-trip
	parsed := ParseAgentFields(formatted)
	if parsed.ExitType != "COMPLETED" {
		t.Errorf("ExitType: got %q, want %q", parsed.ExitType, "COMPLETED")
	}
	if parsed.MRID != "gt-mr-xyz" {
		t.Errorf("MRID: got %q, want %q", parsed.MRID, "gt-mr-xyz")
	}
	if parsed.Branch != "miner/nux/gt-abc@hash" {
		t.Errorf("Branch: got %q, want %q", parsed.Branch, "miner/nux/gt-abc@hash")
	}
	if parsed.LastSourceIssue != "gt-abc" {
		t.Errorf("LastSourceIssue: got %q, want %q", parsed.LastSourceIssue, "gt-abc")
	}
	if parsed.MRFailed != false {
		t.Errorf("MRFailed: got %v, want false", parsed.MRFailed)
	}
	if parsed.CompletionTime != "2026-02-28T01:00:00Z" {
		t.Errorf("CompletionTime: got %q, want %q", parsed.CompletionTime, "2026-02-28T01:00:00Z")
	}
	// Verify non-completion fields survive
	if parsed.RoleType != "miner" {
		t.Errorf("RoleType: got %q, want %q", parsed.RoleType, "miner")
	}
	if parsed.HookBead != "gt-abc" {
		t.Errorf("HookBead: got %q, want %q", parsed.HookBead, "gt-abc")
	}
}

func TestAgentFieldsMRFailedTrue(t *testing.T) {
	fields := &AgentFields{
		RoleType:   "miner",
		Rig:        "excavation",
		AgentState: "done",
		ExitType:   "COMPLETED",
		MRFailed:   true,
	}

	formatted := FormatAgentDescription("Miner nux", fields)
	if !strings.Contains(formatted, "mr_failed: true") {
		t.Errorf("missing mr_failed: true in formatted output:\n%s", formatted)
	}

	parsed := ParseAgentFields(formatted)
	if !parsed.MRFailed {
		t.Errorf("MRFailed: got false, want true")
	}
}

func TestAgentFieldsCompletionOmittedWhenEmpty(t *testing.T) {
	fields := &AgentFields{
		RoleType:   "miner",
		Rig:        "excavation",
		AgentState: "working",
		// All completion fields intentionally empty
	}

	formatted := FormatAgentDescription("Miner nux", fields)
	for _, keyword := range []string{"exit_type:", "mr_id:", "branch:", "last_source_issue:", "mr_failed:", "completion_time:"} {
		if strings.Contains(formatted, keyword) {
			t.Errorf("empty completion field %q should not appear in output:\n%s", keyword, formatted)
		}
	}
}

func TestParseAgentFields_WithCompletionMetadata(t *testing.T) {
	desc := "role_type: miner\nrig: excavation\nagent_state: done\nhook_bead: gt-abc\nexit_type: ESCALATED\nbranch: miner/nux/gt-abc@hash\nlast_source_issue: gt-abc\nmr_failed: true\ncompletion_time: 2026-02-28T02:00:00Z"
	got := ParseAgentFields(desc)
	if got.ExitType != "ESCALATED" {
		t.Errorf("ExitType = %q, want %q", got.ExitType, "ESCALATED")
	}
	if got.Branch != "miner/nux/gt-abc@hash" {
		t.Errorf("Branch = %q, want %q", got.Branch, "miner/nux/gt-abc@hash")
	}
	if !got.MRFailed {
		t.Errorf("MRFailed = false, want true")
	}
	if got.LastSourceIssue != "gt-abc" {
		t.Errorf("LastSourceIssue = %q, want %q", got.LastSourceIssue, "gt-abc")
	}
	if got.CompletionTime != "2026-02-28T02:00:00Z" {
		t.Errorf("CompletionTime = %q, want %q", got.CompletionTime, "2026-02-28T02:00:00Z")
	}
	if got.MRID != "" {
		t.Errorf("MRID = %q, want empty (not in desc)", got.MRID)
	}
}

// --- Minecart watcher tests ---

func TestMinecartFieldsWatchersRoundTrip(t *testing.T) {
	original := &MinecartFields{
		Owner:         "overseer/",
		Notify:        "witness/",
		Watchers:      "excavation/crew/mel,excavation/crew/tom",
		NudgeWatchers: "excavation/crew/joe",
	}
	formatted := FormatMinecartFields(original)
	parsed := ParseMinecartFields(&Issue{Description: formatted})
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.Watchers != original.Watchers {
		t.Errorf("Watchers: got %q, want %q", parsed.Watchers, original.Watchers)
	}
	if parsed.NudgeWatchers != original.NudgeWatchers {
		t.Errorf("NudgeWatchers: got %q, want %q", parsed.NudgeWatchers, original.NudgeWatchers)
	}
}

func TestMinecartFieldsAddWatcher(t *testing.T) {
	f := &MinecartFields{}

	// First add
	if !f.AddWatcher("excavation/crew/mel") {
		t.Error("AddWatcher should return true for new address")
	}
	if f.Watchers != "excavation/crew/mel" {
		t.Errorf("Watchers = %q, want %q", f.Watchers, "excavation/crew/mel")
	}

	// Second add
	if !f.AddWatcher("excavation/crew/tom") {
		t.Error("AddWatcher should return true for new address")
	}
	if f.Watchers != "excavation/crew/mel,excavation/crew/tom" {
		t.Errorf("Watchers = %q, want %q", f.Watchers, "excavation/crew/mel,excavation/crew/tom")
	}

	// Duplicate add
	if f.AddWatcher("excavation/crew/mel") {
		t.Error("AddWatcher should return false for duplicate")
	}
}

func TestMinecartFieldsAddNudgeWatcher(t *testing.T) {
	f := &MinecartFields{}

	if !f.AddNudgeWatcher("overseer/") {
		t.Error("AddNudgeWatcher should return true for new address")
	}
	if f.NudgeWatchers != "overseer/" {
		t.Errorf("NudgeWatchers = %q, want %q", f.NudgeWatchers, "overseer/")
	}

	if f.AddNudgeWatcher("overseer/") {
		t.Error("AddNudgeWatcher should return false for duplicate")
	}
}

func TestMinecartFieldsRemoveWatcher(t *testing.T) {
	f := &MinecartFields{Watchers: "a,b,c"}

	if !f.RemoveWatcher("b") {
		t.Error("RemoveWatcher should return true for existing address")
	}
	if f.Watchers != "a,c" {
		t.Errorf("Watchers = %q, want %q", f.Watchers, "a,c")
	}

	if f.RemoveWatcher("d") {
		t.Error("RemoveWatcher should return false for non-existing address")
	}
}

func TestMinecartFieldsRemoveNudgeWatcher(t *testing.T) {
	f := &MinecartFields{NudgeWatchers: "x,y"}

	if !f.RemoveNudgeWatcher("x") {
		t.Error("RemoveNudgeWatcher should return true for existing address")
	}
	if f.NudgeWatchers != "y" {
		t.Errorf("NudgeWatchers = %q, want %q", f.NudgeWatchers, "y")
	}
}

func TestNotificationAddressesIncludesWatchers(t *testing.T) {
	f := &MinecartFields{
		Owner:    "overseer/",
		Notify:   "witness/",
		Watchers: "excavation/crew/mel,overseer/", // overseer/ overlaps with Owner
	}
	addrs := f.NotificationAddresses()

	// Should be deduplicated: overseer/, witness/, excavation/crew/mel
	want := map[string]bool{"overseer/": true, "witness/": true, "excavation/crew/mel": true}
	got := make(map[string]bool)
	for _, a := range addrs {
		got[a] = true
	}
	if len(got) != len(want) {
		t.Errorf("NotificationAddresses: got %v, want %v", addrs, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("NotificationAddresses missing %q, got %v", k, addrs)
		}
	}
}

func TestNudgeNotificationAddresses(t *testing.T) {
	f := &MinecartFields{
		NudgeWatchers: "excavation/crew/mel,excavation/crew/tom",
	}
	addrs := f.NudgeNotificationAddresses()
	if len(addrs) != 2 {
		t.Errorf("NudgeNotificationAddresses: got %d addresses, want 2", len(addrs))
	}
}

func TestSetMinecartFieldsPreservesWatchers(t *testing.T) {
	issue := &Issue{Description: "Some text\nWatchers: a,b\nnudge_watchers: c"}
	fields := &MinecartFields{
		Owner:         "new/",
		Watchers:      "a,b,d",
		NudgeWatchers: "c,e",
	}
	got := SetMinecartFields(issue, fields)

	if !strings.Contains(got, "Watchers: a,b,d") {
		t.Errorf("missing updated Watchers, got:\n%s", got)
	}
	if !strings.Contains(got, "nudge_watchers: c,e") {
		t.Errorf("missing updated nudge_watchers, got:\n%s", got)
	}
	if !strings.Contains(got, "Some text") {
		t.Errorf("lost prose, got:\n%s", got)
	}
}
