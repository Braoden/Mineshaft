# Mineshaft/Beads Cleanup Commands Reference

A comprehensive catalog of all cleanup-related commands in the mineshaft/beads ecosystem, organized by scope and severity.

---

## Process Cleanup

| Command | What it does |
|---------|-------------|
| `gt cleanup` | Kills orphaned Claude processes not tied to active tmux sessions |
| `gt orphans procs list` | Lists orphaned Claude processes (PPID=1) |
| `gt orphans procs kill` | Kills orphaned Claude processes (`--aggressive` for tmux-verified) |
| `gt supervisor cleanup-orphans` | Kills orphaned Claude subagent processes (no controlling TTY) |
| `gt supervisor zombie-scan` | Finds/kills zombie Claude processes not in active tmux sessions |

## Miner (Agent Sandbox) Cleanup

| Command | What it does |
|---------|-------------|
| `gt miner remove <rig>/<miner>` | Removes miner worktree/directory (fails if session running) |
| `gt miner nuke <rig>/<miner>` | Nuclear: kills session, deletes worktree, deletes branch, closes bead |
| `gt miner nuke <rig> --all` | Nukes all miners in a rig |
| `gt miner gc <rig>` | GC stale miner branches (orphaned, old timestamped) |
| `gt miner stale <rig>` | Detects stale miners; `--cleanup` auto-nukes them |
| `gt miner check-recovery` | Pre-nuke safety check (SAFE_TO_NUKE vs NEEDS_RECOVERY) |
| `gt miner identity remove <rig> <name>` | Removes a miner identity |
| `gt done` | Miner self-cleaning: pushes branch, submits MR (by default), self-nukes worktree, kills own session. MR skipped for `--status ESCALATED\|DEFERRED` or `no_merge` paths |

## Git Artifact Cleanup

| Command | What it does |
|---------|-------------|
| `gt prune-branches` | Removes stale local miner tracking branches (`git fetch --prune` + safe delete) |
| `gt orphans` | Finds orphaned commits never merged (detection only) |
| `gt orphans kill` | Prunes orphaned commits (`git gc --prune=now`) + kills orphaned processes |

## Rig-Level Cleanup

| Command | What it does |
|---------|-------------|
| `gt rig reset` | Resets handoff content, stale mail, orphaned in_progress issues |
| `gt rig reset --handoff` | Clears handoff content only |
| `gt rig reset --mail` | Clears stale mail only |
| `gt rig reset --stale` | Resets orphaned in_progress issues |
| `gt rig remove <name>` | Unregisters rig from registry, cleans up beads routes |
| `gt rig shutdown <rig>` | Stops all agents: miners, refinery, witness |
| `gt rig stop <rig>...` | Stop one or more rigs |
| `gt rig restart <rig>...` | Stop then start (stop phase cleans up) |

## Town-Wide Shutdown

| Command | What it does |
|---------|-------------|
| `gt down` | Stops all infrastructure (refinery, witness, overseer, boot, supervisor, daemon, dolt) |
| `gt down --miners` | Also stops all miner sessions |
| `gt down --all` | Full shutdown with orphan cleanup and verification |
| `gt down --nuke` | Kills entire tmux server (DESTRUCTIVE - kills non-GT sessions too) |
| `gt shutdown` | "Done for the day" - stops agents AND removes miner worktrees/branches. Flags control aggressiveness (`--graceful`, `--force`, `--nuclear`, `--miners-only`, etc.) |

## Crew Workspace Cleanup

| Command | What it does |
|---------|-------------|
| `gt crew stop [name]` | Stops crew tmux sessions |
| `gt crew restart [name]` | Kills and restarts crew fresh ("clean slate", no handoff mail) |
| `gt crew remove <name>` | Removes workspace, closes agent bead |
| `gt crew remove <name> --purge` | Full obliteration: deletes agent bead, unassigns beads, clears mail |
| `gt crew pristine [name]` | Syncs workspaces with remote (`git pull`) |

## Ephemeral Data / Event Cleanup

| Command | What it does |
|---------|-------------|
| `gt compact` | TTL-based compaction: promotes/deletes wisps past their TTL |
| `gt krc prune` | Prunes expired events from the KRC event store |
| `gt krc config reset` | Resets KRC TTL configuration to defaults |
| `gt krc decay` | Shows forensic value decay report (pruning guidance) |

