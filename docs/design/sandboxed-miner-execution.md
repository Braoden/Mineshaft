# Sandboxed Miner Execution (exitbox + daytona)

> **Date:** 2026-03-02
> **Author:** overseer
> **Status:** Proposal
> **Related:** miner-lifecycle-patrol.md, architecture.md

---

## 1. Problem Statement

Every miner today runs directly on the host machine in a tmux session under the
user's own UID, with full access to the host filesystem, network, and credentials.
This creates two distinct problems:

**Security.** A misbehaving or manipulated agent (e.g. via a malicious MCP server)
can read files outside its worktree, write to `~/.ssh` or `~/.gitconfig`, make
arbitrary outbound network connections, or call `ms`/`bd` with a fabricated
identity. Credential exfiltration is a real threat.

**Scalability.** A developer laptop cannot sustain 10–20 simultaneous Claude
sessions without resource contention. Distributing workloads to cloud containers
(daytona) decouples throughput from local hardware.

Both problems are addressed by a single mechanism: configurable miner execution
backends.

---

## 2. Core Problem Decomposition

An agent session does two independent things that require different treatment:

| Plane | What runs | Where it must run |
|---|---|---|
| **Agent work** | LLM inference, file edits, code execution, `git` operations | Inside the sandbox / container — needs the worktree |
| **Control plane** | `ms prime`, `ms done`, `ms mail`, `bd show/update`, events, nudges | Reaches back to the host — needs Dolt, `.runtime/`, mail |

Keeping these planes separate is the key to a clean design.

---

## 3. Architecture

### 3.1 Current (local-only)

```
Host machine
┌─────────────────────────────────────────────────────┐
│                                                     │
│  Mineshaft daemon                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │  SessionManager.Start()                      │   │
│  │    exec env MS_RIG=... MS_MINER=...        │   │
│  │    claude --mode=direct                      │   │
│  └──────────────┬───────────────────────────────┘   │
│                 │  tmux new-session                  │
│                 ▼                                    │
│           ┌──────────┐   ms prime / ms done          │
│           │  miner │ ──────────────────────────►  │
│           │  (tmux)  │   bd show / bd update         │
│           └──────────┘   (direct, loopback Dolt)     │
│                                                     │
│   Dolt SQL  127.0.0.1:3307                          │
│   .runtime/  ~/ms/                                  │
└─────────────────────────────────────────────────────┘
```

### 3.2 Target: exitbox (local sandbox)

Keeps everything on the host; wraps the agent process in a filesystem and network
policy enforced by exitbox. The control-plane path is unchanged because loopback
is still reachable.

```
Host machine
┌─────────────────────────────────────────────────────┐
│                                                     │
│  Mineshaft daemon                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │  exec env MS_RIG=... MS_MINER=...          │   │
│  │  exitbox run --profile=mineshaft-miner --    │   │
│  │  claude --mode=direct                        │   │
│  └──────────────┬───────────────────────────────┘   │
│                 │  tmux new-session                  │
│                 ▼                                    │
│  ┌─────────────────────────┐                        │
│  │  exitbox sandbox        │  ms / bd calls          │
│  │  ┌─────────────────┐    │ ──────────────────────► │
│  │  │ miner (agent) │    │   loopback — direct     │
│  │  └─────────────────┘    │   (Dolt, .runtime/)     │
│  │  policy:                │                        │
│  │  - rw: worktree only    │                        │
│  │  - net: loopback only   │                        │
│  └─────────────────────────┘                        │
│                                                     │
│   Dolt SQL  127.0.0.1:3307   (loopback reachable)   │
└─────────────────────────────────────────────────────┘
```

### 3.3 Target: daytona (remote cloud container)

The agent runs in a remote Linux container. All communication — control-plane,
git fetch, and git push — goes through the host's mTLS proxy. The container has
**zero outbound internet access**.

