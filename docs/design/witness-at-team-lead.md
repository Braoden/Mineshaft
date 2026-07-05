# Witness AT Team Lead: Implementation Spec

> **Status: Future architecture — NOT YET IMPLEMENTED**
> The current system uses tmux-based session management. This document describes
> a planned architectural change to use Claude Code Agent Teams (AT) as the
> transport layer. No code for this exists yet.

> **Bead:** ms-ky4jf
> **Date:** 2026-02-08
> **Author:** furiosa (mineshaft miner)
> **Depends on:** AT spike report (ms-3nqoz), AT integration design (agent-teams-integration.md)
> **Status:** Phase 1 implementation spec

---

## Overview

This document specifies how the Witness becomes an AT team lead, replacing the
current tmux-based miner session management with Claude Code Agent Teams.

The Witness enters delegate mode (structurally enforced coordination-only), spawns
miner teammates for assigned work, monitors them via AT's native lifecycle hooks,
and syncs completions to beads at task boundaries.

**What changes:** Session management layer (tmux → AT).
**What stays:** Beads as ledger, ms mail for cross-rig, molecules/formulas, `ms done`.

---

## AT Spike Findings Summary

> Condensed from the AT spike report (ms-3nqoz, 2026-02-08, author: nux).

**Recommendation: CONDITIONAL GO for Phase 1 experiment.**

### Go/No-Go Decision Matrix

| Criterion | Status | Notes |
|-----------|--------|-------|
| Teammate working directories | WORKAROUND | PreToolUse hook for enforcement |
| Hooks fire for teammates | GO | All relevant hooks confirmed |
| Custom agent definitions | GO | `.claude/agents/*.md` works |
| Delegate mode enforcement | GO | Structural, not behavioral |
| Teammate cycling | WORKAROUND | Handoff + respawn pattern |
| Token cost acceptable | CONDITIONAL | Sonnet teammates reduce cost |
| ms/bd command access | GO | PATH via SessionStart hook |
| Task list with dependencies | GO | Native match to Mineshaft workflow |

5/8 clear GO. 2 require workarounds (viable mitigations). 1 conditional on Phase 1 cost validation.

### Critical Blockers

1. **No per-teammate working directory** — AT teammates inherit lead's cwd. Workaround: `cd` in spawn prompt + PreToolUse hook (`ms validate-worktree-scope`) for structural enforcement.
2. **No session resumption for teammates** — Crashed teammates cannot resume. Workaround: PreCompact handoff + beads state recovery + Witness respawn.
3. **Token cost ~7x per teammate** — Mitigated by using Sonnet for miner teammates, Opus for Witness lead only.

### Risk Register Summary

| Risk Level | Key Risks |
|------------|-----------|
| **High** | No per-teammate cwd, no session resumption, experimental feature |
| **Medium** | 7x token cost, hook compatibility gaps, AT API changes |
| **Low** | PATH/env setup, task list mapping, delegate mode gaps |

### Key Advantage

AT's file-locked task claiming eliminates Dolt write contention (estimated 80-90% reduction). This is the strongest argument for adoption.

---

## 1. Witness in Delegate Mode

### What Tools the Witness Keeps

In delegate mode, the Witness has access to:

| Tool | Purpose |
|------|---------|
| `Teammate` | Spawn/shutdown teammates, send messages, manage team |
| `TaskCreate` | Create AT tasks for miner work |
| `TaskUpdate` | Update task status, set dependencies |
| `TaskList` | Monitor team progress |
| `TaskGet` | Read task details |
| `Bash` | **Not available** in delegate mode |
| `Read/Write/Edit` | **Not available** in delegate mode |
| `Glob/Grep` | **Not available** in delegate mode |

### The ZFC Upgrade

Current state: "Witness doesn't implement" is enforced by CLAUDE.md instructions.
Agents can and do violate this under pressure.

New state: Delegate mode structurally removes implementation tools. The Witness
literally *cannot* edit files. This is the strongest possible ZFC compliance —
the constraint is in the machinery, not in the instructions.