## Dolt Database Cleanup

| Command | What it does |
|---------|-------------|
| `gt dolt cleanup` | Removes orphaned databases from `.dolt-data/` |
| `gt dolt stop` | Stops the Dolt SQL server |
| `gt dolt rollback [backup-dir]` | Restores `.beads` from backup, resets metadata |

## Bead / Hook Cleanup

| Command | What it does |
|---------|-------------|
| `gt close <bead-id>` | Closes beads (lifecycle termination) |
| `gt unsling` / `gt unhook` | Removes work from agent's hook, resets bead status to "open" |
| `gt hook clear` | Alias for unsling |

## Dog (Infrastructure Worker) Cleanup

| Command | What it does |
|---------|-------------|
| `gt dog remove <name>` | Removes worktrees and dog directory |
| `gt dog remove --all` | Removes all dogs |
| `gt dog clear <name>` | Resets stuck dog to idle state |
| `gt dog done [name]` | Marks dog as done, clears work field |

## Minecart Cleanup

| Command | What it does |
|---------|-------------|
| `gt minecart close <id>` | Closes a minecart bead |
| `gt minecart land <id>` | Closes minecart, cleans up miner worktrees, sends completion notifications |

## Mail Cleanup

| Command | What it does |
|---------|-------------|
| `gt mail delete <msg-id>` | Deletes specific messages |
| `gt mail archive <msg-id>` | Archives messages (`--stale` for stale ones) |
| `gt mail clear [target]` | Deletes all messages from an inbox (town quiescence) |

## Misc State Cleanup

| Command | What it does |
|---------|-------------|
| `gt namepool reset` | Releases all claimed miner names |
| `gt checkpoint clear` | Removes checkpoint file |
| `gt issue clear` | Clears issue from tmux status line |
| `gt doctor --fix` | Auto-fixes: orphan sessions, wisp GC, stale redirects, worktree validity |

## System-Level Cleanup

| Command | What it does |
|---------|-------------|
| `gt disable --clean` | Disables mineshaft + removes shell integration |
| `gt shell remove` | Removes shell integration from RC files |
| `gt config agent remove <name>` | Removes custom agent definition |
| `gt uninstall` | Full removal: shell integration, wrapper scripts, state/config/cache dirs |
| `make clean` | Removes compiled `gt` binary |

## Scripts

| Command | What it does |
|---------|-------------|
| `scripts/migration-test/reset-vm.sh` | Restores VM to pristine v0.5.0 state (test environments) |

## Internal (Automatic / Side-Effect)

| Function | Where | What it does |
|----------|-------|-------------|
| `cleanupOrphanedProcesses()` | `miner.go` | Auto-runs after nuke/stale cleanup |
| `selfNukeMiner()` | `done.go` | Self-destructs worktree during `gt done` |
| `selfKillSession()` | `done.go` | Self-terminates tmux session |
| `rollbackSlingArtifacts()` | `sling.go` | Cleans up partial sling failures |
| `cleanStaleHookedBeads()` | `unsling.go` | Repairs beads stuck in "hooked" state |
| `gt signal stop` | `signal_stop.go` | Clears stop-state temp files at turn boundaries |
| `make install` | `Makefile` | Removes stale `~/go/bin/gt` and `~/bin/gt` binaries |

---

## Cleanup Layers (Low to High Severity)

| Layer | Scope | Key Commands |
|-------|-------|-------------|
| **L0** | Ephemeral data | `gt compact`, `gt krc prune` (TTL-based lifecycle) |
| **L1** | Processes | `gt cleanup`, `gt orphans procs kill`, `gt supervisor cleanup-orphans` |
| **L2** | Git artifacts | `gt prune-branches`, `gt miner gc`, `gt orphans kill` |
| **L3** | Agents/sessions | `gt miner nuke`, `gt done`, `gt shutdown`, `gt down` |
| **L4** | Workspace | `gt rig reset`, `gt doctor --fix`, `gt dolt cleanup` |
| **L5** | System | `gt uninstall`, `gt disable --clean` |

**Total: ~62 commands/functions** across the cleanup ecosystem.
