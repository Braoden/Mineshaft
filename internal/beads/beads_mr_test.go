package beads

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMatchesMRSourceIssue(t *testing.T) {
	tests := []struct {
		name        string
		description string
		issueID     string
		want        bool
	}{
		{
			name:        "exact match",
			description: "branch: miner/furiosa/ms-abc@mm4heq3e\ntarget: main\nsource_issue: ms-abc\nrig: mineshaft\n",
			issueID:     "ms-abc",
			want:        true,
		},
		{
			name:        "no match different issue",
			description: "branch: miner/furiosa/ms-xyz@mm4heq3e\ntarget: main\nsource_issue: ms-xyz\nrig: mineshaft\n",
			issueID:     "ms-abc",
			want:        false,
		},
		{
			name:        "partial ID must not match — prefix",
			description: "branch: miner/nux/ms-abcdef@mm4heq3e\ntarget: main\nsource_issue: ms-abcdef\nrig: mineshaft\n",
			issueID:     "ms-abc",
			want:        false,
		},
		{
			name:        "partial ID must not match — suffix",
			description: "branch: miner/nux/ms-abc@mm4heq3e\ntarget: main\nsource_issue: ms-abc\nrig: mineshaft\n",
			issueID:     "ms-abcdef",
			want:        false,
		},
		{
			name:        "match with worker field after source_issue",
			description: "branch: miner/furiosa/la-cagb2@mm4heq3e\ntarget: main\nsource_issue: la-cagb2\nworker: miners/furiosa\n",
			issueID:     "la-cagb2",
			want:        true,
		},
		{
			name:        "source_issue at end of description (with trailing newline)",
			description: "branch: fix/thing\nsource_issue: ms-99\n",
			issueID:     "ms-99",
			want:        true,
		},
		{
			name:        "source_issue at end without trailing newline — no match",
			description: "branch: fix/thing\nsource_issue: ms-99",
			issueID:     "ms-99",
			want:        false,
		},
		{
			name:        "empty description",
			description: "",
			issueID:     "ms-abc",
			want:        false,
		},
		{
			name:        "empty issue ID",
			description: "source_issue: ms-abc\n",
			issueID:     "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesMRSourceIssue(tt.description, tt.issueID)
			if got != tt.want {
				t.Errorf("MatchesMRSourceIssue(%q, %q) = %v, want %v",
					tt.description, tt.issueID, got, tt.want)
			}
		})
	}
}