### Witness Needs Bash for ms/bd Commands

**Problem:** Delegate mode removes Bash access, but the Witness needs to run
`ms mail`, `bd show`, `bd close`, and other coordination commands.

**Solution options (in order of preference):**

1. **Custom agent definition with selective tools.** Create
   `.claude/agents/witness-lead.md` that uses `permissionMode: delegate` but
   adds back Bash via the `tools` allowlist. This gives structural enforcement
   for file editing while preserving command access:

   ```yaml
   ---
   name: witness-lead
   permissionMode: delegate
   tools: Teammate, TaskCreate, TaskUpdate, TaskList, TaskGet, Bash
   ---
   ```

   **Risk:** Bash access means the Witness *could* edit files via sed/echo.
   Mitigated by: PreToolUse hook on Bash that rejects file-modifying commands.

2. **Hooks as command proxy.** The Witness doesn't run commands directly.
   Instead, hooks fire at turn boundaries and execute ms/bd commands based on
   AT task state. The Witness coordinates purely through AT tools; the hooks
   handle the beads bridge.

   **Risk:** Less flexible — Witness can't make ad-hoc bd queries. But it's
   the purest delegate mode implementation.

3. **Teammate as command runner.** Spawn a lightweight "ops" teammate whose
   sole job is running ms/bd commands on the Witness's behalf. The Witness
   sends commands via AT messaging; the ops teammate executes and returns results.

   **Risk:** Token overhead for a simple command proxy. But it preserves
   strict delegate mode for the Witness.

**Recommendation:** Option 1 (custom agent with selective tools). It's pragmatic,
preserves the Witness's ability to query beads state, and the PreToolUse hook
provides sufficient guardrails. Pure delegate mode is aspirational but the
Witness genuinely needs to read beads state for coordination decisions.

### PreToolUse Guard for Witness Bash

```json
{
  "PreToolUse": [{
    "matcher": "Bash",
    "hooks": [{
      "type": "command",
      "command": "ms witness-bash-guard"
    }]
  }]
}
```

The `ms witness-bash-guard` script:
- Allows: `ms *`, `bd *`, `git status`, `git log`, read-only commands
- Blocks: `echo >`, `cat >`, `sed -i`, `vim`, `nano`, any write operation
- Returns exit code 2 with reason on block

---

## 2. Teammate Spawn: Work Assignment → AT Task Creation

### The Spawn Flow

When work arrives (via minecart, ms sling, or direct assignment):

```
1. Witness receives work (mail, minecart dispatch, bd ready)
2. Witness creates AT team (if not already active)
3. For each issue to dispatch:
   a. Create AT task with issue details and dependencies
   b. Spawn miner teammate assigned to that task
4. Teammates self-claim tasks and begin execution
```

### Team Creation

```
Teammate({
  operation: "spawnTeam",
  team_name: "<rig-name>-work",
  description: "Miner work team for <minecart/sprint description>"
})
```

Team naming convention: `<rig>-work` for the primary work team.
One team per rig per active minecart. Multiple minecarts = multiple teams
(AT limitation: one team per session, so Witness manages one minecart
at a time).

### AT Task Creation from Beads Issues

For each issue dispatched to a miner:

```
TaskCreate({
  subject: "<issue title>",
  description: "Issue: <issue-id>\n<issue description>\n\nWorktree: /path/to/<miner>/\nFormula: mol-miner-work",
  activeForm: "Working on <issue title>",
  metadata: {
    "bead_id": "<issue-id>",
    "worktree": "/path/to/worktree",
    "molecule": "<mol-id>"
  }
})
```

**Key fields in metadata:**
- `bead_id`: Links AT task back to the beads issue for sync
- `worktree`: The git worktree path this miner should use
- `molecule`: The mol-miner-work instance for this issue

### Dependency Mapping

Beads issue dependencies map to AT task dependencies:

```
# If issue B depends on issue A:
# After creating both tasks:
TaskUpdate({
  taskId: "<task-B-id>",
  addBlockedBy: ["<task-A-id>"]
})
```

