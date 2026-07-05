# Mineshaft/Beads Cleanup Commands Reference

A comprehensive catalog of all cleanup-related commands in the mineshaft/beads ecosystem, organized by scope and severity.

---

## Process Cleanup

| Command | What it does |
|---------|-------------|
| `ms cleanup` | Kills orphaned Claude processes not tied to active tmux sessions |
| `ms orphans procs list` | Lists orphaned Claude processes (PPID=1) |
| `ms orphans procs kill` | Kills orphaned Claude processes (`--aggressive` for tmux-verified) |
| `ms supervisor cleanup-orphans` | Kills orphaned Claude subagent processes (no controlling TTY) |
| `ms supervisor zombie-scan` | Finds/kills zombie Claude processes not in active tmux sessions |

## Miner (Agent Sandbox) Cleanup

| Command | What it does |
|---------|-------------|
| `ms miner remove <rig>/<miner>` | Removes miner worktree/directory (fails if session running) |
| `ms miner nuke <rig>/<miner>` | Nuclear: kills session, deletes worktree, deletes branch, closes bead |
| `ms miner nuke <rig> --all` | Nukes all miners in a rig |
| `ms miner gc <rig>` | GC stale miner branches (orphaned, old timestamped) |
| `ms miner stale <rig>` | Detects stale miners; `--cleanup` auto-nukes them |
| `ms miner check-recovery` | Pre-nuke safety check (SAFE_TO_NUKE vs NEEDS_RECOVERY) |
| `ms miner identity remove <rig> <name>` | Removes a miner identity |
| `ms done` | Miner self-cleaning: pushes branch, submits MR (by default), self-nukes worktree, kills own session. MR skipped for `--status ESCALATED\|DEFERRED` or `no_merge` paths |

## Git Artifact Cleanup

| Command | What it does |
|---------|-------------|
| `ms prune-branches` | Removes stale local miner tracking branches (`git fetch --prune` + safe delete) |
| `ms orphans` | Finds orphaned commits never merged (detection only) |
| `ms orphans kill` | Prunes orphaned commits (`git gc --prune=now`) + kills orphaned processes |

## Rig-Level Cleanup

| Command | What it does |
|---------|-------------|
| `ms rig reset` | Resets handoff content, stale mail, orphaned in_progress issues |
| `ms rig reset --handoff` | Clears handoff content only |
| `ms rig reset --mail` | Clears stale mail only |
| `ms rig reset --stale` | Resets orphaned in_progress issues |
| `ms rig remove <name>` | Unregisters rig from registry, cleans up beads routes |
| `ms rig shutdown <rig>` | Stops all agents: miners, refinery, witness |
| `ms rig stop <rig>...` | Stop one or more rigs |
| `ms rig restart <rig>...` | Stop then start (stop phase cleans up) |

## Town-Wide Shutdown

| Command | What it does |
|---------|-------------|
| `ms down` | Stops all infrastructure (refinery, witness, overseer, boot, supervisor, daemon, dolt) |
| `ms down --miners` | Also stops all miner sessions |
| `ms down --all` | Full shutdown with orphan cleanup and verification |
| `ms down --nuke` | Kills entire tmux server (DESTRUCTIVE - kills non-MS sessions too) |
| `ms shutdown` | "Done for the day" - stops agents AND removes miner worktrees/branches. Flags control aggressiveness (`--graceful`, `--force`, `--nuclear`, `--miners-only`, etc.) |

## Crew Workspace Cleanup

| Command | What it does |
|---------|-------------|
| `ms crew stop [name]` | Stops crew tmux sessions |
| `ms crew restart [name]` | Kills and restarts crew fresh ("clean slate", no handoff mail) |
| `ms crew remove <name>` | Removes workspace, closes agent bead |
| `ms crew remove <name> --purge` | Full obliteration: deletes agent bead, unassigns beads, clears mail |
| `ms crew pristine [name]` | Syncs workspaces with remote (`git pull`) |

## Ephemeral Data / Event Cleanup

| Command | What it does |
|---------|-------------|
| `ms compact` | TTL-based compaction: promotes/deletes wisps past their TTL |
| `ms krc prune` | Prunes expired events from the KRC event store |
| `ms krc config reset` | Resets KRC TTL configuration to defaults |
| `ms krc decay` | Shows forensic value decay report (pruning guidance) |

