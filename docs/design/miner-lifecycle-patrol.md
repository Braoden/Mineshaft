# Miner Lifecycle and Patrol Coordination

> **Bead:** ms-t6muy
> **Date:** 2026-02-20
> **Author:** capable (mineshaft miner)
> **Status:** Implemented — core lifecycle shipped, branch cleanup shipped, overseer notify pending
> **Updated:** 2026-03-07 (ms-o8g8 implementation audit by bear)
> **Related:** ms-dtw9u (Witness monitoring), ms-qpwv4 (Completion detection),
> ms-6qyt1 (Refinery queue), ms-budeb (Auto-nuke), ms-5j3ia (Swarm aggregation),
> ms-1dbcp (Miner auto-start), w-ms-004 (Wasteland lifecycle item)

---

## 1. Overview

This document formalizes how Supervisor, Witness, Refinery, and Miners coordinate
to move work through the Mineshaft propulsion system. It captures the
session-per-step model, defines the two cleanup stages, designs the per-rig
lifecycle channel, and resolves open design questions about step granularity,
recycling, and spawning.

**Core insight:** Miners do NOT complete complex molecules end-to-end. Instead,
each molecule step gets one miner session. The sandbox (branch, worktree)
persists across sessions. Sessions are the pistons; sandboxes are the cylinders.

---

## 2. Session-Per-Step Model

### 2.1 The Relay Race

See [concepts/miner-lifecycle.md](../concepts/miner-lifecycle.md) for the relay race model.

### 2.2 Session Cycling vs Step Cycling

These are distinct concepts:

| Concept | Trigger | What Changes | What Persists |
|---------|---------|-------------|---------------|
| **Session cycle** | Handoff, compaction, crash | Claude context window | Branch, worktree, molecule state |
| **Step cycle** | Step bead closed | Current step focus | Branch, worktree, remaining steps |

A single step may span multiple session cycles (if the step is complex or
compaction occurs). Multiple steps may fit in a single session (if steps are
small and context permits). The session-per-step model is a design target, not a
hard constraint.

### 2.3 When Sessions Cycle

| Trigger | Who Initiates | What Happens |
|---------|--------------|-------------|
| Step completion | Miner | `bd close <step>` then `ms handoff` for next step |
| Context filling | Claude Code | Auto-compaction; PreCompact hook saves state |
| Crash/timeout | Infrastructure | Witness detects, respawns session |
| `ms done` | Miner | Final step; submit to MQ, go idle (sandbox preserved) |

### 2.4 State Continuity

Between sessions, state is preserved through:

- **Git state:** Commits, staged changes, branch position
- **Beads state:** Molecule progress (which steps are closed)
- **Hook state:** `hook_bead` on agent bead persists across sessions
- **Agent bead:** `agent_state`, `cleanup_status`, `hook_bead` fields

The new session discovers its position via:

```bash
ms prime --hook    # Loads role context, reads hook
bd mol current     # Discovers which step is next
bd show <step-id>  # Reads step instructions
```

No explicit "handoff payload" is needed. The beads state IS the handoff.

---

## 3. Two Cleanup Stages

### 3.1 Step Cleanup (Session Dies, Sandbox Lives)

Triggered when a step completes but more steps remain in the molecule.

| Action | Result |
|--------|--------|
| Close step bead | `bd close <step-id>` |
| Session cycles | `ms handoff` (voluntary) or crash recovery |
| Sandbox persists | Branch, worktree, uncommitted work all survive |
| Molecule persists | Remaining steps still open, hook still set |
| Identity persists | Agent bead unchanged, CV accumulates |

**Who handles it:**
- Miner initiates via `ms handoff`
- Witness respawns if crash (via `SessionManager.Start`)
- Daemon triggers if session is dead (`LIFECYCLE:Shutdown` → witness)

### 3.2 Molecule Cleanup (Miner Goes Idle)

Triggered when the molecule's final step completes and work is submitted.

| Action | Result |
|--------|--------|
| Miner runs `ms done` | Pushes branch, submits MR, sets `cleanup_status=clean` |
| Miner sets agent state | `agent_state=idle`, `hook_bead` cleared |
| Miner kills session | Session terminated, sandbox preserved |
| Witness receives `MINER_DONE` | Acknowledges idle transition |
| Refinery merges | Squash-merge to main, closes MR and source issue |
| Identity survives | Agent bead still exists; CV chain has new entry; miner ready for reuse |

