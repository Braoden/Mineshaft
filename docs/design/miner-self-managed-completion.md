# Miner Self-Managed Completion

> **Bead:** gt-0wkk
> **Date:** 2026-02-28
> **Author:** rictus (excavation miner)
> **Status:** Design proposal
> **Related:** gt-4ac (persistent miner model), gt-a6gp (nudge-over-mail),
> gt-6a9d (nuke safety), gt-w0br (bead-based discovery)

---

## 1. Problem Statement

Miners currently depend on the witness to complete their lifecycle. When a
miner runs `gt done`, it performs most of the work (push branch, create MR
bead, write completion metadata, nudge witness) but then **stops and waits for
the witness** to:

1. Discover the completion (via patrol scan of agent beads)
2. Transition the miner from `agent_state=done` to `agent_state=idle`
3. Create a cleanup wisp to track the pending MR
4. Send `MERGE_READY` to the refinery

The witness is single-threaded (one patrol cycle at a time), so at high
throughput it becomes a bottleneck. Zombie miners accumulate in `done` state
waiting for witness processing. This is a regression from the original model
where miners were fully self-contained.

### The Bottleneck in Numbers

With N miners completing simultaneously:
- Each witness patrol cycle takes 30-90 seconds
- `survey-workers` step scans all agent beads sequentially
- Only one completion is processed per cycle (create wisp, nudge refinery)
- N completions queue up, taking N * patrol-cycle-time to process

### How We Got Here

The witness dependency crept in through two well-intentioned changes:

1. **Persistent miner model (gt-4ac):** Preserved sandboxes for reuse,
   requiring someone to manage the idle→reuse lifecycle. The witness became
   that someone because it was already monitoring miners.

2. **Nudge-over-mail (gt-a6gp):** Moved completion discovery from
   miner-sent mail to witness scanning agent beads. This reduced Dolt
   pressure (nudges are free vs mail creating beads) but centralized
   discovery in the witness patrol loop.

Neither change was wrong. But together they created a serial bottleneck where
the witness became a mandatory checkpoint in every completion.

---

## 2. Current Flow (What Happens Today)

```
Miner runs gt done
    │
    ├── 1. Validate clean state (no uncommitted changes)
    ├── 2. Push branch to origin
    ├── 3. Create MR bead (type: merge-request, label: gt:merge-request)
    ├── 4. Write completion metadata to agent bead:
    │      exit_type, mr_id, branch, mr_failed, completion_time
    ├── 5. Set agent_state = "done" (NOT idle)
    ├── 6. Clear hook_bead
    ├── 7. Nudge witness via tmux
    ├── 8. Sync worktree to main, delete old branch
    └── 9. Session goes idle (sandbox preserved)
         │
         ▼
    ┌─── WAIT ──────────────────────────────────────────┐
    │ Miner is in "done" state.                       │
    │ Cannot accept new work until witness processes.    │
    │ If witness is busy: miner sits idle for minutes. │
    └───────────────────────────────────────────────────┘
         │
         ▼ (next witness patrol cycle)
Witness survey-workers step
    │
    ├── Scans all miner agent beads
    ├── Finds exit_type + completion_time set
    ├── If pending MR:
    │   ├── Create cleanup wisp (merge-requested state)
    │   ├── Send MERGE_READY to refinery
    │   └── Clear completion metadata
    ├── Transition agent_state: done → idle
    └── Miner is now available for new work
```

**Time in "done" state:** 30s to several minutes, depending on witness patrol
cycle timing and how many other miners completed simultaneously.

---

## 3. Proposed Flow (Self-Managed Completion)

```
Miner runs gt done
    │
    ├── 1. Validate clean state (no uncommitted changes)
    ├── 2. Push branch to origin
    ├── 3. Create MR bead (type: merge-request, label: gt:merge-request)
    ├── 4. Write completion metadata to agent bead (for audit)
    ├── 5. Nudge refinery directly: "MERGE_READY <mr-id>"     ← NEW
    ├── 6. Set agent_state = "idle"                           ← CHANGED
    ├── 7. Clear hook_bead
    ├── 8. Sync worktree to main, delete old branch
    └── 9. Session goes idle (sandbox preserved)
              │
              └── Miner is IMMEDIATELY available for new work
```