```
Host machine                      Daytona cloud container
┌───────────────────────────┐     ┌──────────────────────────────────────┐
│                           │     │                                      │
│  Mineshaft daemon           │     │  tmux pane: daytona exec <ws>        │
│  ┌──────────────────────┐ │     │  ┌────────────────────────────────┐  │
│  │ SessionManager       │ │     │  │ claude --mode=direct           │  │
│  │  - issues cert       │ │     │  │                                │  │
│  │  - injects env vars  │ │     │  │  ms prime / ms done / bd show  │  │
│  │  - starts proxy      │ │     │  │  ↓ (proxy-client detects env)  │  │
│  └──────────────────────┘ │     │  │  POST /v1/exec over mTLS       │  │
│                           │     │  └───────────────┬────────────────┘  │
│  ms-proxy-server          │     │                  │ mTLS (cert CN     │
│  ┌──────────────────────┐ │◄────┼──────────────────┘  ms-rig-name)     │
│  │ /v1/exec             │ │     │                                      │
│  │  - validates cert CN │ │     │  git fetch / git push origin         │
│  │  - injects --identity│ │     │  (origin = proxy git endpoint)       │
│  │  - runs ms/bd on host│ │     │  ↓                                   │
│  │                      │ │◄────┼──── git smart HTTP over mTLS ────────┘
│  │ /v1/git/<rig>/       │ │     │
│  │  upload-pack (fetch) │ │     │  Container git remote:
│  │  receive-pack (push) │ │     │    origin = https://host:9876/v1/git/<rig>
│  │  ↕ .repo.git on host │ │     │
│  └──────────────────────┘ │     The container needs:
│         │                 │     - ms-proxy-client binary (as ms + bd)
│         │ daemon pushes   │     - MS_PROXY_URL, MS_PROXY_CERT, MS_PROXY_KEY
│         ▼ to GitHub       │     - GIT_SSL_CERT, GIT_SSL_KEY, GIT_SSL_CAINFO
│  GitHub  ◄───────────     │     (all injected at session spawn)
│  (upstream, host-only)    │
└───────────────────────────┘
```

The container never contacts GitHub. All git traffic flows:
**container ↔ proxy ↔ `.repo.git`**. The host daemon pushes to GitHub asynchronously.

---

## 4. Design

### 4.1 Startup command wrapping — `ExecWrapper`

The simplest intervention: add an `ExecWrapper []string` field to `RuntimeConfig`.
The startup command builder inserts the wrapper tokens between
`exec env VAR=val ...` and the agent binary.

```
# Local (no wrapper):
exec env MS_RIG=mineshaft MS_MINER=furiosa ... claude --mode=direct

# exitbox:
exec env MS_RIG=mineshaft MS_MINER=furiosa ... \
    exitbox run --profile=mineshaft-miner -- claude --mode=direct

# daytona:
exec env MS_RIG=mineshaft MS_MINER=furiosa ... \
    daytona exec furiosa-ws -- claude --mode=direct
```

This wraps the entire session; tmux still manages the pane, and `tmux send-keys`
still delivers nudges — no changes to the messaging layer.

Exposed as:
- `settings/config.json`: `agent.exec_wrapper: ["exitbox", "run", "--profile=mineshaft-miner", "--"]`
- CLI flag: `ms sling <bead> --exec-wrapper "..."`

### 4.2 mTLS proxy — `ms-proxy-server` and `ms-proxy-client`

Two new lightweight binaries handle all communication from container → host.

#### ms-proxy-server (runs on host)

- Listens on a configured address and port (`proxy_listen_addr`, e.g. `0.0.0.0:9876`)
- Requires mTLS: client cert must be signed by the Mineshaft CA
- **CLI relay model**: forwards argv to `ms`/`bd` on the host and streams stdout/stderr/exitCode back verbatim
- Injects `--identity <rig>/<name>` (extracted from cert `CN=ms-<rig>-<name>`) for commands that require it
- Maintains an explicit allowlist of permitted subcommands — no arbitrary shell execution

```
POST /v1/exec
  body:     {"argv": ["ms", "mail", "inbox", "--json"]}
  response: {"stdout": "...", "stderr": "...", "exitCode": 0}
```

The CLI relay approach means:
- Zero maintenance overhead: new `ms`/`bd` subcommands and flag changes work automatically
- Correctness by construction: proxy executes the same code path as local invocations
- Identity is established by the cert, injected as a CLI flag — no internal API plumbing

#### ms-proxy-client (runs in container, replaces `ms` and `bd`)

