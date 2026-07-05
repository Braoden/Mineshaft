# Agent Identity and Attribution

> Canonical format for agent identity in Mineshaft

## Why Identity Matters

When you deploy AI agents at scale, anonymous work creates real problems:

- **Debugging:** "The AI broke it" isn't actionable. *Which* AI?
- **Quality tracking:** You can't improve what you can't measure.
- **Compliance:** Auditors ask "who approved this code?" - you need an answer.
- **Performance management:** Some agents are better than others at certain tasks.

Mineshaft solves this with **universal attribution**: every action, every commit,
every bead update is linked to a specific agent identity. This enables work
history tracking, capability-based routing, and objective quality measurement.

## BD_ACTOR Format Convention

The `BD_ACTOR` environment variable identifies agents in slash-separated path format.
This is set automatically when agents are spawned and used for all attribution.

### Format by Role Type

| Role Type | Format | Example |
|-----------|--------|---------|
| **Overseer** | `overseer` | `overseer` |
| **Supervisor** | `supervisor` | `supervisor` |
| **Witness** | `{rig}/witness` | `mineshaft/witness` |
| **Refinery** | `{rig}/refinery` | `mineshaft/refinery` |
| **Crew** | `{rig}/crew/{name}` | `mineshaft/crew/joe` |
| **Miner** | `{rig}/miners/{name}` | `mineshaft/miners/toast` |

### Why Slashes?

The slash format mirrors filesystem paths and enables:
- Hierarchical parsing (extract rig, role, name)
- Consistent mail addressing (`ms mail send mineshaft/witness`)
- Path-like routing in beads operations
- Visual clarity about agent location

## Attribution Model

Mineshaft uses three fields for complete provenance:

### Git Commits

```bash
GIT_AUTHOR_NAME="mineshaft/crew/joe"      # Who did the work (agent)
GIT_AUTHOR_EMAIL="steve@example.com"    # Who owns the work (boss)
```

Result in git log:
```
abc123 Fix bug (mineshaft/crew/joe <steve@example.com>)
```

**Interpretation**:
- The agent `mineshaft/crew/joe` authored the change
- The work belongs to the workspace owner (`steve@example.com`)
- Both are preserved in git history forever

### Beads Records

```json
{
  "id": "ms-xyz",
  "created_by": "mineshaft/crew/joe",
  "updated_by": "mineshaft/witness"
}
```

The `created_by` field is populated from `BD_ACTOR` when creating beads.
The `updated_by` field tracks who last modified the record.

### Event Logging

All events include actor attribution:

```json
{
  "ts": "2025-01-15T10:30:00Z",
  "type": "sling",
  "actor": "mineshaft/crew/joe",
  "payload": { "bead": "ms-xyz", "target": "mineshaft/miners/toast" }
}
```

## Environment Setup

Mineshaft uses a centralized `config.AgentEnv()` function to set environment
variables consistently across all agent spawn paths (managers, daemon, boot).

### Example: Miner Environment

```bash
# Set automatically for miner 'toast' in rig 'mineshaft'
export MS_ROLE="miner"
export MS_RIG="mineshaft"
export MS_MINER="toast"
export BD_ACTOR="mineshaft/miners/toast"
export GIT_AUTHOR_NAME="mineshaft/miners/toast"
export MS_ROOT="/home/user/ms"
export BEADS_DIR="/home/user/ms/mineshaft/.beads"
export BEADS_AGENT_NAME="mineshaft/toast"
```

### Example: Crew Environment

```bash
# Set automatically for crew member 'joe' in rig 'mineshaft'
export MS_ROLE="crew"
export MS_RIG="mineshaft"
export MS_CREW="joe"
export BD_ACTOR="mineshaft/crew/joe"
export GIT_AUTHOR_NAME="mineshaft/crew/joe"
export MS_ROOT="/home/user/ms"
export BEADS_DIR="/home/user/ms/mineshaft/.beads"
export BEADS_AGENT_NAME="mineshaft/joe"
```

### Manual Override

For local testing or debugging:

```bash
export BD_ACTOR="mineshaft/crew/debug"
bd create --title="Test issue"  # Will show created_by: mineshaft/crew/debug
```

