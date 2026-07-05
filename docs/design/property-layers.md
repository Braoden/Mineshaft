# Property Layers: Multi-Level Configuration

> Implementation guide for Mineshaft's configuration system.
> Created: 2025-01-06

## Overview

Mineshaft uses a layered property system for configuration. Properties are
looked up through multiple layers, with earlier layers overriding later ones.
This enables both local control and global coordination.

## The Four Layers

```
┌─────────────────────────────────────────────────────────────┐
│ 1. WISP LAYER (transient, town-local)                       │
│    Location: <rig>/.beads-wisp/config/                      │
│    Synced: Never                                            │
│    Use: Temporary local overrides                           │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. RIG BEAD LAYER (persistent, synced globally)             │
│    Location: <rig>/.beads/ (rig identity bead labels)       │
│    Synced: Via git (all clones see it)                      │
│    Use: Project-wide operational state                      │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. TOWN DEFAULTS                                            │
│    Location: ~/ms/config.json or ~/ms/.beads/               │
│    Synced: N/A (per-town)                                   │
│    Use: Town-wide policies                                  │
└─────────────────────────────┬───────────────────────────────┘
                              │ if missing
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. SYSTEM DEFAULTS (compiled in)                            │
│    Use: Fallback when nothing else specified                │
└─────────────────────────────────────────────────────────────┘
```

## Lookup Behavior

### Override Semantics (Default)

For most properties, the first non-nil value wins:

```go
func GetConfig(key string) interface{} {
    if val := wisp.Get(key); val != nil {
        if val == Blocked { return nil }
        return val
    }
    if val := rigBead.GetLabel(key); val != nil {
        return val
    }
    if val := townDefaults.Get(key); val != nil {
        return val
    }
    return systemDefaults[key]
}
```

### Stacking Semantics (Integers)

For integer properties, values from wisp and bead layers **add** to the base:

```go
func GetIntConfig(key string) int {
    base := getBaseDefault(key)    // Town or system default
    beadAdj := rigBead.GetInt(key) // 0 if missing
    wispAdj := wisp.GetInt(key)    // 0 if missing
    return base + beadAdj + wispAdj
}
```

This enables temporary adjustments without changing the base value.

### Blocking Inheritance

You can explicitly block a property from being inherited:

```bash
ms rig config set mineshaft auto_restart --block
```

This creates a "blocked" marker in the wisp layer. Even if the rig bead
or defaults say `auto_restart: true`, the lookup returns nil.

## Rig Identity Beads

Each rig has an identity bead for operational state:

```yaml
id: ms-rig-mineshaft
type: rig
name: mineshaft
repo: git@github.com:steveyegge/mineshaft.git
prefix: ms

labels:
  - status:operational
  - priority:normal
```

These beads sync via git, so all clones of the rig see the same state.

## Two-Level Rig Control

### Level 1: Park (Local, Ephemeral)

```bash
ms rig park mineshaft      # Stop services, daemon won't restart
ms rig unpark mineshaft    # Allow services to run
```

- Stored in wisp layer (`.beads-wisp/config/`)
- Only affects this town
- Disappears on cleanup
- Use: Local maintenance, debugging

### Level 2: Dock (Global, Persistent)

```bash
ms rig dock mineshaft      # Set status:docked label on rig bead
ms rig undock mineshaft    # Remove label
```

- Stored on rig identity bead
- Syncs to all clones via git
- Permanent until explicitly changed
- Use: Project-wide maintenance, coordinated downtime

### Daemon Behavior

The daemon checks both levels before auto-restarting:

```go
func shouldAutoRestart(rig *Rig) bool {
    status := rig.GetConfig("status")
    if status == "parked" || status == "docked" {
        return false
    }
    return true
}
```

## Configuration Keys

| Key | Type | Behavior | Description |
|-----|------|----------|-------------|
| `status` | string | Override | operational/parked/docked |
| `auto_restart` | bool | Override | Daemon auto-restart behavior |
| `max_miners` | int | Override | Maximum concurrent miners |
| `priority_adjustment` | int | **Stack** | Scheduling priority modifier |
| `maintenance_window` | string | Override | When maintenance allowed |
| `dnd` | bool | Override | Do not disturb mode |

## Commands

### View Configuration

```bash
ms rig config show mineshaft           # Show effective config (all layers)
ms rig config show mineshaft --layer   # Show which layer each value comes from
```

### Set Configuration

```bash
# Set in wisp layer (local, ephemeral)
ms rig config set mineshaft key value

# Set in bead layer (global, permanent)
ms rig config set mineshaft key value --global

# Block inheritance
ms rig config set mineshaft key --block

# Clear from wisp layer
ms rig config unset mineshaft key
```

### Rig Lifecycle

```bash
ms rig park mineshaft          # Local: stop + prevent restart
ms rig unpark mineshaft        # Local: allow restart

ms rig dock mineshaft          # Global: mark as offline
ms rig undock mineshaft        # Global: mark as operational

ms rig status mineshaft        # Show current state
```

## Examples

### Temporary Priority Boost

```bash
# Base priority: 0 (from defaults)
# Give this rig temporary priority boost for urgent work

ms rig config set mineshaft priority_adjustment 10

# Effective priority: 0 + 10 = 10
# When done, clear it:

ms rig config unset mineshaft priority_adjustment
```

### Local Maintenance

```bash
# I'm upgrading the local clone, don't restart services
ms rig park mineshaft

# ... do maintenance ...

ms rig unpark mineshaft
```

