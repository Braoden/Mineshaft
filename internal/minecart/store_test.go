package minecart

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	beadsdk "github.com/steveyegge/beads"
)

// setupTestStore opens a real beads database in a temp dir for integration tests.
// Skips the test if the store cannot be opened (e.g. no CGO, no Dolt).
// Caller must run the returned cleanup when done.
func setupTestStore(t *testing.T) (beadsdk.Storage, func()) {
	t.Helper()

	t.Setenv("BEADS_TEST_MODE", "1")

	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	doltPath := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(doltPath, 0755); err != nil {
		t.Skipf("cannot create test dir: %v", err)
	}

	ctx := context.Background()
	store, err := beadsdk.Open(ctx, doltPath)
	if err != nil {
		t.Skipf("beads store unavailable (CGO/Dolt required): %v", err)
	}

	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		_ = store.Close()
		t.Skipf("SetConfig issue_prefix: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
	}
	return store, cleanup
}

func TestSetupTestStore_OpensStore(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	if store == nil {
		t.Fatal("setupTestStore returned nil store")
	}
}

func TestGetTrackingMinecarts_FiltersByTracksType(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	// Create minecart and tracked issue
	minecartIssue := &beadsdk.Issue{
		ID:        "hq-cv-test1",
		Title:     "Test Minecart",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	trackedIssue := &beadsdk.Issue{
		ID:        "ms-tracked1",
		Title:     "Tracked",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateIssue(ctx, minecartIssue, "test"); err != nil {
		t.Fatalf("CreateIssue minecart: %v", err)
	}
	if err := store.CreateIssue(ctx, trackedIssue, "test"); err != nil {
		t.Fatalf("CreateIssue tracked: %v", err)
	}

	// Add tracks dependency: minecart tracks issue (minecart depends on issue with type tracks)
	dep := &beadsdk.Dependency{
		IssueID:     minecartIssue.ID,
		DependsOnID: trackedIssue.ID,
		Type:        beadsdk.DependencyType("tracks"),
		CreatedAt:   now,
		CreatedBy:   "test",
	}
	if err := store.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}

	// Add blocks dependency (should be filtered out)
	otherIssue := &beadsdk.Issue{
		ID:        "ms-other",
		Title:     "Other",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.CreateIssue(ctx, otherIssue, "test"); err != nil {
		t.Fatalf("CreateIssue other: %v", err)
	}
	blocksDep := &beadsdk.Dependency{
		IssueID:     "hq-cv-other",
		DependsOnID: trackedIssue.ID,
		Type:        beadsdk.DepBlocks,
		CreatedAt:   now,
		CreatedBy:   "test",
	}
	if err := store.CreateIssue(ctx, &beadsdk.Issue{
		ID:        "hq-cv-other",
		Title:     "Other Minecart",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}, "test"); err != nil {
		t.Fatalf("CreateIssue other minecart: %v", err)
	}
	if err := store.AddDependency(ctx, blocksDep, "test"); err != nil {
		t.Fatalf("AddDependency blocks: %v", err)
	}

	// getTrackingMinecarts(trackedIssue.ID) should return only hq-cv-test1 (tracks), not hq-cv-other (blocks)
	minecartIDs := getTrackingMinecarts(ctx, store, trackedIssue.ID, nil)
	if len(minecartIDs) != 1 || minecartIDs[0] != minecartIssue.ID {
		t.Errorf("getTrackingMinecarts = %v, want [%s]", minecartIDs, minecartIssue.ID)
	}
}

func TestIsMinecartClosed_ReturnsCorrectStatus(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	openIssue := &beadsdk.Issue{
		ID:        "hq-cv-open",
		Title:     "Open",
		Status:    beadsdk.StatusOpen,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}
	closedIssue := &beadsdk.Issue{
		ID:        "hq-cv-closed",
		Title:     "Closed",
		Status:    beadsdk.StatusClosed,
		Priority:  2,
		IssueType: beadsdk.TypeTask,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateIssue(ctx, openIssue, "test"); err != nil {
		t.Fatalf("CreateIssue open: %v", err)
	}
	if err := store.CreateIssue(ctx, closedIssue, "test"); err != nil {
		t.Fatalf("CreateIssue closed: %v", err)
	}

	if isMinecartClosed(ctx, store, openIssue.ID) {
		t.Error("isMinecartClosed(open) = true, want false")
	}
	if !isMinecartClosed(ctx, store, closedIssue.ID) {
		t.Error("isMinecartClosed(closed) = false, want true")
	}
}
