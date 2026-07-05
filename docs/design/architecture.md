# Mineshaft Architecture

Technical architecture for Mineshaft multi-agent workspace management.

## Two-Level Beads Architecture

Mineshaft uses a two-level beads architecture to separate organizational coordination
from project implementation work.

| Level | Location | Prefix | Purpose |
|-------|----------|--------|---------|
| **Town** | `~/ms/.beads/` | `hq-*` | Cross-rig coordination, Overseer mail, agent identity |
| **Rig** | `<rig>/overseer/rig/.beads/` | project prefix | Implementation work, MRs, project issues |

### Town-Level Beads (`~/ms/.beads/`)

Organizational chain for cross-rig coordination:
- Overseer mail and messages
- Minecart coordination (batch work across rigs)
- Strategic issues and decisions
- **Town-level agent beads** (Overseer, Supervisor)
- **Role definition beads** (global templates)

### Rig-Level Beads (`<rig>/overseer/rig/.beads/`)

Project chain for implementation work:
- Bugs, features, tasks for the project
- Merge requests and code reviews
- Project-specific molecules
- **Rig-level agent beads** (Witness, Refinery, Miners)

## Agent Bead Storage

Agent beads track lifecycle state for each agent. Storage location depends on
the agent's scope.

| Agent Type | Scope | Bead Location | Bead ID Format |
|------------|-------|---------------|----------------|
| Overseer | Town | `~/ms/.beads/` | `hq-overseer` |
| Supervisor | Town | `~/ms/.beads/` | `hq-supervisor` |
| Boot | Town | `~/ms/.beads/` | `hq-boot` |
| Dogs | Town | `~/ms/.beads/` | `hq-dog-<name>` |
| Witness | Rig | `<rig>/.beads/` | `<prefix>-<rig>-witness` |
| Refinery | Rig | `<rig>/.beads/` | `<prefix>-<rig>-refinery` |
| Miners | Rig | `<rig>/.beads/` | `<prefix>-<rig>-miner-<name>` |
| Crew | Rig | `<rig>/.beads/` | `<prefix>-<rig>-crew-<name>` |

### Role Beads

Role beads are global templates stored in town beads with `hq-` prefix:
- `hq-overseer-role` - Overseer role definition
- `hq-supervisor-role` - Supervisor role definition
- `hq-boot-role` - Boot role definition
- `hq-witness-role` - Witness role definition
- `hq-refinery-role` - Refinery role definition
- `hq-miner-role` - Miner role definition
- `hq-crew-role` - Crew role definition
- `hq-dog-role` - Dog role definition

Each agent bead references its role bead via the `role_bead` field.

## Agent Taxonomy

### Town-Level Agents (Cross-Rig)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Overseer** | Global coordinator, handles cross-rig communication and escalations | Persistent |
| **Supervisor** | Daemon beacon — receives heartbeats, runs plugins and monitoring | Persistent |
| **Boot** | Supervisor watchdog — spawned by daemon for triage decisions when Supervisor is down | Ephemeral |
| **Dogs** | Long-running workers for cross-rig batch work | Variable |

### Rig-Level Agents (Per-Project)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Witness** | Monitors miner health, handles nudging and cleanup | Persistent |
| **Refinery** | Processes merge queue, runs verification | Persistent |
| **Miners** | Workers with persistent identity, assigned to specific issues | Persistent identity, ephemeral sessions |
| **Crew** | Human workspaces — full git clones, user-managed lifecycle | Persistent |

## Directory Structure

```
~/ms/                           Town root
├── .beads/                     Town-level beads (hq-* prefix)
│   ├── metadata.json           Beads config (dolt_mode, dolt_database)
│   └── routes.jsonl            Prefix → rig routing table
├── .dolt-data/                 Centralized Dolt data directory
│   ├── hq/                     Town beads database (hq-* prefix)
│   ├── mineshaft/                Mineshaft rig database (ms-* prefix)
│   ├── beads/                  Beads rig database (bd-* prefix)
│   └── <other rigs>/           Per-rig databases
├── daemon/                     Daemon runtime state
│   ├── dolt-state.json         Dolt server state (pid, port, databases)
│   ├── dolt-server.log         Server log
│   └── dolt.pid                Server PID file
├── supervisor/                     Supervisor workspace
│   └── dogs/<name>/            Dog worker directories
├── overseer/                      Overseer agent home
│   ├── town.json               Town configuration
│   ├── rigs.json               Rig registry
│   ├── daemon.json             Daemon patrol config
│   └── accounts.json           Claude Code account management
├── settings/                   Town-level settings
│   ├── config.json             Town settings (agents, themes)
│   └── escalation.json         Escalation routes and contacts
├── directives/                 Town-level role directives (operator policy)
│   └── <role>.md               Markdown injected at prime time
├── formula-overlays/           Town-level formula overlays
│   └── <formula>.toml          TOML step overrides (replace/append/skip)
├── config/
│   └── messaging.json          Mail lists, queues, channels
└── <rig>/                      Project container (NOT a git clone)
    ├── config.json             Rig identity and beads prefix
    ├── directives/             Rig-level role directives (overrides town)
    │   └── <role>.md
    ├── formula-overlays/       Rig-level formula overlays (full precedence)
    │   └── <formula>.toml
    ├── overseer/rig/              Canonical clone (beads live here, NOT an agent)
    │   └── .beads/             Rig-level beads (redirected to Dolt)
    ├── refinery/               Refinery agent home
    │   └── rig/                Worktree from overseer/rig
    ├── witness/                Witness agent home (no clone)
    ├── crew/                   Crew parent
    │   └── <name>/             Human workspaces (full clones)
    └── miners/               Miners parent
        └── <name>/<rigname>/   Worker worktrees from overseer/rig
```

