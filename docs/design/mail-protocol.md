# Mineshaft Mail Protocol

> Reference for inter-agent mail communication in Mineshaft

## Overview

Mineshaft agents coordinate via mail messages routed through the beads system.
Mail uses `type=message` beads with routing handled by `gt mail`.

## Message Types

### MINER_DONE

**Route**: Miner → Witness

**Purpose**: Signal work completion, trigger cleanup flow.

**Subject format**: `MINER_DONE <miner-name>`

**Body format**:
```
Exit: MERGED|ESCALATED|DEFERRED
Issue: <issue-id>
MR: <mr-id>          # if exit=MERGED
Branch: <branch>
```

**Trigger**: `gt done` command generates this automatically.

**Handler**: Witness creates a cleanup wisp for the miner.

### MERGE_READY

**Route**: Witness → Refinery

**Purpose**: Signal a branch is ready for merge queue processing.

**Subject format**: `MERGE_READY <miner-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Miner: <miner-name>
Verified: clean git state, issue closed
```

**Trigger**: Witness sends after verifying miner work is complete.

**Handler**: Refinery adds to merge queue, processes when ready.

### MERGED

**Route**: Refinery → Witness

**Purpose**: Confirm branch was merged successfully, safe to nuke miner.

**Subject format**: `MERGED <miner-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Miner: <miner-name>
Rig: <rig>
Target: <target-branch>
Merged-At: <timestamp>
Merge-Commit: <sha>
```

**Trigger**: Refinery sends after successful merge to main.

**Handler**: Witness completes cleanup wisp, nukes miner worktree.

### MERGE_FAILED

**Route**: Refinery → Witness

**Purpose**: Notify that merge attempt failed (tests, build, or other non-conflict error).

**Subject format**: `MERGE_FAILED <miner-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Miner: <miner-name>
Rig: <rig>
Target: <target-branch>
Failed-At: <timestamp>
Failure-Type: <tests|build|push|other>
Error: <error-message>
```

**Trigger**: Refinery sends when merge fails for non-conflict reasons.

**Handler**: Witness notifies miner, assigns work back for rework.

### REWORK_REQUEST

**Route**: Refinery → Witness

**Purpose**: Request miner to rebase branch due to merge conflicts.

**Subject format**: `REWORK_REQUEST <miner-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Miner: <miner-name>
Rig: <rig>
Target: <target-branch>
Requested-At: <timestamp>
Conflict-Files: <file1>, <file2>, ...

Please rebase your changes onto <target-branch>:

  git fetch origin
  git rebase origin/<target-branch>
  # Resolve any conflicts
  git push -f

The Refinery will retry the merge after rebase is complete.
```

**Trigger**: Refinery sends when merge has conflicts with target branch.

**Handler**: Witness notifies miner with rebase instructions.

### RECOVERED_BEAD

**Route**: Witness → Supervisor

**Purpose**: Notify Supervisor that a dead miner's abandoned work has been recovered
and needs re-dispatch.

**Subject format**: `RECOVERED_BEAD <bead-id>`

**Body format**:
```
Recovered abandoned bead from dead miner.

Bead: <bead-id>
Miner: <rig>/<miner-name>
Previous Status: <hooked|in_progress>

The bead has been reset to open with no assignee.
Please re-dispatch to an available miner.
```

**Trigger**: Witness detects a zombie miner with work still hooked/in_progress.
The bead is reset to open status and this mail is sent for re-dispatch.

**Handler**: Supervisor runs `gt supervisor redispatch <bead-id>` which:
- Rate-limits re-dispatches (5-minute cooldown per bead)
- Tracks failure count (after 3 failures, escalates to Overseer)
- Auto-detects target rig from bead prefix
- Slings the bead to an available miner via `gt sling`

### RECOVERY_NEEDED

**Route**: Witness → Supervisor

**Purpose**: Escalate a dirty miner that has unpushed/uncommitted work needing
manual recovery before cleanup.

