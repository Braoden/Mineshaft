# Mineshaft

> Mineshaft is a fork and reskin of [Gastown](https://github.com/gastownhall/gastown).

**Multi-agent orchestration system for Claude Code, GitHub Copilot, and other AI agents with persistent work tracking**

## Overview

Mineshaft is a workspace manager that lets you coordinate multiple AI coding agents (Claude Code, GitHub Copilot, Codex, Gemini, and others) working on different tasks. Instead of losing context when agents restart, Mineshaft persists work state in git-backed hooks, enabling reliable multi-agent workflows.

### What Problem Does This Solve?

| Challenge                       | Mineshaft Solution                            |
| ------------------------------- | -------------------------------------------- |
| Agents lose context on restart  | Work persists in git-backed hooks            |
| Manual agent coordination       | Built-in mailboxes, identities, and handoffs |
| 4-10 agents become chaotic      | Scale comfortably to 20-30 agents            |
| Work state lost in agent memory | Work state stored in Beads ledger            |

### Architecture

```mermaid
graph TB
    Overseer[The Overseer<br/>AI Coordinator]
    Town[Town Workspace<br/>~/ms/]

    Town --> Overseer
    Town --> Rig1[Rig: Project A]
    Town --> Rig2[Rig: Project B]

    Rig1 --> Crew1[Crew Member<br/>Your workspace]
    Rig1 --> Hooks1[Hooks<br/>Persistent storage]
    Rig1 --> Miners1[Miners<br/>Worker agents]

    Rig2 --> Crew2[Crew Member]
    Rig2 --> Hooks2[Hooks]
    Rig2 --> Miners2[Miners]

    Hooks1 -.git worktree.-> GitRepo1[Git Repository]
    Hooks2 -.git worktree.-> GitRepo2[Git Repository]

    style Overseer fill:#e1f5ff,color:#000000
    style Town fill:#f0f0f0,color:#000000
    style Rig1 fill:#fff4e1,color:#000000
    style Rig2 fill:#fff4e1,color:#000000
```

## Core Concepts

### The Overseer 🎩

Your primary AI coordinator. The Overseer is a Claude Code instance with full context about your workspace, projects, and agents. **Start here** - just tell the Overseer what you want to accomplish.

### Town 🏘️

Your workspace directory (e.g., `~/ms/`). Contains all projects, agents, and configuration.

### Rigs 🏗️

Project containers. Each rig wraps a git repository and manages its associated agents.

### Crew Members 👤

Your personal workspace within a rig. Where you do hands-on work.

### Miners 🦨

Worker agents with persistent identity but ephemeral sessions. Spawned for tasks, sessions end on completion, but identity and work history persist.

### Hooks 🪝

Git worktree-based persistent storage for agent work. Survives crashes and restarts.

### Minecarts 🚚

Work tracking units. Bundle multiple beads that get assigned to agents. Minecarts labeled `mountain` get autonomous stall detection and smart skip logic for epic-scale execution.

### Beads Integration 📿

Git-backed issue tracking system that stores work state as structured data.

**Bead IDs** (also called **issue IDs**) use a prefix + 5-character alphanumeric format (e.g., `ms-abc12`, `hq-x7k2m`). The prefix indicates the item's origin or rig. Commands like `ms sling` and `ms minecart` accept these IDs to reference specific work items. The terms "bead" and "issue" are used interchangeably—beads are the underlying data format, while issues are the work items stored as beads.

### Molecules 🧬

Workflow templates that coordinate multi-step work. Formulas (TOML definitions) are instantiated as molecules with tracked steps. Two modes: root-only wisps (steps materialized at runtime, lightweight) and poured wisps (steps materialized as sub-wisps with checkpoint recovery). See [Molecules](docs/concepts/molecules.md).

### Monitoring: Witness, Supervisor, Dogs 🐕

A three-tier watchdog system keeps agents healthy:

- **Witness** - Per-rig lifecycle manager. Monitors miners, detects stuck agents, triggers recovery, manages session cleanup.
- **Supervisor** - Background supervisor running continuous patrol cycles across all rigs.
- **Dogs** - Infrastructure workers dispatched by the Supervisor for maintenance tasks (e.g., Boot for triage).

### Refinery 🏭

Per-rig merge queue processor. When miners complete work via `ms done`, the Refinery batches merge requests, runs verification gates, and merges to main using a Bors-style bisecting queue. Failed MRs are isolated and either fixed inline or re-dispatched.

### Escalation 🚨

Severity-routed issue escalation. Agents that hit blockers escalate via `ms escalate`, which creates tracked beads routed through the Supervisor, Overseer, and (if needed) Boss. Severity levels: CRITICAL (P0), HIGH (P1), MEDIUM (P2). See [Escalation](docs/design/escalation.md).

### Scheduler ⏱️

Config-driven capacity governor for miner dispatch. Prevents API rate limit exhaustion by batching dispatch under configurable concurrency limits. Default is direct dispatch; set `scheduler.max_miners` to enable deferred dispatch with the daemon. See [Scheduler](docs/design/scheduler.md).

### Seance 👻

Session discovery and continuation. Discovers previous agent sessions via `.events.jsonl` logs, enabling agents to query their predecessors for context and decisions from earlier work.

```bash
ms seance                       # List discoverable predecessor sessions
ms seance --talk <id> -p "What did you find?"  # One-shot question
```

### Wasteland 🏜️

Federated work coordination network linking Mineshafts through DoltHub. Rigs post wanted items, claim work from other towns, submit completion evidence, and earn portable reputation via multi-dimensional stamps. See [Wasteland](docs/WASTELAND.md).

> **New to Mineshaft?** See the [Glossary](docs/glossary.md) for a complete guide to terminology and concepts.

## Installation

### Prerequisites

- **Go 1.25+** - [go.dev/dl](https://go.dev/dl/)
- **Git 2.25+** - for worktree support
- **Dolt 2.0.7+** - `brew install dolt` on macOS, or see [github.com/dolthub/dolt](https://github.com/dolthub/dolt)
- **beads (bd) 0.55.4+** - installed by `brew install mineshaft`, or see [github.com/steveyegge/beads](https://github.com/steveyegge/beads)
- **sqlite3** - for minecart database queries (usually pre-installed on macOS/Linux)
- **tmux 3.0+** - recommended for full experience
- **Claude Code CLI** (default runtime) - [claude.ai/code](https://claude.ai/code)
- **Codex CLI** (optional runtime) - [developers.openai.com/codex/cli](https://developers.openai.com/codex/cli)
- **GitHub Copilot CLI** (optional runtime) - [cli.github.com](https://cli.github.com) (requires Copilot seat)

### Setup (Docker-Compose below)

```bash
# Install Mineshaft
$ brew install mineshaft                                    # Homebrew (recommended)
$ npm install -g @mineshaft/ms                              # npm
$ go install github.com/steveyegge/mineshaft/cmd/ms@latest  # From source (Linux only)

# macOS: go install produces unsigned binaries that macOS will SIGKILL.
# Use brew install (above) or install Dolt and clone/build with make:
$ brew install dolt
$ git clone https://github.com/steveyegge/mineshaft.git && cd mineshaft
$ make build && mv ms $HOME/go/bin/

# Windows (or if go install fails): clone and build manually
$ git clone https://github.com/steveyegge/mineshaft.git && cd mineshaft
$ go build -o ms.exe ./cmd/ms
$ mv ms.exe $HOME/go/bin/  # or add mineshaft to PATH

# If using go install, add Go binaries to PATH (add to ~/.zshrc or ~/.bashrc)
export PATH="$PATH:$HOME/go/bin"

# Create workspace with git initialization
ms install ~/ms --git
cd ~/ms

# Add your first project
ms rig add myproject https://github.com/you/repo.git

# Create your crew workspace
ms crew add yourname --rig myproject
cd myproject/crew/yourname

# Start the Overseer session (your main interface)
ms overseer attach
```

### Docker Compose

```bash
export GIT_USER="<your name>"
export GIT_EMAIL="<your email>"
export FOLDER="/Users/you/code"
export DASHBOARD_PORT=8080  # optional, host port for the web dashboard

docker compose build              # only needed on first run or after code changes
docker compose up -d

docker compose exec mineshaft zsh   # or bash

ms up

gh auth login                     # if you want gh to work

ms overseer attach
```

## Quick Start Guide

### Getting Started
Run
```shell
ms install ~/ms --git &&
cd ~/ms &&
ms config agent list &&
ms overseer attach
```
and tell the Overseer what you want to build!

---

### Basic Workflow

```mermaid
sequenceDiagram
    participant You
    participant Overseer
    participant Minecart
    participant Agent
    participant Hook

    You->>Overseer: Tell Overseer what to build
    Overseer->>Minecart: Create minecart with beads
    Overseer->>Agent: Sling bead to agent
    Agent->>Hook: Store work state
    Agent->>Agent: Complete work
    Agent->>Minecart: Report completion
    Overseer->>You: Summary of progress
```

### Example: Feature Development

```bash
# 1. Start the Overseer
ms overseer attach

# 2. In Overseer session, create a minecart with bead IDs
ms minecart create "Feature X" ms-abc12 ms-def34 --notify --human

# 3. Assign work to an agent
ms sling ms-abc12 myproject

# 4. Track progress
ms minecart list

# 5. Monitor agents
ms agents
```

## Common Workflows

### Overseer Workflow (Recommended)

**Best for:** Coordinating complex, multi-issue work

```mermaid
flowchart LR
    Start([Start Overseer]) --> Tell[Tell Overseer<br/>what to build]
    Tell --> Creates[Overseer creates<br/>minecart + agents]
    Creates --> Monitor[Monitor progress<br/>via minecart list]
    Monitor --> Done{All done?}
    Done -->|No| Monitor
    Done -->|Yes| Review[Review work]
```

**Commands:**

```bash
# Attach to Overseer
ms overseer attach

# In Overseer, create minecart and let it orchestrate
ms minecart create "Auth System" ms-x7k2m ms-p9n4q --notify

# Track progress
ms minecart list
```

### Minimal Mode (No Tmux)

Run individual runtime instances manually. Mineshaft just tracks state.

```bash
ms minecart create "Fix bugs" ms-abc12   # Create minecart (sling auto-creates if skipped)
ms sling ms-abc12 myproject            # Assign to worker
claude --resume                        # Agent reads mail, runs work (Claude)
# or: codex                            # Start Codex in the workspace
ms minecart list                         # Check progress
```

### Beads Formula Workflow

**Best for:** Predefined, repeatable processes

Formulas are TOML-defined workflows embedded in the `ms` binary (source in `internal/formula/formulas/`).

**Example Formula** (`internal/formula/formulas/release.formula.toml`):

```toml
description = "Standard release process"
formula = "release"
version = 1

[vars.version]
description = "The semantic version to release (e.g., 1.2.0)"
required = true

[[steps]]
id = "bump-version"
title = "Bump version"
description = "Run ./scripts/bump-version.sh {{version}}"

[[steps]]
id = "run-tests"
title = "Run tests"
description = "Run make test"
needs = ["bump-version"]

[[steps]]
id = "build"
title = "Build"
description = "Run make build"
needs = ["run-tests"]

[[steps]]
id = "create-tag"
title = "Create release tag"
description = "Run git tag -a v{{version}} -m 'Release v{{version}}'"
needs = ["build"]

[[steps]]
id = "publish"
title = "Publish"
description = "Run ./scripts/publish.sh"
needs = ["create-tag"]
```

**Execute:**

```bash
# List available formulas
bd formula list

# Run a formula with variables
bd cook release --var version=1.2.0

# Create formula instance for tracking
bd mol pour release --var version=1.2.0
```

### Manual Minecart Workflow

**Best for:** Direct control over work distribution

```bash
# Create minecart manually
ms minecart create "Bug Fixes" --human

# Add issues to existing minecart
ms minecart add hq-cv-abc ms-m3k9p ms-w5t2x

# Assign to specific agents
ms sling ms-m3k9p myproject/my-agent

# Check status
ms minecart show
```

## Runtime Configuration

Mineshaft supports multiple AI coding runtimes. Per-rig runtime settings are in `settings/config.json`.

```json
{
  "runtime": {
    "provider": "codex",
    "command": "codex",
    "args": [],
    "prompt_mode": "none"
  }
}
```

**Notes:**

- Claude uses hooks in `.claude/settings.json` (managed via `--settings` flag) for mail injection and startup.
- For Codex, set `project_doc_fallback_filenames = ["CLAUDE.md"]` in
  `~/.codex/config.toml` so role instructions are picked up.
- For runtimes without hooks (e.g., Codex), Mineshaft sends a startup fallback
  after the session is ready: `ms prime`, optional `ms mail check --inject`
  for autonomous roles, and `ms nudge supervisor session-started`.
- **GitHub Copilot** (`copilot`) is a built-in preset using `--yolo` for autonomous
  mode. It uses executable lifecycle hooks in `.github/hooks/mineshaft.json` (same events
  as Claude: `sessionStart`, `userPromptSubmitted`, `preToolUse`, `sessionEnd`). Uses a
  5-second ready delay instead of prompt detection. Requires a Copilot seat and org-level
  CLI policy. See [docs/INSTALLING.md](docs/INSTALLING.md).

## Key Commands

### Workspace Management

```bash
ms install <path>           # Initialize workspace
ms rig add <name> <repo>    # Add project
ms rig list                 # List projects
ms crew add <name> --rig <rig>  # Create crew workspace
```

### Agent Operations

```bash
ms agents                   # List active agents
ms sling <bead-id> <rig>    # Assign work to agent
ms sling <bead-id> <rig> --agent cursor   # Override runtime for this sling/spawn
ms overseer attach             # Start Overseer session
ms overseer start --agent auggie           # Run Overseer with a specific agent alias
ms prime                    # Context recovery (run inside existing session)
ms feed                     # Real-time activity feed (TUI)
ms feed --problems          # Start in problems view (stuck agent detection)
```

**Built-in agent presets**: `claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`, `opencode`, `copilot`, `pi`, `omp`

### Minecart (Work Tracking)

```bash
ms minecart create <name> [issues...]   # Create minecart with issues
ms minecart list              # List all minecarts
ms minecart show [id]         # Show minecart details
ms minecart add <minecart-id> <issue-id...>  # Add issues to minecart
```

### Configuration

```bash
# Set custom agent command
ms config agent set claude-glm "claude-glm --model glm-4"
ms config agent set codex-low "codex --thinking low"

# Set default agent
ms config default-agent claude-glm
```

### Monitoring & Health

```bash
ms escalate -s HIGH "description"  # Escalate a blocker
ms escalate list               # List open escalations
ms scheduler status            # Show scheduler state
ms seance                      # Discover previous sessions
ms seance --talk <id>          # Query a predecessor session
```

### Beads Integration

```bash
bd formula list             # List formulas
bd cook <formula>           # Execute formula
bd mol pour <formula>       # Create trackable instance
bd mol list                 # List active instances
```

### Wasteland Federation

```bash
ms wl join <remote>            # Join a wasteland
ms wl browse                   # View wanted board
ms wl claim <id>               # Claim work
ms wl done <id> --evidence <url>  # Submit completion
```

## Cooking Formulas

Mineshaft includes built-in formulas for common workflows. See `internal/formula/formulas/` for available recipes.

## Activity Feed

`ms feed` launches an interactive terminal dashboard for monitoring all agent activity in real-time. It combines beads activity, agent events, and merge queue updates into a three-panel TUI:

- **Agent Tree** - Hierarchical view of all agents grouped by rig and role
- **Minecart Panel** - In-progress and recently-landed minecarts
- **Event Stream** - Chronological feed of creates, completions, slings, nudges, and more

```bash
ms feed                      # Launch TUI dashboard
ms feed --problems           # Start in problems view
ms feed --plain              # Plain text output (no TUI)
ms feed --window             # Open in dedicated tmux window
ms feed --since 1h           # Events from last hour
```

**Navigation:** `j`/`k` to scroll, `Tab` to switch panels, `1`/`2`/`3` to jump to a panel, `?` for help, `q` to quit.

### Problems View

At scale (20-50+ agents), spotting stuck agents in the activity stream becomes difficult. The problems view surfaces agents needing human intervention by analyzing structured beads data.

Press `p` in `ms feed` (or start with `ms feed --problems`) to toggle the problems view, which groups agents by health state:

| State | Condition |
|-------|-----------|
| **GUPP Violation** | Hooked work with no progress for an extended period |
| **Stalled** | Hooked work with reduced progress |
| **Zombie** | Dead tmux session |
| **Working** | Active, progressing normally |
| **Idle** | No hooked work |

**Intervention keys** (in problems view): `n` to nudge the selected agent, `h` to handoff (refresh context).

## Dashboard

Mineshaft includes a web dashboard for monitoring your workspace. The dashboard
must be run from inside a Mineshaft workspace (HQ) directory.

```bash
# Start dashboard (default port 8080)
ms dashboard

# Start on a custom port
ms dashboard --port 3000

# Start and automatically open in browser
ms dashboard --open
```

The dashboard gives you a single-page overview of everything happening in your
workspace: agents, minecarts, hooks, queues, issues, and escalations. It
auto-refreshes via htmx and includes a command palette for running ms commands
directly from the browser.

## Monitoring & Health

Mineshaft uses a three-tier watchdog chain to keep agents healthy at scale:

```
Daemon (Go process) ← heartbeat every 3 min
    └── Boot (AI agent) ← intelligent triage
        └── Supervisor (AI agent) ← continuous patrol
            └── Witnesses & Refineries ← per-rig agents
```

### Witness (Per-Rig)

Each rig has a Witness that monitors its miners. The Witness detects stuck agents, triggers recovery (nudge or handoff), manages session cleanup, and tracks completion. Witnesses delegate work rather than implementing it directly.

### Supervisor (Cross-Rig)

The Supervisor runs continuous patrol cycles across all rigs, checking agent health, dispatching Dogs for maintenance tasks, and escalating issues that individual Witnesses can't resolve.

### Escalation

When agents hit blockers, they escalate rather than waiting:

```bash
ms escalate -s HIGH "Description of blocker"
ms escalate list                    # List open escalations
ms escalate ack <bead-id>           # Acknowledge an escalation
```

Escalations route through Supervisor -> Overseer -> Boss based on severity. See [Escalation design](docs/design/escalation.md).

## Merge Queue (Refinery)

The Refinery processes completed miner work through a bisecting merge queue:

1. Miner runs `ms done` -> branch pushed, MR bead created
2. Refinery batches pending MRs
3. Runs verification gates on the merged stack
4. If green: all MRs in batch merge to main
5. If red: bisects to isolate the failing MR, merges the good ones

This is a Bors-style merge queue — miners never push directly to main.

## Scheduler

The scheduler controls miner dispatch capacity to prevent API rate limit exhaustion:

```bash
ms config set scheduler.max_miners 5   # Enable deferred dispatch (max 5 concurrent)
ms scheduler status                      # Show scheduler state
ms scheduler pause                       # Pause dispatch
ms scheduler resume                      # Resume dispatch
```

Default mode (`max_miners = -1`) dispatches immediately via `ms sling`. When a limit is set, the daemon dispatches incrementally, respecting capacity. See [Scheduler design](docs/design/scheduler.md).

## Seance

Discover and query previous agent sessions:

```bash
ms seance                              # List discoverable predecessor sessions
ms seance --talk <id>                  # Full context conversation with predecessor
ms seance --talk <id> -p "Question?"   # One-shot question to predecessor
```

Seance discovers sessions via `.events.jsonl` logs, enabling agents to recover context and decisions from earlier work without re-reading entire codebases.

## Wasteland Federation

The Wasteland is a federated work coordination network linking multiple Mineshafts through DoltHub:

```bash
ms wl join hop/wl-commons              # Join a wasteland
ms wl browse                           # View wanted board
ms wl claim <id>                       # Claim a wanted item
ms wl done <id> --evidence <url>       # Submit completion with evidence
ms wl post --title "Need X"            # Post new wanted item
```

Completions earn portable reputation via multi-dimensional stamps (quality, speed, complexity). See [Wasteland guide](docs/WASTELAND.md).

## Telemetry (OpenTelemetry)

Mineshaft emits all agent operations as structured logs and metrics to any OTLP-compatible backend (VictoriaMetrics/VictoriaLogs by default):

```bash
# Configure OTLP endpoints
export MS_OTEL_LOGS_URL="http://localhost:9428/insert/jsonline"
export MS_OTEL_METRICS_URL="http://localhost:8428/api/v1/write"
```

**Events emitted:** session lifecycle, agent state changes, bd calls with duration, mail operations, sling/nudge/done workflows, miner spawn/remove, formula instantiation, minecart creation, daemon restarts, and more.

**Metrics include:** `mineshaft.session.starts.total`, `mineshaft.bd.calls.total`, `mineshaft.miner.spawns.total`, `mineshaft.done.total`, `mineshaft.minecart.creates.total`, and others.

See [OTEL data model](docs/otel-data-model.md) and [OTEL architecture](docs/design/otel/) for the complete event schema.

## Advanced Concepts

### The Propulsion Principle

Mineshaft uses git hooks as a propulsion mechanism. Each hook is a git worktree with:

1. **Persistent state** - Work survives agent restarts
2. **Version control** - All changes tracked in git
3. **Rollback capability** - Revert to any previous state
4. **Multi-agent coordination** - Shared through git

### Hook Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Created: Agent spawned
    Created --> Active: Work assigned
    Active --> Suspended: Agent paused
    Suspended --> Active: Agent resumed
    Active --> Completed: Work done
    Completed --> Archived: Hook archived
    Archived --> [*]
```

### MEOW (Overseer-Enhanced Orchestration Workflow)

MEOW is the recommended pattern:

1. **Tell the Overseer** - Describe what you want
2. **Overseer analyzes** - Breaks down into tasks
3. **Minecart creation** - Overseer creates minecart with beads
4. **Agent spawning** - Overseer spawns appropriate agents
5. **Work distribution** - Beads slung to agents via hooks
6. **Progress monitoring** - Track through minecart status
7. **Completion** - Overseer summarizes results

## Shell Completions

```bash
# Bash
ms completion bash > /etc/bash_completion.d/ms

# Zsh
ms completion zsh > "${fpath[1]}/_ms"

# Fish
ms completion fish > ~/.config/fish/completions/ms.fish
```

## Project Roles

| Role            | Description                          | Primary Interface    |
| --------------- | ------------------------------------ | -------------------- |
| **Overseer**       | AI coordinator                       | `ms overseer attach`    |
| **Human (You)** | Crew member                          | Your crew directory  |
| **Miner**     | Worker agent                         | Spawned by Overseer     |
| **Witness**     | Per-rig agent health monitor         | Automatic patrol     |
| **Supervisor**      | Cross-rig supervisor daemon          | `ms patrol`          |
| **Refinery**    | Merge queue processor                | Automatic            |
| **Hook**        | Persistent storage                   | Git worktree         |
| **Minecart**      | Work tracker                         | `ms minecart` commands |

## Tips

- **Always start with the Overseer** - It's designed to be your primary interface
- **Use minecarts for coordination** - They provide visibility across agents
- **Leverage hooks for persistence** - Your work won't disappear
- **Create formulas for repeated tasks** - Save time with Beads recipes
- **Use `ms feed` for live monitoring** - Watch agent activity and catch stuck agents early
- **Monitor the dashboard** - Get real-time visibility in the browser
- **Let the Overseer orchestrate** - It knows how to manage agents

## Design Documentation

For deeper technical details, see the design docs in `docs/`:

| Topic | Document |
|-------|----------|
| Architecture | [docs/design/architecture.md](docs/design/architecture.md) |
| Glossary | [docs/glossary.md](docs/glossary.md) |
| Molecules | [docs/concepts/molecules.md](docs/concepts/molecules.md) |
| Escalation | [docs/design/escalation.md](docs/design/escalation.md) |
| Scheduler | [docs/design/scheduler.md](docs/design/scheduler.md) |
| Wasteland | [docs/WASTELAND.md](docs/WASTELAND.md) |
| OTEL data model | [docs/otel-data-model.md](docs/otel-data-model.md) |
| Witness design | [docs/design/witness-at-team-lead.md](docs/design/witness-at-team-lead.md) |
| Minecart lifecycle | [docs/design/minecart/](docs/design/minecart/) |
| Miner lifecycle | [docs/design/miner-lifecycle-patrol.md](docs/design/miner-lifecycle-patrol.md) |
| Plugin system | [docs/design/plugin-system.md](docs/design/plugin-system.md) |
| Agent providers | [docs/agent-provider-integration.md](docs/agent-provider-integration.md) |
| Hooks | [docs/HOOKS.md](docs/HOOKS.md) |
| Installation guide | [docs/INSTALLING.md](docs/INSTALLING.md) |

## Troubleshooting

### Agents lose connection

Check hooks are properly initialized:

```bash
ms hooks list
ms hooks repair
```

### Minecart stuck

Force refresh:

```bash
ms minecart refresh <minecart-id>
```

### Overseer not responding

Restart Overseer session:

```bash
ms overseer detach
ms overseer attach
```

## License

MIT License - see LICENSE file for details