```
STEP CLEANUP (intermediate)          MOLECULE CLEANUP (final)
┌────────────────────┐               ┌────────────────────────────┐
│ Step bead: closed  │               │ All step beads: closed     │
│ Session: terminated│               │ Session: terminated        │
│ Sandbox: ALIVE     │               │ Sandbox: PRESERVED (idle)  │
│ Molecule: ACTIVE   │               │ Molecule: SQUASHED         │
│ Hook: SET          │               │ Hook: CLEARED              │
│ Agent bead: working│               │ Agent bead: nuked          │
│ Branch: ALIVE      │               │ Branch: PUSHED (idle)      │
└────────────────────┘               └────────────────────────────┘
```

### 3.3 The Cleanup Pipeline

The cleanup pipeline is a chain of handoffs, not a monolithic operation:

```
Miner calls ms done
    │
    ├── Sets cleanup_status=clean on agent bead
    ├── Pushes branch to origin
    ├── Creates MR bead (label: ms:merge-request)
    ├── Sends MINER_DONE mail to witness
    └── Session exits
         │
         ▼
Witness receives MINER_DONE
    │
    ├── Checks cleanup_status (ZFC: trust miner self-report)
    ├── If clean → sends MERGE_READY to refinery
    ├── If dirty → creates cleanup wisp (cannot auto-nuke)
    └── Nudges refinery session
         │
         ▼
Refinery processes MERGE_READY
    │
    ├── Claims MR (sets assignee)
    ├── Acquires merge slot (serialized push lock)
    ├── Runs quality gates
    ├── Squash-merges to main
    ├── Closes MR bead and source issue
    ├── Sends MERGED mail to witness
    └── Releases merge slot
         │
         ▼
Witness receives MERGED
    │
    ├── Verifies commit is on main (all remotes)
    ├── Checks cleanup_status
    ├── Acknowledges merge (miner already idle, sandbox preserved)
    └── If dirty → warns (shouldn't happen post-merge)
```

### 3.4 Failure Recovery in the Cleanup Pipeline

Each stage can fail independently. Recovery is handled by the next patrol cycle:

| Failure | Detection | Recovery |
|---------|-----------|---------|
| `ms done` fails mid-execution | Zombie state: session alive, done-intent label | Witness `DetectZombieMiners()` finds stuck-in-done, recovers |
| `MINER_DONE` mail lost | Witness patrol: finds dead session with `hook_bead` | `DetectZombieMiners()` with agent-dead-in-session |
| Merge conflict | Refinery `doMerge()` detects | Creates conflict resolution task, blocks MR |
| `MERGED` mail lost | Refinery closed the bead; witness patrol finds closed bead with live session | `DetectZombieMiners()` bead-closed-still-running |
| Nuke fails | Session still running after kill attempt | Next patrol detects zombie, retries nuke |

---

## 4. Per-Rig Miner Channel

### 4.1 Design Decision: Mail-Based Channel

The per-rig miner channel is implemented using the existing `ms mail` system.
This was chosen over beads-based queues or state files because:

1. **Consistency:** Mail is already the coordination primitive for all Mineshaft agents
2. **Persistence:** Messages survive process crashes and session cycles
3. **Routing:** Mail addresses (`mineshaft/witness`) already map to rig-level agents
4. **Audit trail:** Mail creates beads entries (observable, discoverable)
5. **No new infrastructure:** No new Dolt tables, no file-based queues

### 4.2 Channel Addresses

Each rig has implicit lifecycle channels via existing mail routing:

| Channel | Address | Purpose | Serviced By |
|---------|---------|---------|-------------|
| Miner lifecycle | `<rig>/witness` | Recycle, nuke, health requests | Witness patrol |
| Merge queue | `<rig>/refinery` | MERGE_READY, conflict reports | Refinery patrol |
| Rig coordination | `<rig>/witness` | Spawn requests, escalations | Witness |
| Town coordination | `overseer/` | Cross-rig, strategic | Overseer |

### 4.3 Lifecycle Message Protocol

Messages in the miner lifecycle channel follow the existing witness protocol
(`protocol.go`):

