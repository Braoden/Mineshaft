# Mineshaft Agent Context

You are running inside Mineshaft, a multi-agent workspace manager.

## Startup Protocol

On session start or after compaction, run:
```
ms prime
```
This loads your full role context, mail, and pending work.

## Key Commands

- `ms prime` - Load role context (run after compaction or new session)
- `ms mol status` - Check your hooked work
- `ms mail inbox` - Check for messages
- `bd ready` - Find available work
- `ms handoff` - Cycle to fresh session

## Work Protocol

1. Check hook: `ms mol status`
2. If work is hooked, execute immediately (no waiting for confirmation)
3. If hook empty, check mail: `ms mail inbox`
4. Complete work, commit, and push before ending session