This enables AT's native self-claim: when task A completes, task B becomes
unblocked and the next idle teammate picks it up automatically.

### Miner Teammate Spawn

```
Task({
  subagent_type: "miner",
  team_name: "<rig>-work",
  name: "<miner-name>",
  model: "sonnet",
  prompt: "You are miner <name>. Your worktree is <path>.\n\nAssigned issue: <id> - <title>\n<description>\n\nWorkflow:\n1. cd <worktree>\n2. Run `ms prime` for full context\n3. Follow mol-miner-work steps\n4. When done: commit, push, run `ms done`"
})
```

**Model selection:**
- Miner teammates: `model: "sonnet"` (execution-focused, cost-efficient)
- Witness lead: Opus (judgment, coordination, quality review)
- Refinery teammate (Phase 2): `model: "sonnet"` (mechanical merge work)

### The `.claude/agents/miner.md` Definition

```yaml
---
name: miner
description: Mineshaft miner worker agent (persistent identity, ephemeral sessions)
model: sonnet
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && ms prime --hook"
  PreToolUse:
    - matcher: "Write|Edit"
      hooks:
        - type: command
          command: "ms validate-worktree-scope"
  PreCompact:
    - matcher: "auto"
      hooks:
        - type: command
          command: "ms handoff --reason compaction"
  Stop:
    - hooks:
        - type: command
          command: "ms signal stop"
---

You are a Mineshaft miner (persistent identity, ephemeral sessions).

## Startup
1. `cd` to your assigned worktree (given in your spawn prompt)
2. Run `ms prime` for full context
3. Check your hook: `ms hook`
4. Follow molecule steps: `bd mol current`

## Work Protocol
- Mark steps in_progress before starting: `bd update <id> --status=in_progress`
- Close steps when done: `bd close <id>`
- Commit frequently with descriptive messages
- Never batch-close steps

## Completion
When all steps done:
1. `git status` — must be clean
2. `git push`
3. `ms done` — submits to merge queue, nukes your sandbox
```

### Worktree Assignment

Each miner teammate operates in its own git worktree. Since AT doesn't support
per-teammate working directories natively, enforcement is via:

1. **Spawn prompt:** First instruction is `cd /path/to/worktree`
2. **PreToolUse hook:** `ms validate-worktree-scope` rejects Write/Edit operations
   targeting paths outside the assigned worktree
3. **Environment variable:** `MS_WORKTREE=/path/to/worktree` set via SessionStart hook

The Witness creates worktrees before spawning teammates:
```bash
git worktree add /path/to/miners/<name>/<rig> -b miner/<name>/<issue-id>
```

This matches the current worktree management — the change is WHO creates them
(Witness via AT, not `ms sling` via Go daemon).

---

## 3. Bead Sync Protocol

### The Two-Layer Model

```
Layer 1 (AT, ephemeral):     Task claiming, status, messaging
Layer 2 (Beads/Dolt, durable): Issue creation, completion, audit trail
```

### Sync Points

| AT Event | Beads Action | Trigger |
|----------|-------------|---------|
| Task claimed (in_progress) | `bd update <id> --status=in_progress` | TaskCompleted hook / miner prompt |
| Task completed | `bd close <step-id>` | TaskCompleted hook |
| New issue discovered | AT task created by Witness | Witness reads miner message |
| Teammate idle | Check beads for more work | TeammateIdle hook |
| Team shutdown | Verify all beads synced | Witness cleanup routine |

### TaskCompleted Hook for Bead Sync

The `TaskCompleted` hook fires when an AT task is marked complete. This is the
primary sync mechanism:

```bash
#!/bin/bash
# .claude/hooks/task-completed-sync.sh
# Fires on TaskCompleted hook

BEAD_ID=$(echo "$TASK_METADATA" | jq -r '.bead_id // empty')
if [ -n "$BEAD_ID" ]; then
  export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"
  bd close "$BEAD_ID" 2>/dev/null
fi
exit 0
```

