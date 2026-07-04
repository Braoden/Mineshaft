package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/excavation/internal/config"
	"github.com/steveyegge/excavation/internal/style"
	"github.com/steveyegge/excavation/internal/workspace"
)

var whoamiCmd = &cobra.Command{
	Use:     "whoami",
	GroupID: GroupDiag,
	Short:   "Show current identity for mail commands",
	Long: `Show the identity that will be used for mail commands.

Identity is determined by:
1. GT_ROLE env var (if set) - indicates an agent session
2. No GT_ROLE - you are the boss (human)

Use --identity flag with mail commands to override.

Examples:
  gt whoami                      # Show current identity
  gt mail inbox                  # Check inbox for current identity
  gt mail inbox --identity overseer/  # Check Overseer's inbox instead`,
	RunE: runWhoami,
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}

func runWhoami(cmd *cobra.Command, args []string) error {
	// Get current identity using same logic as mail commands
	identity := detectSender()

	fmt.Printf("%s %s\n", style.Bold.Render("Identity:"), identity)

	// Show how it was determined
	gtRole := os.Getenv("GT_ROLE")
	if gtRole != "" {
		fmt.Printf("%s GT_ROLE=%s\n", style.Dim.Render("Source:"), gtRole)

		// Show additional env vars if present
		if rig := os.Getenv("GT_RIG"); rig != "" {
			fmt.Printf("%s GT_RIG=%s\n", style.Dim.Render("       "), rig)
		}
		if miner := os.Getenv("GT_MINER"); miner != "" {
			fmt.Printf("%s GT_MINER=%s\n", style.Dim.Render("       "), miner)
		}
		if crew := os.Getenv("GT_CREW"); crew != "" {
			fmt.Printf("%s GT_CREW=%s\n", style.Dim.Render("       "), crew)
		}
	} else {
		fmt.Printf("%s no GT_ROLE set (human at terminal)\n", style.Dim.Render("Source:"))

		// If boss, show their configured identity
		if identity == "boss" {
			townRoot, err := workspace.FindFromCwd()
			if err == nil && townRoot != "" {
				if bossConfig, err := config.LoadBossConfig(config.BossConfigPath(townRoot)); err == nil {
					fmt.Printf("\n%s\n", style.Bold.Render("Boss Identity:"))
					fmt.Printf("  Name:  %s\n", bossConfig.Name)
					if bossConfig.Email != "" {
						fmt.Printf("  Email: %s\n", bossConfig.Email)
					}
					if bossConfig.Username != "" {
						fmt.Printf("  User:  %s\n", bossConfig.Username)
					}
					fmt.Printf("  %s %s\n", style.Dim.Render("(detected via"), style.Dim.Render(bossConfig.Source+")"))
				}
			}
		}
	}

	return nil
}
