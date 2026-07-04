# Persistent Miner Pool

**Issue:** gt-lpop
**Status:** Design
**Author:** Overseer

## Problem

Three concepts are conflated in the miner lifecycle:

| Concept | Lifecycle | Current behavior |
|---------|-----------|-----------------|
| **Identity** | Long-lived (name, CV, ledger) | Destroyed on nuke |
| **Sandbox** | Per-assignment (worktree, branch) | Destroyed on nuke |
| **Session** | Ephemeral (Claude context window) | = miner lifetime |

Consequences:
- Work is lost when miners are nuked before pushing
- 219 stale remote branches from destroyed worktrees
- Slow dispatch (~5s worktree creation per assignment)
- Lost capability record (CV, completion history)
- Idle miners were treated as waste and nuked

## Design

### Lifecycle Separation

```
IDENTITY (persistent)
  Name: "furiosa"
  Agent bead: gt-excavation-miner-furiosa
  CV: work history, languages, completion rate
  Lifecycle: created once, never destroyed (unless explicitly retired)

SANDBOX (per-assignment, reusable)
  Worktree: miners/furiosa/excavation/
  Branch: miner/furiosa/<issue>@<timestamp>
  Lifecycle: synced to main between assignments, not destroyed

SESSION (ephemeral)
  Tmux: gt-excavation-furiosa
  Claude context: cycles on compaction/handoff
  Lifecycle: independent of identity and sandbox
```

### Pool States

```
         ┌──────────┐
    ┌───►│  IDLE    │◄──── sync sandbox to main
    │    └────┬─────┘      clear hook
    │         │ gt sling
    │         ▼
    │    ┌──────────┐
    │    │ WORKING  │◄──── session active, hook set
    │    └────┬─────┘
    │         │ work complete
    │         ▼
    │    ┌──────────┐
    └────┤  DONE    │──── push branch, submit MR
         └──────────┘
```

No `nuke` in the happy path. Miners cycle: IDLE → WORKING → DONE → IDLE.

### Pool Management

**Pool size:** Fixed per rig. Configured in `rig.config.json`:
```json
{
  "miner_pool_size": 4,
  "miner_names": ["furiosa", "nux", "toast", "slit"]
}
```

**Initialization:** `gt rig add` or `gt miner pool init <rig>` creates N miners
with identities and worktrees. They start in IDLE state.

**Dispatch:** `gt sling <bead> <rig>` finds an IDLE miner (already does this via
`FindIdleMiner()`), attaches work, starts session. No worktree creation needed.

**Completion:** When a miner finishes work:
1. Push branch to origin
2. Submit MR (if code changes)
3. Clear hook_bead
4. Sync worktree: `git checkout main && git pull`
5. Set state to IDLE
6. Session stays alive or cycles — doesn't matter, identity persists

### Sandbox Sync (DONE → IDLE transition)

When work completes and MR is merged (or no code changes):

```bash
# In the miner's worktree
git checkout main
git pull origin main
git branch -D miner/furiosa/<old-issue>@<timestamp>
# Worktree is now clean, on main, ready for next assignment
```

When new work is slung:
```bash
# Create fresh branch from current main
git checkout -b miner/furiosa/<new-issue>@<timestamp>
# Start working
```

No worktree add/remove. Just branch operations on an existing worktree.

### Refinery Integration

No changes to refinery. Refinery still:
1. Sees MR from miner branch
2. Reviews and merges to main
3. Deletes remote miner branch (NEW: add this step)

The miner doesn't care — it already moved to main locally during DONE → IDLE.

### Witness Integration

Witness patrol behavior (shipped):
- Sees idle miner → healthy state, skip
- **Stuck detection:** Miner in WORKING state for too long → escalate (don't nuke)
- **Dead session detection:** Session died but state=WORKING → restart session (not nuke miner)

### What Nuke Becomes

`gt miner nuke` is reserved for exceptional cases:
- Miner worktree is irrecoverably broken
- Need to reclaim disk space
- Decommissioning a rig

It should be rare and manual, not part of normal workflow.

### Branch Pollution Solution

With persistent miners, branches have clear owners:
- Active branches: miner is WORKING on them
- Merged branches: refinery deletes after merge
- Abandoned branches: miner syncs to main on DONE → IDLE, old branch deleted locally

The 219 stale branches came from nuked miners that never cleaned up. With persistent
miners, branch lifecycle is managed by the miner itself.

### One-time Cleanup

For the existing 219 stale branches:
```bash
# Delete all remote miner branches that don't belong to active miners
git branch -r | grep 'origin/miner/' | grep -v 'furiosa/gt-ziiu' | grep -v 'nux/gt-uj16' \
  | sed 's/origin\///' | xargs -I{} git push origin --delete {}
```

## Implementation Phases

### Phase 1: Stop the bleeding — SHIPPED
- Witness no longer nukes idle miners
- `gt miner done` transitions to IDLE instead of triggering nuke
- Refinery deletes remote branch after merge

### Phase 2: Pool initialization — DEFERRED
- `gt miner pool init <rig>` creates N persistent miners
- Pool size configured in rig.config.json
- Worktrees created once, reused across assignments

**Status:** Miners are allocated on-demand by `gt sling` via `FindIdleMiner()`
and `AllocateAndAdd()`. Pre-allocation is unnecessary because idle miners are
reused automatically. Pool size enforcement is a future optimization, not a blocker.

### Phase 3: Sandbox sync — SHIPPED
- DONE → IDLE transition syncs worktree to main (`done.go`)
- IDLE → WORKING creates fresh branch (no worktree add) via `ReuseIdleMiner()`
- `gt sling` prefers idle miners via `FindIdleMiner()`
- Branch-only reuse eliminates ~5s worktree creation overhead

### Phase 4: Session independence — SHIPPED
- Session cycling doesn't affect miner state
- Dead sessions restarted by witness (restart-first policy, no auto-nuke)
- Handoff preserves miner identity across session boundaries
- `gt handoff` works for all roles (Overseer, Crew, Witness, Refinery, Miners)

### Phase 5: One-time cleanup — PARTIALLY SHIPPED
- Miner branch cleanup after merge: SHIPPED (landed to main; PRs #2436/#2437 closed)
- Refinery notifies overseer after merge: not yet shipped
- Pool reconciliation (`ReconcilePool`): not yet implemented

### Implementation Status Summary

| Component | Status | Key Files |
|-----------|--------|-----------|
| `gt done` (push, MR, idle, sandbox sync) | SHIPPED | `internal/cmd/done.go` |
| `gt sling` (idle reuse, branch-only repair) | SHIPPED | `internal/cmd/sling.go`, `miner_spawn.go` |
| `gt handoff` (session cycle, all roles) | SHIPPED | `internal/cmd/handoff.go` |
| Witness patrol (zombie, stale, orphan detection) | SHIPPED | `internal/witness/handlers.go`, `internal/miner/manager.go` |
| Cleanup pipeline (MINER_DONE → MERGE_READY → MERGED) | SHIPPED | `internal/witness/handlers.go`, `internal/refinery/engineer.go` |
| Idle miner heresy fix (skip healthy idle) | SHIPPED | `internal/witness/handlers.go` |
| Restart-first policy (no auto-nuke) | SHIPPED | `internal/miner/manager.go` |
| Miner branch always deleted after merge | SHIPPED | `internal/refinery/engineer.go` |
| Refinery notifies overseer after merge | NOT SHIPPED | — |
| Pool size enforcement | DEFERRED | — |
| `ReconcilePool()` | DEFERRED | — |
| `gt miner pool init` command | DEFERRED | — |
