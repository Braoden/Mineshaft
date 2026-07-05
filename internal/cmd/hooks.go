package cmd

import (
	"github.com/spf13/cobra"
)

var hooksCmd = &cobra.Command{
	Use:     "hooks",
	GroupID: GroupConfig,
	Short:   "Centralized hook management for Mineshaft",
	Long: `Manage Claude Code hooks across the Mineshaft workspace.

Provides centralized hook configuration with a base config and
per-role/per-rig overrides. Changes are propagated to all workers
via the sync command.

Subcommands:
  base       Edit the shared base hook config
  override   Edit overrides for a role or rig
  sync       Regenerate all .claude/settings.json files
  diff       Show what sync would change
  list       Show all managed settings.json locations
  scan       Scan workspace for existing hooks
  registry   List hooks from the registry
  install    Install a hook from the registry

Config structure:
  Base:      ~/.ms/hooks-base.json
  Overrides: ~/.ms/hooks-overrides/<target>.json

Merge strategy: base → role → rig+role (more specific wins)

Examples:
  ms hooks sync           # Regenerate all settings.json files
  ms hooks diff           # Preview what sync would change
  ms hooks base           # Edit the shared base config
  ms hooks override crew  # Edit overrides for all crew workers
  ms hooks list           # Show managed locations and sync status`,
	RunE: requireSubcommand,
}

func init() {
	rootCmd.AddCommand(hooksCmd)
}