**Subject format**: `RECOVERY_NEEDED <rig>/<miner-name>`

**Body format**:
```
Miner: <rig>/<miner-name>
Cleanup Status: <has_uncommitted|has_stash|has_unpushed>
Branch: <branch>
Issue: <issue-id>
Detected: <timestamp>
```

**Trigger**: Witness detects zombie miner with dirty git state.

**Handler**: Supervisor coordinates recovery (push branch, save work) before
authorizing cleanup. Only escalates to Overseer if Supervisor cannot resolve.

### HELP

**Route**: Any → escalation target (usually Overseer)

**Purpose**: Request intervention for stuck/blocked work.

**Subject format**: `HELP: <brief-description>`

**Body format**:
```
Agent: <agent-id>
Issue: <issue-id>       # if applicable
Problem: <description>
Tried: <what was attempted>
```

**Trigger**: Agent unable to proceed, needs external help.

**Handler**: Escalation target assesses and intervenes.

### HANDOFF

**Route**: Agent → self (or successor)

**Purpose**: Session continuity across context limits/restarts.

**Subject format**: `🤝 HANDOFF: <brief-context>`

**Body format**:
```
attached_molecule: <molecule-id>   # if work in progress
attached_at: <timestamp>

## Context
<freeform notes for successor>

## Status
<where things stand>

## Next
<what successor should do>
```

**Trigger**: `gt handoff` command, or manual send before session end.

**Handler**: Next session reads handoff, continues from context.

## Format Conventions

### Subject Line

- **Type prefix**: Uppercase, identifies message type
- **Colon separator**: After type for structured info
- **Brief context**: Human-readable summary

Examples:
```
MINER_DONE nux
MERGE_READY greenplace/nux
HELP: Miner stuck on test failures
🤝 HANDOFF: Schema work in progress
```

### Body Structure

