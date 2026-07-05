# miner-pr-flow

A reference harness for rigs that gate miner work on a **GitHub PR** rather
than the canonical Refinery **merge-queue** flow.

It instructs miners, after their final build/pre-verify passes, to push
their branch and open (or confirm) a GitHub PR before running `gt done`.

## What's in here

| File | Purpose |
|------|---------|
| `miner.md` | Role directive for miners in a PR-flow rig. Broad guardrail — applies to any formula a miner runs. |
| `mol-miner-work.toml` | Formula overlay that appends a PR-creation step to `mol-miner-work`'s `submit-and-exit` step. Surgical — only affects this formula. |

Both layers are intentional: the directive sets the rig-level expectation
("open PRs, don't merge them yourself"), and the overlay wires the concrete
commands into the workflow a miner actually sees at `gt prime` time.

## Install

```bash
# Role directive (rig-scoped)
mkdir -p ~/gt/<rig>/directives
cp miner.md ~/gt/<rig>/directives/miner.md

# Formula overlay (rig-scoped)
mkdir -p ~/gt/<rig>/formula-overlays
cp mol-miner-work.toml ~/gt/<rig>/formula-overlays/mol-miner-work.toml
```

Replace `<rig>` with your rig's name (e.g. `mineshaft`, `longeye`). For
town-wide installation, drop the `<rig>/` segment — but this is almost
never what you want, since different rigs legitimately use different flows.

## Verify it's active

```bash
# Validate overlay step IDs against the current formula
gt doctor
# Expect: overlay-health: N overlay(s) healthy

# Inspect the rendered formula with the overlay applied
gt formula overlay show mol-miner-work --rig <rig>

# See the directive text that will be injected at prime time
gt directive show miner --rig <rig>

# End-to-end: see what a miner would see
gt prime --explain
# Expect: "Formula overlay: applying 1 override(s) for mol-miner-work (rig=<rig>)"
```

## What this does / does not do

**Does:**

- Tells the miner to push and open a PR before `gt done`
- Sets a rig-level policy that a PR is the review artifact
- Surfaces `gh pr create` failure as an escalation to Witness, not a silent skip

**Does not:**

- Modify `gt done` behavior (no Go changes)
- Force PR creation via framework-level validation (agents can still misbehave)
- Merge the PR (that's a maintainer / merge-queue concern)
- Replace the directive or overlay if your rig also uses other customizations
  — merge this content with your existing files rather than overwriting them

## When to fork this

If your rig needs additional PR-flow constraints (required reviewers, specific
labels, CODEOWNERS enforcement, CI checks before `gt done`), copy this harness
and adapt it. The point is a starting template, not a drop-in product.

## Fixing an existing PR (gh#3602)

By default `gt sling` creates a fresh `miner/<name>/<bead>@<ts>` branch from
`main`, which means re-slinging a bead with an existing open PR opens a
**duplicate** PR rather than reusing the original. To resume an existing PR
branch, use `--branch` or `--pr`:

```bash
# Resume by branch name (works for any open or stashed branch)
gt sling <bead> <rig> --branch miner/example/gh-1234@abcdef

# Resume by PR number (resolves the head ref via `gh pr view`)
gt sling <bead> <rig> --pr 1234
```

The miner's worktree HEAD will land on the named branch, so its commits
extend the existing PR's history and `gt done` pushes back to the same ref.

**Constraints:**

- `--branch` and `--pr` are mutually exclusive.
- Neither can be combined with `--base-branch` (resume implies its own start point).
- The `--pr` form requires the `gh` CLI to be authenticated against the repo.

The resume branch name is also exposed to formulas as the `resume_branch`
variable, alongside the existing `base_branch`, so overlays can react to it.