func TestUnresolvedBlockingDependencyIDs(t *testing.T) {
	tests := []struct {
		name string
		deps []IssueDep
		want []string
	}{
		{
			name: "open blocks dependency blocks",
			deps: []IssueDep{{ID: "ms-blocker", Status: "open", DependencyType: "blocks"}},
			want: []string{"ms-blocker"},
		},
		{
			name: "blocking types match ready-work semantics",
			deps: []IssueDep{
				{ID: "ms-conditional", Status: "open", DependencyType: "conditional-blocks"},
				{ID: "ms-waits", Status: "open", DependencyType: "waits-for"},
				{ID: "ms-merge", Status: "open", DependencyType: "merge-blocks"},
			},
			want: []string{"ms-conditional", "ms-waits", "ms-merge"},
		},
		{
			name: "closed and tombstone dependencies are resolved",
			deps: []IssueDep{
				{ID: "ms-closed", Status: "closed", DependencyType: "blocks"},
				{ID: "ms-tombstone", Status: "tombstone", DependencyType: "blocks"},
				{ID: "ms-pinned", Status: "pinned", DependencyType: "blocks"},
			},
		},
		{
			name: "merge-blocks requires merged close reason",
			deps: []IssueDep{
				{ID: "ms-closed-only", Status: "closed", DependencyType: "merge-blocks"},
				{ID: "ms-merged", Status: "closed", DependencyType: "merge-blocks", CloseReason: "Merged in abc123"},
			},
			want: []string{"ms-closed-only"},
		},
		{
			name: "nonblocking dependency types do not block",
			deps: []IssueDep{
				{ID: "ms-empty", Status: "open"},
				{ID: "ms-parent", Status: "open", DependencyType: "parent-child"},
				{ID: "ms-track", Status: "open", DependencyType: "tracks"},
				{ID: "ms-related", Status: "open", DependencyType: "related"},
				{ID: "ms-custom", Status: "open", DependencyType: "custom-link"},
			},
		},
		{
			name: "external dependency IDs are normalized",
			deps: []IssueDep{{ID: "external:ms:ms-blocker", Status: "open", DependencyType: "blocks"}},
			want: []string{"ms-blocker"},
		},
		{
			name: "missing status fails closed",
			deps: []IssueDep{{ID: "ms-unknown-status", DependencyType: "blocks"}},
			want: []string{"ms-unknown-status"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := unresolvedBlockingDependencyIDs(&Issue{Dependencies: tt.deps})
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("unresolvedBlockingDependencyIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHasUnresolvedBlockersFallsBackToListFields(t *testing.T) {
	if !HasUnresolvedBlockers(&Issue{BlockedByCount: 1}) {
		t.Fatal("BlockedByCount fallback should block when detailed dependencies are absent")
	}
	if !HasUnresolvedBlockers(&Issue{DependencyCount: 1}) {
		t.Fatal("DependencyCount fallback should fail closed when detailed dependencies are absent")
	}
	if got := FirstUnresolvedBlockerID(&Issue{DependencyCount: 1}); got != "" {
		t.Fatalf("FirstUnresolvedBlockerID() = %q, want empty when only count is available", got)
	}
	if got := FirstUnresolvedBlockerID(&Issue{BlockedBy: []string{"external:ms:ms-blocker"}}); got != "ms-blocker" {
		t.Fatalf("FirstUnresolvedBlockerID() = %q, want ms-blocker", got)
	}
	if HasUnresolvedBlockers(&Issue{Dependencies: []IssueDep{{ID: "ms-closed", Status: "closed", DependencyType: "blocks"}}, BlockedByCount: 1}) {
		t.Fatal("detailed closed dependency should override stale list blocker count")
	}
}

func TestListMergeRequestsHydratesWispMRBlockers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mock for bd")
	}
	installListMergeRequestsBDStub(t, false)

	b := New(t.TempDir())
	issues, err := b.ListMergeRequests(ListOptions{Label: "ms:merge-request", Status: "open", Priority: -1})
	if err != nil {
		t.Fatalf("ListMergeRequests() error = %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("ListMergeRequests() returned %d issues, want 1", len(issues))
	}

	issue := issues[0]
	if !issue.Ephemeral {
		t.Fatal("hydrated wisp MR should preserve Ephemeral=true")
	}
	if !HasUnresolvedBlockers(issue) {
		t.Fatalf("hydrated MR should be blocked: %#v", issue)
	}
	if got := FirstUnresolvedBlockerID(issue); got != "ms-blocker" {
		t.Fatalf("FirstUnresolvedBlockerID() = %q, want ms-blocker", got)
	}
	if issue.BlockedByCount != 1 {
		t.Fatalf("BlockedByCount = %d, want 1", issue.BlockedByCount)
	}
	if len(issue.Dependencies) != 1 {
		t.Fatalf("Dependencies len = %d, want 1", len(issue.Dependencies))
	}
}

func TestListMergeRequestsReturnsHydrationError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mock for bd")
	}
	installListMergeRequestsBDStub(t, true)

	b := New(t.TempDir())
	_, err := b.ListMergeRequests(ListOptions{Label: "ms:merge-request", Status: "open", Priority: -1})
	if err == nil {
		t.Fatal("ListMergeRequests() error = nil, want hydration error")
	}
	if !strings.Contains(err.Error(), "hydrating merge-request dependencies") {
		t.Fatalf("ListMergeRequests() error = %v, want hydration context", err)
	}
}

func TestListMergeRequestsFiltersRigBeforeHydration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mock for bd")
	}
	installListMergeRequestsRigFilterBDStub(t)

	b := New(t.TempDir())
	issues, err := b.ListMergeRequests(ListOptions{Label: "ms:merge-request", Status: "open", Priority: -1, Rig: "mineshaft"})
	if err != nil {
		t.Fatalf("ListMergeRequests() error = %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "ms-current" {
		t.Fatalf("ListMergeRequests() = %#v, want only ms-current", issues)
	}
}

