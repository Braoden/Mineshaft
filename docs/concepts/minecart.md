# Minecarts

Minecarts are the primary unit for tracking batched work across rigs.

## Quick Start

```bash
# Create a minecart tracking some issues
gt minecart create "Feature X" gt-abc gt-def --notify boss

# Check progress
gt minecart status hq-cv-abc

# List active minecarts (the dashboard)
gt minecart list

# See all minecarts including landed ones
gt minecart list --all
```

## Concept

A **minecart** is a persistent tracking unit that monitors related issues across
multiple rigs. When you kick off work - even a single issue - a minecart tracks it
so you can see when it lands and what was included.

```
                 🚚 Minecart (hq-cv-abc)
                         │
            ┌────────────┼────────────┐
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │ gt-xyz  │  │ gt-def  │  │ bd-abc  │
       │ excavation │  │ excavation │  │  beads  │
       └────┬────┘  └────┬────┘  └────┬────┘
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │  nux    │  │ furiosa │  │  amber  │
       │(miner)│  │(miner)│  │(miner)│
       └─────────┘  └─────────┘  └─────────┘
                         │
                    "the swarm"
                    (ephemeral)
```

## Minecart vs Swarm

| Concept | Persistent? | ID | Description |
|---------|-------------|-----|-------------|
| **Minecart** | Yes | hq-cv-* | Tracking unit. What you create, track, get notified about. |
| **Swarm** | No | None | Ephemeral. "The workers currently on this minecart's issues." |
| **Stranded Minecart** | Yes | hq-cv-* | A minecart with ready work but no miners assigned. Needs attention. |

When you "kick off a swarm", you're really:
1. Creating a minecart (the tracking unit)
2. Assigning miners to the tracked issues
3. The "swarm" is just those miners while they're working

When issues close, the minecart lands and notifies you. The swarm dissolves.

## Minecart Lifecycle

```
OPEN ──(all issues close)──► LANDED/CLOSED
  ↑                              │
  └──(add more issues)───────────┘
       (auto-reopens)
```

| State | Description |
|-------|-------------|
| `open` | Active tracking, work in progress |
| `closed` | All tracked issues closed, notification sent |

Adding issues to a closed minecart reopens it automatically.

## Commands

### Create a Minecart

```bash
# Track multiple issues across rigs
gt minecart create "Deploy v2.0" gt-abc bd-xyz --notify excavation/joe

# Track a single issue (still creates minecart for dashboard visibility)
gt minecart create "Fix auth bug" gt-auth-fix

# With default notification (from config)
gt minecart create "Feature X" gt-a gt-b gt-c
```

### Add Issues

```bash
# Add issues to existing minecart
gt minecart add hq-cv-abc gt-new-issue
gt minecart add hq-cv-abc gt-issue1 gt-issue2 gt-issue3

# Adding to closed minecart requires reopening first
bd update hq-cv-abc --status=open
gt minecart add hq-cv-abc gt-followup-fix
```

### Check Status

```bash
# Show issues and active workers (the swarm)
gt minecart status hq-abc

# All active minecarts (the dashboard)
gt minecart status
```

Example output:
```
🚚 hq-cv-abc: Deploy v2.0

  Status:    ●
  Progress:  2/4 completed
  Created:   2025-12-30T10:15:00-08:00

  Tracked Issues:
    ✓ gt-xyz: Update API endpoint [task]
    ✓ bd-abc: Fix validation [bug]
    ○ bd-ghi: Update docs [task]
    ○ gt-jkl: Deploy to prod [task]
```

### List Minecarts (Dashboard)

```bash
# Active minecarts (default) - the primary attention view
gt minecart list

# All minecarts including landed
gt minecart list --all

# Only landed minecarts
gt minecart list --status=closed

# JSON output
gt minecart list --json
```

Example output:
```
Minecarts

  🚚 hq-cv-w3nm6: Feature X ●
  🚚 hq-cv-abc12: Bug fixes ●

Use 'gt minecart status <id>' for detailed view.
```

## Notifications

When a minecart lands (all tracked issues closed), subscribers are notified:

```bash
# Explicit subscriber
gt minecart create "Feature X" gt-abc --notify excavation/joe

# Multiple subscribers
gt minecart create "Feature X" gt-abc --notify overseer/ --notify --human
```

Notification content:
```
🚚 Minecart Landed: Deploy v2.0 (hq-cv-abc)

Issues (3):
  ✓ gt-xyz: Update API endpoint
  ✓ gt-def: Add validation
  ✓ bd-abc: Update docs

Duration: 2h 15m
```

## Create from Epic

Auto-discover tracked issues from an existing epic's children. Useful when
a planning/decomposition tool has already structured work as an epic with
child implementation beads.

```bash
# Auto-discover children from epic
gt minecart create --from-epic gt-epic-abc

# Override the minecart name (defaults to epic title)
gt minecart create --from-epic gt-epic-abc "Custom minecart name"

# Combine with other flags
gt minecart create --from-epic gt-epic-abc --owned --merge=direct
```

**How it works:**
1. Verifies the given bead is an epic
2. BFS-walks the parent-child hierarchy to find slingable descendants
3. Creates a standard minecart (`hq-cv-*`) tracking all slingable children (task, bug, feature, chore)

Non-slingable types (sub-epics, decisions) are recursed into but never
tracked directly. Only leaf work items appear in the minecart.

## Auto-Minecart on Sling

When you sling a single issue without an existing minecart:

```bash
gt sling bd-xyz beads/amber
```

This auto-creates a minecart so all work appears in the dashboard:
1. Creates minecart: "Work: bd-xyz"
2. Tracks the issue
3. Assigns the miner

Even "swarm of one" gets minecart visibility.

## Cross-Rig Tracking

Minecarts live in town-level beads (`hq-cv-*` prefix) and can track issues from any rig:

```bash
# Track issues from multiple rigs
gt minecart create "Full-stack feature" \
  gt-frontend-abc \
  gt-backend-def \
  bd-docs-xyz
```

The `tracks` relation is:
- **Non-blocking**: doesn't affect issue workflow
- **Additive**: can add issues anytime
- **Cross-rig**: minecart in hq-*, issues in gt-*, bd-*, etc.

## Minecart vs Rig Status

| View | Scope | Shows |
|------|-------|-------|
| `gt minecart status [id]` | Cross-rig | Issues tracked by minecart + workers |
| `gt rig status <rig>` | Single rig | All workers in rig + their minecart membership |

Use minecarts for "what's the status of this batch of work?"
Use rig status for "what's everyone in this rig working on?"

## See Also

- [Propulsion Principle](propulsion-principle.md) - Worker execution model
- [Mail Protocol](../design/mail-protocol.md) - Notification delivery