- Detects `MS_PROXY_URL` + `MS_PROXY_CERT` + `MS_PROXY_KEY` in environment
- If set: forwards argv wholesale to proxy server over mTLS, prints response, exits with server's exit code
- If not set: falls through to normal local execution (backward-compatible; used by local miners)
- Installed as both `ms` and `bd` via symlinks

#### Git relay — fetch and push via `.repo.git`

All git operations from the container route through the proxy to `.repo.git` on
the host. The proxy speaks git smart HTTP with mTLS:

```
# Clone / fetch (upload-pack)
GET  /v1/git/<rig>/info/refs?service=git-upload-pack
POST /v1/git/<rig>/git-upload-pack

# Push (receive-pack)
GET  /v1/git/<rig>/info/refs?service=git-receive-pack
POST /v1/git/<rig>/git-receive-pack
```

The proxy runs `git upload-pack` or `git receive-pack` against
`~/ms/<rig>/.repo.git` as a subprocess.

**The container never contacts GitHub.** Its `origin` remote points at the proxy:
```
remote.origin.url = https://<host>:9876/v1/git/<rig>
```

Branch-scoped authorization is enforced by cert CN: a miner may only push refs
under `miner/<cn-name>-*`; attempting to push `main` or another miner's
branch is rejected (403). Fetch is unrestricted (read-only).

`.repo.git` (the bare repo Mineshaft already maintains at `~/ms/<rig>/.repo.git`)
is the ideal endpoint:
- It already has `origin` → GitHub configured on the host side
- It is a bare repo — can both serve fetches and receive pushes unconditionally
- `ms done` already uses it as a fallback push target
- All miner worktrees are created from it

**Host → GitHub sync:** After a successful receive-pack, the proxy enqueues an
async upstream push job (`git -C .repo.git push origin <branch>`). The host also
periodically fetches from GitHub so that `.repo.git` stays up-to-date for new
container clones.

### 4.3 CA and per-miner certificates

Mineshaft generates a self-signed CA at daemon startup (`~/ms/.runtime/ca/`). For
each daytona-mode miner, it issues a short-lived leaf certificate:

- **CN**: `ms-<rig>-<name>` (e.g. `ms-mineshaft-furiosa`)
- **SAN**: `session:<sessionID>`
- **TTL**: configurable via `proxy_cert_ttl` (default 24h)

Five environment variables are set in the miner's startup env:

| Variable | Purpose |
|---|---|
| `MS_PROXY_URL` | `https://<host>:9876` |
| `MS_PROXY_CERT` | Path to client cert PEM |
| `MS_PROXY_KEY` | Path to client key PEM |
| `GIT_SSL_CERT` | Same cert — used by git for mTLS with proxy |
| `GIT_SSL_KEY` | Same key — used by git for mTLS with proxy |
| `GIT_SSL_CAINFO` | CA cert — used by git to trust the proxy TLS cert |

On session end, the certificate is added to an in-memory deny list. Subsequent
proxy calls from that cert are immediately rejected.

### 4.4 Daytona workspace lifecycle

#### `daytona exec` does not create containers

`daytona exec <ws> -- cmd` connects to an already-running workspace container.
It is analogous to `docker exec` or `ssh user@host cmd` — it requires the
workspace to already exist and be running. Mineshaft must own the full workspace
lifecycle:

```
daytona create → daytona start → [daytona exec, repeatedly] → daytona stop → daytona delete
      ▲                ▲                    ▲                      ▲               ▲
  ms sling         auto on create      miner sessions         ms session     cleanup
  (once per                                                         stop
   miner)
```

#### Workspace states and Mineshaft actions

| State | daytona CLI | Mineshaft triggers |
|---|---|---|
| Does not exist | `daytona create <repo> --name <ws>` | `ms sling` (first time for this miner) |
| Stopped | `daytona start <ws>` | `ms session start` / `ms sling` resume |
| Running | `daytona exec <ws> -- cmd` | Normal miner operation |
| Running, miner done | `daytona stop <ws>` | `ms session stop` / TTL expiry |
| No longer needed | `daytona delete <ws>` | `ms miner remove` / manual |

Mineshaft stops (not deletes) workspaces on session end, preserving git state for
the next session. Deletion is an explicit operator action.

