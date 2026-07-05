package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/scheduler/capacity"
)

func setupMinerCapacityTestTown(t *testing.T, maxMiners int) string {
	t.Helper()
	townRoot := t.TempDir()
	configureScheduler(t, townRoot, maxMiners, 1)
	if err := config.SaveRigsConfig(filepath.Join(townRoot, "overseer", "rigs.json"), &config.RigsConfig{Version: config.CurrentRigsVersion}); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}
	return townRoot
}

func setupMinerCapacityRig(t *testing.T, maxMiners int) string {
	t.Helper()
	townRoot := t.TempDir()
	configureScheduler(t, townRoot, maxMiners, 1)
	if err := os.MkdirAll(filepath.Join(townRoot, "mineshaft", "miners"), 0755); err != nil {
		t.Fatalf("mkdir rig: %v", err)
	}
	if err := config.SaveRigsConfig(filepath.Join(townRoot, "overseer", "rigs.json"), &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"mineshaft": {GitURL: "https://example.invalid/mineshaft.git"},
		},
	}); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	return townRoot
}

func TestCapacitySnapshotCleansStaleReservations(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)
	dir := minerAdmissionDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir reservations: %v", err)
	}
	stale := minerAdmissionReservation{
		ID:        "stale",
		PID:       99999999,
		Rig:       "mineshaft",
		Bead:      "ms-stale",
		Operation: "test",
		CreatedAt: time.Now().Add(-2 * minerAdmissionReservationTTL),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatalf("marshal stale reservation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stale.json"), data, 0644); err != nil {
		t.Fatalf("write stale reservation: %v", err)
	}

	snapshot, err := minerCapacitySnapshotForTown(townRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Reservations != 0 || snapshot.Free != 1 {
		t.Fatalf("snapshot after stale cleanup = %+v, want reservations=0 free=1", snapshot)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.json")); !os.IsNotExist(err) {
		t.Fatalf("stale reservation still exists: %v", err)
	}
}

func TestCapacitySnapshotRemovesStructurallyInvalidReservations(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)
	dir := minerAdmissionDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir reservations: %v", err)
	}
	path := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatalf("write invalid reservation: %v", err)
	}

	snapshot, err := minerCapacitySnapshotForTown(townRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Reservations != 0 || snapshot.Free != 1 {
		t.Fatalf("snapshot after invalid cleanup = %+v, want reservations=0 free=1", snapshot)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid reservation still exists: %v", err)
	}
}

func TestCapacitySnapshotRemovesMismatchedReservationFile(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)
	dir := minerAdmissionDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir reservations: %v", err)
	}
	reservation := minerAdmissionReservation{
		ID:        "other",
		PID:       os.Getpid(),
		Rig:       "mineshaft",
		Bead:      "ms-mismatch",
		Operation: "test",
		CreatedAt: time.Now(),
	}
	data, err := json.Marshal(reservation)
	if err != nil {
		t.Fatalf("marshal reservation: %v", err)
	}
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write mismatched reservation: %v", err)
	}

	snapshot, err := minerCapacitySnapshotForTown(townRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Reservations != 0 || snapshot.Free != 1 {
		t.Fatalf("snapshot after mismatch cleanup = %+v, want reservations=0 free=1", snapshot)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("mismatched reservation still exists: %v", err)
	}
}

func TestCapacitySnapshotKeepsOldLiveReservation(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)
	dir := minerAdmissionDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir reservations: %v", err)
	}
	reservation := minerAdmissionReservation{
		ID:        "live",
		PID:       os.Getpid(),
		Rig:       "mineshaft",
		Bead:      "ms-live",
		Operation: "test",
		CreatedAt: time.Now().Add(-2 * minerAdmissionReservationTTL),
	}
	data, err := json.Marshal(reservation)
	if err != nil {
		t.Fatalf("marshal reservation: %v", err)
	}
	path := filepath.Join(dir, "live.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write live reservation: %v", err)
	}

	snapshot, err := minerCapacitySnapshotForTown(townRoot)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Reservations != 1 || snapshot.Free != 0 {
		t.Fatalf("snapshot with old live reservation = %+v, want reservations=1 free=0", snapshot)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("live reservation should remain: %v", err)
	}
}

