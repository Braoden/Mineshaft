# Mineshaft Hooks Management

Centralized hook management for Mineshaft workspaces.

## Overview

Mineshaft manages context injection for all supported agents. The mechanism varies by agent:

| Agent | Hook mechanism | Managed file |
|-------|---------------|-------------|
| Claude Code, Gemini | `settings.json` lifecycle hooks | `<role>/.claude/settings.json` |
| OpenCode | JS plugin | `workDir/.opencode/plugins/mineshaft.js` |
| GitHub Copilot | JSON lifecycle hooks | `workDir/.github/hooks/mineshaft.json` |
| Codex, others | Startup nudge fallback | *(no file — nudge only)* |

> **GitHub Copilot note**: Copilot CLI supports full executable lifecycle hooks
> (`sessionStart`, `userPromptSubmitted`, `preToolUse`, `sessionEnd`) via
> `.github/hooks/mineshaft.json`. This is the same lifecycle coverage as Claude Code,
> delivered in Copilot's JSON format rather than Claude's `settings.json` format.
> The `ms hooks` commands below apply to Claude Code (and Gemini) only.

Mineshaft manages `.claude/settings.json` files in mineshaft-managed parent directories
and passes them to Claude Code via the `--settings` flag. This keeps customer repos
clean while providing role-specific hook configuration. The hooks system provides
a single source of truth with a base config and per-role/per-rig overrides.

## Architecture

```
~/.ms/hooks-base.json              ← Shared base config (all agents)
~/.ms/hooks-overrides/
  ├── crew.json                    ← Override for all crew workers
  ├── witness.json                 ← Override for all witnesses
  ├── mineshaft__crew.json           ← Override for mineshaft crew specifically
  └── ...
```

**Merge strategy:** `base → role → rig+role` (more specific wins)

For a target like `mineshaft/crew`:
1. Start with base config
2. Apply `crew` override (if exists)
3. Apply `mineshaft/crew` override (if exists)

## Generated targets

Each rig generates settings in shared parent directories (not per-worktree):

| Target | Path | Override Key |
|--------|------|--------------|
| Crew (shared) | `<rig>/crew/.claude/settings.json` | `<rig>/crew` |
| Witness | `<rig>/witness/.claude/settings.json` | `<rig>/witness` |
| Refinery | `<rig>/refinery/.claude/settings.json` | `<rig>/refinery` |
| Miners (shared) | `<rig>/miners/.claude/settings.json` | `<rig>/miners` |

Town-level targets:
- `overseer/.claude/settings.json` (key: `overseer`)
- `supervisor/.claude/settings.json` (key: `supervisor`)

Settings are passed to Claude Code via `--settings <path>`, which loads them as
a separate priority tier that merges additively with project settings.

## Commands

### `ms hooks sync`

Regenerate all `.claude/settings.json` files from base + overrides.
Preserves non-hooks fields (editorMode, enabledPlugins, etc.).

```bash
ms hooks sync             # Write all settings files
ms hooks sync --dry-run   # Preview changes without writing
```

### `ms hooks diff`

Show what `sync` would change, without writing anything.

```bash
ms hooks diff             # Show differences
ms hooks diff --no-color  # Plain output
```

### `ms hooks base`

Edit the shared base config in `$EDITOR`.

```bash
ms hooks base             # Open in editor
ms hooks base --show      # Print current base config
```

### `ms hooks override <target>`

Edit overrides for a specific role or rig+role.

```bash
ms hooks override crew              # Edit crew override
ms hooks override mineshaft/witness   # Edit mineshaft witness override
ms hooks override crew --show       # Print current override
```

### `ms hooks list`

Show all managed settings.local.json locations and their sync status.

```bash
ms hooks list             # Show all targets
ms hooks list --json      # Machine-readable output
```

### `ms hooks scan`

Scan the workspace for existing hooks (reads current settings files).

```bash
ms hooks scan             # List all hooks
ms hooks scan --verbose   # Show hook commands
ms hooks scan --json      # JSON output
```

### `ms hooks init`

Bootstrap base config from existing settings.local.json files. Analyzes all
current settings, extracts common hooks as the base, and creates overrides
for per-target differences.

```bash
ms hooks init             # Bootstrap base and overrides
ms hooks init --dry-run   # Preview what would be created
```