**Key changes:**
1. Miner sets `agent_state=idle` directly (not `done`)
2. Miner nudges refinery directly (not via witness relay)
3. No cleanup wisp needed (see Section 5)
4. Witness is NOT in the critical path

### What the Witness Still Does

The witness role **returns to being an observer** — it patrols for anomalies
and intervenes only when something is wrong:

| Witness Action | When | Why |
|---------------|------|-----|
| Zombie detection | Patrol scan | Session dead but agent_state=running |
| Stuck detection | Patrol scan | Hook set but no progress for 30+ min |
| Dirty state recovery | Patrol scan | Uncommitted changes in idle miner |
| MR failure recovery | Patrol scan | MR bead with error state, no retry |
| Escalation relay | On discovery | Problems beyond miner self-repair |

The witness does NOT need to:
- Process every successful completion
- Relay MERGE_READY to refinery
- Create cleanup wisps for routine completions
- Transition agent_state from done→idle

---

## 4. Detailed Design

### 4.1 Miner Self-Transitions

Currently, agent state transitions are split between miner and witness:

| Transition | Current Owner | Proposed Owner |
|-----------|--------------|---------------|
| → working | Miner (gt sling) | Miner (no change) |
| → done | Miner (gt done) | **REMOVED** (skip to idle) |
| done → idle | Witness (patrol) | Miner (gt done) |
| → stuck | Miner (gt done --status=ESCALATED) | Miner (no change) |
| → running | Witness (restart) | Witness (no change — safety net) |

**Elimination of "done" state:** The intermediate `done` state exists solely as
a handoff signal to the witness. With self-managed completion, miners
transition directly from `working` to `idle`. The completion metadata (exit_type,
mr_id, etc.) remains on the agent bead for audit purposes.

### 4.2 Direct Refinery Notification

Currently, the witness creates a cleanup wisp and nudges refinery when it
discovers a completion. The miner can do this directly:

```go
// In gt done, after creating MR bead:
if mrID != "" {
    // Nudge refinery directly (already implemented, but currently
    // only as fallback alongside witness notification)
    nudgeRefinery(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
}
```

The refinery already discovers MRs by **polling beads** for open merge-request
issues (`ListReadyMRs()`). The nudge is just a wake-up signal — even if it's
missed, the refinery finds the MR on its next patrol cycle. This makes the
notification idempotent and loss-tolerant.

**The refinery does NOT depend on the witness for MR discovery.** From
`engineer.go:1194-1252`, `ListReadyMRs()` queries beads directly:
```go
issues, err := e.beads.List(beads.ListOptions{
    Status:   "open",
    Label:    "gt:merge-request",
    Priority: -1,
})
```

So the witness relay was always redundant — the refinery's own polling is the
true discovery mechanism. The witness nudge just reduces latency.

### 4.3 Cleanup Wisp Elimination

Cleanup wisps (`merge-requested` state) were introduced so the witness could
track pending MRs and detect failures. With self-managed completion, this
tracking is unnecessary because:

1. **MR beads are self-tracking.** The MR bead has status (open/closed),
   retry_count, error state. The refinery updates these as it processes.

2. **Failure detection moves to refinery.** If a merge fails, the refinery
   already creates a conflict-resolution task. The witness doesn't need a
   wisp to discover this.

3. **The witness can still detect anomalies** by scanning for stale MR beads
   (open merge-request older than threshold with no refinery assignee). This
   is discovery-based — no wisp required.

**Migration:** Existing cleanup wisps can be drained naturally. The witness
patrol's `process-cleanups` step becomes a no-op and can be removed after
migration.

### 4.4 Completion Metadata Retention

The agent bead completion metadata (exit_type, mr_id, branch, completion_time)
is still written by the miner. This serves two purposes:

1. **Audit trail:** The ledger shows exactly what each miner did.
2. **Anomaly detection:** The witness can scan for unusual patterns
   (repeated escalations, MR failures, etc.) during patrol.

The metadata is NOT used as a handoff signal anymore. The witness reads it
during patrol for observability, not for action routing.

### 4.5 What Changes in `gt done`

