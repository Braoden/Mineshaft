// ABOUTME: Shell integration management commands.
// ABOUTME: Install/remove shell hooks without full HQ setup.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/shell"
	"github.com/steveyegge/mineshaft/internal/state"
	"github.com/steveyegge/mineshaft/internal/style"
)

var shellCmd = &cobra.Command{
	Use:     "shell",
	GroupID: GroupConfig,
	Short:   "Manage shell integration",
	Long: `Manage the Mineshaft shell integration hook.

The shell integration adds a cd hook to your shell RC file that automatically
sets MS_TOWN_ROOT and MS_RIG environment variables when you enter a rig directory.

Subcommands: install, remove, status.`,
	RunE: requireSubcommand,
}

var shellInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install or update shell integration",
	Long: `Install or update the Mineshaft shell integration.

This adds a hook to your shell RC file that:
  - Sets MS_TOWN_ROOT and MS_RIG when you cd into a Mineshaft rig
  - Offers to add new git repos to Mineshaft on first visit

Run this after upgrading ms to get the latest shell hook features.`,
	RunE: runShellInstall,
}

var shellRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove shell integration",
	Long: `Remove the Mineshaft shell integration from your shell RC file.

Removes the hook that was added by 'ms shell install'. You may need
to restart your shell or source the RC file for the change to take effect.`,
	RunE: runShellRemove,
}

var shellStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show shell integration status",
	Long: `Show whether Mineshaft is enabled and whether shell integration is installed.

Displays the current state and which shell RC file contains the integration hook.`,
	RunE: runShellStatus,
}

func init() {
	shellCmd.AddCommand(shellInstallCmd)
	shellCmd.AddCommand(shellRemoveCmd)
	shellCmd.AddCommand(shellStatusCmd)
	rootCmd.AddCommand(shellCmd)
}

func runShellInstall(cmd *cobra.Command, args []string) error {
	if err := shell.Install(); err != nil {
		return err
	}

	if err := state.Enable(Version); err != nil {
		fmt.Printf("%s Could not enable Mineshaft: %v\n", style.Dim.Render("⚠"), err)
	}

	rcPath := shell.RCFilePath(shell.DetectShell())
	fmt.Printf("%s Shell integration installed (%s)\n", style.Success.Render("✓"), rcPath)
	fmt.Println()
	fmt.Printf("Run 'source %s' or open a new terminal to activate.\n", rcPath)
	return nil
}

func runShellRemove(cmd *cobra.Command, args []string) error {
	if err := shell.Remove(); err != nil {
		return err
	}

	fmt.Printf("%s Shell integration removed\n", style.Success.Render("✓"))
	return nil
}

func runShellStatus(cmd *cobra.Command, args []string) error {
	s, err := state.Load()
	if err != nil {
		fmt.Println("Mineshaft: not configured")
		fmt.Println("Shell integration: not installed")
		return nil
	}

	if s.Enabled {
		fmt.Println("Mineshaft: enabled")
	} else {
		fmt.Println("Mineshaft: disabled")
	}

	if s.ShellIntegration != "" {
		fmt.Printf("Shell integration: %s (%s)\n", s.ShellIntegration, shell.RCFilePath(s.ShellIntegration))
	} else {
		fmt.Println("Shell integration: not installed")
	}

	return nil
}