Hook configuration:
```json
{
  "TaskCompleted": [{
    "hooks": [{
      "type": "command",
      "command": ".claude/hooks/task-completed-sync.sh"
    }]
  }]
}
```

**Important:** The hook should NOT block task completion (exit 0 always). If the
`bd close` fails (Dolt contention), it will be retried at the next sync point.
The AT task list is the real-time truth; beads catches up at boundaries.

### Miner-Side Bead Updates

Miners still run `bd update` and `bd close` directly as part of their molecule
workflow. The TaskCompleted hook is a safety net, not the primary mechanism. This
means:

- Miner marks molecule step in_progress → `bd update --status=in_progress`
- Miner completes molecule step → `bd close <step-id>`
- AT task completion → TaskCompleted hook also fires `bd close` (idempotent)

Double-close is safe: `bd close` on an already-closed bead is a no-op.

### Sync Verification at Team Shutdown

Before the Witness shuts down the team, it verifies beads are in sync:

```
For each AT task marked completed:
  1. Read task metadata for bead_id
  2. Verify bead is closed (bd show <id> | check status)
  3. If bead still open: bd close <id> with notes
  4. If close fails: log warning, continue (Dolt retry will handle)
```

This is the "boundary sync" pattern from the integration design: AT handles
real-time coordination, beads catches up at lifecycle boundaries (team shutdown,
minecart completion).

---

## 4. Session Cycling: Compaction → Respawn → Resume

### The Problem

AT teammates cannot be resumed after shutdown. When a teammate hits context
limits and compacts, or crashes, a new teammate must be spawned.

### The Lifecycle

```
Teammate running
    │
    ├── Context filling → PreCompact hook fires
    │   │
    │   └── ms handoff --reason compaction
    │       ├── Saves current molecule step to beads
    │       ├── Saves progress notes
    │       └── Saves git branch state
    │
    ├── Auto-compaction occurs
    │   │
    │   └── SessionStart hook fires (source: "compact")
    │       └── ms prime --compact-resume
    │           └── Reads beads state, restores context
    │
    └── Teammate continues with compressed context
```

### When Compaction Isn't Enough (Teammate Death)

If a teammate crashes or is shut down (not just compacted):

```
Teammate stops
    │
    └── SubagentStop hook fires on Witness (lead)
        │
        ├── Read teammate's last known state from beads
        │   └── Which molecule step was in_progress?
        │   └── What branch was being worked on?
        │
        ├── Assess: recoverable or escalate?
        │   ├── Normal completion: AT task done, beads synced → no action
        │   ├── Incomplete work: respawn with resume context
        │   └── Repeated crashes: escalate to Witness mail → Overseer
        │
        └── If recoverable: spawn replacement teammate
            └── Task({ subagent_type: "miner", ... resume prompt ... })
```

### SubagentStop Hook (Witness Side)

```json
{
  "SubagentStop": [{
    "matcher": "miner",
    "hooks": [{
      "type": "command",
      "command": "ms witness-teammate-stopped"
    }]
  }]
}
```

The `ms witness-teammate-stopped` script:
1. Reads the stopped agent's transcript path (available in hook input)
2. Checks AT task status — was the task completed?
3. Checks beads — was `ms done` run?
4. If completed: no action (normal lifecycle)
5. If incomplete: outputs `{ "decision": "block", "reason": "Teammate <name> stopped before completing task <id>. Beads state: <status>. Respawn needed." }`

The "block" decision prevents the Witness from going idle, injecting the
respawn instruction as context for the Witness to act on.

### Respawn Prompt Template

```
Teammate <name> stopped before completing work.

Last known state:
- Issue: <bead-id> (<title>)
- Molecule step: <step-id> (in_progress)
- Branch: <branch-name>
- Worktree: <path>

Spawn a replacement miner with this context. The new teammate
should read beads state and continue from the last checkpoint.
```

### Crash Loop Prevention

Track respawn attempts per issue. If a teammate crashes 3 times on the
same issue:

1. Mark the AT task as blocked
2. File a bead: `bd create --title "Miner crash loop on <issue>" --type bug`
3. Mail the Witness/Overseer for escalation
4. Do NOT respawn — the issue has a structural problem

Tracking: Use AT task metadata `{ "respawn_count": N }` incremented on
each respawn. This is ephemeral (dies with the team) which is correct —
crash tracking only matters during the current team session.

---

## 5. Error Handling

### Error Categories and Responses

| Error | Detection | Response |
|-------|-----------|----------|
| Teammate crash | SubagentStop hook | Respawn or escalate (see above) |
| Teammate stuck (no progress) | TeammateIdle hook | Send message asking for status |
| Test failures | TaskCompleted hook (exit 2) | Block completion, teammate must fix |
| Merge conflict | Miner messages Witness | Witness advises or reassigns |
| Dolt write failure | bd command exit code | Retry with backoff (existing mechanism) |
| AT team crash | Witness session dies | Daemon/Boot/Supervisor chain detects, restarts Witness |
| Worktree scope violation | PreToolUse hook | Block the operation, warn miner |

### TeammateIdle Hook

```bash
#!/bin/bash
# ms witness-teammate-idle
# Fires when a teammate is about to go idle

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check if there's more work in beads
READY=$(bd ready --count 2>/dev/null)
if [ "$READY" -gt 0 ]; then
  echo "There is more work available. Run 'bd ready' to see unblocked tasks." >&2
  exit 2  # Block idle, send feedback
fi

# Check if ms done was run
if git log --oneline -1 | grep -q "ms done"; then
  exit 0  # Normal completion
fi

# Teammate seems genuinely idle without completing
echo "Your work doesn't appear complete. Run 'bd ready' to check remaining steps, or 'ms done' if finished." >&2
exit 2
```

### TaskCompleted Quality Gate

```bash
#!/bin/bash
# Fires on TaskCompleted hook
# Validates that work meets minimum quality before marking complete

export PATH="$HOME/go/bin:$HOME/.local/bin:$PATH"

# Check for uncommitted changes
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  echo "Uncommitted changes detected. Commit your work before marking complete." >&2
  exit 2
fi

# Check that the branch has been pushed
BRANCH=$(git branch --show-current 2>/dev/null)
if ! git log "origin/$BRANCH" --oneline -1 >/dev/null 2>&1; then
  echo "Branch not pushed to remote. Run 'git push' before completing." >&2
  exit 2
fi

exit 0
```

---

## 6. Minecart Mapping to AT Teams

### The Natural Mapping

| Mineshaft | AT Equivalent |
|----------|--------------|
| Minecart | AT team lifecycle |
| Minecart issues | AT tasks |
| War Rig (per-rig minecart execution) | AT team instance |
| Ready front (unblocked issues) | Unblocked AT tasks |
| Dispatch | AT task creation + teammate spawn |
| Completion tracking | AT task list status |

### One Minecart = One AT Team Session

A minecart arrives at a rig. The Witness creates an AT team for that minecart:

```
Minecart hq-abc arrives at mineshaft
    │
    ├── Witness creates team: "mineshaft-minecart-abc"
    │
    ├── For each issue in minecart:
    │   ├── Create AT task (with bead_id in metadata)
    │   └── Set dependencies (from beads dep graph)
    │
    ├── Spawn N miner teammates (N = min(issues, max_miners))
    │
    ├── Teammates self-claim tasks from ready front
    │
    ├── As tasks complete:
    │   ├── Dependencies unblock next tasks
    │   ├── Idle teammates auto-claim newly ready tasks
    │   └── Beads synced via TaskCompleted hook
    │
    └── All tasks done:
        ├── Witness verifies beads sync
        ├── Witness sends minecart completion to Overseer (ms mail)
        └── Team shutdown
```

### Multiple Minecarts

AT limitation: one team per session. If a second minecart arrives while the
first is active:

**Option A: Sequential processing.** Finish minecart 1, then start minecart 2.
Simple, no concurrency issues. Acceptable if minecart throughput is sufficient.