func TestAcquireMinerAdmissionUsesConfiguredCap(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)

	first, snapshot, err := acquireMinerAdmission(townRoot, "mineshaft", "ms-one", "test")
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	defer first.Release()
	if snapshot.Max != 1 || snapshot.Reservations != 1 || snapshot.Free != 0 {
		t.Fatalf("snapshot after first admission = %+v, want max=1 reservations=1 free=0", snapshot)
	}

	second, deniedSnapshot, err := acquireMinerAdmission(townRoot, "mineshaft", "ms-two", "test")
	if second != nil {
		defer second.Release()
	}
	var admissionErr *minerCapacityAdmissionError
	if !errors.As(err, &admissionErr) {
		t.Fatalf("second admission error = %v, want minerCapacityAdmissionError", err)
	}
	if deniedSnapshot.Max != 1 || deniedSnapshot.Reservations != 1 || deniedSnapshot.Free != 0 {
		t.Fatalf("denied snapshot = %+v, want max=1 reservations=1 free=0", deniedSnapshot)
	}
	if !strings.Contains(err.Error(), "scheduler.max_miners") {
		t.Fatalf("denial error %q should mention scheduler.max_miners", err.Error())
	}

	first.Release()
	third, snapshot, err := acquireMinerAdmission(townRoot, "mineshaft", "ms-three", "test")
	if err != nil {
		t.Fatalf("third admission after release: %v", err)
	}
	defer third.Release()
	if snapshot.Max != 1 || snapshot.Reservations != 1 || snapshot.Free != 0 {
		t.Fatalf("snapshot after third admission = %+v, want max=1 reservations=1 free=0", snapshot)
	}
}

func TestAcquireMinerAdmissionDisabledWhenSchedulerCapNonPositive(t *testing.T) {
	for _, maxMiners := range []int{-1, 0} {
		t.Run("max", func(t *testing.T) {
			townRoot := t.TempDir()
			configureScheduler(t, townRoot, maxMiners, 1)

			handle, snapshot, err := acquireMinerAdmission(townRoot, "mineshaft", "ms-one", "test")
			if err != nil {
				t.Fatalf("admission with max=%d: %v", maxMiners, err)
			}
			defer handle.Release()
			if !handle.disabled {
				t.Fatalf("admission handle should be disabled for max=%d", maxMiners)
			}
			if snapshot.Max != maxMiners {
				t.Fatalf("snapshot max = %d, want %d", snapshot.Max, maxMiners)
			}
			if _, err := os.Stat(minerAdmissionDir(townRoot)); !os.IsNotExist(err) {
				t.Fatalf("reservation dir exists for disabled admission: %v", err)
			}
		})
	}
}

func TestConcurrentMinerAdmissionReservationsDoNotExceedCap(t *testing.T) {
	townRoot := setupMinerCapacityTestTown(t, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var handles []*minerAdmissionHandle
	successes := 0
	denials := 0

	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			handle, _, err := acquireMinerAdmission(townRoot, "mineshaft", "ms-race", "test")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
				handles = append(handles, handle)
				return
			}
			var admissionErr *minerCapacityAdmissionError
			if errors.As(err, &admissionErr) || strings.Contains(err.Error(), "admission is busy") {
				denials++
				return
			}
			t.Errorf("unexpected admission error: %v", err)
		}()
	}
	close(start)
	wg.Wait()
	for _, handle := range handles {
		handle.Release()
	}

	if successes != 1 {
		t.Fatalf("successful admissions = %d, want 1", successes)
	}
	if denials != 5 {
		t.Fatalf("denied admissions = %d, want 5", denials)
	}
}

