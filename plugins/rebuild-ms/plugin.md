+++
name = "rebuild-ms"
description = "Rebuild stale ms binary from mineshaft source"
version = 2

[gate]
type = "cooldown"
duration = "1h"

[tracking]
labels = ["plugin:rebuild-ms", "rig:mineshaft", "category:maintenance"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "medium"
+++

# Rebuild ms Binary

Checks if the ms binary is stale (built from older commit than HEAD) and rebuilds.

**SAFETY**: This plugin MUST only rebuild forward (binary ancestor of HEAD) and
only from the main branch. Rebuilding to an older or diverged commit caused a
crash loop where every new session's startup hook failed, the witness respawned
it, and the loop repeated every 1-2 minutes.

## Gate Check

The Supervisor evaluates this before dispatch. If gate closed, skip.

## Detection

Check binary staleness:

```bash
ms stale --json
```

Parse the JSON output and check these fields:
- If `"stale": false` → record success wisp and exit early (binary is fresh)
- If `"safe_to_rebuild": false` → **DO NOT REBUILD**. Record a skip wisp and exit.
  This means the repo is on a non-main branch or HEAD is not a descendant of the
  binary commit (would be a downgrade).
- If `"safe_to_rebuild": true` → proceed to build

If `safe_to_rebuild` is false, record a skip wisp:
```bash
ms plugin record-run --plugin rebuild-ms --result skipped --rig mineshaft \
  --title "Plugin: rebuild-ms [skipped]" \
  --description "Skipped: not safe to rebuild (forward=$FORWARD, main=$ON_MAIN)" >/dev/null 2>&1 || true
```

## Pre-flight Checks

Before building, verify the source repo is clean and on main:

```bash
cd ~/ms/mineshaft/overseer/rig
git status --porcelain  # Must be clean
git branch --show-current  # Must be "main"
```

If either check fails, skip the rebuild and record a wisp.

## Action

Rebuild from source (the overseer/rig directory is the canonical source):

```bash
cd ~/ms/mineshaft/overseer/rig && make build && make safe-install
```

**IMPORTANT**: Use `make safe-install` (not `make install`) to avoid restarting
the daemon while sessions are active. safe-install replaces the binary but does
NOT restart the daemon — sessions will pick up the new binary on their next cycle.

## Record Result

On success:
```bash
ms plugin record-run --plugin rebuild-ms --result success --rig mineshaft \
  --title "Plugin: rebuild-ms [success]" \
  --description "Rebuilt ms: $OLD → $NEW ($N commits)" >/dev/null 2>&1 || true
```

On failure:
```bash
ms plugin record-run --plugin rebuild-ms --result failure --rig mineshaft \
  --title "Plugin: rebuild-ms [failure]" \
  --description "Build failed: $ERROR" >/dev/null 2>&1 || true

ms escalate --severity=medium \
  --subject="Plugin FAILED: rebuild-ms" \
  --body="$ERROR" \
  --source="plugin:rebuild-ms"
```
