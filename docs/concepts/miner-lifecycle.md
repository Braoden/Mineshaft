# Miner Lifecycle

> Understanding the three-layer architecture of miner workers

## Overview

Miners have three distinct lifecycle layers that operate independently. The
key design principle: **miners are persistent**. They survive work completion
and can be reused across assignments.

## The Four Operating States

Miners have four operating states:

| State | Description | How it happens |
|-------|-------------|----------------|
| **Working** | Actively doing assigned work | Normal operation after `gt sling` |
| **Idle** | Work completed, sandbox preserved for reuse | After `gt done` completes successfully |
| **Stalled** | Session stopped mid-work | Interrupted, crashed, or timed out without being nudged |
| **Zombie** | Completed work but failed to exit | `gt done` failed during cleanup |

**State cycle (happy path):**

```
         ┌──────────┐
    ┌───>│  IDLE    │<──── sync sandbox to main, clear hook
    │    └────┬─────┘
    │         │ gt sling
    │         v
    │    ┌──────────┐
    │    │ WORKING  │<──── session active, hook set
    │    └────┬─────┘
    │         │ gt done
    │         v
    │    ┌──────────┐
    └────┤  IDLE    │──── push branch, submit MR, go idle
         └──────────┘
```

No `nuke` in the happy path. Miners cycle: IDLE -> WORKING -> IDLE.

**Key distinctions:**

- **Working** = actively executing. Session alive, hook set, doing work.
- **Idle** = work done, session killed, sandbox preserved. Ready for next `gt sling`.
- **Stalled** = supposed to be working, but stopped. Needs Witness intervention.
- **Zombie** = finished work, tried to exit, but cleanup failed. Stuck in limbo.

## The Persistent Miner Model (gt-4ac)

**Miners persist after completing work.** When a miner finishes its assignment:

1. Signals completion via `gt done`
2. Pushes branch, submits MR to merge queue
3. Clears its hook (work is done)
4. Sets agent state to "idle"
5. Kills its own session
6. **Sandbox (worktree) is preserved for reuse**

The next `gt sling` reuses idle miners before allocating new ones, avoiding
the overhead of creating fresh worktrees.

### Why Persistent?

- **Faster turnaround** — Reusing an existing worktree is faster than creating one
- **Preserved identity** — The miner's agent bead, CV chain, and work history persist
- **Simpler lifecycle** — No nuke/respawn cycle between assignments
- **Done means idle** — Session dies, sandbox lives, miner awaits next assignment

### What About Pending Merges?

The Refinery owns the merge queue. Once `gt done` submits work:
- The branch is pushed to origin
- Work exists in the MQ, not in the miner
- If rebase fails, Refinery creates a conflict-resolution task
- The idle miner can be reused for the conflict resolution work

## The Three Layers

### The Problem: Three Concepts Were Conflated

Early designs treated miners as monolithic. This caused recurring issues:

| Concept | Lifecycle | Old behavior |
|---------|-----------|-----------------|
| **Identity** | Long-lived (name, CV, ledger) | Destroyed on nuke |
| **Sandbox** | Per-assignment (worktree, branch) | Destroyed on nuke |
| **Session** | Ephemeral (Claude context window) | = miner lifetime |

Separating these three layers means idle miners are a healthy state (not waste),
eliminates unnecessary worktree creation overhead, and preserves capability
records (CV, completion history) across assignments.

### Layer Summary

| Layer | Component | Lifecycle | Persistence |
|-------|-----------|-----------|-------------|
| **Identity** | Agent bead, CV chain, work history | Permanent | Never dies |
| **Sandbox** | Git worktree, branch | Persistent across assignments | Created on first sling, reused thereafter |
| **Session** | Claude (tmux pane), context window | Ephemeral per step | Cycles per step/handoff |

### Identity Layer

The miner's **identity is permanent**. It includes:

- Agent bead (created once, never deleted)
- CV chain (work history accumulates across all assignments)
- Mailbox and attribution record

