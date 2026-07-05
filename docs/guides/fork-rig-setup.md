# Fork-Based Rig Setup

When you run a rig against a repository you **don't own**, the rig has to
fetch canonical history from upstream but push all work to your fork.
`ms rig add` supports this directly through `--push-url` and
`--upstream-url`. Without them, the default `ms rig add <name> <fork-url>`
produces a rig whose refinery merges miner work into your fork's `main`,
diverging it from upstream.

## When you need fork mode

Use fork mode whenever:

- You have read-only access to the canonical repo (e.g. you're an external
  contributor), and
- You push your work to a personal/organisation fork and open PRs from there.

If you own the canonical repo and push directly to it, you do **not** need
these flags — the plain `ms rig add <name> <git-url>` is correct.

## Setup

```bash
ms rig add <name> <upstream-url> \
  --push-url     <your-fork-url> \
  --upstream-url <upstream-url>
```

Concretely, for a Mineshaft contributor:

```bash
ms rig add mineshaft https://github.com/mineshafthall/mineshaft \
  --push-url     https://github.com/<you>/mineshaft \
  --upstream-url https://github.com/mineshafthall/mineshaft
```

What each flag does:

| Flag | Effect |
|---|---|
| positional `<git-url>` | `origin`'s **fetch** URL — where canonical history is pulled from |
| `--push-url` | `origin`'s **push** URL — where all pushes go (your fork) |
| `--upstream-url` | Adds a separate named `upstream` remote for rebases against `upstream/main` |

These remotes are configured on **both** the bare canonical clone
(`<rig>/refinery/rig`) and the overseer's working clone (`<rig>/overseer/rig`).

## Verifying the setup

Check the remotes in the refinery's bare clone and the overseer's clone:

```bash
cd <town>/<rig>/refinery/rig && git remote -v
cd <town>/<rig>/overseer/rig    && git remote -v
```

Expect (substituting your fork and the canonical repo):

```
origin    https://github.com/mineshafthall/mineshaft (fetch)
origin    https://github.com/<you>/mineshaft       (push)
upstream  https://github.com/mineshafthall/mineshaft (fetch)
upstream  https://github.com/mineshafthall/mineshaft (push)
```

The key invariant: **`origin`'s fetch URL is upstream, `origin`'s push URL
is your fork.** If `origin (push)` points at the canonical repo, the flags
did not take effect — re-add the rig.

## Current limitation: the refinery is not yet fork-aware

Even a correctly-configured fork rig will, today, have its refinery attempt
to **merge miner branches into the fork's `main`** rather than open a PR
to upstream. The foundation flags (`--push-url` / `--upstream-url`) shipped
in [mineshafthall/mineshaft#2018](https://github.com/mineshafthall/mineshaft/issues/2018),
but the behavioral half — refinery raising PRs to upstream instead of
merging to `origin` — is tracked in
[mineshafthall/mineshaft#1794](https://github.com/mineshafthall/mineshaft/issues/1794)
and is not yet implemented.

Until then, for strict PR-only behavior:

- Do **not** start the refinery. Park the rig with `ms rig park <rig>`.
- Use the miner → feature branch → manual PR path. Push the branch to
  your fork and open the PR by hand.

## Recovery: a polluted fork `main`

If you added a rig **without** the fork-routing flags, the refinery may
have already merged miner work into your fork's `main`, leaving it with
mixed `Merge branch ...` and refinery-generated commits diverged from
upstream.

> **Destructive — consult a maintainer before running.** Resetting `main`
> rewrites your fork's history. If any of the diverged commits contain work
> you still need (unmerged PRs, local-only fixes), stop and recover those
> branches first. `git reflog` is your escape hatch if you reset too far.

1. Inspect the divergence before touching anything:

   ```bash
   cd <town>/<rig>/overseer/rig
   git fetch upstream
   git log --oneline --graph upstream/main...origin/main
   ```

2. Confirm every commit on `origin/main` that is *not* on `upstream/main`
   is safe to discard (it's refinery merge noise, not real work). Salvage
   anything you need onto a separate branch first.

3. Reset `main` to track upstream and force-publish to your fork:

   ```bash
   git checkout main
   git reset --hard upstream/main
   git push --force-with-lease origin main
   ```

4. Re-add the rig **with** the fork-routing flags (see [Setup](#setup)) so
   this doesn't recur.

## See also

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — "Setting up a rig to contribute
  to Mineshaft" (Mineshaft-specific worked example)
- [Local Rig Bootstrap](local-rig-bootstrap.md) — local/private repo setup
