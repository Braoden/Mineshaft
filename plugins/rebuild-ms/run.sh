#!/usr/bin/env bash
# rebuild-ms/run.sh — Rebuild ms binary from mineshaft source if stale.
#
# SAFETY: Only rebuilds forward (binary is ancestor of HEAD) and only
# from main branch. A bad rebuild caused a crash loop (every session's
# startup hook failed, witness respawned, loop repeated every 1-2 min).

set -euo pipefail

TOWN_ROOT="${MS_TOWN_ROOT:-$(ms town root 2>/dev/null)}"
RIG_ROOT="${TOWN_ROOT}/mineshaft/overseer/rig"

log() { echo "[rebuild-ms] $*"; }

# --- Detection ---------------------------------------------------------------

log "Checking binary staleness..."
STALE_JSON=$(ms stale --json 2>/dev/null) || {
  log "ms stale --json failed, skipping"
  exit 0
}

IS_STALE=$(echo "$STALE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('stale', False))" 2>/dev/null || echo "False")
SAFE=$(echo "$STALE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('safe_to_rebuild', False))" 2>/dev/null || echo "False")

if [ "$IS_STALE" != "True" ]; then
  log "Binary is fresh. Nothing to do."
  ms plugin record-run --plugin rebuild-ms --result success --rig mineshaft \
    --title "rebuild-ms: binary is fresh" >/dev/null 2>&1 || true
  exit 0
fi

if [ "$SAFE" != "True" ]; then
  log "Not safe to rebuild (not on main or would be a downgrade). Skipping."
  ms plugin record-run --plugin rebuild-ms --result skipped --rig mineshaft \
    --title "Plugin: rebuild-ms [skipped]" \
    --description "Skipped: not safe to rebuild" >/dev/null 2>&1 || true
  exit 0
fi

# --- Pre-flight checks -------------------------------------------------------

log "Pre-flight checks..."

if [ ! -d "$RIG_ROOT" ]; then
  log "Rig root $RIG_ROOT does not exist. Skipping."
  exit 0
fi

DIRTY=$(git -C "$RIG_ROOT" status --porcelain 2>/dev/null)
if [ -n "$DIRTY" ]; then
  log "Repo is dirty, skipping rebuild."
  ms plugin record-run --plugin rebuild-ms --result skipped --rig mineshaft \
    --title "Plugin: rebuild-ms [skipped]" \
    --description "Skipped: repo has uncommitted changes" >/dev/null 2>&1 || true
  exit 0
fi

BRANCH=$(git -C "$RIG_ROOT" branch --show-current 2>/dev/null)
if [ "$BRANCH" != "main" ]; then
  log "Not on main branch (on $BRANCH), skipping rebuild."
  ms plugin record-run --plugin rebuild-ms --result skipped --rig mineshaft \
    --title "Plugin: rebuild-ms [skipped]" \
    --description "Skipped: not on main branch (on $BRANCH)" >/dev/null 2>&1 || true
  exit 0
fi

# --- Build -------------------------------------------------------------------

OLD_VER=$(ms version 2>/dev/null | head -1 || echo "unknown")
log "Rebuilding ms from $RIG_ROOT..."

if (cd "$RIG_ROOT" && make build && make safe-install) 2>&1; then
  NEW_VER=$(ms version 2>/dev/null | head -1 || echo "unknown")
  log "Rebuilt: $OLD_VER -> $NEW_VER"
  ms plugin record-run --plugin rebuild-ms --result success --rig mineshaft \
    --title "rebuild-ms: $OLD_VER -> $NEW_VER" >/dev/null 2>&1 || true
else
  ERROR="make build/safe-install failed"
  log "FAILED: $ERROR"
  ms plugin record-run --plugin rebuild-ms --result failure --rig mineshaft \
    --title "Plugin: rebuild-ms [failure]" \
    --description "Build failed: $ERROR" >/dev/null 2>&1 || true
  ms escalate "Plugin FAILED: rebuild-ms" -s medium 2>/dev/null || true
  exit 1
fi
