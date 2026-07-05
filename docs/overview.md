# Understanding Mineshaft

This document provides a conceptual overview of Mineshaft's architecture, focusing on
the role taxonomy and how different agents interact.

## Why Mineshaft Exists

As AI agents become central to engineering workflows, teams face new challenges:

- **Accountability:** Who did what? Which agent introduced this bug?
- **Quality:** Which agents are reliable? Which need tuning?
- **Efficiency:** How do you route work to the right agent?
- **Scale:** How do you coordinate agents across repos and teams?

Mineshaft is an orchestration layer that treats AI agent work as structured data.
Every action is attributed. Every agent has a track record. Every piece of work
has provenance. See [Why These Features](why-these-features.md) for the full rationale,
and [Glossary](glossary.md) for terminology.

## Role Taxonomy

Mineshaft has several agent types, each with distinct responsibilities and lifecycles.

### Infrastructure Roles

These roles manage the Mineshaft system itself:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Overseer** | Global coordinator at overseer/ | Singleton, persistent |
| **Supervisor** | Background supervisor daemon ([watchdog chain](design/watchdog-chain.md)) | Singleton, persistent |
| **Witness** | Per-rig miner lifecycle manager | One per rig, persistent |
| **Refinery** | Per-rig merge queue processor | One per rig, persistent |

### Worker Roles

These roles do actual project work:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Miner** | Worker with persistent identity, ephemeral sessions | Witness-managed ([details](concepts/miner-lifecycle.md)) |
| **Crew** | Persistent worker with own clone | Long-lived, user-managed |
| **Dog** | Supervisor helper for infrastructure tasks | Persistent identity, Supervisor-managed |

## Minecarts: Tracking Work

A **minecart** (🚚) is how you track batched work in Mineshaft. When you kick off work -
even a single issue - create a minecart to track it.

```bash
# Create a minecart tracking some issues
gt minecart create "Feature X" gt-abc gt-def --notify boss

# Check progress
gt minecart status hq-cv-abc

# Dashboard of active minecarts
gt minecart list
```

**Why minecarts matter:**
- Single view of "what's in flight"
- Cross-rig tracking (minecart in hq-*, issues in gt-*, bd-*)
- Auto-notification when work lands
- Historical record of completed work (`gt minecart list --all`)

The "swarm" is the set of workers currently assigned to a minecart's issues.
When issues close, the minecart lands. See [Minecarts](concepts/minecart.md) for details.

## Crew vs Miners

Both do project work, but with key differences:

| Aspect | Crew | Miner |
|--------|------|---------|
| **Lifecycle** | Persistent (user controls) | Transient (Witness controls) |
| **Monitoring** | None | Witness watches, nudges, recycles |
| **Work assignment** | Human-directed or self-assigned | Slung via `gt sling` |
| **Git state** | Pushes to main directly | Works on branch, Refinery merges |
| **Cleanup** | Manual | Automatic on completion |
| **Identity** | `<rig>/crew/<name>` | `<rig>/miners/<name>` |

**When to use Crew**:
- Exploratory work
- Long-running projects
- Work requiring human judgment
- Tasks where you want direct control

**When to use Miners**:
- Discrete, well-defined tasks
- Batch work (tracked via minecarts)
- Parallelizable work
- Work that benefits from supervision

## Dogs vs Crew

**Dogs are NOT workers**. This is a common misconception.

| Aspect | Dogs | Crew |
|--------|------|------|
| **Owner** | Supervisor | Human |
| **Purpose** | Infrastructure tasks | Project work |
| **Scope** | Narrow, focused utilities | General purpose |
| **Lifecycle** | Very short (single task) | Long-lived |
| **Example** | Boot (triages Supervisor health) | Joe (fixes bugs, adds features) |

Dogs are the Supervisor's helpers for system-level tasks:
- **Boot**: Triages Supervisor health on daemon tick
- Future dogs might handle: log rotation, health checks, etc.

If you need to do work in another rig, use **worktrees**, not dogs.

## Cross-Rig Work Patterns

When a crew member needs to work on another rig:

### Option 1: Worktrees (Preferred)

Create a worktree in the target rig:

```bash
# mineshaft/crew/joe needs to fix a beads bug
gt worktree beads
# Creates ~/gt/beads/crew/mineshaft-joe/
# Identity preserved: BD_ACTOR = mineshaft/crew/joe
```

Directory structure:
```
~/gt/beads/crew/mineshaft-joe/     # joe from mineshaft working on beads
~/gt/mineshaft/crew/beads-wolf/    # wolf from beads working on mineshaft
```

### Option 2: Dispatch to Local Workers

For work that should be owned by the target rig:

```bash
# Create issue in target rig
bd create --repo beads "Fix authentication bug"

# Create minecart and sling to target rig
gt minecart create "Auth fix" bd-xyz
gt sling bd-xyz beads
```

### When to Use Which

| Scenario | Approach |
|----------|----------|
| You need to fix something quick | Worktree |
| Work should appear in your CV | Worktree |
| Work should be done by target rig team | Dispatch |
| Infrastructure/system task | Let Supervisor handle it |

## Directory Structure

The town root (`~/gt/`) contains infrastructure directories (`overseer/`, `supervisor/`)
and per-project rigs. Each rig holds a bare repo (`.repo.git/`), a canonical beads
database (`overseer/rig/.beads/`), and agent directories (`witness/`, `refinery/`,
`crew/`, `miners/`).

> For the full directory tree, see [architecture.md](design/architecture.md).

## Identity and Attribution

All work is attributed to the actor who performed it:

```
Git commits:      Author: mineshaft/crew/joe <owner@example.com>
Beads issues:     created_by: mineshaft/crew/joe
Events:           actor: mineshaft/crew/joe
```

Identity is preserved even when working cross-rig:
- `mineshaft/crew/joe` working in `~/gt/beads/crew/mineshaft-joe/`
- Commits still attributed to `mineshaft/crew/joe`
- Work appears on joe's CV, not beads rig's workers

## The Propulsion Principle

All Mineshaft agents follow the same core principle:

> **If you find something on your hook, YOU RUN IT.**

This applies regardless of role. The hook is your assignment. Execute it immediately
without waiting for confirmation. Mineshaft is a steam engine - agents are pistons.

## Model Evaluation and A/B Testing

Mineshaft's attribution system enables objective model comparison by tracking
completion time, quality signals, and revision count per agent. Deploy different
models on similar tasks and compare outcomes with `bd stats`.

See [Why These Features](why-these-features.md) for details on work history and
capability-based routing.

## Common Mistakes

1. **Using dogs for user work**: Dogs are Supervisor infrastructure. Use crew or miners.
2. **Confusing crew with miners**: Crew is persistent and human-managed. Miners are transient and Witness-managed.
3. **Working in wrong directory**: Mineshaft uses cwd for identity detection. Stay in your home directory.
4. **Waiting for confirmation when work is hooked**: The hook IS your assignment. Execute immediately.
5. **Creating worktrees when dispatch is better**: If work should be owned by the target rig, dispatch it instead.
