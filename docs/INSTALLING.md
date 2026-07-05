# Installing Mineshaft

Complete setup guide for Mineshaft multi-agent orchestrator.

## Prerequisites

### Required

| Tool | Version | Check | Install |
|------|---------|-------|---------|
| **Go** | 1.25.8+ | `go version` | See [golang.org](https://go.dev/doc/install) |
| **Git** | 2.20+ | `git --version` | See below |
| **Dolt** | >= 2.0.7 | `dolt version` | macOS: `brew install dolt`; other platforms: see [dolthub/dolt](https://github.com/dolthub/dolt?tab=readme-ov-file#installation) |
| **Beads** | >= 0.55.4 | `bd version` | Installed by `brew install mineshaft`, or from source with `go install github.com/steveyegge/beads/cmd/bd@latest` |

### Optional (for Full Stack Mode)

| Tool | Version | Check | Install |
|------|---------|-------|---------|
| **tmux** | 3.0+ | `tmux -V` | See below |
| **Claude Code** (default) | >= 2.0.20 | `claude --version` | See [claude.ai/claude-code](https://claude.ai/claude-code) |
| **Codex CLI** (optional) | latest | `codex --version` | See [developers.openai.com/codex/cli](https://developers.openai.com/codex/cli) |
| **OpenCode CLI** (optional) | latest | `opencode --version` | See [opencode.ai](https://opencode.ai) |
| **GitHub Copilot CLI** (optional) | latest | `copilot --version` | See [cli.github.com](https://cli.github.com) (requires Copilot seat) |

## Installing Prerequisites

### macOS

```bash
# Install Homebrew if needed
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Required
brew install go git dolt

# Optional (for full stack mode)
brew install tmux
```

### Linux (Debian/Ubuntu)

```bash
# Required
sudo apt update
sudo apt install -y git

# Install Go (apt version may be outdated, use official installer)
wget https://go.dev/dl/go1.25.8.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.25.8.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install Dolt: see https://github.com/dolthub/dolt?tab=readme-ov-file#installation

# Optional (for full stack mode)
sudo apt install -y tmux
```

### Linux (Fedora/RHEL)

```bash
# Required
sudo dnf install -y git golang
# Install Dolt: see https://github.com/dolthub/dolt?tab=readme-ov-file#installation

# Optional
sudo dnf install -y tmux
```

### Verify Prerequisites

```bash
# Check all prerequisites
go version        # Should show go1.25.8 or higher
git --version     # Should show 2.20 or higher
dolt version      # Should show 2.0.7 or higher
tmux -V           # (Optional) Should show 3.0 or higher
```

## Installing Mineshaft

### Step 1: Install the Binaries

```bash
# Install Mineshaft CLI
brew install mineshaft

# Verify installation
ms version
bd version
dolt version
```

Homebrew installs the runtime dependencies declared by the core formula. The
`mineshafthall/mineshaft` tap is reserved for emergency updates. If you build from
source instead, install `dolt` first, install `bd` with Go, ensure `$GOPATH/bin`
(usually `~/go/bin`) is in your PATH, and ensure `~/.local/bin` appears before
older install locations. On macOS, do not install `ms` with `go install`:
unsigned binaries may be killed by the OS. Clone the repository and use `make`
instead.

```bash
brew install dolt
go install github.com/steveyegge/beads/cmd/bd@latest
export PATH="$HOME/.local/bin:$PATH:$HOME/go/bin"
git clone https://github.com/steveyegge/mineshaft.git
cd mineshaft
make install
```

### Step 2: Create Your Workspace

```bash
# Create a Mineshaft workspace (HQ)
ms install ~/ms --shell

# This creates:
#   ~/ms/
#   ├── CLAUDE.md          # Identity anchor (run ms prime)
#   ├── overseer/             # Overseer config and state
#   ├── rigs/              # Project containers (initially empty)
#   └── .beads/            # Town-level issue tracking
```

### Step 3: Add a Project (Rig)

```bash
# Add your first project
ms rig add myproject https://github.com/you/repo.git

# This clones the repo and sets up:
#   ~/ms/myproject/
#   ├── .beads/            # Project issue tracking
#   ├── overseer/rig/         # Overseer's clone (canonical)
#   ├── refinery/rig/      # Merge queue processor
#   ├── witness/           # Worker monitor
#   └── miners/          # Worker clones (created on demand)
```

### Step 4: Verify Installation

```bash
cd ~/ms

ms enable              # enable Mineshaft system-wide
ms git-init            # initialize a git repo for your HQ
ms up                  # Start all services. Use ms down or ms shutdown for stopping. 

ms doctor              # Run health checks
ms status              # Show workspace status
```

### Step 5: Configure Agents (Optional)

Mineshaft supports built-in runtimes (`claude`, `gemini`, `codex`, `cursor`, `auggie`, `amp`, `opencode`, `copilot`) plus custom agent aliases.

```bash
# List available agents
ms config agent list

# Create an alias (aliases can encode model/thinking flags)
ms config agent set codex-low "codex --thinking low"
ms config agent set claude-haiku "claude --model haiku --dangerously-skip-permissions"

# Set the town default agent (used when a rig doesn't specify one)
ms config default-agent codex-low
```

You can also override the agent per command without changing defaults:

```bash
ms start --agent codex-low
ms sling ms-abc12 myproject --agent claude-haiku
```

## Minimal Mode vs Full Stack Mode

Mineshaft supports two operational modes:

### Minimal Mode (No Daemon)

Run individual runtime instances manually. Mineshaft only tracks state.

```bash
# Create and assign work
ms minecart create "Fix bugs" ms-abc12
ms sling ms-abc12 myproject

# Run runtime manually
cd ~/ms/myproject/miners/<worker>
claude --resume          # Claude Code
# or: codex              # Codex CLI

# Check progress
ms minecart list
```

**When to use**: Testing, simple workflows, or when you prefer manual control.

### Full Stack Mode (With Daemon)

Agents run in tmux sessions. Daemon manages lifecycle automatically.

```bash
# Start the daemon
ms daemon start

# Create and assign work (workers spawn automatically)
ms minecart create "Feature X" ms-abc12 ms-def34
ms sling ms-abc12 myproject
ms sling ms-def34 myproject

# Monitor on dashboard
ms minecart list

# Attach to any agent session
ms overseer attach
ms witness attach myproject
```

**When to use**: Production workflows with multiple concurrent agents.

### Choosing Roles

Mineshaft is modular. Enable only what you need:

| Configuration | Roles | Use Case |
|--------------|-------|----------|
| **Miners only** | Workers | Manual spawning, no monitoring |
| **+ Witness** | + Monitor | Automatic lifecycle, stuck detection |
| **+ Refinery** | + Merge queue | MR review, code integration |
| **+ Overseer** | + Coordinator | Cross-project coordination |

## Troubleshooting

### `ms: command not found`

The Mineshaft binary directory is not in PATH. Homebrew usually handles this for
Homebrew installs. Source installs place `ms` in `~/.local/bin`:

```bash
# Add to your shell config (~/.bashrc, ~/.zshrc)
export PATH="$HOME/.local/bin:$PATH"
source ~/.bashrc  # or restart terminal
```

If you also installed Beads with Go, keep `$HOME/go/bin` in PATH for `bd`.

### `bd: command not found`

Beads CLI not installed:

```bash
go install github.com/steveyegge/beads/cmd/bd@latest
```

### `ms doctor` shows errors

Run with `--fix` to auto-repair common issues:

```bash
ms doctor --fix
```

For persistent issues, check specific errors:

```bash
ms doctor --verbose
```

### Daemon not starting

Check if tmux is installed and working:

```bash
tmux -V                    # Should show version
tmux new-session -d -s test && tmux kill-session -t test  # Quick test
```

### Git authentication issues

Ensure SSH keys or credentials are configured:

```bash
# Test SSH access
ssh -T git@github.com

# Or configure credential helper
git config --global credential.helper cache
```

### Beads issues

If experiencing beads problems:

```bash
cd ~/ms/myproject/overseer/rig
bd status                  # Check database health
bd doctor                  # Run beads health check
```

## Updating

Update Mineshaft through the same channel you used to install it. For the
recommended Homebrew install:

```bash
brew update
brew upgrade mineshaft
command -v ms              # Should be Homebrew's ms, e.g. /opt/homebrew/bin/ms
ms version
ms doctor --fix            # Fix any post-update issues
```

If you installed from source, update the checkout and rebuild with `make` rather
than installing `ms` with `go install` on macOS:

```bash
git pull --ff-only
make install
command -v ms              # Should be ~/.local/bin/ms
ms version
ms doctor --fix
```

If you maintain Beads separately from Homebrew, update `bd` from its own source:

```bash
go install github.com/steveyegge/beads/cmd/bd@latest
```

Run the `command -v ms` and `ms version` checks before `ms doctor --fix` so a
stale shadow binary does not run the repair step first.

If `command -v ms` points at a different install channel than the one you just
updated, fix your PATH before continuing.

## Uninstalling

```bash
# Remove binaries
rm $(which ms) $(which bd)

# Remove workspace (CAUTION: deletes all work)
rm -rf ~/ms
```

## Next Steps

After installation:

1. **Read the README** - Core concepts and workflows
2. **Try a simple workflow** - `bd create "Test task"` then `ms minecart create "Test" <bead-id>`
3. **Explore docs** - `docs/reference.md` for command reference
4. **Run doctor regularly** - `ms doctor` catches problems early
5. **Join the Wasteland** - `ms wl join hop/wl-commons` to browse and claim federated work (see [WASTELAND.md](WASTELAND.md))
