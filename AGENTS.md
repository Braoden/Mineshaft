# Agent Instructions

See **CLAUDE.md** for complete agent context and instructions.

This file exists for compatibility with tools that look for AGENTS.md.

> **Recovery**: Run `ms prime` after compaction, clear, or new session

Full context is injected by `ms prime` at session start.

<!-- beads-agent-instructions-v2 -->

---

## Beads Workflow Integration

This project uses [beads](https://github.com/steveyegge/beads) for issue tracking. Issues live in `.beads/` and are tracked in git.

Two CLIs: **bd** (issue CRUD) and **bv** (graph-aware triage, read-only).

### bd: Issue Management

```bash
bd ready              # Unblocked issues ready to work
bd list --status=open # All open issues
bd show <id>          # Full details with dependencies
bd create --title="..." --type=task --priority=2
bd update <id> --status=in_progress
bd close <id>         # Mark complete
bd close <id1> <id2>  # Close multiple
bd dep add <a> <b>    # a depends on b
bd sync               # Sync with git
```

### bv: Graph Analysis (read-only)

**NEVER run bare `bv`** — it launches interactive TUI. Always use `--robot-*` flags:

```bash
bv --robot-triage     # Ranked picks, quick wins, blockers, health
bv --robot-next       # Single top pick + claim command
bv --robot-plan       # Parallel execution tracks
bv --robot-alerts     # Stale issues, cascades, mismatches
bv --robot-insights   # Full graph metrics: PageRank, betweenness, cycles
```

### Workflow

1. **Start**: `bd ready` (or `bv --robot-triage` for graph analysis)
2. **Claim**: `bd update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: `bd close <id>`
5. **Sync**: `bd sync` at session end

### Session Close Protocol

```bash
git status            # Check what changed
git add <files>       # Stage code changes
bd sync               # Commit beads changes
git commit -m "..."   # Commit code
bd sync               # Commit any new beads changes
git push              # Push to remote
```

### Key Concepts

- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (numbers only)
- **Types**: task, bug, feature, epic, question, docs
- **Dependencies**: `bd ready` shows only unblocked work

<!-- end-beads-agent-instructions -->

<!-- mineshaft-agent-instructions-v1 -->

---

## Mineshaft Multi-Agent Communication

This workspace is part of a **Mineshaft** multi-agent environment. You communicate
with other agents using `ms` commands — never by printing text or using raw tmux.

### Nudging Agents (Immediate Delivery)

`ms nudge` sends a message directly to another agent's active session:

```bash
ms nudge overseer "Status update: PR review complete"
ms nudge laneassist/crew/dom "Check your mail — PR ready for review"
ms nudge witness "Miner health check needed"
ms nudge refinery "Merge queue has items"
```

**Target formats:**
- Role shortcuts: `overseer`, `supervisor`, `witness`, `refinery`
- Full path: `<rig>/crew/<name>`, `<rig>/miners/<name>`

**Important:** `ms nudge` is the ONLY way to send text to another agent's session.
Never print "Hey @name" — the other agent cannot see your terminal output.

### Sending Mail (Persistent Messages)

`ms mail` sends messages that persist across session restarts:

```bash
# Reading
ms mail inbox                    # List messages
ms mail read <id>                # Read a specific message

# Sending (use --stdin for multi-line content)
ms mail send overseer/ -s "Subject" -m "Short message"
ms mail send laneassist/crew/dom -s "PR Review" --stdin <<'BODY'
Multi-line message content here.
Details about the PR and what to look for.
BODY
ms mail send --human -s "Subject" -m "Message to boss"
```

### When to Use Which

| Want to... | Command | Why |
|------------|---------|-----|
| Wake a sleeping agent | `ms nudge <target> "msg"` | Immediate delivery |
| Send detailed task/info | `ms mail send <target> -s "..." --stdin` | Persists across restarts |
| Both: send + wake | `ms mail send` then `ms nudge` | Mail carries payload, nudge wakes |

### Context Recovery

After compaction or new session, run `ms prime` to reload your full role context,
identity, and any pending work.

```bash
ms prime              # Full context reload
ms hook               # Check for assigned work
ms mail inbox         # Check for messages
```

<!-- end-mineshaft-agent-instructions -->

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `ms prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `ms prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