Only works when no base config exists yet. Use `ms hooks base` to edit
an existing base config.

### `ms hooks registry` / `ms hooks install`

Browse and install hooks from the registry.

```bash
ms hooks registry                  # List available hooks
ms hooks install <hook-id>         # Install a hook to base config
```

## Current Registry Hooks

The registry (`~/ms/hooks/registry.toml`) defines 7 hooks, 5 enabled by default:

| Hook | Event | Enabled | Roles |
|---|---|---|---|
| pr-workflow-guard | PreToolUse | Yes | crew, miner |
| session-prime | SessionStart | Yes | all |
| pre-compact-prime | PreCompact | Yes | all |
| mail-check | UserPromptSubmit | Yes | all |
| costs-record | Stop | Yes | crew, miner, witness, refinery |
| clone-guard | PreToolUse | No | crew, miner |
| dangerous-command-guard | PreToolUse | Yes | crew, miner |

Additional hooks exist in settings.json files but are not yet in the registry:

- **bd init guard** (mineshaft/crew, beads/crew) - blocks `bd init*` inside `.beads/`
- **mol patrol guards** (mineshaft roles) - blocks persistent patrol molecules
- **tmux clear-history** (mineshaft root) - clears terminal history on session start
- **SessionStart .beads/ validation** (mineshaft/crew, beads/crew) - validates CWD

## Design Decision: Registry as Catalog vs Source of Truth

> **Decision: The registry is a catalog, not the source of truth.**
>
> The registry (`registry.toml`) lists available hooks. The base/overrides system
> (`~/.ms/hooks-base.json` + `~/.ms/hooks-overrides/`) defines what is active.
> `ms hooks install` copies from the registry into the base/overrides config.
>
> This separation provides:
> - Per-machine customization (PATH differences across machines)
> - Per-role overrides without polluting the shared registry
> - Clear distinction between "what hooks exist" and "what hooks are active where"
>
> The registry is the menu. The base/overrides are the order.

## Known Gaps

1. **Registry doesn't cover all active hooks** — Several hooks in settings.json
   files are not in `registry.toml` (bd-init-guard, mol-patrol-guard, tmux-clear,
   cwd-validation). These should be added so `ms hooks install` can manage them.

2. **No `ms tap` commands beyond pr-workflow** — The tap framework has only one
   guard implemented. `ms tap guard dangerous-command` is referenced in the
   registry but does not exist yet. Priority order: dangerous-command, bd-init,
   mol-patrol, then audit git-push.

3. **No `ms tap disable/enable` convenience commands** — Per-worktree
   enable/disable is possible via the override mechanism (`ms hooks override`
   with empty hooks list), but there is no convenience wrapper yet.

4. **Private hooks (settings.local.json)** — Claude Code supports
   `settings.local.json` for personal overrides. Mineshaft doesn't manage
   these yet. Low priority since Mineshaft is primarily agent-operated.

5. **Hook ordering** — No action needed currently. The merge chain
   (base -> override) produces deterministic order, and per-matcher merge
   ensures one entry per event type.

## Integration

### `ms rig add`

When a new rig is created, hooks are automatically synced for all the
new rig's targets (crew, witness, refinery, miners).

### `ms doctor`

The `hooks-sync` check verifies all settings.local.json files match what
`ms hooks sync` would generate. Use `ms doctor --fix` to auto-fix
out-of-sync targets.

## Per-matcher merge semantics

When an override has the same matcher as a base entry, the override
**replaces** the base entry entirely. Different matchers are appended.
An override entry with an empty hooks list **removes** that matcher.

Example base:
```json
{
  "SessionStart": [
    { "matcher": "", "hooks": [{ "type": "command", "command": "ms prime" }] }
  ]
}
```

Override for witness:
```json
{
  "SessionStart": [
    { "matcher": "", "hooks": [{ "type": "command", "command": "ms prime --witness" }] }
  ]
}
```

Result: The witness gets `ms prime --witness` instead of `ms prime`
(same matcher = replace).

## Default base config

When no base config exists, the system uses sensible defaults:

- **SessionStart**: PATH setup + `ms prime --hook`
- **PreCompact**: PATH setup + `ms prime --hook`
- **UserPromptSubmit**: PATH setup + `ms mail check --inject`
- **Stop**: PATH setup + `ms costs record`
