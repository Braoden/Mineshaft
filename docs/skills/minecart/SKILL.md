---
name: minecart
description: The definitive guide for working with mineshaft's minecart system -- batch work tracking, event-driven feeding, stage-launch workflow, and dispatch safety guards. Use when writing minecart code, debugging minecart behavior, adding minecart features, testing minecart changes, or answering questions about how minecarts work. Triggers on minecart, minecart manager, minecart feeding, dispatch, stranded minecart, feedFirstReady, feedNextReadyIssue, IsSlingableType, isIssueBlocked, CheckMinecartsForIssue, gt minecart, gt sling, stage, launch, staged, wave.
---

# Mineshaft Minecart System

The minecart system tracks batches of work across rigs. A minecart is a bead that `tracks` other beads via dependencies. The daemon monitors close events and feeds the next ready issue when one completes.

## Architecture

```
+================================ CREATION =================================+
|                                                                            |
|   gt sling <beads>      gt minecart create ...     gt minecart stage <epic>    |
|        |  (auto-minecart)       |  (explicit)            |  (validated)     |
|        v                      v                        v                  |
|   +-----------+          +-----------+         +----------------+         |
|   |  status:  |          |  status:  |         |    status:     |         |
|   |   open    |          |   open    |         | staged:ready   |         |
|   +-----------+          +-----------+         | staged:warnings|         |
|                                                +----------------+         |
|                                                        |                  |
|                                              gt minecart launch             |
|                                                        |                  |
|                                                        v                  |
|                                                +----------------+         |
|                                                |    status:     |         |
|                                                |     open       |         |
|                                                | (Wave 1 slung) |         |
|                                                +----------------+         |
|                                                                            |
|   All paths produce: MINECART (hq-cv-*)                                      |
|                      tracks: issue1, issue2, ...                           |
+============================================================================+
              |                              |
              v                              v
+= EVENT-DRIVEN FEEDER (5s) =+   +=== STRANDED SCAN (30s) ===+
|                              |   |                            |
|   GetAllEventsSince (SDK)    |   |   findStranded             |
|     |                        |   |     |                      |
|     v                        |   |     v                      |
|   close event detected       |   |   minecart has ready issues  |
|     |                        |   |   but no active workers    |
|     v                        |   |     |                      |
|   CheckMinecartsForIssue       |   |     v                      |
|     |                        |   |   feedFirstReady           |
|     v                        |   |   (iterates all ready)     |
|   feedNextReadyIssue         |   |     |                      |
|   (iterates all ready)       |   |     v                      |
|     |                        |   |   gt sling <next-bead>     |
|     v                        |   |   or closeEmptyMinecart     |
|   gt sling <next-bead>       |   |                            |
|                              |   +============================+
+==============================+
```

Three creation paths (sling, create, stage), two feed paths, same safety guards:
- **Event-driven** (`operations.go`): Polls beads stores every ~5s for close events. Calls `feedNextReadyIssue` which checks `IsSlingableType` + `isIssueBlocked` before dispatch. **Skips staged minecarts** (`isMinecartStaged` check).
- **Stranded scan** (`minecart_manager.go`): Runs every 30s. `feedFirstReady` iterates all ready issues. The ready list is pre-filtered by `IsSlingableType` in `findStrandedMinecarts` (cmd/minecart.go). **Only sees open minecarts** — staged minecarts never appear.

## Safety guards (the three rules)

These prevent the event-driven feeder from dispatching work it shouldn't:

### 1. Type filtering (`IsSlingableType`)

Only leaf work items dispatch. Defined in `operations.go`:

```go
var slingableTypes = map[string]bool{
    "task": true, "bug": true, "feature": true, "chore": true,
    "": true, // empty defaults to task
}
```

Epics, sub-epics, minecarts, decisions -- all skip. Applied in both `feedNextReadyIssue` (event path) and `findStrandedMinecarts` (stranded path).

### 2. Blocks dep checking (`isIssueBlocked`)

Issues with unclosed `blocks`, `conditional-blocks`, or `waits-for` dependencies skip. `parent-child` is **not** blocking -- a child task dispatches even if its parent epic is open. This is consistent with `bd ready` and molecule step behavior.

