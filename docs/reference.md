# Mineshaft Reference

Technical reference for Mineshaft internals. Read the README first.

> For directory structure details, see [architecture.md](design/architecture.md).

## Beads Routing

Mineshaft `ms` commands route beads work based on issue ID prefix. For direct
`bd` commands, run from the owning repository/root so the active `.beads`
directory matches the database you intend to touch.

```bash
bd -C ~/ms/greenplace/overseer/rig show gp-xyz  # Greenplace rig beads
bd -C ~/ms show hq-abc                       # Town-level beads
bd -C ~/ms/wyvern/overseer/rig show wyv-123     # Wyvern rig beads
```

**How it works**: Routes are defined in `~/ms/.beads/routes.jsonl`. Each rig's
prefix maps to its beads location (the overseer's clone in that rig).

| Prefix | Routes To | Purpose |
|--------|-----------|---------|
| `hq-*` | `~/ms/.beads/` | Overseer mail, cross-rig coordination |
| `gp-*` | `~/ms/greenplace/overseer/rig/.beads/` | Greenplace project issues |
| `wyv-*` | `~/ms/wyvern/overseer/rig/.beads/` | Wyvern project issues |

Debug routing: `BD_DEBUG_ROUTING=1 bd -C <owning-root> show <id>`

`bd --global` is not Mineshaft's town database. In Beads it targets a separate
shared-server database named `beads_global`; run `bd -C ~/ms ...` for
town-level Mineshaft beads.

## Configuration

### Rig Config (`config.json`)

```json
{
  "type": "rig",
  "name": "myproject",
  "git_url": "https://github.com/...",
  "default_branch": "main",
  "beads": { "prefix": "mp" }
}
```

**Rig config fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_branch` | `string` | `"main"` | Default branch for the rig. Auto-detected from remote during `ms rig add`. Used as the merge target by the Refinery and as the base for miners when no integration branch is active. |

### Settings (`settings/config.json`)

```json
{
  "theme": {
    "disabled": false,
    "name": "forest",
    "custom": {
      "bg": "#111111",
      "fg": "#eeeeee"
    },
    "role_themes": {
      "witness": "rust",
      "refinery": "plum",
      "crew": "none"
    }
  },
  "merge_queue": {
    "enabled": true,
    "run_tests": true,
    "setup_command": "",
    "typecheck_command": "",
    "lint_command": "",
    "test_command": "",
    "build_command": "",
    "on_conflict": "assign_back",
    "delete_merged_branches": true,
    "retry_flaky_tests": 1,
    "poll_interval": "30s",
    "max_concurrent": 1,
    "integration_branch_miner_enabled": true,
    "integration_branch_refinery_enabled": true,
    "integration_branch_template": "integration/{title}",
    "integration_branch_auto_land": false
  }
}
```

**Theme fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `disabled` | `bool` | `false` | Disable tmux status/window theming for the rig |
| `name` | `string` | auto-assigned by rig name | Use a named built-in palette theme |
| `custom.bg` | `string` | unset | Custom tmux background color |
| `custom.fg` | `string` | unset | Custom tmux foreground color |
| `role_themes` | `map[string]string` | unset | Per-role overrides for `witness`, `refinery`, `crew`, `miner`; use `"none"` to disable theming for a role |

Theme resolution:
- No `theme` config: auto-assign a built-in palette theme by rig name
- `disabled: true`: skip both `status-style` and `window-style`
- `name`: use that built-in theme
- `custom`: use exact `{bg, fg}` colors
- `role_themes`: override role-specific sessions within the rig

Town-level role defaults live in `overseer/config.json` under:

```json
{
  "theme": {
    "disabled": false,
    "name": "forest",
    "custom": {
      "bg": "#111111",
      "fg": "#eeeeee"
    },
    "role_defaults": {
      "overseer": "forest",
      "supervisor": "plum",
      "witness": "rust",
      "crew": "none"
    }
  }
}
```

`role_defaults` supports `overseer`, `supervisor`, `witness`, `refinery`, `crew`, and `miner`.

**Merge queue fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `bool` | `true` | Whether the merge queue is active |
| `run_tests` | `bool` | `true` | Run tests before merging |
| `setup_command` | `string` | `""` | Setup/install command (e.g., `pnpm install`) |
| `typecheck_command` | `string` | `""` | Type check command (e.g., `tsc --noEmit`) |
| `lint_command` | `string` | `""` | Lint command (e.g., `eslint .`) |
| `test_command` | `string` | `""` | Test command to run. Empty = skip. |
| `build_command` | `string` | `""` | Build command (e.g., `go build ./...`) |
| `on_conflict` | `string` | `"assign_back"` | Conflict strategy: `assign_back` or `auto_rebase` |
| `delete_merged_branches` | `bool` | `true` | Delete source branches after merging |
| `retry_flaky_tests` | `int` | `1` | Number of times to retry flaky tests |
| `poll_interval` | `string` | `"30s"` | How often Refinery polls for new MRs |
| `max_concurrent` | `int` | `1` | Maximum concurrent merges |
| `integration_branch_miner_enabled` | `*bool` | `true` | Miners auto-source worktrees from integration branches |
| `integration_branch_refinery_enabled` | `*bool` | `true` | `ms done` / `ms mq submit` auto-target integration branches |
| `integration_branch_template` | `string` | `"integration/{title}"` | Branch name template (`{title}`, `{epic}`, `{prefix}`, `{user}`) |
| `integration_branch_auto_land` | `*bool` | `false` | Refinery patrol auto-lands when all children closed |

See [Integration Branches](concepts/integration-branches.md) for integration branch details.

### Runtime (`.runtime/` - gitignored)

Process state, PIDs, ephemeral data.

### Rig-Level Configuration

Rigs support layered configuration through:
1. **Wisp layer** (`.beads-wisp/config/`) - transient, local overrides
2. **Rig identity bead labels** - persistent rig settings
3. **Town defaults** (`~/ms/settings/config.json`)
4. **System defaults** - compiled-in fallbacks

#### Miner Branch Naming

Configure custom branch name templates for miners:

```bash
# Set via wisp (transient - for testing)
echo '{"miner_branch_template": "adam/{year}/{month}/{description}"}' > \
  ~/ms/.beads-wisp/config/myrig.json

# Or set via rig identity bead labels (persistent)
bd update ms-rig-myrig --labels="miner_branch_template:adam/{year}/{month}/{description}"
```

**Template Variables:**

| Variable | Description | Example |
|----------|-------------|---------|
| `{user}` | From `git config user.name` | `adam` |
| `{year}` | Current year (YY format) | `26` |
| `{month}` | Current month (MM format) | `01` |
| `{name}` | Miner name | `alpha` |
| `{issue}` | Issue ID without prefix | `123` (from `ms-123`) |
| `{description}` | Sanitized issue title | `fix-auth-bug` |
| `{timestamp}` | Unique timestamp | `1ks7f9a` |

**Default Behavior (backward compatible):**

When `miner_branch_template` is empty or not set:
- With issue: `miner/{name}/{issue}@{timestamp}`
- Without issue: `miner/{name}-{timestamp}`

**Example Configurations:**

```bash
# GitHub enterprise format
"adam/{year}/{month}/{description}"

# Simple feature branches
"feature/{issue}"

# Include miner name for clarity
"work/{name}/{issue}"
```

## Formula Format

```toml
formula = "name"
type = "workflow"           # workflow | expansion | aspect
version = 1
description = "..."

[vars.feature]
description = "..."
required = true

[[steps]]
id = "step-id"
title = "{{feature}}"
description = "..."
needs = ["other-step"]      # Dependencies
```

**Composition:**

```toml
extends = ["base-formula"]

[compose]
aspects = ["cross-cutting"]

[[compose.expand]]
target = "step-id"
with = "macro-formula"
```

## Molecule Lifecycle

> For the full lifecycle diagram and detailed command reference, see [concepts/molecules.md](concepts/molecules.md).

**Summary**: Formula (TOML) --`bd cook`--> Protomolecule --`bd mol pour`--> Mol (persistent) or Wisp (ephemeral) --`bd squash`--> Digest.

| Operation | bd (data) | ms (agent) |
|-----------|-----------|------------|
| Cook/pour/wisp | `bd cook`, `bd mol pour/wisp` | — |
| Squash/burn | `bd mol squash/burn <id>` | `ms mol squash/burn` (attached) |
| Navigate | `bd mol current`, `bd mol show` | `ms hook`, `ms mol current` |
| Attach | — | `ms mol attach/detach` |

## Agent Lifecycle

### Miner Shutdown

```
1. Work through formula checklist (shown inline by ms prime)
2. Submit to merge queue via ms done
3. ms done nukes sandbox and exits
4. Witness removes worktree + branch
```

### Session Cycling

```
1. Agent notices context filling
2. ms handoff (sends mail to self)
3. Manager kills session
4. Manager starts new session
5. New session reads handoff mail
```

## Environment Variables

Mineshaft sets environment variables for each agent session via `config.AgentEnv()`.
These are set in tmux session environment when agents are spawned.

### Core Variables (All Agents)

| Variable | Purpose | Example |
|----------|---------|---------|
| `MS_ROLE` | Agent role type | `overseer`, `witness`, `miner`, `crew` |
| `MS_ROOT` | Town root directory | `/home/user/ms` |
| `BD_ACTOR` | Agent identity for attribution | `mineshaft/miners/toast` |
| `GIT_AUTHOR_NAME` | Commit attribution (same as BD_ACTOR) | `mineshaft/miners/toast` |
| `BEADS_DIR` | Beads database location | `/home/user/ms/mineshaft/.beads` |

### Rig-Level Variables

| Variable | Purpose | Roles |
|----------|---------|-------|
| `MS_RIG` | Rig name | witness, refinery, miner, crew |
| `MS_MINER` | Miner worker name | miner only |
| `MS_CREW` | Crew worker name | crew only |
| `BEADS_AGENT_NAME` | Agent name for beads operations | miner, crew |

### Other Variables

| Variable | Purpose |
|----------|---------|
| `GIT_AUTHOR_EMAIL` | Workspace owner email (from git config) |
| `MS_TOWN_ROOT` | Override town root detection (manual use) |
| `CLAUDE_RUNTIME_CONFIG_DIR` | Custom Claude settings directory |

### Environment by Role

| Role | Key Variables |
|------|---------------|
| **Overseer** | `MS_ROLE=overseer`, `BD_ACTOR=overseer` |
| **Supervisor** | `MS_ROLE=supervisor`, `BD_ACTOR=supervisor` |
| **Boot** | `MS_ROLE=supervisor/boot`, `BD_ACTOR=supervisor-boot` |
| **Witness** | `MS_ROLE=witness`, `MS_RIG=<rig>`, `BD_ACTOR=<rig>/witness` |
| **Refinery** | `MS_ROLE=refinery`, `MS_RIG=<rig>`, `BD_ACTOR=<rig>/refinery` |
| **Miner** | `MS_ROLE=miner`, `MS_RIG=<rig>`, `MS_MINER=<name>`, `BD_ACTOR=<rig>/miners/<name>` |
| **Crew** | `MS_ROLE=crew`, `MS_RIG=<rig>`, `MS_CREW=<name>`, `BD_ACTOR=<rig>/crew/<name>` |

### Doctor Check

The `ms doctor` command verifies that running tmux sessions have correct
environment variables. Mismatches are reported as warnings:

```
⚠ env-vars: Found 3 env var mismatch(es) across 1 session(s)
    hq-overseer: missing MS_ROOT (expected "/home/user/ms")
```

Fix by restarting sessions: `ms shutdown && ms up`

## Agent Working Directories and Settings

Each agent runs in a specific working directory and has its own Claude settings.
Understanding this hierarchy is essential for proper configuration.

### Working Directories by Role

| Role | Working Directory | Notes |
|------|-------------------|-------|
| **Overseer** | `~/ms/overseer/` | Town-level coordinator, isolated from rigs |
| **Supervisor** | `~/ms/supervisor/` | Background supervisor daemon |
| **Witness** | `~/ms/<rig>/witness/` | No git clone, monitors miners only |
| **Refinery** | `~/ms/<rig>/refinery/rig/` | Worktree on main branch |
| **Crew** | `~/ms/<rig>/crew/<name>/rig/` | Persistent human workspace clone |
| **Miner** | `~/ms/<rig>/miners/<name>/rig/` | Miner worktree (ephemeral sandbox) |

Note: The per-rig `<rig>/overseer/rig/` directory is NOT a working directory—it's
a git clone that holds the canonical `.beads/` database for that rig.

### Settings File Locations

Settings are installed in mineshaft-managed parent directories and passed to
Claude Code via the `--settings` flag. This keeps customer repos clean:

```
~/ms/
├── overseer/.claude/settings.json              # Overseer settings (cwd = settings dir)
├── supervisor/.claude/settings.json             # Supervisor settings (cwd = settings dir)
└── <rig>/
    ├── crew/.claude/settings.json           # Shared by all crew members
    ├── miners/.claude/settings.json       # Shared by all miners
    ├── witness/.claude/settings.json        # Witness settings
    └── refinery/.claude/settings.json       # Refinery settings
```

The `--settings` flag loads these as a separate priority tier that merges
additively with any project-level settings in the customer repo.

### CLAUDE.md

Only `~/ms/CLAUDE.md` exists on disk — a minimal identity anchor that prevents
agents from losing their Mineshaft identity after context compaction or new sessions.

Full role context (~300-500 lines per role) is injected ephemerally by `ms prime`
via the SessionStart hook. No per-directory CLAUDE.md or AGENTS.md files are created.

**Why no per-directory files?**
- Claude Code traverses upward from CWD for CLAUDE.md — all agents under `~/ms/` find the town-root file
- AGENTS.md (for Codex) uses downward traversal from git root — parent directories are invisible, so per-directory AGENTS.md never worked
- The real context comes from `ms prime`, making on-disk bootstrap pointers redundant

### Customer Repo Files (CLAUDE.md and .claude/)

Mineshaft no longer uses git sparse checkout to hide customer repo files. Customer
repositories can have their own `.claude/` directory and `CLAUDE.md` — these are
preserved in all worktrees (crew, miners, refinery, overseer/rig).

Mineshaft's context comes from the town-root `CLAUDE.md` identity anchor
(picked up by all agents via Claude Code's upward directory traversal),
`ms prime` via the SessionStart hook, and the customer repo's own `CLAUDE.md`.
These coexist safely because:

- **`--settings` flag provides Mineshaft settings** as a separate tier that merges
  additively with customer project settings, so both coexist cleanly
- **`ms prime` injects role context** ephemerally via SessionStart hook, which is
  additive with the customer's `CLAUDE.md` — both are loaded
- Mineshaft settings live in parent directories (not in customer repos), so
  customer `.claude/` files are fully preserved

**Doctor check**: `ms doctor` warns if legacy sparse checkout is still configured.
Run `ms doctor --fix` to remove it. Tracked `settings.json` files in worktrees are
recognized as customer project config and are not flagged as stale.

### Settings Inheritance

Claude Code's settings are layered from multiple sources:

1. `.claude/settings.json` in current working directory (customer project)
2. `.claude/settings.json` in parent directories (traversing up)
3. `~/.claude/settings.json` (user global settings)
4. `--settings <path>` flag (loaded as a separate additive tier)

Mineshaft uses the `--settings` flag to inject role-specific settings from
mineshaft-managed parent directories. This merges additively with customer
project settings rather than overriding them.

### Settings Templates

Mineshaft uses two settings templates based on role type:

| Type | Roles | Key Difference |
|------|-------|----------------|
| **Interactive** | Overseer, Crew | Mail injected on `UserPromptSubmit` hook |
| **Autonomous** | Miner, Witness, Refinery, Supervisor | Mail injected on `SessionStart` hook |

Autonomous agents may start without user input, so they need mail checked
at session start. Interactive agents wait for user prompts.

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Agent using wrong settings | Check `ms doctor`, verify `.claude/settings.json` in role parent dir |
| Settings not found | Run `ms install` to recreate settings, or `ms doctor --fix` |
| Source repo settings leaking | Run `ms doctor --fix` to remove legacy sparse checkout |
| Overseer settings affecting miners | Overseer should run in `overseer/`, not town root |

## CLI Reference

### Town Management

```bash
ms install [path]            # Create town
ms install --git             # With git init
ms doctor                    # Health check
ms doctor --fix              # Auto-repair
```

### Configuration

```bash
# Agent management
ms config agent list [--json]     # List all agents (built-in + custom)
ms config agent get <name>        # Show agent configuration
ms config agent set <name> <cmd>  # Create or update custom agent
ms config agent remove <name>     # Remove custom agent (built-ins protected)

# Default agent
ms config default-agent [name]    # Get or set town default agent
```

**Built-in agents**: `claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`, `opencode`, `copilot`

> **Note on GitHub Copilot**: The `copilot` preset uses executable lifecycle hooks in
> `.github/hooks/mineshaft.json` (`sessionStart`, `userPromptSubmitted`, `preToolUse`,
> `sessionEnd`) — the same lifecycle events as Claude Code, in Copilot's JSON format.
> Copilot uses a 5-second ready delay instead of prompt-based detection. Requires a
> Copilot seat and org-level CLI policy enabled.

**Custom agents**: Define per-town via CLI or JSON:
```bash
ms config agent set claude-glm "claude-glm --model glm-4"
ms config agent set claude "claude-opus"  # Override built-in
ms config default-agent claude-glm       # Set default
```

**Advanced agent config** (`settings/agents.json`):
```json
{
  "version": 1,
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": [],
      "resume_flag": "--session",
      "resume_style": "flag",
      "non_interactive": {
        "subcommand": "run",
        "output_flag": "--format json"
      }
    }
  }
}
```

**Rig-level agents** (`<rig>/settings/config.json`):
```json
{
  "type": "rig-settings",
  "version": 1,
  "agent": "opencode",
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": ["--session"]
    }
  }
}
```

**ACP-enabled custom agents** (`settings/config.json`):
```json
{
  "type": "town-settings",
  "version": 1,
  "default_agent": "opencode-acp-debug",
  "agents": {
    "opencode-acp-debug": {
      "command": "opencode",
      "acp": {
        "command": "acp",
        "args": ["--debug", "--print-logs"]
      }
    }
  }
}
```

The `acp` field configures Agent Communication Protocol support:
- `command`: ACP subcommand (e.g., `"acp"` for `opencode acp`)
- `args`: Additional arguments passed to the ACP subcommand

Custom agents inherit ACP support from their base command's preset. For example,
a custom agent with `"command": "opencode"` automatically inherits ACP support
from the opencode preset. You can override or extend the ACP args by specifying
the `acp` field explicitly.

**Agent resolution order**: rig-level → town-level → built-in presets.

For OpenCode autonomous mode, set env var in your shell profile:
```bash
export OPENCODE_PERMISSION='{"*":"allow"}'
```

### Rig Management

```bash
ms rig add <name> <url>
ms rig list
ms rig remove <name>
```

### Minecart Management (Primary Dashboard)

```bash
ms minecart list                          # Dashboard of active minecarts
ms minecart status [minecart-id]            # Show progress (🚚 hq-cv-*)
ms minecart create "name" [issues...]     # Create minecart tracking issues
ms minecart create "name" ms-a bd-b --notify overseer/  # With notification
ms minecart list --all                    # Include landed minecarts
ms minecart list --status=closed          # Only landed minecarts
```

Note: "Swarm" is ephemeral (workers on a minecart's issues). See [Minecarts](concepts/minecart.md).

### Work Assignment

```bash
# Standard workflow: minecart first, then sling
ms minecart create "Feature X" ms-abc ms-def
ms sling ms-abc <rig>                    # Assign to miner
ms sling ms-abc <rig> --agent codex      # Override runtime for this sling/spawn
ms sling <proto> --on ms-def <rig>       # With workflow template