See [reference.md](reference.md#environment-variables) for the complete
environment variable reference.

## Identity Parsing

The format supports programmatic parsing:

```go
// identityToBDActor converts daemon identity to BD_ACTOR format
// Town level: overseer, supervisor
// Rig level: {rig}/witness, {rig}/refinery
// Workers: {rig}/crew/{name}, {rig}/miners/{name}
```

| Input | Parsed Components |
|-------|-------------------|
| `overseer` | role=overseer |
| `supervisor` | role=supervisor |
| `mineshaft/witness` | rig=mineshaft, role=witness |
| `mineshaft/refinery` | rig=mineshaft, role=refinery |
| `mineshaft/crew/joe` | rig=mineshaft, role=crew, name=joe |
| `mineshaft/miners/toast` | rig=mineshaft, role=miner, name=toast |

## Audit Queries

Attribution enables powerful audit queries:

```bash
# All work by an agent
bd audit --actor=mineshaft/crew/joe

# All work in a rig
bd audit --actor=mineshaft/*

# All miner work
bd audit --actor=*/miners/*

# Git history by agent
git log --author="mineshaft/crew/joe"
```

## Design Principles

1. **Agents are not anonymous** - Every action is attributed
2. **Work is owned, not authored** - Agent creates, boss owns
3. **Attribution is permanent** - Git commits preserve history
4. **Format is parseable** - Enables programmatic analysis
5. **Consistent across systems** - Same format in git, beads, events

## CV and Skill Accumulation

### Human Identity is Global

The global identifier is your **email** - it's already in every git commit. No separate "entity bead" needed.

```
steve@example.com                ← global identity (from git author)
├── Town A (home)                ← workspace
│   ├── mineshaft/crew/joe         ← agent executor
│   └── mineshaft/miners/toast   ← agent executor
└── Town B (work)                ← workspace
    └── acme/miners/nux        ← agent executor
```

### Agent vs Owner

| Field | Scope | Purpose |
|-------|-------|---------|
| `BD_ACTOR` | Local (town) | Agent attribution for debugging |
| `GIT_AUTHOR_EMAIL` | Global | Human identity for CV |
| `created_by` | Local | Who created the bead |
| `owner` | Global | Who owns the work |

**Agents execute. Humans own.** The miner name in `completed-by: mineshaft/miners/toast` is executor attribution. The CV credits the human owner (`steve@example.com`).

### Miners Have Persistent Identities

Miners have **persistent identities but ephemeral sessions**. Like employees who
clock in/out: each work session is fresh (new tmux, new worktree), but the identity
persists across sessions.

- **Identity (persistent)**: Agent bead, CV chain, work history
- **Session (ephemeral)**: Claude instance, context window
- **Sandbox (ephemeral)**: Git worktree, branch

Work credits the miner identity, enabling:
- Performance tracking per miner
- Capability-based routing (send Go work to miners with Go track records)
- Model comparison (A/B test different models via different miners)

See [miner-lifecycle.md](miner-lifecycle.md#miner-identity) for details.

### Skills Are Derived

Your CV emerges from querying work evidence:

```bash
# All work by owner (across all agents)
git log --author="steve@example.com"
bd list --owner=steve@example.com

# Skills derived from evidence
# - .go files touched → Go skill
# - issue tags → domain skills
# - commit patterns → activity types
```

### Multi-Town Aggregation

A human with multiple towns has one CV:

```bash
# Future: federated CV query
bd cv steve@example.com
# Discovers all towns, aggregates work, derives skills
```

See `~/ms/docs/hop/decisions/008-identity-model.md` for architectural rationale.

## Enterprise Use Cases

### Compliance and Audit

```bash
# Who touched this file in the last 90 days?
git log --since="90 days ago" -- path/to/sensitive/file.go

# All changes by a specific agent
bd audit --actor=mineshaft/miners/toast --since=2025-01-01
```

### Performance Tracking

```bash
# Completion rate by agent
bd stats --group-by=actor

# Average time to completion
bd stats --actor=mineshaft/miners/* --metric=cycle-time
```

### Model Comparison

When agents use different underlying models, attribution enables A/B comparison:

```bash
# Tag agents by model
# mineshaft/miners/claude-1 uses Claude
# mineshaft/miners/gpt-1 uses GPT-4

# Compare quality signals
bd stats --actor=mineshaft/miners/claude-* --metric=revision-count
bd stats --actor=mineshaft/miners/gpt-* --metric=revision-count
```

Lower revision counts suggest higher first-pass quality.