#### Full provisioning sequence at `ms sling`

```
ms sling <bead> --daytona
  │
  ├─ 1. Create miner branch (host, instant):
  │       git -C ~/ms/<rig>/.repo.git fetch origin
  │       git -C ~/ms/<rig>/.repo.git branch miner/<name>-<ts> origin/main
  │
  ├─ 2. Issue miner mTLS cert (host, instant)
  │
  ├─ 3. Provision daytona workspace (slow: 30–120s):
  │       daytona create https://<host>:9876/v1/git/<rig>
  │         --name ms-<rig>-<miner>
  │         --branch miner/<name>-<ts>
  │         --devcontainer-path .devcontainer/mineshaft-miner
  │       (clones from proxy → .repo.git; runs onCreateCommand)
  │
  ├─ 4. Inject cert into workspace:
  │       daytona exec ms-<rig>-<miner> -- mkdir -p /run/ms-proxy
  │       daytona exec ms-<rig>-<miner> -- tee /run/ms-proxy/client.crt < <cert>
  │       daytona exec ms-<rig>-<miner> -- tee /run/ms-proxy/client.key < <key>
  │       daytona exec ms-<rig>-<miner> -- tee /run/ms-proxy/ca.crt < <ca>
  │
  ├─ 5. Post-create setup:
  │       daytona exec ms-<rig>-<miner> -- ms prime --write-prime-md
  │       daytona exec ms-<rig>-<miner> -- [overlay files, setup hooks]
  │
  ├─ 6. Register agent bead via proxy:
  │       (proxy client calls bd create/update with state=spawning)
  │
  └─ 7. Start tmux pane:
          tmux new-window -n <miner>
          tmux send-keys "daytona exec ms-<rig>-<miner> \
            --env MS_RIG=<rig> --env MS_MINER=<name> \
            --env MS_PROXY_URL=... --env MS_PROXY_CERT=... \
            --env MS_PROXY_KEY=... --env GIT_SSL_CERT=... \
            --env GIT_SSL_KEY=... --env GIT_SSL_CAINFO=... \
            -- claude --mode=direct" Enter
```

Step 3 is the slow step. Steps 1–2 are instant. For production, workspaces can
be pre-provisioned (warm pool) with generic devcontainer setup; step 3 then
becomes `daytona start` instead of `daytona create`.

#### Git topology: proxy-served clone

For local miners, `AddWithOptions` creates a git worktree — a linked checkout
from `.repo.git`, sharing the object store. For daytona miners, the container
clones from the proxy's git endpoint independently. The branch is created locally
in `.repo.git`; no GitHub push is required before provisioning.

```
Host (.repo.git)                     Container
┌──────────────────┐                 ┌──────────────────────┐
│ origin → GitHub  │   git clone     │  origin → proxy      │
│                  │ ◄──── via ────► │  (full standalone     │
│ miner/nova-42  │   mTLS proxy    │   .git, not worktree) │
└──────────────────┘                 └──────────────────────┘
        ▲                                     │
        │ daemon pushes                       │ git push origin
        ▼                                     ▼
      GitHub                            proxy receive-pack
                                        → .repo.git → GitHub
```

#### What is NOT needed for daytona that is required locally

- No host-side `miners/<name>/<rig>/` directory — the container IS the worktree
- No `git worktree add` — container clones from proxy, which serves from `.repo.git`
- No `.beads` redirect file — all Dolt access goes through the mTLS proxy
- No `WorktreeAddFromRef` call in `manager.go` — daytona-mode skips it
- No GitHub push before provisioning — branch only needs to exist in `.repo.git`
- No separate `pushurl` override — `origin` points at the proxy for both fetch and push

#### Devcontainer profile

```json
// .devcontainer/mineshaft-miner/devcontainer.json
{
  "name": "Mineshaft Miner",
  "image": "ubuntu:24.04",
  "onCreateCommand": "bash .devcontainer/mineshaft-miner/setup.sh",
  "remoteUser": "vscode"
}
```

