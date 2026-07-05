package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/git"
	"github.com/steveyegge/mineshaft/internal/miner"
	"github.com/steveyegge/mineshaft/internal/rig"
	"github.com/steveyegge/mineshaft/internal/scheduler/capacity"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/tmux"
)

const minerAdmissionReservationTTL = 30 * time.Minute

var acquireMinerAdmissionFn = acquireMinerAdmission

type minerCapacitySnapshot struct {
	Max             int `json:"max"`
	Working         int `json:"working"`
	RecoveryBlocked int `json:"recovery_blocked"`
	ReusableIdle    int `json:"reusable_idle"`
	PendingMR       int `json:"pending_mr"`
	Reservations    int `json:"reservations"`
	Free            int `json:"free"`
	ActiveSessions  int `json:"active_sessions"`
}

func (s minerCapacitySnapshot) occupied() int {
	return s.Working + s.RecoveryBlocked + s.Reservations
}

type minerAdmissionReservation struct {
	ID        string    `json:"id"`
	PID       int       `json:"pid"`
	Rig       string    `json:"rig,omitempty"`
	Bead      string    `json:"bead,omitempty"`
	Operation string    `json:"operation"`
	CreatedAt time.Time `json:"created_at"`
}

type minerAdmissionHandle struct {
	townRoot string
	id       string
	path     string
	disabled bool
}

func (h *minerAdmissionHandle) Release() {
	if h == nil || h.disabled || h.path == "" {
		return
	}
	_ = os.Remove(h.path)
}

type minerCapacityAdmissionError struct {
	Snapshot minerCapacitySnapshot
	Rig      string
	Bead     string
	Reason   string
}

func (e *minerCapacityAdmissionError) Error() string {
	if e == nil {
		return "miner admission denied"
	}
	if e.Snapshot.Max <= 0 {
		return fmt.Sprintf("miner admission denied: %s", e.Reason)
	}
	return fmt.Sprintf(
		"miner admission denied: %s (max=%d occupied=%d working=%d recovery_blocked=%d reservations=%d reusable_idle=%d pending_mr=%d free=%d). Resolve recovery-needed miners or raise scheduler.max_miners; inspect with `ms scheduler status --json` or `ms miner list --all --json`",
		e.Reason,
		e.Snapshot.Max,
		e.Snapshot.occupied(),
		e.Snapshot.Working,
		e.Snapshot.RecoveryBlocked,
		e.Snapshot.Reservations,
		e.Snapshot.ReusableIdle,
		e.Snapshot.PendingMR,
		e.Snapshot.Free,
	)
}

func acquireMinerAdmission(townRoot, rigName, beadID, operation string) (*minerAdmissionHandle, minerCapacitySnapshot, error) {
	max, err := configuredSchedulerMaxMiners(townRoot)
	if err != nil {
		return nil, minerCapacitySnapshot{}, err
	}
	if max <= 0 {
		return &minerAdmissionHandle{disabled: true}, minerCapacitySnapshot{Max: max, ActiveSessions: countActiveMiners()}, nil
	}

	lock, err := acquireMinerAdmissionLock(townRoot)
	if err != nil {
		return nil, minerCapacitySnapshot{}, err
	}
	defer func() { _ = lock.Unlock() }()

	if err := cleanupStaleMinerAdmissionReservations(townRoot, time.Now()); err != nil {
		return nil, minerCapacitySnapshot{}, err
	}

	snapshot, err := minerCapacitySnapshotForTownNoCleanup(townRoot)
	if err != nil {
		return nil, minerCapacitySnapshot{}, err
	}
	if snapshot.Free <= 0 {
		return nil, snapshot, &minerCapacityAdmissionError{
			Snapshot: snapshot,
			Rig:      rigName,
			Bead:     beadID,
			Reason:   "configured scheduler.max_miners capacity is full",
		}
	}

	reservation, path, err := writeMinerAdmissionReservation(townRoot, rigName, beadID, operation)
	if err != nil {
		return nil, snapshot, err
	}
	snapshot.Reservations++
	snapshot.Free--
	return &minerAdmissionHandle{townRoot: townRoot, id: reservation.ID, path: path}, snapshot, nil
}

func configuredSchedulerMaxMiners(townRoot string) (int, error) {
	settings, err := config.LoadOrCreateTownSettings(config.TownSettingsPath(townRoot))
	if err != nil {
		return 0, fmt.Errorf("loading town settings for miner admission: %w", err)
	}
	schedulerCfg := settings.Scheduler
	if schedulerCfg == nil {
		schedulerCfg = capacity.DefaultSchedulerConfig()
	}
	return schedulerCfg.GetMaxMiners(), nil
}