**Option B: Minecart queue.** The Witness queues incoming minecarts and processes
them in order. The queue lives in beads (mail inbox) — the Witness checks for
new minecarts when the current team finishes.

**Option C: Multiple Witness sessions.** The daemon spawns a second Witness
session for the second minecart. Each Witness manages its own AT team. This
requires the daemon to support multiple Witness instances per rig.

**Recommendation:** Option A for Phase 1 (sequential). Option C for Phase 2+
if throughput demands it. The minecart queue in Option B is implicit in beads
already (unprocessed minecart mail = queued work).

### Steady-State Worker Pool

For large minecarts (20+ issues), the Witness doesn't spawn 20 teammates at once.
Instead:

```
max_teammates = 5  # configurable per rig

1. Spawn max_teammates miners
2. Create all AT tasks (with dependencies)
3. Teammates self-claim from ready front
4. As teammates complete tasks:
   - Auto-claim next unblocked task
   - No respawn needed (same teammate, new task)
5. When all tasks done: team shutdown
```

AT's self-claim mechanism is the key enabler. Teammates don't die after one
task — they pick up the next one. This eliminates the current spawn/nuke
overhead per issue.

**When a teammate needs to cycle** (compaction), the Witness spawns a
replacement, not an additional teammate. The pool size stays at max_teammates.

---

## 7. Mail Bridge: ms mail ↔ AT Messages

### The Boundary

```
                    ┌─────────────────┐
                    │    Witness       │
                    │  (AT Team Lead)  │
                    │                  │
    ms mail ←──────│── Bridge ──────→ AT messaging
    (cross-rig,    │                  (intra-team,
     persistent)   │                   ephemeral)
                    └─────────────────┘
```

### Inbound: ms mail → AT message

When the Witness receives ms mail relevant to an active teammate:

```
ms mail inbox
    │
    ├── From Overseer: "Priority shift — issue X is now P0"
    │   └── Witness sends AT message to relevant teammate:
    │       Teammate({ operation: "write", target_agent_id: "<miner>",
    │                  value: "Priority update: <issue> is now P0. Expedite." })
    │
    ├── From Refinery: "Merge conflict on <branch>"
    │   └── Witness sends AT message to the miner on that branch:
    │       Teammate({ operation: "write", target_agent_id: "<miner>",
    │                  value: "Merge conflict detected. Rebase on main." })
    │
    └── From another rig's Witness: "Dependency <issue> is done"
        └── Witness creates/unblocks AT task for downstream work
```

### Outbound: AT event → ms mail

When AT events need to reach entities outside the team:

```
Teammate completes final task
    │
    └── Witness detects all tasks done
        │
        ├── ms mail send mineshaft/refinery -s "MERGE_READY: <branch>"
        │   └── Refinery processes merge queue
        │
        ├── ms mail send overseer/ -s "MINECART COMPLETE: hq-abc"
        │   └── Overseer updates minecart tracking
        │
        └── ms mail send mineshaft/witness -s "MINER_DONE: <name>"
            └── (Self-mail for beads record)
```

### What Goes Where

| Communication | Channel | Why |
|--------------|---------|-----|
| Witness ↔ Miner | AT messaging | Same team, real-time, ephemeral |
| Miner ↔ Miner | AT messaging | Same team, coordination chatter |
| Witness → Refinery | ms mail | Different lifecycle, needs persistence |
| Witness → Overseer | ms mail | Cross-rig, needs persistence |
| Overseer → Witness | ms mail | Cross-rig, needs persistence |
| Miner escalation | AT message to Witness, Witness relays via ms mail | Bridge pattern |

### The Relay Pattern

Miners can't send ms mail directly to entities outside their team (AT
messaging is team-scoped). Instead:

```
Miner needs to escalate to Overseer:
    │
    ├── Miner sends AT message to Witness:
    │   "ESCALATE: Need Overseer decision on auth approach"
    │
    └── Witness relays via ms mail:
        ms mail send overseer/ -s "ESCALATE from miner <name>" -m "..."
```