```diff
 func runDone(ctx context.Context, exitType ExitType, ...) error {
     // ... validation, push, MR creation ...

     if mrID != "" {
-        // Nudge witness (witness relays to refinery)
-        nudgeWitness(rigName, fmt.Sprintf("MINER_DONE %s exit=%s", name, exitType))
+        // Nudge refinery directly (witness not in critical path)
+        nudgeRefinery(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
     }

-    // Set agent_state to "done" (witness will transition to idle)
-    setAgentState(agentBeadID, "done")
+    // Set agent_state to "idle" directly (self-managed)
+    setAgentState(agentBeadID, "idle")

     // ... clear hook, sync worktree ...
 }
```

### 4.6 What Changes in Witness Patrol

The `survey-workers` step simplifies:

```diff
 func surveyWorkers() {
     for _, miner := range allMiners {
-        // Check for completions (done state)
-        if miner.AgentState == "done" && miner.CompletionTime != "" {
-            handleDiscoveredCompletion(miner)
-        }

         // Check for zombies (dead session, agent says running)
         if miner.AgentState == "running" && !isSessionAlive(miner) {
             handleZombie(miner)
         }

+        // Check for stuck idle miners (idle but sandbox dirty)
+        if miner.AgentState == "idle" && hasDirtyState(miner) {
+            handleDirtyIdle(miner)
+        }
+
+        // Check for stale MRs (open MR bead with no refinery claim)
+        if miner.MRID != "" && isMRStale(miner.MRID) {
+            handleStaleMR(miner)
+        }
     }
 }
```

The witness patrol gains new anomaly-detection checks but loses the
completion-processing responsibility. Net effect: faster patrol cycles
(no wisp creation, no refinery nudging) with better anomaly coverage.

---

## 5. Edge Cases and Failure Modes

### 5.1 Miner Crashes During `gt done`

**Current:** Witness detects `done-intent` label + live session = stuck-in-done.
Witness kills session and continues cleanup pipeline.

**Proposed:** Same mechanism. The `done-intent` label is set at the start of
`gt done` (before any state changes). If the miner crashes mid-done:
- Agent state is still `working` (not yet transitioned to idle)
- `done-intent` label is set
- Witness zombie detection finds: dead session + done-intent = crashed in done
- Witness restarts session (restart-first policy, gt-dsgp)
- New session discovers done-intent, resumes `gt done`

**No change needed.** The done-intent safety mechanism is independent of who
manages the idle transition.

### 5.2 Miner Sets Idle But Push Failed

**Current:** Not possible — push happens before witness processing.

**Proposed:** Same. The push happens early in `gt done`, before the idle
transition. If push fails, `gt done` errors out and the miner remains in
`working` state. The witness detects this as a zombie (dead session but
agent_state=working) and restarts.

### 5.3 Refinery Misses the Nudge

**Current:** Refinery polls for MRs independently. Nudge is latency optimization.

**Proposed:** Same. Whether the nudge comes from the witness or the miner,
the refinery's polling (`ListReadyMRs`) is the reliable discovery mechanism.
A missed nudge adds at most one patrol cycle of latency.

### 5.4 Two Miners Complete Simultaneously

**Current:** Witness processes them sequentially (serial bottleneck).

**Proposed:** Each miner transitions itself to idle and nudges refinery
independently. No serialization. The refinery processes MRs from its queue
(already serialized by merge slot). This is the primary throughput improvement.

### 5.5 Witness is Down

**Current:** Completions queue up as `done` state miners. When witness
returns, it drains the queue. Miners are unavailable during the outage.

**Proposed:** Miners self-transition to idle and nudge refinery directly.
Witness downtime has **zero impact on routine completions**. The witness is
only needed for anomaly recovery (zombies, dirty state), which can wait.

---

## 6. Migration Strategy

### Phase 1: Dual-Signal (Low Risk)

Add direct refinery nudge to `gt done` alongside existing witness notification.
Miner still sets `agent_state=done` (witness still processes).

```go
// gt done sends BOTH signals
nudgeWitness(rigName, fmt.Sprintf("MINER_DONE %s", name))
nudgeRefinery(rigName, fmt.Sprintf("MERGE_READY %s", mrID))  // NEW
```

