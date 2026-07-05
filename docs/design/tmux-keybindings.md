# Tmux Keybindings

Mineshaft overrides several tmux keybindings to provide session navigation
and operational shortcuts. All bindings are conditional — they only activate
in Mineshaft sessions (those matching a registered rig prefix or `hq-`).
Non-MS sessions retain the user's original bindings.

## Session Cycle Groups (prefix+n / prefix+p)

`ms cycle next` and `ms cycle prev` are bound to `C-b n` and `C-b p`.
They cycle within groups based on the current session type:

| Group | Sessions included | Example |
|-------|-------------------|---------|
| **Town** | Overseer + Supervisor | `hq-overseer` ↔ `hq-supervisor` |
| **Crew** | All crew in the same rig | `ms-crew-max` ↔ `ms-crew-joe` |
| **Rig ops** | Witness + Refinery + Miners in the same rig | `ms-witness` ↔ `ms-refinery` ↔ `ms-furiosa` ↔ `ms-nux` |

Groups are per-rig: `ms-witness` cycles with `ms-refinery` and mineshaft
miners, but NOT with `bd-witness` or `bd-refinery`.

If a group has only one session, prefix+n/p is a no-op.

## Other Bindings

| Key | Command | Purpose |
|-----|---------|---------|
| `C-b a` | `ms feed --window` | Open/switch to activity feed window |
| `C-b g` | `ms agents menu` | Open agent switcher popup |

## How Bindings Are Set Up

Bindings are configured by `ConfigureMineshaftSession()` in the tmux package,
which is called whenever a session is created (by the daemon for patrol
agents, by the witness for miners, by `ms crew at` for crew). This means:

- Bindings are set on the **first** Mineshaft session created on a tmux server
- They apply server-wide (tmux keybindings are global, not per-session)
- The `if-shell` guard scopes them to MS sessions at press time
- Subsequent calls are no-ops (idempotent)

## Implementation Details

### Prefix pattern

The `if-shell` guard uses a regex built from all registered rig prefixes:

```bash
echo '#{session_name}' | grep -Eq '^(bd|ms|hq)-'
```

The pattern is built dynamically by `sessionPrefixPattern()` from
`config.AllRigPrefixes()`. The `hq` and `ms` prefixes are always included.

### run-shell context

Bindings use `run-shell` which executes in the tmux server process, not
in any session. Key variables:

- `#{session_name}` — expanded by tmux at key-press time (reliable)
- `#{client_tty}` — identifies which client pressed the key (for multi-attach)
- `$TMUX` — set in run-shell subprocesses, points to the socket
- CWD — the tmux server's CWD, typically `$HOME`

Because CWD is `$HOME`, the `ms` binary finds the workspace via
`MS_TOWN_ROOT` in the tmux global environment (set by the daemon at
startup). This is verified by `ms doctor --check tmux-global-env`.

### Fallback preservation

When bindings are first set, the existing binding for each key is captured
and used as the `else` branch of `if-shell`. This preserves the user's
original `C-b n` (next-window) and `C-b p` (previous-window) for
non-MS sessions.
