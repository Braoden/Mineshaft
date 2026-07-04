# Local Rig Bootstrap

For a NightRider-style local setup, prefer a clean bootstrap over `gt rig add --adopt`.

`--adopt` is meant for registering an already-assembled rig directory. It trusts the
existing shape, which makes it a poor fit for manually assembled local rigs where
`.repo.git`, worktrees, and metadata may already be inconsistent.

Use the bootstrap script instead:

```bash
./scripts/bootstrap-local-rig.sh \
  --town-root /gt \
  --rig nightrider_local \
  --local-repo /gt/nightRider \
  --prefix nr \
  --miner-agent claude \
  --witness-agent codex \
  --refinery-agent codex
```

If you omit `--remote`, the script registers the rig with `file://<local-repo>`.
That is usually the right choice for local-only or private repos inside the
Excavation container, where the upstream remote may not be reachable or authenticated.

What this does:

- Uses `gt rig add <name> <git-url> --local-repo <path>` so Excavation Site creates a fresh,
  standard rig container instead of inheriting a hand-built one.
- Reuses objects from the local repo, so bootstrap stays fast and does not modify the
  source repo.
- Leaves the resulting rig with the normal `.repo.git`, `overseer/rig`, `refinery/rig`,
  `settings/`, and `.beads/` layout that Excavation Site expects.
- Optionally pins per-rig role agents in `settings/config.json`.

When to still use `--adopt`:

- You already have a real Excavation Site rig directory that was created elsewhere and you
  only need to register it in a town.