Identity survives all session cycles and sandbox resets. In the HOP model, this IS
the miner — everything else is infrastructure that comes and goes. See
[Miner Identity](#miner-identity) below for details.

### Session Layer

The Claude session is **ephemeral**. It cycles frequently:

- After each molecule step (via `gt handoff`)
- On context compaction
- On crash/timeout
- After extended work periods

**Key insight:** Session cycling is **normal operation**, not failure. The miner
continues working—only the Claude context refreshes.

```
Session 1: Steps 1-2 → handoff
Session 2: Steps 3-4 → handoff
Session 3: Step 5 → gt done
```

All three sessions are the **same miner**. The sandbox persists throughout.

### Sandbox Layer

The sandbox is the **git worktree**—the miner's working directory:

```
~/gt/excavation/miners/Toast/
```

This worktree:
- Exists from first `gt sling` and persists across assignments
- Survives all session cycles
- Is repaired (reset to fresh branch from main) when reused by `gt sling`
- Contains uncommitted work, staged changes, branch state during active work

The Witness never destroys sandboxes. Only explicit `gt miner nuke` removes them.

#### Sandbox Sync (Between Assignments)

When work completes and the miner goes idle, the sandbox is synced to main:

```bash
# In the miner's worktree (done automatically by gt done / gt sling)
git checkout main
git pull origin main
git branch -D miner/<name>/<old-issue>@<timestamp>
# Worktree is now clean, on main, ready for next assignment
```

When new work is slung:
```bash
# Create fresh branch from current main
git checkout -b miner/<name>/<new-issue>@<timestamp>
# Start working
```

No worktree add/remove between assignments. Just branch operations on an
existing worktree. This avoids the ~5s overhead of creating fresh worktrees.

### Slot Layer

The slot is the **name allocation** from the miner pool:

```bash
# Pool: [Toast, Shadow, Copper, Ash, Storm...]
# Toast is allocated to work gt-abc
```

The slot:
- Determines the sandbox path (`miners/Toast/`)
- Maps to a tmux session (`gt-excavation-Toast`)
- Appears in attribution (`excavation/miners/Toast`)
- Persists until explicit nuke

## Correct Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                        gt sling                             │
│  → Find idle miner OR allocate slot from pool (Toast)    │
│  → Create/repair sandbox (worktree on new branch)          │
│  → Start session (Claude in tmux)                          │
│  → Hook molecule to miner                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Work Happens                            │
│                                                             │
│  Session cycles happen here:                               │
│  - gt handoff between steps                                │
│  - Compaction triggers respawn                             │
│  - Crash → Witness respawns                                │
│                                                             │
│  Sandbox persists through ALL session cycles               │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                  gt done (persistent model)                  │
│  → Push branch to origin                                   │
│  → Submit work to merge queue (MR bead)                    │
│  → Set agent state to "idle"                               │
│  → Kill session                                            │
│                                                             │
│  Work now lives in MQ. Miner is IDLE, not gone.          │
│  Sandbox preserved for reuse by next gt sling.             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Refinery: merge queue                     │
│  → Rebase and merge to target branch                       │
│    (main or integration branch — see below)                │
│  → Close the issue                                         │
│  → If conflict: create task for available miner          │
│                                                             │
│  Integration branch path:                                  │
│  → MRs from epic children merge to integration/<epic>      │
│  → When all children closed: land to main as one commit    │
└─────────────────────────────────────────────────────────────┘
```

## What "Recycle" Means

**Session cycling**: Normal. Claude restarts, sandbox stays, slot stays.

```bash
gt handoff  # Session cycles, miner continues
```

**Sandbox repair**: On reuse. `gt sling` resets the worktree to a fresh branch.

```bash
gt sling gt-xyz excavation  # Reuses idle Toast, repairs worktree
```

Session cycling happens constantly. Sandbox repair happens between assignments.

## Anti-Patterns

### Manual State Transitions

**Anti-pattern:**
```bash
gt miner done Toast    # DON'T: external state manipulation
gt miner reset Toast   # DON'T: manual lifecycle control
```

**Correct:**
```bash
# Miner signals its own completion:
gt done  # (from inside the miner session)

# Only explicit nuke destroys miners:
gt miner nuke Toast  # (destroys sandbox, identity persists)
```

Miners manage their own session lifecycle. External manipulation bypasses verification.

### Sandboxes Without Work (Idle vs Stalled)

An idle miner has no hook and no session — this is **normal**. It completed
its work and is waiting for the next `gt sling`.

A **stalled** miner has a hook but no session — this is a **failure**:
- The session crashed and wasn't nudged back to life
- The hook was lost during a crash
- State corruption occurred

**Recovery for stalled:**
```bash
# Witness respawns the session in the existing sandbox
# Or, if unrecoverable:
gt miner nuke Toast        # Clean up the stalled miner
gt sling gt-abc excavation      # Respawn with fresh miner
```

### Confusing Session with Sandbox

**Anti-pattern:** Thinking session restart = losing work.

```bash
# Session ends (handoff, crash, compaction)
# Work is NOT lost because:
# - Git commits persist in sandbox
# - Staged changes persist in sandbox
# - Molecule state persists in beads
# - Hook persists across sessions
```

The new session picks up where the old one left off via `gt prime`.

## Session Lifecycle Details

Sessions cycle for these reasons:

| Trigger | Action | Result |
|---------|--------|--------|
| `gt handoff` | Voluntary | Clean cycle to fresh context |
| Context compaction | Automatic | Forced by Claude Code |
| Crash/timeout | Failure | Witness respawns |
| `gt done` | Completion | Session exits, miner goes idle |

All except `gt done` result in continued work. Only `gt done` signals completion
and transitions the miner to idle.

## Witness Responsibilities

The Witness monitors miners but does NOT:
- Force session cycles (miners self-manage via handoff)
- Interrupt mid-step (unless truly stuck)
- Nuke miners after completion (persistent model)

The Witness DOES:
- Detect and nudge stalled miners (sessions that stopped unexpectedly)
- Clean up zombie miners (sessions where `gt done` failed)
- Respawn crashed sessions
- Handle escalations from stuck miners (miners that explicitly asked for help)

## Miner Identity

**Key insight:** Miner *identity* is permanent; sessions are ephemeral, sandboxes are persistent.

In the HOP model, every entity has a chain (CV) that tracks:
- What work they've done
- Success/failure rates
- Skills demonstrated
- Quality metrics

The miner *name* (Toast, Shadow, etc.) is a slot from a pool — persistent until
explicit nuke. The *agent identity* that executes as that miner accumulates a
work history across all assignments.

```
MINER IDENTITY (permanent)      SESSION (ephemeral)     SANDBOX (persistent)
├── CV chain                      ├── Claude instance     ├── Git worktree
├── Work history                  ├── Context window      ├── Branch
├── Skills demonstrated           └── Dies on handoff     └── Repaired on reuse
└── Credit for work                   or gt done              by gt sling
```

This distinction matters for:
- **Attribution** - Who gets credit for the work?
- **Skill routing** - Which agent is best for this task?
- **Cost accounting** - Who pays for inference?
- **Federation** - Agents having their own chains in a distributed world

## Implementation Status

As of 2026-03-07 (gt-o8g8 audit), all core lifecycle operations are **shipped and
running in production**. See [design/miner-lifecycle-patrol.md § 10](../design/miner-lifecycle-patrol.md#10-implementation-status-gt-o8g8-audit-2026-03-07)
for the full implementation matrix and [design/persistent-miner-pool.md](../design/persistent-miner-pool.md)
for phase-by-phase shipping status.

Key files:
- `internal/cmd/done.go` — work submission, sandbox sync, idle transition
- `internal/cmd/sling.go` + `miner_spawn.go` — idle reuse, branch-only repair
- `internal/cmd/handoff.go` — session cycling for all roles
- `internal/witness/handlers.go` — cleanup pipeline, MINER_DONE routing, zombie/orphan detection
- `internal/miner/manager.go` — stale detection, idle reuse (`FindIdleMiner`, `ReuseIdleMiner`), pool management

## Related Documentation

- [Overview](../overview.md) - Role taxonomy and architecture
- [Molecules](molecules.md) - Molecule execution and miner workflow
- [Propulsion Principle](propulsion-principle.md) - Why work triggers immediate execution
- [Miner Lifecycle Patrol](../design/miner-lifecycle-patrol.md) - Implementation details, cleanup stages, patrol coordination
- [Persistent Miner Pool](../design/persistent-miner-pool.md) - Pool management design and shipping status