func minerCapacitySnapshotForTown(townRoot string) (minerCapacitySnapshot, error) {
	max, err := configuredSchedulerMaxMiners(townRoot)
	if err != nil {
		return minerCapacitySnapshot{}, err
	}
	if max > 0 {
		if err := cleanupStaleMinerAdmissionReservationsWithLock(townRoot, time.Now()); err != nil {
			return minerCapacitySnapshot{}, err
		}
	}
	return minerCapacitySnapshotForTownNoCleanup(townRoot)
}

func minerCapacitySnapshotForTownNoCleanup(townRoot string) (minerCapacitySnapshot, error) {
	max, err := configuredSchedulerMaxMiners(townRoot)
	if err != nil {
		return minerCapacitySnapshot{}, err
	}
	snapshot := minerCapacitySnapshot{Max: max, ActiveSessions: countActiveMiners()}
	if max <= 0 {
		return snapshot, nil
	}

	rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return snapshot, fmt.Errorf("loading rigs config for miner capacity: %w", err)
	}

	tmuxClient := tmux.NewTmux()
	for rigName := range rigsConfig.Rigs {
		rigPath := filepath.Join(townRoot, rigName)
		if _, err := os.Stat(rigPath); err != nil {
			continue
		}
		minerNames, err := listMinerDirectoryNames(rigPath)
		if err != nil {
			return snapshot, fmt.Errorf("listing miner dirs for %s capacity: %w", rigName, err)
		}
		if len(minerNames) == 0 {
			continue
		}

		agents, err := beads.New(rigPath).ListAgentBeads()
		if err != nil {
			return snapshot, fmt.Errorf("listing agent beads for %s capacity: %w", rigName, err)
		}
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		for _, name := range minerNames {
			agentID := beads.MinerBeadIDWithPrefix(prefix, rigName, name)
			issue := agents[agentID]
			fields := (*beads.AgentFields)(nil)
			if issue != nil {
				fields = beads.ParseAgentFields(issue.Description)
				fields.AgentState = beads.ResolveAgentState(issue.Description, issue.AgentState)
			}
			applyAgentFieldsToCapacitySnapshot(&snapshot, rigPath, rigName, name, fields, tmuxClient)
		}
	}

	reservations, err := readMinerAdmissionReservations(townRoot)
	if err != nil {
		return snapshot, err
	}
	snapshot.Reservations = len(reservations)
	if max > 0 {
		snapshot.Free = max - snapshot.occupied()
		if snapshot.Free < 0 {
			snapshot.Free = 0
		}
	}
	return snapshot, nil
}