```bash
# .devcontainer/mineshaft-miner/setup.sh
set -e
npm install -g @anthropic-ai/claude-code
curl -fsSL https://releases.mineshaft.dev/ms-proxy-client/latest/linux-amd64 -o /usr/local/bin/ms
chmod +x /usr/local/bin/ms
ln -sf /usr/local/bin/ms /usr/local/bin/bd
apt-get install -y git
```

Alternatively, Mineshaft can distribute a pre-built Docker image
(`ghcr.io/steveyegge/mineshaft-miner:latest`) and reference it directly,
bypassing the setup script. This is more reliable for production use.

The `DaytonaConfig` struct:

```go
type DaytonaConfig struct {
    WorkspaceID string `json:"workspace_id"`
    Profile     string `json:"profile,omitempty"`     // devcontainer name
    Image       string `json:"image,omitempty"`       // override image directly
    AutoStop    bool   `json:"auto_stop,omitempty"`   // stop workspace after session ends
    AutoDelete  bool   `json:"auto_delete,omitempty"` // delete workspace after session ends
}
```

---

## 5. Nudging, Observation, and Multi-Miner Sessions

### 5.1 How nudging still works

`NudgeSession` works by sending keystrokes to a local tmux pane via
`tmux send-keys -l`. `daytona exec <ws> -- claude --mode=direct` behaves exactly
like an SSH-connected process: the local tmux pane runs the `daytona` CLI, which
proxies stdin/stdout to the remote container. From the local tmux server's
perspective, the pane is live and accepting input; `send-keys` delivers keystrokes
into the `daytona exec` stdin stream, which forwards them to the remote Claude
process. **No changes are needed to `NudgeSession`, `WaitForIdle`, or the nudge
queue.**

```
Host tmux server
┌──────────────────────────────────────────────────────────────────┐
│ session: ms-mineshaft-furiosa                                      │
│ pane %3                                                          │
│   process: daytona ◄── tmux send-keys targets this pane         │
│              │                                                   │
│              │ stdin/stdout tunnel (daytona exec protocol)       │
│              ▼                                                   │
│        ┌────────────────────────────────────┐  (remote)         │
│        │ daytona workspace: furiosa-ws       │                   │
│        │   claude --mode=direct              │                   │
│        └────────────────────────────────────┘                   │
└──────────────────────────────────────────────────────────────────┘
```

### 5.2 Liveness detection

Currently `IsAgentAlive` walks the local process tree looking for `claude`. With
`daytona exec` as the pane process, `claude` is running remotely and is invisible
to the local process tree.

**Option 1 (chosen for initial implementation):** Add `daytona` to
`MS_PROCESS_NAMES` at session spawn — liveness is "the daytona exec connection is
up". Simple and correct in practice: if `daytona exec` exits, the session is dead.
This is handled by G5 (`ExecWrapper[0]` auto-added to accepted process names).

**Option 2 (future):** Health check endpoint — miner periodically writes a
heartbeat via the mTLS proxy; daemon checks for stale heartbeats. More accurate
but more complex.

### 5.3 Human observation

Attach to any miner's tmux pane on the host:

```bash
tmux attach -t ms-mineshaft-furiosa        # interactive
tmux attach -t ms-mineshaft-furiosa -r     # read-only
```

The terminal output is the remote Claude TUI rendered through the `daytona exec`
tunnel — identical to watching a local miner.

### 5.4 Multi-miner window grouping (optional)

For remote miners it is ergonomic to group them into one tmux session with
multiple windows — one window per miner:

```
tmux session: ms-mineshaft (one session per rig)
  window 0: furiosa    ← daytona exec furiosa-ws -- claude
  window 1: nova       ← daytona exec nova-ws -- claude
  window 2: drake      ← daytona exec drake-ws -- claude
  window 3: boss   ← free shell for human operator
```

`FindAgentPane` already handles multi-window sessions (enumerates all panes via
`tmux list-panes -s`), so the nudge path requires no changes. Window-grouping is
enabled per-rig with `group_sessions: true`. When enabled, `ms sling` creates a
new window in the existing rig session rather than a new session.

### 5.5 Summary of changes needed for nudge / observation