Fail-open on store errors (assumes not blocked) to avoid stalling minecarts on transient Dolt issues.

### 3. Dispatch failure iteration

Both feed paths iterate past failures instead of giving up:
- `feedNextReadyIssue`: `continue` on dispatch failure, try next ready issue
- `feedFirstReady`: `for range ReadyIssues` with `continue` on skip/failure, `return` on first success

## CLI commands

### Stage and launch (validated creation)

```bash
gt minecart stage <epic-id>            # analyze deps, build DAG, compute waves, create staged minecart
gt minecart stage gt-task1 gt-task2    # stage from explicit task list
gt minecart stage hq-cv-abc            # re-stage existing staged minecart
gt minecart stage <epic-id> --json     # machine-readable output
gt minecart stage <epic-id> --launch   # stage + immediately launch if no errors
gt minecart launch hq-cv-abc           # transition staged → open, dispatch Wave 1
gt minecart launch <epic-id>           # stage + launch in one step (delegates to stage --launch)
```

### Create and manage

```bash
gt minecart create "Auth overhaul" gt-task1 gt-task2 gt-task3
gt minecart add hq-cv-abc gt-task4
```

### Check and monitor

```bash
gt minecart check hq-cv-abc       # auto-closes if all tracked issues done
gt minecart check                  # check all open minecarts
gt minecart status hq-cv-abc       # single minecart detail
gt minecart list                   # all minecarts
gt minecart list --all             # include closed
```

### Find stranded work

```bash
gt minecart stranded               # ready work with no active workers
gt minecart stranded --json        # machine-readable
```

### Close and land

```bash
gt minecart close hq-cv-abc --reason "done"
gt minecart land hq-cv-abc         # cleanup worktrees + close
```

### Interactive TUI

```bash
gt minecart -i                     # opens interactive minecart browser
gt minecart --interactive          # long form
```

## Batch sling behavior

`gt sling <bead1> <bead2> <bead3>` creates **one minecart** tracking all beads. The rig is auto-resolved from the beads' prefixes (via `routes.jsonl`). The minecart title is `"Batch: N beads to <rig>"`. Each bead gets its own miner, but they share a single minecart for tracking.

The minecart ID and merge strategy are stored on each bead, so `gt done` can find the minecart via the fast path (`getMinecartInfoFromIssue`).

### Rig resolution

- **Auto-resolve (preferred):** `gt sling gt-task1 gt-task2 gt-task3` -- resolves rig from the `gt-` prefix. All beads must resolve to the same rig.
- **Explicit rig (deprecated):** `gt sling gt-task1 gt-task2 gt-task3 myrig` -- still works, prints a deprecation warning. If any bead's prefix doesn't match the explicit rig, errors with suggested actions.
- **Mixed prefixes:** If beads resolve to different rigs, errors listing each bead's resolved rig and suggested actions (sling separately, or `--force`).
- **Unmapped prefix:** If a prefix has no route, errors with diagnostic info (`cat .beads/routes.jsonl | grep <prefix>`).

### Conflict handling

If any bead is already tracked by another minecart, batch sling **errors** with detailed conflict info (which minecart, all beads in it with statuses, and 4 recommended actions). This prevents accidental double-tracking.

```bash
# Auto-resolve: one minecart, three miners (preferred)
gt sling gt-task1 gt-task2 gt-task3
# -> Created minecart hq-cv-xxxxx tracking 3 beads

# Explicit rig still works but prints deprecation warning
gt sling gt-task1 gt-task2 gt-task3 mineshaft
# -> Deprecation: gt sling now auto-resolves the rig from bead prefixes.
# -> Created minecart hq-cv-xxxxx tracking 3 beads
```

## Stage-launch workflow