This is analogous to the current model where miners mail the Witness and
the Witness escalates. The difference: AT messaging is real-time (no Dolt
sync lag), and the Witness can relay immediately.

---

## 8. Configuration

### `.claude/settings.json` (Project Level)

```json
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  },
  "hooks": {
    "TaskCompleted": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/task-completed-sync.sh"
      }]
    }],
    "TeammateIdle": [{
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-idle-check.sh"
      }]
    }],
    "SubagentStop": [{
      "matcher": "miner",
      "hooks": [{
        "type": "command",
        "command": ".claude/hooks/teammate-stopped.sh"
      }]
    }]
  }
}
```

### `.claude/agents/witness-lead.md`

```yaml
---
name: witness-lead
description: Mineshaft Witness operating as AT team lead
model: opus
permissionMode: delegate
hooks:
  SessionStart:
    - hooks:
        - type: command
          command: "export PATH=\"$HOME/go/bin:$HOME/.local/bin:$PATH\" && ms prime --hook"
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: "ms witness-bash-guard"
  Stop:
    - hooks:
        - type: command
          command: "ms signal stop"
---

You are the Mineshaft Witness for this rig.

## Role
You coordinate miner workers. You NEVER implement code directly.
Delegate mode enforces this structurally — you cannot edit files.

## Startup
1. Check for incoming work: `ms mail inbox`, `bd ready`
2. Create AT team if work is available
3. Spawn miner teammates for each issue
4. Monitor progress via AT task list

## During Work
- Monitor teammate progress via TaskList
- Relay cross-rig messages (ms mail ↔ AT messages)
- Handle teammate crashes (respawn or escalate)
- Enforce quality via plan approval

## Completion
- Verify all AT tasks completed
- Verify beads are synced (all issues closed)
- Send MERGE_READY to Refinery via ms mail
- Send minecart completion to Overseer via ms mail
- Shutdown team
```

### `.claude/agents/miner.md`

See Section 2 above for the full definition.

---

## 9. What Gets Replaced

### Infrastructure Removed (Phase 1)

| Component | Replacement | Notes |
|-----------|-------------|-------|
| `ms sling` (miner spawn) | `Teammate({ operation: "spawn" })` | AT native |
| `ms miner nuke` | `Teammate({ operation: "requestShutdown" })` | AT native |
| tmux session management | AT manages teammate sessions | No more tmux for miners |
| `ms nudge` (tmux send-keys) | `Teammate({ operation: "write" })` | AT messaging |
| Zombie detection (tmux-based) | SubagentStop / TeammateIdle hooks | Structural |
| Witness "are you stuck?" polling | TeammateIdle hook (automatic) | Event-driven |
| Miner-to-miner isolation | Prompt + PreToolUse hook | Behavioral → hook-enforced |

### Infrastructure Kept (Phase 1)

| Component | Why |
|-----------|-----|
| Beads (Dolt) | Durable ledger — AT tasks are ephemeral |
| ms mail | Cross-rig communication — AT is team-scoped |
| Molecules/formulas | Work templates — AT tasks created from these |
| `ms done` | Miner self-clean — unchanged lifecycle |
| Git worktrees | Filesystem isolation — AT doesn't provide this |
| Daemon/Boot/Supervisor | Health monitoring — AT has no crash recovery |
| Refinery (separate) | Different lifecycle (Phase 2 brings it in-band) |
| Minecart tracking | Cross-rig work orders — above AT scope |

### Dolt Write Pressure Reduction

**Current:** Every `bd update`, `bd close`, `bd create` from every miner
= concurrent Dolt writes. 20 miners = 20+ concurrent commits.

**With AT:** Real-time task coordination happens in AT (file-locked, no Dolt).
Dolt writes only at boundaries:
- `bd close` when a molecule step completes (1 per task)
- `bd create` when miners discover new issues (rare)

**Estimated reduction: 80-90%.** The remaining writes are naturally staggered
across minutes (task completions), not milliseconds (concurrent status updates).

---