| Concern | Change needed |
|---|---|
| Nudge delivery | **None** — `send-keys` to local pane, daytona exec tunnels it |
| Mail nudge queue | **None** — same path, same code |
| Liveness detection | **G5** — add `daytona` to `MS_PROCESS_NAMES` |
| Human observation | **None** — `tmux attach` works as-is |
| Multi-miner window grouping | **Optional** — new `group_sessions` setting + window creation in G6 |

---

## 6. Implementation Plan

Deliverables are ordered with standalone work first (no Mineshaft changes) followed
by Mineshaft changes in dependency order.

### 6.1 Standalone deliverables (no Mineshaft changes)

**S1 — exitbox policy profile**

Write the policy file permitting a miner session:
- Read + execute: `ms`, `bd`, `claude`, `node`, `git`
- Read + write: miner worktree (`~/ms/<rig>/miners/<name>/`)
- Read: town shared dirs (`~/ms/.beads/`, `~/ms/.runtime/`)
- Network: loopback only (`127.0.0.1:3307`)
- Write: heartbeat and nudge queue dirs

Manually test: `exitbox run --profile=mineshaft-miner -- claude --mode=direct` in
a tmux pane. Run `ms prime` → `ms done`.

**S2 — standalone `ms-proxy-server` + `ms-proxy-client`**

Build and test entirely outside Mineshaft. Spin up any Docker container, inject the
cert env vars, run `ms prime` and `ms done` from inside.

Open question answered by this step: does `daytona exec` inherit parent env or
require explicit `--env` flags?

**S3 — daytona smoke test**

With the S2 proxy running on the host, manually exercise the full miner lifecycle:
1. Test whether `daytona create` accepts a custom git endpoint URL as the repo
   source:
   ```bash
   daytona create https://<host>:9876/v1/git/<rig> \
     --name test-miner --branch miner/test-1
   ```
   If this works: container clones from proxy → `.repo.git`. Ideal path.
   If daytona only accepts GitHub URLs: fallback — `daytona create <github-url>`
   + post-create `git remote set-url origin https://<proxy>/v1/git/<rig>` via
   `daytona exec`.
2. Inject cert and env vars explicitly, run `ms prime`, `ms hook`, `ms done`.
3. Verify `git push origin` routes to proxy → lands in `.repo.git` on host.
4. Verify `git fetch origin` pulls from proxy → `.repo.git` (not from GitHub).
5. `daytona stop test-miner` — verify workspace persists; `daytona start` +
   re-exec works.

This step confirms: (a) which host IP/address is reachable from inside a daytona
container, (b) that `GIT_SSL_*` vars are honoured by the container's git binary,
(c) whether daytona supports custom git endpoints for cloning.

### 6.2 Mineshaft code changes

| ID | Change | Files | Size |
|---|---|---|---|
| G1 | `BD_DOLT_HOST` / `BD_DOLT_PORT` env vars | `internal/beads/beads.go` | ~8 lines |
| G2 | CA management + cert issuance | `internal/proxy/ca.go` (new) | ~50 lines |
| G3 | Proxy server integrated into daemon | `internal/proxy/server.go` (new) | ~80 lines |
| G4 | `ExecWrapper` field + startup command threading | `internal/config/types.go`, `internal/config/loader.go` | ~35 lines |
| G5 | Process detection for wrapped launchers | `internal/tmux/tmux.go` | ~12 lines |
| G6 | `DaytonaConfig` + workspace provisioning | `internal/config/types.go`, `internal/daytona/` (new) | ~150 lines |
| G7 | Skip local worktree creation for daytona-mode miners | `internal/miner/manager.go` | ~25 lines |

### 6.3 Dependency order

```
S1 ──────────────────────────────────────────────────────► exitbox proven
S2 ──────────────────────────────────────────────────────► proxy proven
S3 (depends on S2) ──────────────────────────────────────► daytona unknowns resolved
        │
        ▼
G1  BD_DOLT_HOST/PORT
G4  ExecWrapper in RuntimeConfig
G5  process detection fix
        │
        ├──────────────────────────────────────────────────► exitbox end-to-end ✓
        │
G2  CA + cert issuance
G3  proxy server in daemon (wraps S2 binary)
G6  DaytonaConfig + provisioning
G7  skip local worktree
        │
        └──────────────────────────────────────────────────► daytona end-to-end ✓
```

---

## 7. Alternatives Considered