| Subject Pattern | Type | Sender | Action |
|----------------|------|--------|--------|
| `MINER_DONE <name>` | Completion | Miner | Verify clean, forward to refinery |
| `LIFECYCLE:Shutdown <name>` | External shutdown | Daemon | Auto-nuke or cleanup wisp |
| `LIFECYCLE:Cycle <name>` | Session restart | Daemon | Kill and restart session |
| `HELP: <topic>` | Escalation | Miner | Witness evaluates, relays if needed |
| `MERGED <id>` | Post-merge | Refinery | Nuke miner sandbox |
| `MERGE_FAILED <id>` | Merge failure | Refinery | Notify miner, rework needed |
| `RECOVERED_BEAD <id>` | Orphan recovery | Witness | Supervisor re-dispatches work |
| `GUPP_VIOLATION: <name>` | Stall detected | Daemon | Witness investigates |
| `ORPHANED_WORK: <name>` | Dead session + work | Daemon | Witness recovers or nukes |

### 4.4 Channel Processing

The witness processes its channel during patrol cycles. Processing is
first-come-first-served within each cycle. The patrol pattern:

```
Witness patrol cycle:
    │
    ├── 1. Check inbox (ms mail inbox)
    │   └── Process lifecycle messages in order
    │
    ├── 2. Detect zombie miners
    │   └── For each zombie: nuke or escalate
    │
    ├── 3. Detect orphaned beads
    │   └── For each orphan: reset status, mail supervisor
    │
    ├── 4. Detect stalled miners
    │   └── For each stalled: nudge or escalate
    │
    ├── 5. Check for pending spawns
    │   └── Process spawn requests from daemon
    │
    └── 6. Write patrol receipt
        └── Machine-readable summary of findings
```

### 4.5 Who Services the Channel

The witness is the primary consumer, but the design supports opportunistic
servicing by other patrol agents:

| Agent | When It Services | What It Can Do |
|-------|-----------------|---------------|
| **Witness** | Every patrol cycle | Full lifecycle: spawn, nuke, escalate |
| **Supervisor** | During rig-wide patrol | Detect unserviced requests, nudge witness |
| **Daemon** | Every heartbeat tick | Detect dead sessions, send LIFECYCLE messages |
| **Refinery** | During merge processing | Send MERGED/MERGE_FAILED to witness |

This creates redundant monitoring: if the witness misses a message, the supervisor or
daemon detects the resulting state (dead session, orphaned bead) and either
handles it directly or nudges the witness.

---

## 5. GUPP + Pinned Work = Completion Guarantee

### 5.1 The Completion Invariant

As long as three conditions hold, a molecule WILL eventually complete:

1. **Work is pinned** (`hook_bead` set on agent bead)
2. **Sandbox persists** (branch + worktree exist)
3. **Someone keeps spawning sessions** (witness respawn on crash)

GUPP ensures that when a session starts with a hook, it executes. The hook
persists across session cycles. The sandbox provides continuity. The witness
provides resurrection. Together, these guarantee eventual completion.

### 5.2 The Completion Loop

```
┌─────────────────────────────────────────────┐
│              COMPLETION LOOP                 │
│                                              │
│   Session spawns → ms prime → discovers hook │
│        │                                     │
│        ▼                                     │
│   GUPP fires → execute current step          │
│        │                                     │
│        ▼                                     │
│   Step complete → bd close → handoff         │
│        │                                     │
│        ▼                                     │
│   More steps? ──yes──▶ Respawn session ──┐   │
│        │                                 │   │
│        no                                │   │
│        │                                 │   │
│        ▼                                 │   │
│   ms done → merge → nuke                 │   │
│                                          │   │
│   Session crashes? ──▶ Witness respawns ─┘   │
│                                              │
└─────────────────────────────────────────────┘
```

### 5.3 What Breaks the Guarantee

| Failure | Effect | Recovery |
|---------|--------|---------|
| Witness down | No respawn on crash | Supervisor detects, restarts witness |
| Sandbox corrupted | Branch or worktree broken | `RepairWorktree()` or nuke and respawn |
| Hook cleared accidentally | GUPP doesn't fire | Witness `DetectOrphanedBeads()` finds in-progress bead, resets for re-dispatch |
| Dolt server down | Cannot read beads state | Daemon auto-restarts Dolt; miner retries |
| Crash loop (3+ crashes) | Same step keeps failing | Witness escalates to overseer; filed as bug |

### 5.4 Liveness vs Safety

The system prioritizes **liveness** (work eventually completes) over strict safety
(no duplicate work). This means:

- **Duplicate detection is best-effort.** If two sessions somehow run the same
  step, the git branch serializes writes and one will fail to push.
- **Idempotent operations are preferred.** Closing an already-closed bead is a
  no-op. Pushing an already-pushed branch is safe.
- **Crash recovery may re-execute partial work.** A step that crashed mid-way
  will be re-executed from the start. Git state helps: if commits were made,
  the new session sees them.

---

## 6. Patrol Coordination