func TestApplyAgentFieldsToCapacitySnapshotSeparatesPendingMR(t *testing.T) {
	tests := []struct {
		name   string
		fields *beads.AgentFields
		want   minerCapacitySnapshot
	}{
		{
			name:   "active mr is pending capacity",
			fields: &beads.AgentFields{AgentState: string(beads.AgentStateIdle), CleanupStatus: "clean", ActiveMR: "ms-mr-open"},
			want:   minerCapacitySnapshot{PendingMR: 1},
		},
		{
			name:   "push failed remains recovery blocked",
			fields: &beads.AgentFields{AgentState: string(beads.AgentStateIdle), CleanupStatus: "clean", ActiveMR: "ms-mr-open", PushFailed: true},
			want:   minerCapacitySnapshot{RecoveryBlocked: 1},
		},
		{
			name:   "clean idle is reusable",
			fields: &beads.AgentFields{AgentState: string(beads.AgentStateIdle), CleanupStatus: "clean"},
			want:   minerCapacitySnapshot{ReusableIdle: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := minerCapacitySnapshot{}
			applyAgentFieldsToCapacitySnapshot(&snapshot, "", "mineshaft", "synth", tt.fields, nil)
			if snapshot.Working != tt.want.Working || snapshot.RecoveryBlocked != tt.want.RecoveryBlocked || snapshot.ReusableIdle != tt.want.ReusableIdle || snapshot.PendingMR != tt.want.PendingMR {
				t.Fatalf("snapshot = %+v, want %+v", snapshot, tt.want)
			}
		})
	}
}

func TestPrintDryRunPlanUsesCapacitySnapshot(t *testing.T) {
	out := captureStdout(t, func() {
		printDryRunPlan(capacity.DispatchPlan{
			ToDispatch: []capacity.PendingBead{{ID: "ctx-1", WorkBeadID: "ms-one", TargetRig: "mineshaft"}},
			Skipped:    2,
			Reason:     "capacity",
		}, minerCapacitySnapshot{
			Max:             2,
			Working:         1,
			RecoveryBlocked: 1,
			Reservations:    0,
			ReusableIdle:    3,
			PendingMR:       2,
			Free:            0,
		}, 5)
	})
	for _, want := range []string{"0 free of 2", "working: 1", "recovery_blocked: 1", "reusable_idle: 3", "pending_mr: 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output %q missing %q", out, want)
		}
	}
}

func TestResolveTargetRigPassesHeldAdmissionToSpawn(t *testing.T) {
	townRoot := setupMinerCapacityRig(t, 1)
	oldSpawn := spawnMinerForSling
	t.Cleanup(func() { spawnMinerForSling = oldSpawn })
	called := false
	spawnMinerForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedMinerInfo, error) {
		called = true
		if rigName != "mineshaft" {
			t.Fatalf("rigName = %q, want mineshaft", rigName)
		}
		if !opts.SkipAdmission {
			t.Fatal("spawn should skip admission when caller already holds reservation")
		}
		if opts.TownRoot != townRoot {
			t.Fatalf("TownRoot = %q, want %q", opts.TownRoot, townRoot)
		}
		return &SpawnedMinerInfo{
			RigName:     "mineshaft",
			MinerName: "toast",
			ClonePath:   filepath.Join(townRoot, "mineshaft", "miners", "toast", "mineshaft"),
			SessionName: "ms-mineshaft-miner-toast",
		}, nil
	}

	resolved, err := resolveTarget("mineshaft", ResolveTargetOptions{
		TownRoot:             townRoot,
		SkipMinerAdmission: true,
		NoBoot:               true,
	})
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if !called {
		t.Fatal("spawnMinerForSling was not called")
	}
	if resolved.Agent != "mineshaft/miners/toast" {
		t.Fatalf("resolved agent = %q, want mineshaft/miners/toast", resolved.Agent)
	}
}