### 7.1 `SessionBackend` interface / remote tmux

An abstraction layer replacing `tmux new-session` with a generic backend
interface. Rejected for initial implementation: `daytona exec` already behaves
like a local process from tmux's perspective, so a backend abstraction buys
nothing. Revisit only if `daytona exec` proves insufficient for nudge delivery.

### 7.2 exitbox using the mTLS proxy

Overkill. Since exitbox keeps everything on the host and loopback Dolt access is
already secure, the proxy adds no security benefit for the exitbox case.

### 7.3 Other runtimes (Docker, Nix sandbox, Firecracker)

`ExecWrapper` generalises to all of them once the pattern is proven. Runtime-
specific config structs (like `DaytonaConfig`) can be added individually without
architectural changes.

### 7.4 Multi-host federation or proxy chaining

Out of scope.

---

## 8. Acceptance Criteria

### exitbox

- [ ] `exitbox run --profile=mineshaft-miner -- ms prime` succeeds inside sandbox (loopback Dolt reachable)
- [ ] `ms sling <bead> --exec-wrapper "exitbox run --profile=mineshaft-miner --"` starts a live session
- [ ] Miner receives nudge via `tmux send-keys` into the exitbox pane
- [ ] `ms done` completes fully inside sandbox: git push to remote + bd update via loopback Dolt
- [ ] Liveness detection sees the correct process (exitbox or agent, depending on exec behavior)
- [ ] Existing local miners unaffected (no regression)

### daytona + proxy

- [ ] `ms-proxy-server` starts on host; CA initialised at `~/ms/.runtime/ca/`
- [ ] Miner cert issued and injected into daytona workspace at `/run/ms-proxy/`
- [ ] `ms prime` inside container succeeds (control-plane routed via proxy)
- [ ] `ms done` inside container: `git push origin` → proxy receive-pack → `.repo.git` on host → daemon pushes to GitHub
- [ ] `git fetch origin` inside container: fetches from proxy → `.repo.git` (not from GitHub)
- [ ] Proxy rejects a push to `main` or another miner's branch (CN-scoped authorization)
- [ ] Proxy rejects control-plane calls from a revoked or mismatched cert
- [ ] `ms sling <bead> --daytona <workspace>` provisions workspace, issues cert, starts session end-to-end
- [ ] Nudge delivered via tmux pane running `daytona exec`
- [ ] Local worktree creation skipped for daytona-mode miners
- [ ] Session end: cert deny-listed; subsequent proxy calls rejected
- [ ] Container operates with zero outbound internet access and all operations succeed

---

## 9. Open Questions

1. **Host reachability** — What address is reachable from inside a daytona cloud
   container: fixed host IP, `host.docker.internal`, or a daytona-specific
   tunnel? Determines the value of `MS_PROXY_URL`. Answered by S3.

2. **Custom git endpoint for `daytona create`** — Does `daytona create` accept an
   arbitrary HTTPS URL as the repo source, or only GitHub/GitLab URLs? If the
   latter, the fallback is: `daytona create <github-url>` + post-create
   `git remote set-url origin <proxy-url>` via `daytona exec`. Answered by S3.

3. **Upstream push trigger** — How does the daemon detect a new branch landing in
   `.repo.git` to push it to GitHub? Options: proxy-side enqueue after successful
   receive-pack (current plan); post-receive hook in `.repo.git/hooks/post-receive`;
   daemon ref-watcher. Proxy-side enqueue is simplest.

4. **Host-side `.repo.git` freshness** — The daemon must periodically
   `git fetch origin` into `.repo.git` so container fetches see up-to-date refs.
   How often? On-demand triggered by proxy upload-pack, or on a timer?

5. **Workspace warm pool** — First-time `daytona create` takes 30–120s. For
   low-latency `ms sling`, should Mineshaft maintain a pool of pre-provisioned warm
   workspaces? Optional optimisation, not required for initial implementation.

6. **Devcontainer distribution** — Ship `.devcontainer/mineshaft-miner/` in the
   Mineshaft repo, or publish a standalone Docker image
   (`ghcr.io/steveyegge/mineshaft-miner:latest`)? The image approach is more
   reliable for production; devcontainer is more transparent and self-contained.