func installListMergeRequestsBDStub(t *testing.T, failShow bool) {
	t.Helper()
	ResetBdAllowStaleCacheForTest()
	t.Cleanup(ResetBdAllowStaleCacheForTest)

	binDir := t.TempDir()
	showCase := `
    printf '%s\n' '[{"id":"ms-wisp-mr","title":"Merge: ms-source","description":"branch: miner/test/ms-source@abc\ntarget: main\nsource_issue: ms-source\nrig: mineshaft\n","status":"open","priority":1,"created_at":"2026-06-29T00:00:00Z","updated_at":"2026-06-29T00:00:00Z","ephemeral":true,"labels":["ms:merge-request"],"dependencies":[{"id":"ms-blocker","title":"Blocker","status":"open","priority":1,"issue_type":"task","dependency_type":"blocks"}],"dependency_count":1}]'
    exit 0
`
	if failShow {
		showCase = `
    echo 'show failed' >&2
    exit 2
`
	}

	script := `#!/bin/sh
if [ "${1:-}" = "--allow-stale" ]; then
  if [ "${2:-}" = "version" ]; then
    echo "Error: unknown flag: --allow-stale" >&2
    exit 0
  fi
  shift
fi
case "${1:-}" in
  list)
    printf '%s\n' '[]'
    exit 0
    ;;
  sql)
    printf '%s\n' '[{"id":"ms-wisp-mr","title":"Merge: ms-source","description":"branch: miner/test/ms-source@abc\ntarget: main\nsource_issue: ms-source\nrig: mineshaft\n","status":"open","priority":1,"assignee":"","created_at":"2026-06-29T00:00:00Z","updated_at":"2026-06-29T00:00:00Z","created_by":"tester","labels_csv":"ms:merge-request"}]'
    exit 0
    ;;
  show)` + showCase + `
    ;;
  *)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installListMergeRequestsRigFilterBDStub(t *testing.T) {
	t.Helper()
	ResetBdAllowStaleCacheForTest()
	t.Cleanup(ResetBdAllowStaleCacheForTest)

	binDir := t.TempDir()
	script := `#!/bin/sh
if [ "${1:-}" = "--allow-stale" ]; then
  if [ "${2:-}" = "version" ]; then
    echo "Error: unknown flag: --allow-stale" >&2
    exit 0
  fi
  shift
fi
case "${1:-}" in
  list)
    printf '%s\n' '[]'
    exit 0
    ;;
  sql)
    printf '%s\n' '[{"id":"ms-current","title":"Merge: ms-source","description":"branch: miner/test/ms-source@abc\ntarget: main\nsource_issue: ms-source\nrig: mineshaft\n","status":"open","priority":1,"assignee":"","created_at":"2026-06-29T00:00:00Z","updated_at":"2026-06-29T00:00:00Z","created_by":"tester","labels_csv":"ms:merge-request"},{"id":"ms-other","title":"Merge: ms-other","description":"branch: miner/test/ms-other@abc\ntarget: main\nsource_issue: ms-other-source\nrig: other-rig\n","status":"open","priority":1,"assignee":"","created_at":"2026-06-29T00:00:00Z","updated_at":"2026-06-29T00:00:00Z","created_by":"tester","labels_csv":"ms:merge-request"}]'
    exit 0
    ;;
  show)
    case "$*" in
      *ms-other*) echo 'other rig should not be hydrated' >&2; exit 7 ;;
    esac
    printf '%s\n' '[{"id":"ms-current","title":"Merge: ms-source","description":"branch: miner/test/ms-source@abc\ntarget: main\nsource_issue: ms-source\nrig: mineshaft\n","status":"open","priority":1,"created_at":"2026-06-29T00:00:00Z","updated_at":"2026-06-29T00:00:00Z","ephemeral":true,"labels":["ms:merge-request"]}]'
    exit 0
    ;;
  *)
    printf '%s\n' '[]'
    exit 0
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