### 6.1 The Four Patrol Agents

Mineshaft has four agents that perform patrol (periodic health monitoring):

| Agent | Scope | Frequency | Key Checks |
|-------|-------|-----------|-----------|
| **Daemon** | Town-wide | 3-minute heartbeat | Session liveness, GUPP violations, orphaned work |
| **Boot/Supervisor** | Town-wide | Per daemon tick | Supervisor health, witness health, cross-rig issues |
| **Witness** | Per-rig | Continuous | Miner health, zombie detection, completion handling |
| **Refinery** | Per-rig | On demand | Merge queue processing, conflict detection |

### 6.2 Patrol Overlap as Resilience

Multiple agents observing overlapping state is intentional redundancy:

```
               Daemon                          Supervisor
           (mechanical)                    (intelligent)
                │                               │
    ┌───────────┼───────────┐       ┌──────────┼──────────┐
    │           │           │       │          │          │
 Session    GUPP         Orphan   Witness   Refinery    Cross-rig
 liveness   violations   work    health    health      minecart
    │           │           │       │          │
    └───────────┤           │       │          │
                │           │       │          │
                ▼           ▼       ▼          ▼
              Witness               Witness    Refinery
           (per-rig patrol)      (responds)   (responds)
                │
    ┌───────────┼───────────┐
    │           │           │
 Zombie      Orphaned     Stalled
 detection   beads        miners
```

**Key property:** If any single patrol agent fails, the others detect the
resulting state degradation and compensate. The daemon detects dead sessions.
The supervisor detects dead witnesses. The witness detects dead miners.

### 6.3 Information Flow Between Patrol Agents

```
Daemon ───LIFECYCLE:──────▶ Witness inbox
Daemon ───GUPP_VIOLATION:─▶ Witness inbox
Daemon ───ORPHANED_WORK:──▶ Witness inbox

Supervisor ◀──heartbeat.json──── Daemon
Supervisor ───nudge────────────▶ Witness (if stale)
Supervisor ───nudge────────────▶ Refinery (if stale)

Witness ──MERGE_READY:────▶ Refinery inbox
Witness ──RECOVERED_BEAD:─▶ Supervisor (for re-dispatch)
Witness ──patrol receipt───▶ Beads (audit trail)

Refinery ─MERGED:─────────▶ Witness inbox
Refinery ─MERGE_FAILED:───▶ Witness inbox
Refinery ─minecart check─────▶ Supervisor (for stranded minecarts)
```

### 6.4 Convergent State

All patrol agents converge on the same observable state: beads (via Dolt), git
(via branches and worktrees), and tmux (via session liveness). No agent maintains
private state that others depend on. This is the "discover, don't track" principle
applied to monitoring.

If state diverges (e.g., a message is lost), the next patrol cycle re-derives
state from observables and self-heals.

---

## 7. Resolved Design Questions

### Q1: Spoon-Feeding and Step Granularity

**Question:** How many logical steps per physical molecule step? How many steps
per miner session?

**Answer:** Use formulas to define granularity, and let context pressure determine
session boundaries.

**Step granularity guidelines:**

| Step Type | Granularity | Example |
|-----------|-------------|---------|
| Setup / teardown | One physical step | "Set up working branch" |
| Implementation | One per logical unit | "Implement the solution" (may span sessions) |
| Verification | One per check type | "Run quality checks", "Self-review" |
| Handoff | One per lifecycle event | "Commit changes", "Submit work" |

The `mol-miner-work` formula currently uses 10 steps. This is appropriate for
most work because:

- Each step has clear entry/exit criteria
- Steps are independently resumable (a crash mid-step loses at most one step's work)
- Context stays focused (one step's instructions, not the whole molecule)

**Session-per-step is a guideline, not a rule.** A miner may complete multiple
steps in one session if context permits. The key constraint is that each step
is closed individually (no batch-closing — the Batch-Closure Heresy).

**Anti-patterns:**
- Steps so small they're just `git add` commands (overhead exceeds value)
- Steps so large they exhaust context (implementation + testing + review in one step)
- Steps that can't be independently resumed (step 3 requires step 2's context window)

### Q2: Mechanical vs Agent-Driven Recycling

**Question:** When is mechanical intervention (daemon-driven) appropriate vs
agent-driven (miner requests its own recycle)?

**Answer:** Prefer explicit self-recycling. Use mechanical intervention only as a
safety net.

**The spectrum:**

```
AGENT-DRIVEN (preferred)              MECHANICAL (safety net)
├── ms done (miner goes idle)       ├── Daemon detects dead session
├── ms handoff (miner self-cycles)  ├── Daemon detects GUPP violation
├── ms escalate (miner asks help)   ├── Witness zombie sweep
└── HELP mail (miner signals)       └── Supervisor restart on stale heartbeat
```

**Design principle:** The miner is the authority on its own state. External
intervention should only occur when the miner cannot speak for itself (dead
session, hung process, stuck-in-done).

**Concrete thresholds (agent-determined, not hardcoded):**

The daemon uses broad thresholds for safety-net detection:
- **GUPP violation:** 30 minutes with `hook_bead` but no progress
- **Hung session:** 30 minutes of no tmux output (`HungSessionThresholdMinutes`)
- **Stuck-in-done:** 60 seconds with `done-intent` label

These thresholds are intentionally generous. The goal is to catch truly stuck
miners, not miners that are thinking hard. False positives (the "Supervisor
murder spree" bug) are worse than slow detection.

**The murder spree lesson:** Mechanical detection of "stuck" is fragile because
distinguishing "thinking deeply" from "hung" requires intelligence. This is why
Boot exists (intelligent triage) and why the daemon's thresholds are conservative.
Only the witness (an AI agent) should make judgment calls about whether a miner
is truly stuck.

### Q3: Channel Implementation

**Question:** Mail-based, beads-based, or state file?

**Answer:** Mail-based. See [Section 4](#4-per-rig-miner-channel) for full design.

**Why not beads-based (special issue type)?**
- Beads issues are durable work artifacts. Lifecycle requests are transient signals.
- Creating/closing beads for "recycle me" adds unnecessary Dolt write pressure.
- Mail is already the coordination primitive and has the right lifecycle (read → process → delete).

**Why not state files (rig/miner-queue.json)?**
- State files require explicit locking for concurrent access.
- No audit trail (file gets overwritten).
- Doesn't integrate with existing patrol patterns (agents already check mail).
- Recovery after crash is harder (partially-written JSON).

### Q4: Who Spawns the Next Step?

**Question:** After a miner completes a step and hands off, who spawns the
next session to continue the molecule?

**Answer:** The witness, triggered by either handoff detection or daemon lifecycle
request.

**The spawn chain:**

```
Miner completes step
    │
    ├── Closes step bead
    ├── Calls ms handoff (creates handoff mail)
    └── Session exits
         │
         ▼
Daemon heartbeat tick
    │
    ├── Detects dead miner session
    ├── Finds hook_bead still set (work isn't done)
    └── Triggers session restart
         │
         ▼
SessionManager.Start()
    │
    ├── Creates new tmux session in existing worktree
    ├── Injects env vars (MS_MINER, MS_RIG)
    ├── SessionStart hook fires: ms prime --hook
    └── New session discovers next step via bd mol current
```

**Current implementation:** The daemon's `processLifecycleRequests()` handles
this. When a session dies but the hook is still set, the daemon either sends a
`LIFECYCLE:` message to the witness or directly restarts the session (depending
on configuration). Miner startup is handled end-to-end by the GUPP/beacon
flow (SessionManager → StartupNudge → BuildStartupPrompt → SessionStart hook
→ ms prime).

**Future (AT integration):** The witness spawns replacement teammates directly
via `Teammate({ operation: "spawn" })`. The SubagentStop hook detects teammate
death and triggers respawn. See `docs/design/witness-at-team-lead.md` for details.

---

## 8. Edge Cases and Failure Modes

### 8.1 The Stuck-in-Done Zombie

A miner runs `ms done` but the session hangs before cleanup completes.

**Detection:** Witness `DetectZombieMiners()` checks for `done-intent` label
older than 60 seconds with a live session.

**Recovery:** Witness kills the session and continues the cleanup pipeline
(verify `cleanup_status`, forward to refinery if MR exists).

### 8.2 The Orphaned Sandbox

A miner directory exists but no tmux session and no `hook_bead`.

**Detection:** `Manager.ReconcilePool()` finds directories without sessions.
`DetectStaleMiners()` identifies sandboxes far behind main with no work.

**Recovery:** If no uncommitted work and no active MR, nuke the sandbox. If
uncommitted work exists, escalate (someone needs to decide if the work matters).

### 8.3 The Split-Brain Merge

The refinery starts merging while the miner is still pushing.

**Prevention:** The `cleanup_status=clean` field on the agent bead serializes
this. The witness only sends `MERGE_READY` after verifying the miner has
exited and the branch is clean. The merge slot provides additional serialization.

### 8.4 The Infinite Cycle

A step keeps failing and the session keeps restarting.

**Detection:** Track crash count per miner (via `ReconcilePool` or
ephemeral state). Three crashes on the same step triggers escalation.

**Recovery:** Witness stops respawning, creates a bug bead, mails the overseer.
The molecule stays in its current state (recoverable when the bug is fixed).

### 8.5 Concurrent Miners on Same Issue

Should not happen because the hook is exclusive (one `hook_bead` per agent bead,
one agent bead per miner name). But if it does:

**Prevention:** Git branch naming includes a unique suffix (`@<timestamp>`).
The TOCTOU guard in `DetectZombieMiners()` (records `detectedAt`, re-verifies
before destructive action) prevents racing between detection and action.

**Recovery:** The second session fails to push (branch diverged) and escalates.

---

## 9. Future: AT Integration Impact

The Agent Teams (AT) integration (see `docs/design/witness-at-team-lead.md`)
changes the transport layer but preserves the lifecycle model:

| Aspect | Current (tmux) | Future (AT) |
|--------|---------------|-------------|
| Session management | tmux sessions | AT teammates |
| Spawning | `SessionManager.Start()` | `Teammate({ operation: "spawn" })` |
| Health monitoring | tmux liveness + pane output | AT lifecycle hooks (SubagentStop) |
| Messaging | `ms nudge` (tmux send-keys) | AT messaging |
| Cleanup | Session kill (sandbox preserved) | `Teammate({ operation: "requestShutdown" })` (sandbox preserved) |

**What stays the same:**
- Beads as the durable ledger
- Molecules as workflow templates
- `ms done` as the miner idle signal
- Two-stage cleanup (step vs molecule)
- Mail for cross-rig communication
- The completion guarantee (GUPP + pinned work + respawn)

**What changes:**
- The witness becomes an AT team lead (delegate mode)
- Zombie detection becomes structural (hooks vs polling)
- Miner-to-miner isolation is hook-enforced, not tmux-enforced
- Real-time coordination moves from tmux to AT (ephemeral), reducing Dolt pressure

---

## 10. Implementation Status (ms-o8g8 audit, 2026-03-07)

### Shipped

All core lifecycle operations are implemented and running in production:

| Operation | Command/Component | Key Implementation |
|-----------|------------------|-------------------|
| Spawn/assign | `ms sling` | `sling.go`, `miner_spawn.go` — finds idle miner or allocates new slot |
| Work execution | `ms prime --hook` | Session discovers hook via `bd mol current`, GUPP fires |
| Session cycling | `ms handoff` | `handoff.go` — all roles, preserves sandbox and identity |
| Step completion | `bd close` + `ms handoff` | Step cleanup: session dies, sandbox lives |
| Work submission | `ms done` | `done.go` — push, MR, sandbox sync, set idle |
| Idle miner reuse | `ms sling` | `miner/manager.go`: `FindIdleMiner()` + `ReuseIdleMiner()` — branch-only repair |
| Zombie detection | Witness patrol | `witness/handlers.go`: `DetectZombieMiners()` — restart-first, no auto-nuke |
| Stale detection | Witness patrol | `miner/manager.go`: `DetectStaleMiners()` — tmux-based, protects paused states |
| Orphan recovery | Witness patrol | `witness/handlers.go`: `DetectOrphanedBeads()` — reset and re-dispatch |
| Cleanup pipeline | Mail-based | MINER_DONE → Witness → MERGE_READY → Refinery → MERGED |
| Merge queue | Refinery | Squash-merge, close MR and issue, minecart check |

### Pending

| Feature | Description | Impact |
|---------|-------------|--------|
| Refinery notifies overseer after merge | PRs #2436/#2437 closed; branch cleanup shipped, overseer notify not yet | Unblocks dependent work dispatch |

### Deferred (design only)

| Feature | Rationale for deferral |
|---------|----------------------|
| Pool size enforcement | On-demand allocation works; fixed pool is optimization, not correctness |
| `ms miner pool init` | Miners created naturally by first `ms sling`; pre-allocation unnecessary |
| `ReconcilePool()` | Witness patrol already detects state drift via zombie/stale/orphan checks |

---

## 11. Summary

See [concepts/miner-lifecycle.md](../concepts/miner-lifecycle.md) for the
complete lifecycle model (three layers, four states, persistent miner design).
This document covers the implementation details: cleanup stages, mail channels,
patrol coordination, and edge case handling.