func TestStandaloneFormulaRigTargetAcquiresSingleAdmission(t *testing.T) {
	townRoot := setupMinerCapacityRig(t, 1)
	oldAcquire := acquireMinerAdmissionFn
	oldSpawn := spawnMinerForSling
	oldFind := findHookedFormulaSingletonFn
	oldDryRun, oldNoBoot := slingDryRun, slingNoBoot
	t.Cleanup(func() {
		acquireMinerAdmissionFn = oldAcquire
		spawnMinerForSling = oldSpawn
		findHookedFormulaSingletonFn = oldFind
		slingDryRun, slingNoBoot = oldDryRun, oldNoBoot
	})
	slingDryRun = false
	slingNoBoot = true
	admissions := 0
	acquireMinerAdmissionFn = func(townRootArg, rigName, beadID, operation string) (*minerAdmissionHandle, minerCapacitySnapshot, error) {
		admissions++
		if townRootArg != townRoot || rigName != "mineshaft" || beadID != "test-formula" || operation != "formula" {
			t.Fatalf("admission args = (%q,%q,%q,%q)", townRootArg, rigName, beadID, operation)
		}
		return &minerAdmissionHandle{disabled: true}, minerCapacitySnapshot{Max: 1, Free: 0}, nil
	}
	spawnMinerForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedMinerInfo, error) {
		if !opts.SkipAdmission {
			t.Fatal("formula rig spawn should use caller-held admission")
		}
		return &SpawnedMinerInfo{
			RigName:     "mineshaft",
			MinerName: "toast",
			ClonePath:   filepath.Join(townRoot, "mineshaft", "miners", "toast", "mineshaft"),
			SessionName: "ms-mineshaft-miner-toast",
		}, nil
	}
	findHookedFormulaSingletonFn = func(workDir, targetAgent, formulaName string) (*beads.Issue, error) {
		return &beads.Issue{ID: "ms-wisp-existing"}, nil
	}

	if err := runSlingFormula(context.Background(), []string{"test-formula", "mineshaft"}); err != nil {
		t.Fatalf("runSlingFormula: %v", err)
	}
	if admissions != 1 {
		t.Fatalf("admissions = %d, want 1", admissions)
	}
}

func TestStandaloneFormulaExistingMinerNoopDoesNotRequireCapacity(t *testing.T) {
	townRoot := setupMinerCapacityRig(t, 1)
	oldAcquire := acquireMinerAdmissionFn
	oldResolve := resolveTargetAgentFn
	oldFind := findHookedFormulaSingletonFn
	oldDryRun := slingDryRun
	t.Cleanup(func() {
		acquireMinerAdmissionFn = oldAcquire
		resolveTargetAgentFn = oldResolve
		findHookedFormulaSingletonFn = oldFind
		slingDryRun = oldDryRun
	})
	slingDryRun = false
	acquireMinerAdmissionFn = func(townRootArg, rigName, beadID, operation string) (*minerAdmissionHandle, minerCapacitySnapshot, error) {
		t.Fatalf("no-op existing formula should not acquire capacity, got (%q,%q,%q,%q)", townRootArg, rigName, beadID, operation)
		return nil, minerCapacitySnapshot{}, nil
	}
	resolveTargetAgentFn = func(target string) (string, string, string, error) {
		if target != "mineshaft/miners/toast" {
			t.Fatalf("target = %q, want mineshaft/miners/toast", target)
		}
		return "mineshaft/miners/toast", "%1", filepath.Join(townRoot, "mineshaft", "miners", "toast", "mineshaft"), nil
	}
	findHookedFormulaSingletonFn = func(workDir, targetAgent, formulaName string) (*beads.Issue, error) {
		return &beads.Issue{ID: "ms-wisp-existing"}, nil
	}

	if err := runSlingFormula(context.Background(), []string{"test-formula", "mineshaft/miners/toast"}); err != nil {
		t.Fatalf("runSlingFormula: %v", err)
	}
}
