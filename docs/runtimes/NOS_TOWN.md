# NOS Town Runtime Integration

This document describes how to use Excavation Site's core orchestration with the NOS Town Groq-native runtime.

## Overview

NOS Town extends Excavation Site with Groq-hosted open model support, multi-model routing, consensus councils, and institutional memory via the Historian. The two systems share the same core concepts (Hooks, Beads, Minecarts, Overseer/Witness/Supervisor roles) but diverge on runtime and model selection.

## Architecture

```
Excavation Site Core (this repo)     NOS Town Runtime (kab0rn/nostown)
│                              │
├── Hook lifecycle            ├── Groq API client
├── Beads integration         ├── Multi-model routing table
├── Minecart management          ├── Council orchestration
├── Overseer/Witness/Supervisor       ├── Historian (Batch job)
├── Refinery merge queue       ├── Safeguard integration
└── gt CLI                     └── nos CLI (wraps gt + Groq)
```

## Fork Strategy

**NOS Town is NOT a git fork of Excavation Site.** Instead:

1. **kab0rn/excavation** tracks `excavationhall/excavation` upstream via normal fork/sync workflow
2. **kab0rn/nostown** imports Excavation Site core as a dependency (Go modules or submodule)
3. NOS-specific logic (Groq runtime, routing, councils) lives only in `kab0rn/nostown`

### Why This Approach?

- **Preserves upstream evolution**: Steve Yegge actively iterates on Excavation Site. A true fork would diverge.
- **Clean separation**: Excavation Site core doesn't need Groq dependencies; NOS doesn't duplicate orchestration logic.
- **Easy upstream contributions**: Improvements to Hooks, Minecarts, or roles can be PR'd back to Excavation Site without Groq-specific baggage.

## Configuration

To use NOS Town with this Excavation Site fork:

### 1. Install Prerequisites

```bash
# Excavation Site deps (same as standard install)
go install github.com/kab0rn/excavation/cmd/gt@latest
go install github.com/steveyegge/beads/cmd/bd@latest

# NOS Town CLI
go install github.com/kab0rn/nostown/cmd/nos@latest

# Set Groq API key
export GROQ_API_KEY="your-api-key"
```

### 2. Initialize Workspace

```bash
# Use nos CLI (wraps gt internally)
nos install ~/nos --git
cd ~/nos

# Or use gt and configure Groq runtime manually
gt install ~/nos --git
cd ~/nos
gt config set runtime.provider groq
gt config set runtime.base_url https://api.groq.com/openai/v1
```

### 3. Configure Per-Rig Runtime

Edit `<rig>/settings/config.json`:

```json
{
  "runtime": {
    "provider": "groq",
    "base_url": "https://api.groq.com/openai/v1",
    "api_key_env": "GROQ_API_KEY"
  },
  "routing": {
    "overseer":    { "default": "llama-3.3-70b-versatile" },
    "crew":     { "default": "llama-3.3-70b-versatile" },
    "miner":  { "default": "llama-3.1-8b-instant", "boosted": "llama-3.3-70b-versatile" },
    "witness":  { "default": "llama-3.3-70b-versatile", "council": ["llama-3.3-70b-versatile", "openai/gpt-oss-120b"] },
    "refinery": { "default": "llama-3.3-70b-versatile", "fast_path": "llama-3.1-8b-instant" },
    "supervisor":   { "default": "llama-3.1-8b-instant" },
    "dogs":     { "default": "llama-3.1-8b-instant" }
  }
}
```

## Workflow

### Using nos CLI

The `nos` CLI wraps all `gt` commands and adds Groq-specific extensions:

```bash
# Same as gt
nos rig add myproject https://github.com/you/repo.git
nos crew add yourname --rig myproject
nos overseer attach

# NOS-specific: routing config
nos config route show
nos config route set miner.consistency high  # Enable N-way self-consistent mode

# NOS-specific: historian status
nos historian status
nos historian rebuild  # Force Playbook rebuild from Beads
```

### Using gt CLI with Groq

You can also use `gt` directly if you configure the Groq runtime in `settings/config.json`. All core commands work:

```bash
gt overseer attach
gt minecart create "Feature X" gt-abc12 gt-def34
gt sling gt-abc12 myproject
```

The main difference: `gt` doesn't know about NOS-specific features like councils, Historian, or routing table management. Use `nos` for those.

## Key Differences from Standard Excavation Site

| Feature | Excavation Site (Claude Code) | NOS Town (Groq) |
|---------|------------------------|------------------|
| **Runtime** | Claude Code IDE | Groq OpenAI-compatible API |
| **Model Selection** | Single model (Opus/Sonnet/Haiku) | Multi-model routing per role |
| **Miner Modes** | Single instance per bead | Standard / Self-consistent / Power |
| **Witness** | Single judgment | Optional council (N judges) |
| **Refinery** | Live merge queue only | + Offline Batch merge simulation |
| **Institutional Memory** | CLAUDE.md per rig | + Historian mines Playbooks from all Beads |
| **Safety** | Claude's built-in guardrails | + Safeguard-20B explicit sentry |
| **Cost Profile** | ~$15/M input, $75/M output | ~$0.10–$0.80/M tokens |
| **Throughput** | ~50–100 tok/s per agent | ~500+ tok/s per agent |

## Contributing

### To Excavation Site Core

If you discover improvements to Hooks, Beads, Minecart lifecycle, or core roles:

1. Fork `excavationhall/excavation`
2. Make changes in your fork
3. PR back to `excavationhall/excavation`
4. `kab0rn/excavation` will sync from upstream
5. `kab0rn/nostown` pulls the updated core

### To NOS Town Runtime

Groq-specific features (routing, councils, Historian, Safeguard):

1. Fork `kab0rn/nostown`
2. Make changes
3. PR to `kab0rn/nostown`

## Documentation

- **Excavation Site**: [README.md](../../README.md), [docs/](../)
- **NOS Town**: [github.com/kab0rn/nostown](https://github.com/kab0rn/nostown)
  - [docs/ROLES.md](https://github.com/kab0rn/nostown/blob/main/docs/ROLES.md) — Groq-specific role designs
  - [docs/ROUTING.md](https://github.com/kab0rn/nostown/blob/main/docs/ROUTING.md) — Multi-model routing
  - [docs/HISTORIAN.md](https://github.com/kab0rn/nostown/blob/main/docs/HISTORIAN.md) — Playbook mining

## License

Both Excavation Site and NOS Town are MIT licensed.