**Note**: No per-directory CLAUDE.md or AGENTS.md is created. Only `~/ms/CLAUDE.md`
(town-root identity anchor) exists on disk. Full context is injected by `ms prime`
via SessionStart hook.

### Worktree Architecture

Miners and refinery are git worktrees, not full clones. This enables fast spawning
and shared object storage. The worktree base is `overseer/rig`:

```go
// From miner/manager.go - worktrees are based on overseer/rig
git worktree add -b miner/<name>-<timestamp> miners/<name>
```

Crew workspaces (`crew/<name>/`) are full git clones for human developers who need
independent repos. Miner sessions are ephemeral and benefit from worktree efficiency.

## Storage Layer: Dolt SQL Server

All beads data is stored in a single Dolt SQL Server process per town. There is
no embedded Dolt fallback — if the server is down, `bd` fails fast with a clear
error pointing to `ms dolt start`.

```
┌─────────────────────────────────┐
│  Dolt SQL Server (per town)     │
│  Port 3307, managed by daemon   │
│  Data: ~/ms/.dolt-data/         │
└──────────┬──────────────────────┘
           │ MySQL protocol
    ┌──────┼──────┬──────────┐
    │      │      │          │
  USE hq  USE mineshaft  USE beads  ...
```

Each rig database is a subdirectory under `.dolt-data/`. The daemon monitors
the server on every heartbeat and auto-restarts on crash.

For write concurrency, all agents write directly to `main` using transaction
discipline (`BEGIN` / `DOLT_COMMIT` / `COMMIT` atomically). This eliminates
branch proliferation and ensures immediate cross-agent visibility.

See [dolt-storage.md](dolt-storage.md) for full details.

## Beads Routing

The `routes.jsonl` file maps issue ID prefixes to rig locations (relative to town root):

```jsonl
{"prefix":"hq-","path":"."}
{"prefix":"ms-","path":"mineshaft/overseer/rig"}
{"prefix":"bd-","path":"beads/overseer/rig"}
```

Routes point to `overseer/rig` because that's where the canonical `.beads/` lives.
This enables transparent cross-rig beads operations:

```bash
bd show hq-overseer    # Routes to town beads (~/.ms/.beads)
bd show ms-xyz      # Routes to mineshaft/overseer/rig/.beads
```

## Beads Redirects

Worktrees (miners, refinery, crew) don't have their own beads databases. Instead,
they use a `.beads/redirect` file that points to the canonical beads location:

```
miners/alpha/.beads/redirect → ../../overseer/rig/.beads
refinery/rig/.beads/redirect   → ../../overseer/rig/.beads
```

`ResolveBeadsDir()` follows redirect chains (max depth 3) with circular detection.
This ensures all agents in a rig share a single beads database via the Dolt server.

## Merge Queue: Batch-then-Bisect

The refinery processes MRs through a batch-then-bisect merge queue (Bors-style).
This is a core capability, not a pluggable strategy.

### How It Works

```
MRs waiting:  [A, B, C, D]
                    ↓
Batch:        Rebase A..D as a stack on main
                    ↓
Test tip:     Run tests on D (tip of stack)
                    ↓
If PASS:      Fast-forward merge all 4 → done
If FAIL:      Binary bisect → test B (midpoint)
                    ↓
              If B passes: C or D broke it → bisect [C,D]
              If B fails:  A or B broke it → bisect [A,B]
```

### Implementation Phases

| Phase | Bead | What | Status |
|-------|------|------|--------|
| 1: GatesParallel | ms-8b2i | Run test + lint concurrently per MR | In progress |
| 2: Batch-then-bisect | ms-i2vm | Bors-style batching with binary bisect | Blocked by Phase 1 |
| 3: Pre-verification | ms-lu84 | Miners run tests before MR submission | Blocked by Phase 2 |

Gates (test command, lint, etc.) are pluggable. The batching strategy is core.

Design doc: produced by ms-yxx0 review.

## Miner Lifecycle: Self-Managed Completion

Miners manage their own lifecycle end-to-end. The Witness observes but does NOT
gate completion. This prevents the Witness from becoming a bottleneck.

### Miner Completion Flow

```
Miner finishes work
  → Push branch to remote
  → Submit MR (bd update --mr-ready)
  → Update bead status
  → Tear down worktree
  → Go idle (available for next assignment)
```