- **Key-value pairs**: For structured data (one per line)
- **Blank line**: Separates structured data from freeform content
- **Markdown sections**: For freeform content (##, lists, code blocks)

### Addresses

Format: `<rig>/<role>` or `<rig>/<type>/<name>`

Examples:
```
greenplace/witness       # Witness for greenplace rig
beads/refinery           # Refinery for beads rig
greenplace/miners/nux  # Specific miner
overseer/                # Town-level Overseer
supervisor/               # Town-level Supervisor
```

## Protocol Flows

### Miner Completion Flow

```
Miner                    Witness                    Refinery
   │                          │                          │
   │ MINER_DONE             │                          │
   │─────────────────────────>│                          │
   │                          │                          │
   │                    (verify clean)                   │
   │                          │                          │
   │                          │ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                          │
   │                          │                    (merge attempt)
   │                          │                          │
   │                          │ MERGED (success)         │
   │                          │<─────────────────────────│
   │                          │                          │
   │                    (nuke miner)                   │
   │                          │                          │
```

### Merge Failure Flow

```
                           Witness                    Refinery
                              │                          │
                              │                    (merge fails)
                              │                          │
                              │ MERGE_FAILED             │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (failure notification)   │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Miner (rework needed)
```

### Rebase Required Flow

```
                           Witness                    Refinery
                              │                          │
                              │                    (conflict detected)
                              │                          │
                              │ REWORK_REQUEST           │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (rebase instructions)    │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Miner                       │                          │
   │                          │                          │
   │ (rebases, gt done)       │                          │
   │─────────────────────────>│ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                    (retry merge)
```

### Abandoned Work Recovery Flow

```
Dead Miner               Witness                    Supervisor
     │                        │                          │
     │ (session dies)         │                          │
     │                        │                          │
     │                  (detects zombie)                 │
     │                  (bead status=hooked)             │
     │                        │                          │
     │                  resetAbandonedBead()             │
     │                  bd update --status=open          │
     │                        │                          │
     │                        │ RECOVERED_BEAD           │
     │                        │─────────────────────────>│
     │                        │                          │
     │                        │                    gt supervisor redispatch
     │                        │                    gt sling <bead> <rig>
     │                        │                          │
     │                        │                          ├──> New Miner
     │                        │                          │    (re-dispatched)
```

### Second-Order Monitoring

```
Witness-1 ──┐
            │ (check agent bead last_activity)
Witness-2 ──┼────────────────> Supervisor agent bead
            │
Witness-N ──┘
                                 │
                          (if stale >5min)
                                 │
            ─────────────────────┘
            ALERT to Overseer (mail only on failure)
```

## Communication Hygiene: Mail vs Nudge

Agents overuse mail for routine communication, generating permanent beads and
Dolt commits for messages that should be ephemeral. Every `gt mail send` creates
a wisp bead in Dolt -- a permanent record with its own commit in the git-like
history. This is a critical pollution source.

### The Two Channels

**`gt nudge` (ephemeral, preferred for routine comms)**
- Sends a message directly to an agent's tmux session
- No beads created. No Dolt commits. Zero storage cost.
- Message appears as a `<system-reminder>` in the agent's context
- Suitable for: health checks, status requests, simple instructions, "wake up" signals
- Limitation: if the target session is dead, the nudge is lost

**`gt mail send` (persistent, for structured protocol messages only)**
- Creates a bead (wisp) in the Dolt database
- Generates at least one Dolt commit (the write)
- Persists across session restarts -- survives agent death
- Suitable for: HANDOFF context, MERGE_READY/MERGED protocol, escalations, HELP
  requests, anything that MUST survive session death

### The Rule

**Default to `gt nudge`. Only use `gt mail send` when the message MUST survive
the recipient's session death.**

The litmus test: "If the recipient's session dies and restarts, do they need this
message?" If yes -> mail. If no -> nudge.

### Role-Specific Guidance

| Role | Mail Budget | When to Mail | When to Nudge |
|------|-------------|-------------|---------------|
| **Miner** | 0-1 per session | HELP/ESCALATE only (gt escalate preferred) | Everything else |
| **Witness** | Protocol msgs only | MERGE_READY, RECOVERED_BEAD, RECOVERY_NEEDED, escalations to Overseer | Miner health checks, status pings, nudge-and-observe |
| **Refinery** | Protocol msgs only | MERGED, MERGE_FAILED, REWORK_REQUEST | Status updates to Witness |
| **Supervisor** | Escalations only | Escalations to Overseer, HANDOFF to self | TIMER callbacks, HEALTH_CHECK, lifecycle pokes |
| **Dogs** | Zero | Never (results go to event beads or logs) | Report completion to Supervisor via nudge |
| **Overseer** | Strategic only | Cross-rig coordination, HANDOFF to self | Instructions to Supervisor/Witness |

### Why This Matters (The Commit Graph)

Dolt is git under the hood. Every mail creates a Dolt commit. Over a day of
normal operations:
- 4 agents x 15 patrol cycles x 2 mails per cycle = 120 commits just for routine chatter
- These commits live in the git history forever, even after mail rows are deleted
- Rebase can remove them, but prevention is always cheaper than cleanup

### Anti-Patterns

**DOG_DONE as mail** -- Dogs should not mail their completion status. Use
`gt nudge supervisor/ "DOG_DONE: plugin-name success"` instead.

**Duplicate escalations** -- Witnesses sending 2+ mails about the same issue
minutes apart. Check inbox before sending: if you already sent about this topic,
don't send again.

**HANDOFF for routine cycles** -- Patrol agents (Witness, Supervisor) doing routine
handoffs should use minimal mail. If there's nothing extraordinary, just cycle --
the next session discovers state from beads, not from mail.

**Health check responses via mail** -- When Supervisor sends a health check nudge, do
NOT respond with mail. The Supervisor tracks health via session status, not mail
responses.

## Implementation

### Sending Mail

```bash
# Basic send
gt mail send <addr> -s "Subject" -m "Body"

# With structured body
gt mail send greenplace/witness -s "MERGE_READY nux" -m "Branch: feature-xyz
Issue: gp-abc
Miner: nux
Verified: clean"
```

### Receiving Mail

```bash
# Check inbox
gt mail inbox

# Read specific message
gt mail read <msg-id>

# Mark as read
gt mail ack <msg-id>
```

### In Patrol Formulas

Formulas should:
1. Check inbox at start of each cycle
2. Parse subject prefix to route handling
3. Extract structured data from body
4. Take appropriate action
5. Mark mail as read after processing

## Extensibility

New message types follow the pattern:
1. Define subject prefix (TYPE: or TYPE_SUBTYPE)
2. Document body format (key-value pairs + freeform)
3. Specify route (sender → receiver)
4. Implement handlers in relevant patrol formulas

The protocol is intentionally simple - structured enough for parsing,
flexible enough for human debugging.

## Beads-Native Messaging

Beyond direct agent-to-agent mail, the messaging system supports three bead-backed
primitives for group and broadcast communication. All use the `hq-` prefix
(town-level entities that span rigs).

### Groups (`gt:group`)

Named collections of addresses for mail distribution. Sending to a group
delivers to all members.

**Bead ID format:** `hq-group-<name>`

**Member types:** direct addresses (`mineshaft/crew/max`), wildcard patterns
(`*/witness`, `mineshaft/crew/*`), special patterns (`@town`, `@crew`,
`@witnesses`), or nested group names.

### Queues (`gt:queue`)

Work queues where each message goes to exactly one claimant (unlike groups).

**Bead ID format:** `hq-q-<name>` (town-level) or `gt-q-<name>` (rig-level)

Fields: `status` (active/paused/closed), `max_concurrency`, `processing_order`
(fifo/priority), plus count fields (available, processing, completed, failed).

### Channels (`gt:channel`)

Pub/sub broadcast streams with configurable message retention.

**Bead ID format:** `hq-channel-<name>`

Fields: `subscribers`, `status` (active/closed), `retention_count`,
`retention_hours`.

### Group and Channel CLI Commands

```bash
# Groups
gt mail group list
gt mail group show <name>
gt mail group create <name> [members...]
gt mail group add <name> <member>
gt mail group remove <name> <member>
gt mail group delete <name>

# Channels
gt mail channel list
gt mail channel show <name>
gt mail channel create <name> [--retain-count=N] [--retain-hours=N]
gt mail channel delete <name>
```

### Sending to Groups, Queues, and Channels

```bash
gt mail send my-group -s "Subject" -m "Body"           # group (expands to members)
gt mail send queue:my-queue -s "Work item" -m "Details" # queue (single claimant)
gt mail send channel:alerts -s "Alert" -m "Content"     # channel (broadcast)
```

### Address Resolution Order

When sending mail, addresses are resolved in this order:

1. **Explicit prefix** -- `group:`, `queue:`, or `channel:` uses that type directly
2. **Contains `/`** -- Treat as agent address or pattern (direct delivery)
3. **Starts with `@`** -- Special pattern (`@town`, `@crew`, etc.) or group
4. **Name lookup** -- Search group -> queue -> channel by name

If a name matches multiple types, the resolver returns an error requiring an
explicit prefix.

### Retention Policy

Channels support count-based (`--retain-count=N`) and time-based
(`--retain-hours=N`) retention. Retention is enforced on-write (after posting)
and on-patrol (Supervisor runs `PruneAllChannels()` with a 10% buffer to avoid
thrashing).

## Related Documents

- `docs/agent-as-bead.md` - Agent identity and slots
- `.beads/formulas/mol-witness-patrol.formula.toml` - Witness handling
- `internal/mail/` - Mail routing implementation
- `internal/protocol/` - Protocol handlers for Witness-Refinery communication
