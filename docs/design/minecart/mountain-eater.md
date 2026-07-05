# The Mountain-Eater: Autonomous Epic Grinding

> Judgment layer for minecart-driven epic execution.
>
> **Status**: Design
> **Depends on**: Minecart Milestones 0-2 (MinecartManager, stage-launch)
> **Related**: [roadmap.md](roadmap.md) | [spec.md](spec.md) | [swarm-architecture.md](../../../docs/swarm-architecture.md)

---

## 1. Problem Statement

Mineshaft has all the pieces for autonomous epic execution:
- MinecartManager feeds ready issues as blocking deps close (event-driven, 5s)
- Stranded scan catches missed feeding (periodic, 30s)
- Stage-launch validates DAGs and computes waves (Kahn's algorithm)
- Miners execute individual issues
- Witnesses monitor miners, Refineries merge

Yet users report that large epics "get stuck." They create a mountain of
beads, launch a minecart, go away for a few hours, and come back to find the
minecart stalled at 40% with no indication of why.

**Root cause**: The MinecartManager is mechanical. It feeds the next ready
issue when one closes, but it cannot reason about failure patterns, make
skip decisions, or escalate intelligently. When a miner fails repeatedly
on the same issue, the mechanical system re-slings it endlessly. When a
subtle blocking condition exists outside the dep graph, nothing notices.

The Mountain-Eater adds a judgment layer — agent-driven stall detection,
skip-after-N-failures, intelligent escalation, and completion notification
— on top of the existing mechanical feeding.

---

## 2. Design Principle: No Agent Holds the Thread

The reason single-coordinator approaches fail is **hysteresis**. Any agent
maintaining an "I'm driving this epic" loop will lose that thread at
compaction. Even with the epic hooked, the re-primed agent doesn't remember
the coordination context.

The Mountain-Eater sidesteps this entirely:

- **The epic IS the thread.** The beads ARE the state.
- **No agent needs to remember anything.** Each check discovers state fresh.
- **Dogs bring fresh context every time.** Zero hysteresis by construction.
- **The label triggers patrol behavior.** No persistent coordinator needed.

This aligns with core Mineshaft principles:
- **ZFC**: Agents decide, Go transports. MinecartManager is transport; Dogs make judgment calls.
- **NDI**: Any Dog can check any mountain. Different agents, same outcome.
- **Discover, Don't Track**: `bd ready --epic=X` and minecart status derive state.
- **Float over Integer**: A stuck issue doesn't halt the mountain — work flows around it.

---

## 3. Architecture: Four-Layer Grinding

```
Layer 0: MINECART MANAGER (mechanical, Go daemon — already built)
    Event-driven feeding + stranded scan
    Handles the happy path: issue closes → feed next ready

Layer 1: WITNESS (reactive, per-rig — enhancement)
    Miner failure tracking for mountain minecart issues
    Same issue failed 3+ times → mark blocked, skip, feed next

Layer 2: SUPERVISOR DOG (periodic, cross-rig — new)
    "Has this mountain progressed since last check?"
    Fresh Dog investigates stalls with full context
    Makes judgment calls: skip, restructure, escalate
    Notifies Overseer on stalls and completion

Layer 3: OVERSEER (strategic, user-facing — enhancement)
    Receives stall escalations from Layer 2
    Cross-rig judgment calls
    Notifies user on completion or unrecoverable stalls
```

**Layer 0** already exists and handles ~80% of minecart execution.
**Layers 1-2** are the Mountain-Eater — they handle the 20% that gets stuck.
**Layer 3** is the escalation path for the ~2% that requires human judgment.

### Why Four Layers?

Redundant monitoring is resilience. If the Witness misses a completion
(crash, compaction), the MinecartManager catches it (5s event poll). If the
MinecartManager feeds a bad issue repeatedly, the Witness catches the failure
pattern. If both miss a stall, the Supervisor Dog catches it on the next patrol
cycle. Each layer operates independently and discovers state from beads.

---

## 4. The `mountain` Label

A mountain is a minecart with the `mountain` label. No new entity types,
no new database schema. The label IS the opt-in for Layers 1-2.

```bash
# Activate the Mountain-Eater on an epic
gt mountain <epic-id>

# Internally:
#   1. gt minecart stage <epic-id>          ← validate DAG, compute waves
#   2. bd update <minecart> --add-label mountain  ← trigger judgment layers
#   3. gt minecart launch <minecart-id>       ← dispatch wave 1, MinecartManager takes over

# Check progress
gt mountain status [epic-id|minecart-id]

# Pause/resume (keeps label, stops/starts dispatch)
gt mountain pause <epic-id|minecart-id>
gt mountain resume <epic-id|minecart-id>

# Cancel (removes label, leaves minecart for manual management)
gt mountain cancel <epic-id|minecart-id>
```

Regular minecarts (no `mountain` label) continue working exactly as today.
The `mountain` label opts a minecart into enhanced stall detection,
skip-after-N-failures, and active progress monitoring.

### When to Use Mountains vs Regular Minecarts

| Scenario | Use |
|----------|-----|
| Batch sling of 3-5 tasks | Regular minecart (MinecartManager is sufficient) |
| Large epic with 10+ tasks and DAG deps | Mountain |
| Cross-rig epic | Mountain (needs the Dog's cross-rig visibility) |
| "Go to lunch and come back to it done" | Mountain |
| Quick parallel tasks, no deps | Regular minecart |

---

## 5. Layer 1: Witness Failure Tracking

### Problem

When a miner fails on a mountain issue, the MinecartManager's stranded
scan re-slings it. If the issue has a fundamental problem (bad description,
impossible task, missing context), this creates an infinite sling-fail loop.

### Enhancement

The Witness already monitors miner completions. Add failure tracking
for issues belonging to mountain minecarts:

```
WITNESS PATROL — mountain failure tracking:

For each miner that exited without completing its issue:
  issue = miner's hooked bead
  minecart = tracking minecart for this issue (if any)
  if minecart has "mountain" label:
    increment failure count for this issue (stored as issue note or label)
    if failure_count >= 3:
      bd update <issue> --status=blocked --add-label mountain:skipped
      bd update <issue> --notes "Skipped by Mountain-Eater after 3 miner failures"
      log: "Mountain: skipped <issue> after 3 failures"
      # MinecartManager's next feed will skip this issue (blocked status)
      # and feed the next ready issue instead
```

**Failure count storage**: Use a label like `mountain:failures:3` on the
issue. Labels are cheap, queryable, and visible in `bd show`. No new
schema needed.

**Why the Witness and not the MinecartManager?** The Witness already observes
miner lifecycle. It knows whether a miner completed successfully or
crashed. The MinecartManager only sees issue status changes — it can't
distinguish "miner failed" from "miner is still working."

### Skip Semantics

A skipped issue (`mountain:skipped` label, `blocked` status) is:
- Excluded from the ready front (blocked status)
- Visible in `gt mountain status` output
- Escalated to Overseer by Layer 2 (Supervisor Dog)
- Recoverable: `bd update <issue> --status=open --remove-label mountain:skipped`

The mountain continues grinding around the skipped issue. If the skipped
issue was blocking other work in the DAG, those dependents remain blocked.
The Dog reports this in its stall diagnosis.

---

## 6. Layer 2: Supervisor Dog Mountain Audit

### The Core Loop

The Supervisor's patrol formula gains a `mountain-audit` step:

```
SUPERVISOR PATROL — mountain-audit step:

mountains = bd list --label mountain --status=open --type=minecart
for each mountain:
  dog_needed = false

  # Progress check (compare against last audit)
  current_closed = count of closed issues in this minecart
  last_closed = read from mountain:audit:<minecart-id> label on supervisor bead

  if current_closed > last_closed:
    # Making progress — update audit mark, continue
    update mountain:audit:<minecart-id> = current_closed

  else if current_closed == total_issues:
    # Complete — dispatch Dog for cleanup + notification
    dog_needed = true
    dog_task = "complete"

  else:
    # No progress since last check — dispatch Dog to investigate
    dog_needed = true
    dog_task = "stall"

  if dog_needed:
    sling mountain-dog formula to a Dog with minecart-id and task type
```

### The Mountain Dog Formula

`mol-mountain-dog.formula.toml` — a short-lived Dog formula for
investigating mountain progress:

```toml
[formula]
name = "mountain-dog"
description = "Investigate mountain minecart progress"
type = "worker"

[formula.variables]
minecart_id = { required = true }
task = { required = true }  # "stall" or "complete"

[[formula.steps]]
name = "investigate"
description = """
You are a Mountain Dog investigating a mountain minecart.

Minecart: {{minecart_id}}
Task: {{task}}

If task is "stall":
  1. Run: gt minecart status {{minecart_id}}
  2. Identify why no progress:
     - Are there skipped issues (mountain:skipped label)?
     - Are all remaining issues blocked? By what?
     - Are miners active but slow?
     - Is the refinery backed up?
  3. If there are ready issues with no miners: sling them
  4. If all remaining issues are skipped/blocked:
     Mail Overseer: "Mountain {{minecart_id}} stalled: N skipped, M blocked.
     Remaining DAG cannot progress without intervention."
  5. If miners are active: this is fine, no action needed

If task is "complete":
  1. Run: gt minecart status {{minecart_id}}
  2. Verify all tracked issues are closed
  3. If any skipped issues remain:
     Mail Overseer: "Mountain {{minecart_id}} finished with N skipped issues.
     Review skipped work: [list issue IDs]"
  4. If all clean:
     Mail Overseer: "Mountain {{minecart_id}} complete. N issues closed in Xh Ym."
  5. Run: gt minecart close {{minecart_id}}
"""
```

### Dog Properties That Make This Work

- **Fresh context**: The Dog starts with zero state. It reads the minecart
  and beads from scratch. No hysteresis from prior sessions.
- **Narrow scope**: One minecart, one question ("stalled?" or "complete?").
  Fits easily in a single context window.
- **Ephemeral**: Does its job, reports, dies. No long-running coordination.
- **Cross-rig visibility**: Dogs have worktrees into multiple rigs. They can
  check beads status across rigs for cross-rig minecarts.

### Audit Frequency

The Supervisor patrol cycle determines how often mountains are audited. Current
Supervisor patrol runs on a feed-driven + heartbeat model. For mountains, the
relevant question is: "How long can a mountain be stalled before someone
notices?"

- **Target**: Stall detected within 10-15 minutes
- **Mechanism**: Supervisor's heartbeat interval (daemon pokes Supervisor every
  5-10 minutes depending on activity). Each heartbeat runs the patrol
  formula including the mountain-audit step.
- **Cost**: One `bd list --label mountain` query per patrol cycle (cheap),
  plus one Dog spawn per stalled mountain (only when needed).

---

## 7. Layer 3: Overseer Notification

The Overseer receives two types of mountain mail from Dogs:

### Stall Notification

```
Subject: Mountain stalled: <minecart-title>
Body:
  Minecart: hq-cv-abc "Rebuild auth system"
  Progress: 23/35 closed (65%)
  Stalled for: 15 minutes

  Skipped issues (miner failure):
    gt-xyz "Migrate session store" (failed 3 times)
    gt-abc "Update JWT validation" (failed 3 times)

  Blocked issues (DAG):
    gt-def "Integration tests" (blocked by gt-xyz)
    gt-ghi "E2E tests" (blocked by gt-def)

  Active miners: 0
  Ready issues: 0

  Action needed: Review skipped issues. Possible fixes:
    bd update gt-xyz --status=open --remove-label mountain:skipped  (retry)
    bd close gt-xyz --reason="Descoped"  (skip permanently, unblocks dependents)
```

### Completion Notification

```
Subject: Mountain complete: <minecart-title>
Body:
  Minecart: hq-cv-abc "Rebuild auth system"
  Result: 33/35 closed, 2 skipped
  Elapsed: 3h 42m

  Skipped issues:
    gt-xyz "Migrate session store" (failed 3 times — needs manual review)
    gt-abc "Update JWT validation" (failed 3 times — needs manual review)
```

### Overseer's Role

The Overseer is NOT part of the grinding loop. It receives notifications and
can take action, but the mountain grinds autonomously without Overseer
involvement. The Overseer's actions are:

- **Retry a skipped issue**: `bd update <id> --status=open --remove-label mountain:skipped`
- **Permanently skip**: `bd close <id> --reason="Descoped"` (unblocks dependents)
- **Notify user**: Forward the stall/completion notification
- **Restructure DAG**: Remove or add dependencies to work around a blocker

---

## 8. User Experience

### Starting a Mountain

```bash
$ gt mountain gt-epic-auth-rebuild

Validating epic structure...
  Epic: gt-epic-auth-rebuild "Rebuild auth system"
  Tasks: 35 (31 slingable, 4 epics)
  Waves: 6 (computed from blocking deps)
  Max parallelism: 4

  Warnings:
    gt-migrate-sessions has no description (may cause miner confusion)

  Errors: none

Creating minecart...
  Minecart: hq-cv-m7x "Mountain: Rebuild auth system"
  Label: mountain

Launching Wave 1 (4 tasks)...
  Slung gt-foundation-types → mineshaft
  Slung gt-config-schema → mineshaft
  Slung gt-test-fixtures → mineshaft
  Slung gt-error-types → mineshaft

Mountain active. MinecartManager will feed subsequent waves.
Supervisor will audit progress every ~10 minutes.
Check status: gt mountain status hq-cv-m7x
```

### Checking Status

```bash
$ gt mountain status

Active Mountains:
  hq-cv-m7x "Rebuild auth system"
    Progress: ████████████░░░░░░░░ 23/35 (65%)
    Active: 3 miners working
    Ready: 1 issue waiting for miner
    Blocked: 6 issues (DAG deps)
    Skipped: 2 issues (miner failures)
    Elapsed: 1h 47m

  hq-cv-n9y "Migrate database layer"
    Progress: ██████████████████░░ 18/20 (90%)
    Active: 2 miners working
    Elapsed: 52m
```

### Detailed Status

```bash
$ gt mountain status hq-cv-m7x

Mountain: hq-cv-m7x "Rebuild auth system"
Epic: gt-epic-auth-rebuild

Progress: 23/35 closed (65%)
Elapsed: 1h 47m
Wave: 4 of 6

Completed (23):
  ✓ gt-foundation-types, gt-config-schema, gt-test-fixtures, ...

Active (3):
  ⟳ gt-session-handler (miner: mineshaft/nux, 12m)
  ⟳ gt-middleware-chain (miner: mineshaft/furiosa, 8m)
  ⟳ gt-rate-limiter (miner: mineshaft/max, 3m)

Ready (1):
  ○ gt-cache-layer (unblocked, waiting for miner)

Skipped (2):
  ⊘ gt-migrate-sessions (failed 3 times — no description)
  ⊘ gt-jwt-validation (failed 3 times — test dependency missing)

Blocked (6):
  ◌ gt-auth-integration (needs: gt-session-handler, gt-jwt-validation⊘)
  ◌ gt-e2e-auth-tests (needs: gt-auth-integration)
  ...

Stall risk: gt-jwt-validation⊘ blocks 4 downstream issues.
  Fix: bd update gt-jwt-validation --status=open --remove-label mountain:skipped
  Or:  bd close gt-jwt-validation --reason="Descoped"
```

---

## 9. Global Improvements (All Minecarts)

The Mountain-Eater design reveals improvements that benefit ALL minecarts,
not just mountains. These should be applied globally:

### 9.1 Miner Failure Tracking

Even non-mountain minecarts benefit from knowing "this issue has failed 3
times." The Witness should track failure counts for all minecart-tracked
issues, not just mountain ones. The difference: mountains auto-skip after
3 failures; regular minecarts just log a warning.

### 9.2 Stall Detection in Stranded Scan

The MinecartManager's stranded scan currently feeds the first ready issue.
Add: if the same issue has been slung 3+ times and keeps appearing as
stranded, stop re-slinging it and log a warning. This prevents the
infinite sling-fail loop for all minecarts.

### 9.3 Progress Visibility

`gt minecart status` should show the same rich information as
`gt mountain status` — active miners, ready front, blocked issues,
skipped issues. This is useful for all minecarts, not just mountains.

---

## 10. Relationship to Swarm Architecture

The [swarm architecture doc](../../../docs/swarm-architecture.md) describes
a design where swarms are persistent molecules coordinated by a dedicated
agent. The Mountain-Eater achieves the same outcome through a different
mechanism:

| Swarm Architecture | Mountain-Eater |
|--------------------|----------------|
| Dedicated coordinator agent | No coordinator — patrol steps + Dogs |
| Swarm molecule tracks state | Label triggers patrol behavior |
| Coordinator survives via molecule | Dogs bring fresh context (no survival needed) |
| Ready Front computed by coordinator | Ready Front computed by MinecartManager + Dogs |
| Recovery via molecule resume | Recovery via beads state discovery |

The Mountain-Eater is the implementation path for the swarm architecture's
goals. The swarm doc's "ready front" model, "gate issues," and "batch
management" concepts apply directly. The difference is mechanism:
patrol-driven grinding instead of coordinator-driven grinding.

The swarm architecture doc should be updated to reference the Mountain-Eater
as the concrete implementation.

---

## 11. Implementation Plan

See [roadmap.md](roadmap.md) Milestone 5 for the phased implementation.

### Summary of Changes

| Component | Change | Scope |
|-----------|--------|-------|
| `gt mountain` CLI | New command (stage + label + launch) | ~200 lines |
| `gt mountain status` | New command (query + format) | ~300 lines |
| `gt mountain pause/resume/cancel` | Label management | ~100 lines |
| Witness patrol formula | Failure tracking for minecart issues | Formula step |
| Supervisor patrol formula | Mountain audit step | Formula step |
| `mol-mountain-dog.formula.toml` | Dog formula for stall investigation | New formula |
| MinecartManager stranded scan | Skip after N failures (global) | ~30 lines |
| `gt minecart status` | Enhanced output (active, ready, blocked) | ~100 lines |

### What Does NOT Change

- Minecart data model (still `hq-cv-*` beads with `tracks` deps)
- MinecartManager event poll (still 5s, still feeds on close)
- MinecartManager stranded scan (still 30s, enhanced with skip logic)
- Stage-launch workflow (mountain uses it directly)
- Miner lifecycle (unchanged)
- Refinery (unchanged)

---

## 12. Open Questions

1. **Should `gt mountain` auto-undock a docked rig?** If the epic's issues
   route to a docked rig, should the mountain automatically undock it?
   Current thinking: no — require the rig to be active. Mountains only
   grind active rigs.

2. **Max concurrent miners per mountain.** Should mountains have a
   configurable concurrency limit? The MinecartManager feeds one issue per
   close event. For mountains, we might want to dispatch multiple ready
   issues when a wave transition happens (e.g., wave 1 completes, wave 2
   has 8 ready issues — dispatch all 8, not one-at-a-time).

3. **Mountain-to-mountain dependencies.** Can one mountain depend on
   another? Probably not needed initially — cross-mountain deps are just
   cross-issue deps in the DAG.

4. **Notification channel.** Overseer mail is the current notification path.
   Should mountains also support webhook/Slack notification for the user?
   Defer to future work.