The Witness monitors for stuck/zombie miners (no activity for extended period)
and nudges or escalates. It does NOT process completion — that's the miner's job.

Design bead: ms-0wkk.

## Data Plane Lifecycle

All beads data flows through a six-stage lifecycle managed by Dogs:

```
CREATE → LIVE → CLOSE → DECAY → COMPACT → FLATTEN
  │        │       │        │        │          │
  Dolt   active   done   DELETE   REBASE     SQUASH
  commit  work    bead    rows    commits    all history
                         >7-30d  together   to 1 commit
```

Stages 1-3 are automated today. Stages 4-6 are being shipped via Dog automation
(ms-at0i Reaper DELETE, ms-l8dc Compactor REBASE, ms-emm4 Doctor gc).

See [dolt-storage.md](dolt-storage.md) for full details.

## Deployment Artifacts

Mineshaft and Beads are distributed through multiple channels. Tag pushes (`v*`)
trigger GitHub Actions release workflows that build and publish everything.

### Mineshaft (`ms`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries (darwin/linux/windows, amd64/arm64) + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/mineshaft/ms` — formula auto-updated on release | `update-homebrew` job pushes to `steveyegge/homebrew-mineshaft` |
| **npm** | `npx @mineshaft/ms` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **Local build** | `go build -o $(go env GOPATH)/bin/ms ./cmd/ms` | Manual |

### Beads (`bd`)

| Channel | Artifact | Trigger |
|---------|----------|---------|
| **GitHub Releases** | Platform binaries + checksums | GoReleaser on tag push |
| **Homebrew** | `brew install steveyegge/beads/bd` | `update-homebrew` job |
| **npm** | `npx @beads/bd` — wrapper that downloads the correct binary | OIDC trusted publishing (no token) |
| **PyPI** | `beads-mcp` — MCP server integration | `publish-pypi` job with `PYPI_API_TOKEN` secret |
| **Local build** | `go build -o $(go env GOPATH)/bin/bd ./cmd/bd` | Manual |

### npm Authentication

Both repos use **OIDC trusted publishing** — no `NPM_TOKEN` secret needed.
Authentication is handled by GitHub's OIDC provider. The workflow needs:

```yaml
permissions:
  id-token: write  # Required for npm trusted publishing
```

Configure on npmjs.com: Package Settings → Trusted Publishers → link to the
GitHub repo and `release.yml` workflow file.

### What the binary embeds

The Go binary is the primary distribution vehicle. It embeds:
- **Role templates** — Agent priming context, served by `ms prime`
- **Formula definitions** — Workflow molecules, served by `bd mol`
- **Doctor checks** — Health diagnostics, including migration checks
- **Default configs** — `daemon.json` lifecycle defaults, operational thresholds

This means upgrading the binary automatically propagates most fixes. Files that
are NOT embedded (and require `ms doctor` or `ms upgrade` to update):
- Town-root `CLAUDE.md` (created at `ms install` time)
- `daemon.json` patrol entries (created at install, extended by `EnsureLifecycleDefaults`)
- Claude Code hooks (`.claude/settings.json` managed sections)
- Dolt schema (migrations run on first `bd` command after upgrade)

## Role Directives and Formula Overlays

Operators can customize agent behavior at the town or rig level without
modifying the Go binary or embedded templates. This follows the property layer
model (rig > town > system) and the hooks override precedent.

### Role Directives

Per-role Markdown files injected during `ms prime`, after the role template but
before context files and handoff content. Operator policy that overrides formula
instructions where they conflict.

```
~/ms/directives/<role>.md              # Town-level (all rigs)
~/ms/<rig>/directives/<role>.md        # Rig-level
```

Both levels concatenate (rig content appears last and wins conflicts).
Implemented in `internal/config/directives.go` (`LoadRoleDirective`),
integrated via `outputRoleDirectives()` in `internal/cmd/prime_output.go`.

### Formula Overlays

Per-formula TOML files that modify individual steps. Applied post-parse before
rendering in `showFormulaStepsFull()`.

```
~/ms/formula-overlays/<formula>.toml   # Town-level
~/ms/<rig>/formula-overlays/<formula>.toml  # Rig-level (full precedence)
```

Rig-level overlays fully replace town-level (not merged). Three override modes:

| Mode | Effect |
|------|--------|
| `replace` | Swap the step description entirely |
| `append` | Add text after the existing step description |
| `skip` | Remove the step (dependents inherit its needs) |

Implemented in `internal/formula/overlay.go` (`LoadFormulaOverlay`,
`ApplyOverlays`). `ms doctor` validates overlay step IDs against current
formula definitions and can auto-fix stale references.

See [directives-and-overlays.md](directives-and-overlays.md) for the full
reference with examples and design rationale.

## See Also

- [dolt-storage.md](dolt-storage.md) - Dolt storage architecture
- [reference.md](../reference.md) - Command reference
- [directives-and-overlays.md](directives-and-overlays.md) - Directives and overlays reference
- [molecules.md](../concepts/molecules.md) - Workflow molecules
- [identity.md](../concepts/identity.md) - Agent identity and BD_ACTOR