# Quick sling (auto-creates minecart)
ms sling <bead> <rig>                    # Auto-minecart for dashboard visibility
```

Agent overrides:

- `ms start --agent <alias>` overrides the Overseer/Supervisor runtime for this launch.
- `ms overseer start|attach|restart --agent <alias>` and `ms supervisor start|attach|restart --agent <alias>` do the same.
- `ms start crew <name> --agent <alias>` and `ms crew at <name> --agent <alias>` override the crew worker runtime.

### Communication

```bash
ms mail inbox
ms mail read <id>
ms mail send <addr> -s "Subject" -m "Body"
ms mail send --human -s "..."    # To boss
```

### Escalation

```bash
ms escalate "topic"              # Default: MEDIUM severity
ms escalate -s CRITICAL "msg"    # Urgent, immediate attention
ms escalate -s HIGH "msg"        # Important blocker
ms escalate -s MEDIUM "msg" -m "Details..."
```

See [escalation.md](design/escalation.md) for full protocol.

### Sessions

```bash
ms handoff                   # Request cycle (context-aware)
ms handoff --shutdown        # Terminate (miners)
ms session stop <rig>/<agent>
ms peek <agent>              # Check health
ms nudge <agent> "message"   # Send message to agent
ms seance                    # List discoverable predecessor sessions
ms seance --talk <id>        # Talk to predecessor (full context)
ms seance --talk <id> -p "Where is X?"  # One-shot question
```

**Session Discovery**: Each session has a startup nudge that becomes searchable
in Claude's `/resume` picker:

```
[MINESHAFT] recipient <- sender • timestamp • topic[:mol-id]
```

Example: `[MINESHAFT] mineshaft/crew/gus <- human • 2025-12-30T15:42 • restart`

**IMPORTANT**: Always use `ms nudge` to send messages to Claude sessions.
Never use raw `tmux send-keys` - it doesn't handle Claude's input correctly.
`ms nudge` uses literal mode + debounce + separate Enter for reliable delivery.

### Emergency

```bash
ms stop --all                # Kill all sessions
ms stop --rig <name>         # Kill rig sessions
```

### Health Check

```bash
ms supervisor health-check <agent>   # Send health check ping, track response
ms supervisor health-state           # Show health check state for all agents
```

### Merge Queue (MQ)

```bash
ms mq list [rig]             # Show the merge queue
ms mq next [rig]             # Show highest-priority merge request
ms mq submit                 # Submit current branch to merge queue
ms mq status <id>            # Show detailed merge request status
ms mq retry <id>             # Retry a failed merge request
ms mq reject <id>            # Reject a merge request
```

#### Integration Branch Commands

```bash
ms mq integration create <epic-id>              # Create integration branch
ms mq integration create <epic-id> --branch "feat/{title}"  # Custom template
ms mq integration create <epic-id> --base-branch develop   # Non-main base
ms mq integration status <epic-id>              # Show branch status
ms mq integration status <epic-id> --json       # JSON output
ms mq integration land <epic-id>                # Merge to base branch (default: main)
ms mq integration land <epic-id> --dry-run      # Preview only
ms mq integration land <epic-id> --force        # Land with open MRs
ms mq integration land <epic-id> --skip-tests   # Skip test run
```

See [Integration Branches](concepts/integration-branches.md) for the full workflow.

## Beads Commands (bd)

```bash
bd ready                     # Work with no blockers
bd list --status=open
bd list --status=in_progress
bd show <id>
bd create --title="..." --type=task
bd update <id> --status=in_progress
bd close <id>
bd dep add <child> <parent>  # child depends on parent
```

## Patrol Agents

Supervisor, Witness, and Refinery run continuous patrol loops using wisps:

| Agent | Patrol Molecule | Responsibility |
|-------|-----------------|----------------|
| **Supervisor** | `mol-supervisor-patrol` | Agent lifecycle, plugin execution, health checks |
| **Witness** | `mol-witness-patrol` | Monitor miners, nudge stuck workers |
| **Refinery** | `mol-refinery-patrol` | Process merge queue, review MRs, check integration branches |

```
1. ms patrol new               # Create root-only wisp
2. ms prime                    # Shows patrol checklist inline
3. Work through each step
4. ms patrol report --summary "..."  # Close + start next cycle
```

## Plugin Molecules

Plugins are molecules with specific labels:

```json
{
  "id": "mol-security-scan",
  "labels": ["template", "plugin", "witness", "tier:haiku"]
}
```

Patrol molecules bond plugins dynamically:

```bash
bd mol bond mol-security-scan $PATROL_ID --var scope="$SCOPE"
```

## Formula Invocation Patterns

**CRITICAL**: Different formula types require different invocation methods.

### Workflow Formulas (sequential steps, single miner)

Examples: `shiny`, `shiny-enterprise`, `mol-miner-work`

```bash
ms sling <formula> --on <bead-id> <target>
ms sling shiny-enterprise --on ms-abc123 mineshaft
```

### Minecart Formulas (parallel legs, multiple miners)

Examples: `code-review`

**DO NOT use `ms sling` for minecart formulas!** It fails with "minecart type not supported".

```bash
# Correct invocation - use ms formula run:
ms formula run code-review --pr=123
ms formula run code-review --files="src/*.go"

# Dry run to preview:
ms formula run code-review --pr=123 --dry-run
```

### Identifying Formula Type

```bash
ms formula show <name>   # Shows "Type: minecart" or "Type: workflow"
bd formula list          # Lists formulas by type
```

### Why This Matters

- `ms sling` attempts to cook+pour the formula, which fails for minecart type
- `ms formula run` handles minecart dispatch directly, spawning parallel miners
- Minecart formulas create multiple miners (one per leg) + synthesis step

## Common Issues

| Problem | Solution |
|---------|----------|
| Agent in wrong directory | Check cwd, `ms doctor` |
| Beads prefix mismatch | Check `bd show` vs rig config |
| Worktree conflicts | Check worktree state, `ms doctor` |
| Stuck worker | `ms nudge`, then `ms peek` |
| Dirty git state | Commit or discard, then `ms handoff` |

> For architecture details (bare repo pattern, beads as control plane, nondeterministic idempotence), see [architecture.md](design/architecture.md).
