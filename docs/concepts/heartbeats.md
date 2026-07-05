# Heartbeats

Mineshaft has **three distinct heartbeat stores**. They have different readers
and thresholds, so Supervisor heartbeat commands refresh the Supervisor-specific stores
together to avoid false "stuck agent" escalations (see hq-qxl9: a Supervisor
refreshed its session heartbeat while the file store aged past threshold).

## The three stores

### 1. Supervisor heartbeat file — `<townRoot>/supervisor/heartbeat.json`

- **Written by:** `gt supervisor heartbeat [action]` and `gt heartbeat` when
  `GT_ROLE=supervisor` → `supervisor.Touch()` / `supervisor.TouchWithAction()`
  (`internal/supervisor/heartbeat.go`).
- **Read by:** the stuck-agent-dog plugin (parses the JSON `timestamp`, falling
  back to mtime for malformed legacy files, and cross-checks tmux activity
  before escalating) and the Go daemon (`supervisor.ReadHeartbeat`; thresholds 5m
  stale / 20m very-stale → poke).
- **Also touches:** the legacy `supervisor/.supervisor-heartbeat` mtime file for old
  shell scripts.

### 2. Session heartbeat (per-session state store)

- **Written by:** `gt heartbeat [--state=working|idle|exiting|stuck]` →
  `miner.TouchSessionHeartbeatWithState()`. Requires `GT_SESSION`.
- **Read by:** the Witness, which reads the self-reported state instead of
  inferring liveness from timers (ZFC: gt-3vr5). This is the store miners
  refresh.

### 3. Agent-bead label — `heartbeat:<EPOCH>` on the agent bead (e.g. `hq-supervisor`)

- **Written by:** `gt mol await-signal` on each timeout/signal wake
  (`updateAgentHeartbeat` in `internal/cmd/molecule_await_signal.go`). A
  label rewrite is used because `bd agent heartbeat` was never shipped
  (steveyegge/beads#2828). Supervisor heartbeat commands also sync this label when
  it is older than half of the stale threshold.
- **Read by:** Witness second-order monitoring ("who watches the watchers"):
  Witnesses check the Supervisor's bead activity and alert the Overseer if it looks
  unresponsive (>5 minutes per the patrol formula).
- **Gotcha:** a session that never reaches `await-signal` (handoff churn,
  session limits, one very long patrol turn) leaves this label stale for
  hours even though the agent is healthy.

## Rules of thumb

- **Supervisor sessions:** `gt supervisor heartbeat` refreshes the Supervisor file and
  throttled bead label. `gt heartbeat` also refreshes the session store and,
  when `GT_ROLE=supervisor`, uses the same Supervisor file/label sync path.
- **Miners / Witness / Refinery:** `gt heartbeat` (session store) is the
  one that matters.
- **Monitoring scripts:** never declare an agent stuck from a single store.
  Cross-check tmux session activity (`tmux display-message -p
  '#{window_activity}'`) before escalating — a live session with a stale
  store is *heartbeat-write divergence*, not a stuck agent. The
  stuck-agent-dog plugin does this since hq-qxl9.