## 10. Witness Startup Flow (Updated)

```
Witness session starts (managed by daemon)
    │
    ├── SessionStart hook: ms prime --hook
    │   └── Loads role context, checks hook
    │
    ├── Check for work:
    │   ├── ms mail inbox (minecart dispatch, priority changes)
    │   ├── bd ready (unblocked issues)
    │   └── ms hook (hooked work)
    │
    ├── If work available:
    │   │
    │   ├── Create AT team:
    │   │   Teammate({ operation: "spawnTeam", team_name: "<rig>-work" })
    │   │
    │   ├── Create AT tasks from beads issues:
    │   │   For each issue: TaskCreate({ subject, description, metadata: { bead_id } })
    │   │   Set dependencies: TaskUpdate({ addBlockedBy: [...] })
    │   │
    │   ├── Create worktrees for miners:
    │   │   For each miner: git worktree add ...
    │   │
    │   ├── Spawn miner teammates:
    │   │   For each (up to max_teammates):
    │   │     Task({ subagent_type: "miner", team_name: "...", name: "..." })
    │   │
    │   └── Enter monitoring loop:
    │       ├── Watch AT task list for completions
    │       ├── Handle teammate crashes (SubagentStop)
    │       ├── Relay ms mail ↔ AT messages
    │       ├── Check for new minecart arrivals
    │       └── When all tasks done: cleanup and report
    │
    └── If no work:
        └── Stop hook checks for queued work periodically
            └── If work arrives: wake and create team
```

---

## 11. Phase 1 Scope and Validation Criteria

### In Scope

1. Witness as AT team lead in delegate mode (with Bash for ms/bd)
2. Miner teammates with `.claude/agents/miner.md`
3. Bead sync via TaskCompleted hook
4. Session cycling via PreCompact handoff + respawn
5. Basic error handling (crash detection, respawn, crash loop prevention)
6. Mail bridge (ms mail ↔ AT messaging)
7. Single-minecart sequential processing

### Out of Scope (Phase 2+)

1. Refinery as AT teammate
2. Multiple concurrent minecarts
3. Cross-rig AT coordination
4. Crew squads / shadow workers
5. Advanced plan approval workflows
6. Performance optimization (token cost tuning)

### Validation Criteria

| Criterion | Test |
|-----------|------|
| Witness stays in delegate mode | Verify Witness cannot write/edit files |
| Miners complete work | End-to-end: spawn → implement → push → ms done |
| Beads sync correctly | AT task completion → bd close fires → bead is closed |
| Session cycling works | Force compaction → new teammate resumes from beads |
| Crash recovery works | Kill a teammate → Witness detects → respawns |
| Mail bridge works | Overseer sends mail → Witness relays to miner |
| Dolt writes reduced | Measure bd command frequency: before vs after |
| Token cost acceptable | `/cost` shows < 3x overhead vs current model |
| Minecart completes | Full minecart lifecycle: dispatch → work → merge → done |

---

## 12. Migration Path

### Current Architecture → Phase 1

The transition is additive: AT runs alongside existing infrastructure during
validation. The Witness can fall back to tmux-based management if AT fails.

```
Step 1: Enable AT feature flag in mineshaft .claude/settings.json
Step 2: Create .claude/agents/miner.md and .claude/agents/witness-lead.md
Step 3: Implement hook scripts (task-completed-sync, teammate-idle, teammate-stopped)
Step 4: Implement ms witness-bash-guard
Step 5: Implement ms validate-worktree-scope
Step 6: Implement ms witness-teammate-stopped
Step 7: Update Witness startup to create AT team instead of tmux miner sessions
Step 8: Test with 2 miners on a small minecart
Step 9: Validate all criteria above
Step 10: If validated: expand to 3-5 miners, larger minecarts
```

### Rollback Plan

If Phase 1 fails:
1. Disable AT feature flag
2. Witness reverts to tmux-based miner management
3. No beads data lost (beads sync is additive)
4. File lessons-learned bead for Phase 1 retry

---

*"The transport changes. The ledger endures."*