func listMinerDirectoryNames(rigPath string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(rigPath, "miners"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func applyAgentFieldsToCapacitySnapshot(snapshot *minerCapacitySnapshot, rigPath, rigName, minerName string, fields *beads.AgentFields, tmuxClient *tmux.Tmux) {
	running := false
	if tmuxClient != nil {
		running, _ = tmuxClient.HasSession(session.MinerSessionName(session.PrefixFor(rigName), minerName))
	}
	if fields == nil {
		if running {
			snapshot.Working++
		} else {
			snapshot.RecoveryBlocked++
		}
		return
	}

	state := strings.TrimSpace(fields.AgentState)
	if state == "working" || state == "spawning" {
		if running {
			snapshot.Working++
		} else {
			snapshot.RecoveryBlocked++
		}
		return
	}
	if fields.HookBead != "" {
		if running {
			snapshot.Working++
		} else if applyCanonicalCapacitySnapshot(snapshot, rigPath, rigName, minerName, fields, tmuxClient) {
			return
		} else {
			snapshot.RecoveryBlocked++
		}
		return
	}
	if fields.PushFailed || fields.MRFailed {
		snapshot.RecoveryBlocked++
		return
	}
	if fields.ActiveMR != "" || (fields.CleanupStatus != "" && fields.CleanupStatus != "clean") {
		if applyCanonicalCapacitySnapshot(snapshot, rigPath, rigName, minerName, fields, tmuxClient) {
			return
		}
	}
	if fields.ActiveMR != "" {
		snapshot.PendingMR++
		return
	}
	if fields.CleanupStatus == "clean" || state == "nuked" {
		snapshot.ReusableIdle++
		return
	}
	snapshot.RecoveryBlocked++
}

func applyCanonicalCapacitySnapshot(snapshot *minerCapacitySnapshot, rigPath, rigName, minerName string, fields *beads.AgentFields, tmuxClient *tmux.Tmux) bool {
	if snapshot == nil || fields == nil || rigPath == "" {
		return false
	}
	state := miner.State(strings.TrimSpace(fields.AgentState))
	if state == "" {
		state = miner.StateIdle
	}
	issueID := fields.LastSourceIssue
	if issueID == "" {
		issueID = fields.HookBead
	}
	mgr := miner.NewManager(&rig.Rig{Name: rigName, Path: rigPath}, git.NewGit(rigPath), tmuxClient)
	disposition := mgr.WorkstateDispositionForMiner(minerName, state, issueID)
	applyWorkstateDispositionToCapacitySnapshot(snapshot, state, disposition)
	return true
}

func applyWorkstateDispositionToCapacitySnapshot(snapshot *minerCapacitySnapshot, state miner.State, disposition miner.WorkstateDisposition) {
	if disposition.ReuseStatus == "idle-pr-open" {
		snapshot.PendingMR++
		return
	}
	if disposition.Reusable {
		snapshot.ReusableIdle++
		return
	}
	if !disposition.CountsTowardCapacity {
		return
	}
	if state == miner.StateWorking || disposition.Verdict == miner.WorkstateVerdictWorking {
		snapshot.Working++
		return
	}
	snapshot.RecoveryBlocked++
}

func acquireMinerAdmissionLock(townRoot string) (*flock.Flock, error) {
	lockDir := filepath.Join(townRoot, ".runtime", "locks")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating miner admission lock dir: %w", err)
	}
	lock := flock.New(filepath.Join(lockDir, "miner-admission.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquiring miner admission lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("miner admission is busy; retry shortly")
	}
	return lock, nil
}

func minerAdmissionDir(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "miner-admission")
}

func writeMinerAdmissionReservation(townRoot, rigName, beadID, operation string) (minerAdmissionReservation, string, error) {
	dir := minerAdmissionDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return minerAdmissionReservation{}, "", fmt.Errorf("creating miner admission dir: %w", err)
	}
	now := time.Now().UTC()
	id := fmt.Sprintf("%d-%d", os.Getpid(), now.UnixNano())
	reservation := minerAdmissionReservation{
		ID:        id,
		PID:       os.Getpid(),
		Rig:       rigName,
		Bead:      beadID,
		Operation: operation,
		CreatedAt: now,
	}
	path := filepath.Join(dir, id+".json")
	tmpPath := path + ".tmp"
	data, err := json.MarshalIndent(reservation, "", "  ")
	if err != nil {
		return minerAdmissionReservation{}, "", err
	}
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return minerAdmissionReservation{}, "", fmt.Errorf("writing miner admission reservation: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return minerAdmissionReservation{}, "", fmt.Errorf("publishing miner admission reservation: %w", err)
	}
	return reservation, path, nil
}

func readMinerAdmissionReservations(townRoot string) ([]minerAdmissionReservation, error) {
	dir := minerAdmissionDir(townRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading miner admission reservations: %w", err)
	}
	reservations := make([]minerAdmissionReservation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			_ = os.Remove(path)
			continue
		}
		var reservation minerAdmissionReservation
		if err := json.Unmarshal(data, &reservation); err != nil {
			_ = os.Remove(path)
			continue
		}
		if reservation.ID == "" || reservation.PID <= 0 || reservation.CreatedAt.IsZero() || reservation.ID+".json" != entry.Name() {
			_ = os.Remove(path)
			continue
		}
		reservations = append(reservations, reservation)
	}
	return reservations, nil
}

func cleanupStaleMinerAdmissionReservations(townRoot string, now time.Time) error {
	dir := minerAdmissionDir(townRoot)
	reservations, err := readMinerAdmissionReservations(townRoot)
	if err != nil {
		return err
	}
	for _, reservation := range reservations {
		if reservation.PID <= 0 {
			continue
		}
		age := now.Sub(reservation.CreatedAt)
		if processAlive(reservation.PID) {
			continue
		}
		if age < minerAdmissionReservationTTL {
			continue
		}
		_ = os.Remove(filepath.Join(dir, reservation.ID+".json"))
	}
	return nil
}

func cleanupStaleMinerAdmissionReservationsWithLock(townRoot string, now time.Time) error {
	lock, err := acquireMinerAdmissionLock(townRoot)
	if err != nil {
		if strings.Contains(err.Error(), "admission is busy") {
			return nil
		}
		return err
	}
	defer func() { _ = lock.Unlock() }()
	return cleanupStaleMinerAdmissionReservations(townRoot, now)
}