## Dolt Database Cleanup

| Command | What it does |
|---------|-------------|
| `ms dolt cleanup` | Removes orphaned databases from `.dolt-data/` |
| `ms dolt stop` | Stops the Dolt SQL server |
| `ms dolt rollback [backup-dir]` | Restores `.beads` from backup, resets metadata |

## Bead / Hook Cleanup

| Command | What it does |
|---------|-------------|
| `ms close <bead-id>` | Closes beads (lifecycle termination) |
| `ms unsling` / `ms unhook` | Removes work from agent's hook, resets bead status to "open" |
| `ms hook clear` | Alias for unsling |

## Dog (Infrastructure Worker) Cleanup

| Command | What it does |
|---------|-------------|
| `ms dog remove <name>` | Removes worktrees and dog directory |
| `ms dog remove --all` | Removes all dogs |
| `ms dog clear <name>` | Resets stuck dog to idle state |
| `ms dog done [name]` | Marks dog as done, clears work field |

## Minecart Cleanup

| Command | What it does |
|---------|-------------|
| `ms minecart close <id>` | Closes a minecart bead |
| `ms minecart land <id>` | Closes minecart, cleans up miner worktrees, sends completion notifications |

## Mail Cleanup

| Command | What it does |
|---------|-------------|
| `ms mail delete <msg-id>` | Deletes specific messages |
| `ms mail archive <msg-id>` | Archives messages (`--stale` for stale ones) |
| `ms mail clear [target]` | Deletes all messages from an inbox (town quiescence) |

## Misc State Cleanup

| Command | What it does |
|---------|-------------|
| `ms namepool reset` | Releases all claimed miner names |
| `ms checkpoint clear` | Removes checkpoint file |
| `ms issue clear` | Clears issue from tmux status line |
| `ms doctor --fix` | Auto-fixes: orphan sessions, wisp GC, stale redirects, worktree validity |

## System-Level Cleanup

| Command | What it does |
|---------|-------------|
| `ms disable --clean` | Disables mineshaft + removes shell integration |
| `ms shell remove` | Removes shell integration from RC files |
| `ms config agent remove <name>` | Removes custom agent definition |
| `ms uninstall` | Full removal: shell integration, wrapper scripts, state/config/cache dirs |
| `make clean` | Removes compiled `ms` binary |

## Scripts

| Command | What it does |
|---------|-------------|
| `scripts/migration-test/reset-vm.sh` | Restores VM to pristine v0.5.0 state (test environments) |

## Internal (Automatic / Side-Effect)

| Function | Where | What it does |
|----------|-------|-------------|
| `cleanupOrphanedProcesses()` | `miner.go` | Auto-runs after nuke/stale cleanup |
| `selfNukeMiner()` | `done.go` | Self-destructs worktree during `ms done` |
| `selfKillSession()` | `done.go` | Self-terminates tmux session |
| `rollbackSlingArtifacts()` | `sling.go` | Cleans up partial sling failures |
| `cleanStaleHookedBeads()` | `unsling.go` | Repairs beads stuck in "hooked" state |
| `ms signal stop` | `signal_stop.go` | Clears stop-state temp files at turn boundaries |
| `make install` | `Makefile` | Removes stale `~/go/bin/ms` and `~/bin/ms` binaries |

---

## Cleanup Layers (Low to High Severity)

| Layer | Scope | Key Commands |
|-------|-------|-------------|
| **L0** | Ephemeral data | `ms compact`, `ms krc prune` (TTL-based lifecycle) |
| **L1** | Processes | `ms cleanup`, `ms orphans procs kill`, `ms supervisor cleanup-orphans` |
| **L2** | Git artifacts | `ms prune-branches`, `ms miner gc`, `ms orphans kill` |
| **L3** | Agents/sessions | `ms miner nuke`, `ms done`, `ms shutdown`, `ms down` |
| **L4** | Workspace | `ms rig reset`, `ms doctor --fix`, `ms dolt cleanup` |
| **L5** | System | `ms uninstall`, `ms disable --clean` |

**Total: ~62 commands/functions** across the cleanup ecosystem.