**Validation:** Verify refinery processes MRs from both signal sources.
No behavior change — just redundancy.

### Phase 2: Self-Transition (Medium Risk)

Miner sets `agent_state=idle` directly. Witness patrol skips completion
processing (no `done` state to discover). Witness nudge becomes optional.

```go
// gt done: self-manage
setAgentState(agentBeadID, "idle")
nudgeRefinery(rigName, fmt.Sprintf("MERGE_READY %s", mrID))
// Witness nudge: optional, for observability only
```

**Validation:** Verify miners become immediately available for new work.
Verify witness patrol doesn't break when no `done` state miners exist.

### Phase 3: Cleanup (Low Risk)

Remove witness completion-processing code:
- Remove `DiscoverCompletions()` function
- Remove `handleDiscoveredCompletion()` function
- Remove cleanup wisp creation for routine completions
- Remove `process-cleanups` patrol step (or repurpose for anomaly wisps)
- Update `mol-witness-patrol.formula.toml` to remove completion references

**Validation:** Full patrol cycle test. Verify zombie detection still works.

### Rollback

At each phase, rollback is trivial:
- Phase 1: Remove the extra nudge line
- Phase 2: Revert to `agent_state=done` and re-enable witness processing
- Phase 3: Re-add witness completion code

---

## 7. Impact Assessment

### Throughput

| Metric | Current | Proposed |
|--------|---------|----------|
| Completion latency | 30s-3min (witness cycle) | ~0s (immediate) |
| Concurrent completions | Serial (1 per cycle) | Parallel (unlimited) |
| Witness patrol time | 30-90s (processing completions) | 10-30s (anomaly scan only) |
| Miner idle time | Minutes waiting | Zero waiting |

### Dolt Pressure

No change — both flows use nudges (free) and direct bead writes.

### Robustness

**Improved:** Removes single point of failure (witness) from the critical path.
Routine completions succeed even if witness is down, restarting, or slow.

**Preserved:** Witness still provides safety net for edge cases (zombies,
dirty state, stale MRs). The "discover, don't track" principle is maintained.

### Complexity

**Reduced:** Eliminates cleanup wisps, completion discovery code, and the
done→idle state machine in the witness. The `gt done` command becomes the
single source of truth for completion lifecycle.

---

## 8. Alignment with Design Principles

| Principle | How This Design Aligns |
|-----------|----------------------|
| **GUPP** | Miners become available for new work faster → higher throughput |
| **ZFC** | Miner self-reports idle (already does cleanup_status). Witness verifies by exception |
| **Discover Don't Track** | Witness discovers anomalies by scanning state, not by processing events |
| **Self-recycling preferred** | From miner-lifecycle-patrol.md Q2: "Prefer explicit self-recycling. Use mechanical intervention only as a safety net." This design delivers on that stated preference |
| **Persistent miner model** | Fully compatible — sandbox preservation and identity persistence are unchanged |

### The Missed Implication of gt-4ac

The persistent miner model (gt-4ac) was designed so miners survive and
get reused. But the witness was inserted as a gatekeeper for the idle
transition, defeating part of the benefit. A miner that completes work
but can't accept new work for 3 minutes because the witness hasn't processed
it is effectively dead capacity.

This design completes the promise of gt-4ac: persistent miners that
self-manage their full lifecycle, with the witness as a safety net rather
than a required checkpoint.

---

## 9. Summary

**The core insight:** The witness relay for routine completions is redundant.
The refinery already discovers MRs by polling beads. The miner already
writes all the metadata. The witness is only needed for anomaly detection —
and it can do that by scanning state, not by processing every completion.

**Three changes:**
1. Miner sets `agent_state=idle` directly (skip the `done` intermediate)
2. Miner nudges refinery directly (skip the witness relay)
3. Witness removes completion-processing code (patrol focuses on anomalies)

**Result:** Completion latency drops from minutes to zero. The witness returns
to its designed role as an observer. The system scales linearly with miner
count instead of being bottlenecked by a single-threaded patrol loop.

---

*"Self-recycling is preferred. Mechanical intervention is the safety net,
not the primary mechanism." — miner-lifecycle-patrol.md, Q2*