> Implemented in [PR #1820](https://github.com/steveyegge/mineshaft/pull/1820). Depends on the feeder safety guards from [PR #1759](https://github.com/steveyegge/mineshaft/pull/1759). Design docs: `docs/design/minecart/stage-launch/prd.md`, `docs/design/minecart/stage-launch/testing.md`.

The stage-launch workflow is a two-phase minecart creation path that validates dependencies and computes wave dispatch order **before** any work is dispatched. This is the preferred path for epic delivery.

### Input types

`gt minecart stage` accepts three mutually exclusive input types:

| Input | Example | Behavior |
|-------|---------|----------|
| Epic ID | `gt minecart stage bcc-nxk2o` | BFS walks entire parent-child tree, collects all descendants |
| Task list | `gt minecart stage gt-t1 gt-t2 gt-t3` | Analyzes exactly those tasks |
| Minecart ID | `gt minecart stage hq-cv-abc` | Re-reads tracked beads from existing staged minecart (re-stage) |

Mixed types (e.g., epic + task together) error. Multiple epics or multiple minecarts error.

### Processing pipeline

```
1. validateStageArgs     — reject empty/flag-like args
2. bdShow each arg       — resolve bead types
3. resolveInputKind      — classify Epic / Tasks / Minecart
4. collectBeads          — gather BeadInfo + DepInfo (BFS for epic, direct for tasks)
5. buildMinecartDAG        — construct in-memory DAG (nodes + edges)
6. detectErrors          — cycle detection + missing rig checks
7. detectWarnings        — orphans, parked rigs, cross-rig, capacity, missing branches
8. categorizeFindings    — split into errors / warnings
9. chooseStatus          — staged:ready, staged:warnings, or abort on errors
10. computeWaves         — Kahn's algorithm (only when no errors)
11. renderDAGTree        — print ASCII dependency tree
12. renderWaveTable      — print wave dispatch plan
13. createStagedMinecart   — bd create --type=minecart --status=<staged-status>
```

### Wave computation (Kahn's algorithm)

Only slingable types participate in waves: `task`, `bug`, `feature`, `chore`. Epics are excluded.

Execution edges (create wave ordering):
- `blocks`
- `conditional-blocks`
- `waits-for`

Non-execution edges (ignored for wave ordering):
- `parent-child` — hierarchy only
- `related`, `tracks`, `discovered-from`

**Algorithm:**
1. Filter to slingable nodes only
2. Calculate in-degree for each node (count BlockedBy edges to other slingable nodes)
3. Peel loop: collect all nodes with in-degree 0 → Wave N; remove them; decrement neighbors; repeat
4. Sort within each wave alphabetically for determinism

Output example:
```
  Wave   ID              Title                     Rig       Blocked By
  ──────────────────────────────────────────────────────────────────────
  1      bcc-nxk2o.1.1   Init scaffolding          bcc       —
  2      bcc-nxk2o.1.2   Shared types              bcc       bcc-nxk2o.1.1
  3      bcc-nxk2o.1.3   CLI wrapper               bcc       bcc-nxk2o.1.2

  3 tasks across 3 waves (max parallelism: 1 in wave 1)
```

### Minecart status model

Four statuses with defined transitions:

| Status | Meaning |
|--------|---------|
| `staged:ready` | Validated, no errors or warnings, ready to launch |
| `staged:warnings` | Validated, no errors but has warnings. Fix and re-stage, or launch anyway. |
| `open` | Active — daemon feeds work as beads close |
| `closed` | Complete or cancelled |

Valid transitions:

| From → To | Allowed? |
|-----------|----------|
| `staged:ready` → `open` | Yes (launch) |
| `staged:warnings` → `open` | Yes (launch) |
| `staged:*` → `closed` | Yes (cancel) |
| `staged:ready` ↔ `staged:warnings` | Yes (re-stage) |
| `open` → `closed` | Yes |
| `closed` → `open` | Yes (reopen) |
| `open` → `staged:*` | **No** |
| `closed` → `staged:*` | **No** |

### Error vs warning classification

**Errors** (fatal — prevent minecart creation):

| Category | Trigger | Fix |
|----------|---------|-----|
| `cycle` | Cycle detected in execution edges | Remove one blocking dep in the cycle |
| `no-rig` | Slingable bead has no rig (prefix not in routes.jsonl) | Add routes.jsonl entry |

**Warnings** (non-fatal — minecart created as `staged:warnings`):

| Category | Trigger |
|----------|---------|
| `orphan` | Slingable task with no blocking deps in either direction (epic input only) |
| `blocked-rig` | Bead targets a parked or docked rig |
| `cross-rig` | Bead on a different rig than the majority |
| `capacity` | A wave has more than 5 tasks |
| `missing-branch` | Sub-epic with children but no integration branch |

### Launch behavior

`gt minecart launch <minecart-id>` transitions a staged minecart to open and dispatches Wave 1:

1. Validate minecart exists and is staged
2. Transition status to `open`
3. Re-read tracked beads, rebuild DAG, recompute waves
5. Dispatch every task in Wave 1 via `gt sling <beadID> <rig>`
6. Individual sling failures do NOT abort remaining dispatches
7. Print dispatch results (checkmark/X per task)
8. Subsequent waves handled automatically by the daemon

If `gt minecart launch` receives an epic or task list (not a staged minecart), it delegates to `gt minecart stage --launch` to stage-then-launch in one step.

### Staged minecart daemon safety

**Staged minecarts are completely inert to the daemon.** Neither feed path processes them:

- **Event-driven feeder:** `isMinecartStaged` check in `CheckMinecartsForIssue` skips any minecart with `staged:*` status. Fail-open on read errors (assumes not staged → processes, which is safe since a read error on a non-existent minecart does nothing).
- **Stranded scan:** `gt minecart stranded` only returns open minecarts. Staged minecarts never appear.

This means you can stage a minecart, review the wave plan, and launch when ready — no risk of premature dispatch.

### Re-staging

Running `gt minecart stage <minecart-id>` on an existing staged minecart re-analyzes and updates:
- Re-reads tracked beads from the minecart's `tracks` deps
- Rebuilds DAG, re-detects errors/warnings, recomputes waves
- Updates status via `bd update` (e.g., `staged:warnings` → `staged:ready` if warnings resolved)
- Does NOT create a new minecart or re-add track dependencies

## Testing minecart changes

### Running tests

```bash
# Full minecart suite (all packages)
go test ./internal/minecart/... ./internal/daemon/... ./internal/cmd/... -count=1

# By area:
go test ./internal/minecart/... -v -count=1                       # feeding logic
go test ./internal/daemon/... -v -count=1 -run TestMinecart       # MinecartManager
go test ./internal/daemon/... -v -count=1 -run TestFeedFirstReady
go test ./internal/cmd/... -v -count=1 -run TestCreateBatchMinecart  # batch sling
go test ./internal/cmd/... -v -count=1 -run TestBatchSling
go test ./internal/cmd/... -v -count=1 -run TestResolveRig      # rig resolution
go test ./internal/daemon/... -v -count=1 -run Integration      # real beads stores

# Stage-launch:
go test ./internal/cmd/... -v -count=1 -run TestMinecartStage     # staging logic
go test ./internal/cmd/... -v -count=1 -run TestMinecartLaunch    # launch + Wave 1 dispatch
go test ./internal/cmd/... -v -count=1 -run TestDetectCycles    # cycle detection
go test ./internal/cmd/... -v -count=1 -run TestComputeWaves    # wave computation
go test ./internal/cmd/... -v -count=1 -run TestBuildMinecartDAG  # DAG construction
```

### Key test invariants

- `feedFirstReady` dispatches exactly 1 issue per call (first success wins)
- `feedFirstReady` iterates past failures (sling exit 1 -> try next)
- Parked rigs are skipped in both event poll and feedFirstReady
- hq store is never skipped even if `isRigParked` returns true for everything
- High-water marks prevent event reprocessing across poll cycles
- First poll cycle is warm-up only (seeds marks, no processing)
- `IsSlingableType("epic") == false`, `IsSlingableType("task") == true`, `IsSlingableType("") == true`
- `isIssueBlocked` is fail-open (store error -> not blocked)
- `parent-child` deps are NOT blocking
- Batch sling creates exactly 1 minecart for N beads (not N minecarts)
- `resolveRigFromBeadIDs` errors on mixed prefixes, unmapped prefixes, town-level prefixes
- Cycles in blocking deps prevent staged minecart creation (exit non-zero, no side effects)
- Wave 1 contains ONLY tasks with zero unsatisfied blocking deps among slingable nodes
- Epics and non-slingable types are NEVER placed in waves
- Daemon does NOT feed issues from `staged:*` minecarts (both feed paths skip)
- `staged:warnings` minecarts can still be launched (warnings are informational)
- Re-staging a minecart does NOT create duplicates (updates in place)
- Launch dispatches ONLY Wave 1, not subsequent waves
- Wave computation is deterministic (same input → same output, alphabetical sort within waves)

### Deeper test engineering

See `docs/design/minecart/stage-launch/testing.md` for the full stage-launch test plan (105 tests across unit, integration, snapshot, and property tiers).

See `docs/design/minecart/testing.md` for the general minecart test plan covering failure modes, coverage gaps, harness scorecard, test matrix, and recommended test strategy.

## Common pitfalls

- **`parent-child` is never blocking.** This is a deliberate design choice, not a bug. Consistent with `bd ready`, beads SDK, and molecule step behavior.
- **Batch sling errors on already-tracked beads.** If any bead is already in a minecart, the entire batch sling fails with conflict details. The user must resolve the conflict before proceeding.
- **The stranded scan has its own blocked check.** `isReadyIssue` in cmd/minecart.go reads `t.Blocked` from issue details. `isIssueBlocked` in operations.go covers the event-driven path. Don't consolidate them without understanding both paths.
- **Empty IssueType is slingable.** Beads default to type "task" when IssueType is unset. Treating empty as non-slingable would break all legacy beads.
- **`isIssueBlocked` is fail-open.** Store errors assume not blocked. A transient Dolt error should not permanently stall a minecart -- the next feed cycle retries with fresh state.
- **Explicit rig in batch sling is deprecated.** `gt sling beads... rig` still works but prints a warning. Prefer `gt sling beads...` with auto-resolution.
- **Staged minecarts are inert.** The daemon ignores them completely. Don't expect auto-feeding until you `gt minecart launch`.
- **Review `staged:warnings` before launching.** Warnings are informational — fix and re-stage if possible, or launch anyway if they're acceptable.
- **`gt minecart launch` on a non-staged input delegates to stage.** If you pass an epic or task list to `launch`, it runs `stage --launch` internally. Only an already-staged minecart gets the fast path.
- **Wave computation is informational.** Waves are computed at stage time for display. Runtime dispatch uses the daemon's per-cycle `isIssueBlocked` checks, which are more dynamic.
- **You cannot un-stage an open minecart.** Once launched, a minecart cannot return to staged status. The `open → staged:*` transition is rejected.

## Key source files

| File | What it does |
|------|-------------|
| `internal/minecart/operations.go` | Core feeding: `CheckMinecartsForIssue`, `feedNextReadyIssue`, `IsSlingableType`, `isIssueBlocked` |
| `internal/daemon/minecart_manager.go` | `MinecartManager` goroutines: `runEventPoll` (5s), `runStrandedScan` (30s), `feedFirstReady` |
| `internal/cmd/minecart.go` | All `gt minecart` subcommands + `findStrandedMinecarts` type filter |
| `internal/cmd/sling.go` | Batch detection at ~242, auto-rig-resolution, deprecation warning |
| `internal/cmd/sling_batch.go` | `runBatchSling`, `resolveRigFromBeadIDs`, `allBeadIDs`, cross-rig guard |
| `internal/cmd/sling_minecart.go` | `createAutoMinecart`, `createBatchMinecart`, `printMinecartConflict` |
| `internal/cmd/minecart_stage.go` | `gt minecart stage`: DAG walking, wave computation, error/warning detection, staged minecart creation |
| `internal/cmd/minecart_launch.go` | `gt minecart launch`: status transition, Wave 1 dispatch via `dispatchWave1` |
| `internal/daemon/daemon.go` | Daemon startup -- creates `MinecartManager` at ~237 |
