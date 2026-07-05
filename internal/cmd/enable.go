// ABOUTME: Command to enable Mineshaft system-wide.
// ABOUTME: Sets the global state to enabled for all agentic coding tools.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/state"
	"github.com/steveyegge/mineshaft/internal/style"
)

var enableCmd = &cobra.Command{
	Use:     "enable",
	GroupID: GroupConfig,
	Short:   "Enable Mineshaft system-wide",
	Long: `Enable Mineshaft for all agentic coding tools.

When enabled:
  - Shell hooks set MS_TOWN_ROOT and MS_RIG environment variables
  - Claude Code SessionStart hooks run 'ms prime' for context
  - Git repos are auto-registered as rigs (configurable)

Use 'ms disable' to turn off. Use 'ms status' to check state.

Environment overrides:
  MINESHAFT_DISABLED=1  - Disable for current session only
  MINESHAFT_ENABLED=1   - Enable for current session only`,
	RunE: runEnable,
}

func init() {
	rootCmd.AddCommand(enableCmd)
}

func runEnable(cmd *cobra.Command, args []string) error {
	if err := state.Enable(Version); err != nil {
		return fmt.Errorf("enabling Mineshaft: %w", err)
	}

	fmt.Printf("%s Mineshaft enabled\n", style.Success.Render("✓"))
	fmt.Println()
	fmt.Println("Mineshaft will now:")
	fmt.Println("  • Inject context into Claude Code sessions")
	fmt.Println("  • Set MS_TOWN_ROOT and MS_RIG environment variables")
	fmt.Println("  • Auto-register git repos as rigs (if configured)")
	fmt.Println()
	fmt.Printf("Use %s to disable, %s to check status\n",
		style.Dim.Render("ms disable"),
		style.Dim.Render("ms status"))

	return nil
}