### Project-Wide Maintenance

```bash
# Major refactor in progress, all clones should pause
ms rig dock mineshaft

# Syncs via git - other towns see the rig as docked
bd sync

# When done:
ms rig undock mineshaft
bd sync
```

### Block Auto-Restart Locally

```bash
# Rig bead says auto_restart: true
# But I'm debugging and don't want that here

ms rig config set mineshaft auto_restart --block

# Now auto_restart returns nil for this town only
```

## Implementation Notes

### Wisp Storage

Wisp config stored in `.beads-wisp/config/<rig>.json`:

```json
{
  "rig": "mineshaft",
  "values": {
    "status": "parked",
    "priority_adjustment": 10
  },
  "blocked": ["auto_restart"]
}
```

### Rig Bead Labels

Rig operational state stored as labels on the rig identity bead:

```bash
bd label add ms-rig-mineshaft status:docked
bd label remove ms-rig-mineshaft status:docked
```

### Daemon Integration

The daemon's lifecycle manager checks config before starting services:

```go
func (d *Daemon) maybeStartRigServices(rig string) {
    r := d.getRig(rig)

    status := r.GetConfig("status")
    if status == "parked" || status == "docked" {
        log.Info("Rig %s is offline, skipping auto-start", rig)
        return
    }

    d.ensureWitness(rig)
    d.ensureRefinery(rig)
}
```

## Operational State Events

Operational state changes are tracked as event beads, providing an immutable audit
trail. Labels cache the current state for fast queries.

### Event Types

| Event Type | Description | Payload |
|------------|-------------|---------|
| `patrol.muted` | Patrol cycle disabled | `{reason, until?}` |
| `patrol.unmuted` | Patrol cycle re-enabled | `{reason?}` |
| `agent.started` | Agent session began | `{session_id?}` |
| `agent.stopped` | Agent session ended | `{reason, outcome?}` |
| `mode.degraded` | System entered degraded mode | `{reason}` |
| `mode.normal` | System returned to normal | `{}` |

### Creating and Querying Events

```bash
# Create operational event
bd create --type=event --event-type=patrol.muted \
  --actor=human:boss --target=agent:supervisor \
  --payload='{"reason":"fixing minecart deadlock","until":"ms-abc1"}'

# Query recent events for an agent
bd list --type=event --target=agent:supervisor --limit=10

# Query current state via labels
bd list --type=role --label=patrol:muted
```

### Labels-as-State Pattern

Events capture the full history. Labels cache the current state:

- `patrol:muted` / `patrol:active`
- `mode:degraded` / `mode:normal`
- `status:idle` / `status:working`

State change flow: create event bead (immutable), then update role bead labels (cache).

```bash
# Mute patrol
bd create --type=event --event-type=patrol.muted ...
bd update role-supervisor --add-label=patrol:muted --remove-label=patrol:active
```

### Configuration vs State

| Type | Storage | Example |
|------|---------|---------|
| **Static config** | TOML files | Daemon tick interval |
| **Role directives** | Markdown files | Operator behavioral policy per role |
| **Formula overlays** | TOML files | Per-step formula modifications |
| **Operational state** | Beads (events + labels) | Patrol muted |
| **Runtime flags** | Marker files | `.supervisor-disabled` |

*Events are the source of truth. Labels are the cache.*

For Boot triage and degraded mode details, see [Watchdog Chain](watchdog-chain.md).

## Role Directives and Formula Overlays

Directives and overlays extend the property layer model to agent behavior.
They follow the same rig > town > system precedence as other config.

### Directives (Behavioral Policy)

Per-role Markdown files that modify agent behavior at prime time:

```
SYSTEM LAYER:   Embedded role template (compiled in)
                        │ if directive exists
                        ▼
TOWN LAYER:     ~/ms/directives/<role>.md
                        │ concatenated with
                        ▼
RIG LAYER:      ~/ms/<rig>/directives/<role>.md
```

Both town and rig directives concatenate. Rig content appears last and wins
conflicts (same as CSS specificity — later rules override earlier ones).

### Overlays (Formula Modifications)

Per-formula TOML files that modify individual steps:

```
SYSTEM LAYER:   Embedded formula (compiled in)
                        │ if overlay exists
                        ▼
TOWN LAYER:     ~/ms/formula-overlays/<formula>.toml
                        │ rig replaces town entirely
                        ▼
RIG LAYER:      ~/ms/<rig>/formula-overlays/<formula>.toml
```

Unlike directives, overlays use **full replacement** at the rig level — if a
rig overlay exists, the town overlay is ignored entirely. This prevents
conflicting step modifications from merging unpredictably.

### Precedence Summary

| Config Type | Town + Rig Interaction | Rationale |
|-------------|----------------------|-----------|
| Rig properties | First non-nil wins (override) | Standard config lookup |
| Integer properties | Values stack (additive) | Allows adjustments |
| Role directives | Concatenate (rig last) | Additive policy; rig gets last word |
| Formula overlays | Rig replaces town | Step mods can conflict; full replacement is safer |

See [directives-and-overlays.md](directives-and-overlays.md) for the full
reference with TOML format, examples, and `ms doctor` integration.

## Related Documents

- `~/ms/docs/hop/PROPERTY-LAYERS.md` - Strategic architecture
- `wisp-architecture.md` - Wisp system design
- `agent-as-bead.md` - Agent identity beads (similar pattern)
- [directives-and-overlays.md](directives-and-overlays.md) - Full reference
