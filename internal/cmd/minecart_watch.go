package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/style"
)

// minecart watch flags
var (
	minecartWatchNudge bool
	minecartWatchAddr  string
	minecartWatchJSON  bool
)

func init() {
	minecartWatchCmd.Flags().BoolVar(&minecartWatchNudge, "nudge", false, "Subscribe for nudge notification instead of mail")
	minecartWatchCmd.Flags().StringVar(&minecartWatchAddr, "addr", "", "Address to notify (default: caller's identity)")
	minecartWatchCmd.Flags().BoolVar(&minecartWatchJSON, "json", false, "Output as JSON")

	minecartUnwatchCmd.Flags().StringVar(&minecartWatchAddr, "addr", "", "Address to remove (default: caller's identity)")

	minecartCmd.AddCommand(minecartWatchCmd)
	minecartCmd.AddCommand(minecartUnwatchCmd)
}

var minecartWatchCmd = &cobra.Command{
	Use:   "watch <minecart-id>",
	Short: "Subscribe to minecart completion notifications",
	Long: `Subscribe to be notified when a minecart completes (all tracked issues close).

By default, sends a mail notification to the caller's identity when the
minecart lands. Use --nudge for lightweight nudge notifications instead.

The watcher list is stored in the minecart's description fields and processed
by notifyMinecartCompletion when the minecart closes.

Examples:
  gt minecart watch hq-cv-abc                    # Mail notification to caller
  gt minecart watch hq-cv-abc --nudge            # Nudge notification to caller
  gt minecart watch hq-cv-abc --addr mineshaft/crew/mel  # Mail notification to mel
  gt minecart watch hq-cv-abc --nudge --addr overseer/    # Nudge overseer on completion`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartWatch,
}

var minecartUnwatchCmd = &cobra.Command{
	Use:   "unwatch <minecart-id>",
	Short: "Unsubscribe from minecart completion notifications",
	Long: `Remove yourself (or a specified address) from a minecart's watcher list.

Removes from both mail and nudge watcher lists.

Examples:
  gt minecart unwatch hq-cv-abc                        # Remove caller from watchers
  gt minecart unwatch hq-cv-abc --addr mineshaft/crew/mel # Remove mel from watchers`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartUnwatch,
}

func runMinecartWatch(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	// Resolve numeric shortcut
	if n, err := strconv.Atoi(minecartID); err == nil && n > 0 {
		townBeads, err := getTownBeadsDir()
		if err != nil {
			return err
		}
		resolved, err := resolveMinecartNumber(townBeads, n)
		if err != nil {
			return err
		}
		minecartID = resolved
	}

	// Determine watcher address
	addr := minecartWatchAddr
	if addr == "" {
		addr = detectSender()
	}
	if addr == "" {
		return fmt.Errorf("could not determine caller identity; use --addr to specify")
	}

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Get minecart details
	minecart, err := getMinecartForWatch(townBeads, minecartID)
	if err != nil {
		return err
	}

	// Parse existing minecart fields
	fields := beads.ParseMinecartFields(&beads.Issue{Description: minecart.Description})
	if fields == nil {
		fields = &beads.MinecartFields{}
	}

	// Add watcher
	var added bool
	var watchType string
	if minecartWatchNudge {
		added = fields.AddNudgeWatcher(addr)
		watchType = "nudge"
	} else {
		added = fields.AddWatcher(addr)
		watchType = "mail"
	}

	if !added {
		if minecartWatchJSON {
			out, _ := json.Marshal(map[string]interface{}{
				"minecart_id":  minecartID,
				"address":    addr,
				"watch_type": watchType,
				"status":     "already_watching",
			})
			fmt.Println(string(out))
		} else {
			fmt.Printf("%s %s is already watching minecart %s (%s)\n", style.Dim.Render("○"), addr, minecartID, watchType)
		}
		return nil
	}

	// Update minecart description with new watcher
	newDesc := beads.SetMinecartFields(&beads.Issue{Description: minecart.Description}, fields)
	if err := updateMinecartDescription(townBeads, minecartID, newDesc); err != nil {
		return fmt.Errorf("updating minecart watchers: %w", err)
	}

	if minecartWatchJSON {
		out, _ := json.Marshal(map[string]interface{}{
			"minecart_id":  minecartID,
			"address":    addr,
			"watch_type": watchType,
			"status":     "subscribed",
		})
		fmt.Println(string(out))
	} else {
		emoji := "📬"
		if minecartWatchNudge {
			emoji = "🔔"
		}
		fmt.Printf("%s %s subscribed to minecart %s (%s notification)\n", emoji, addr, minecartID, watchType)
	}

	return nil
}

func runMinecartUnwatch(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	// Resolve numeric shortcut
	if n, err := strconv.Atoi(minecartID); err == nil && n > 0 {
		townBeads, err := getTownBeadsDir()
		if err != nil {
			return err
		}
		resolved, err := resolveMinecartNumber(townBeads, n)
		if err != nil {
			return err
		}
		minecartID = resolved
	}

	// Determine watcher address
	addr := minecartWatchAddr
	if addr == "" {
		addr = detectSender()
	}
	if addr == "" {
		return fmt.Errorf("could not determine caller identity; use --addr to specify")
	}

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Get minecart details
	minecart, err := getMinecartForWatch(townBeads, minecartID)
	if err != nil {
		return err
	}

	// Parse existing minecart fields
	fields := beads.ParseMinecartFields(&beads.Issue{Description: minecart.Description})
	if fields == nil {
		fmt.Printf("%s %s is not watching minecart %s\n", style.Dim.Render("○"), addr, minecartID)
		return nil
	}

	// Remove from both watcher lists
	removedMail := fields.RemoveWatcher(addr)
	removedNudge := fields.RemoveNudgeWatcher(addr)

	if !removedMail && !removedNudge {
		fmt.Printf("%s %s is not watching minecart %s\n", style.Dim.Render("○"), addr, minecartID)
		return nil
	}

	// Update minecart description
	newDesc := beads.SetMinecartFields(&beads.Issue{Description: minecart.Description}, fields)
	if err := updateMinecartDescription(townBeads, minecartID, newDesc); err != nil {
		return fmt.Errorf("updating minecart watchers: %w", err)
	}

	var types []string
	if removedMail {
		types = append(types, "mail")
	}
	if removedNudge {
		types = append(types, "nudge")
	}
	fmt.Printf("🔕 %s unsubscribed from minecart %s (%s)\n", addr, minecartID, strings.Join(types, "+"))

	return nil
}

// minecartForWatch is a minimal minecart struct for watch operations.
type minecartForWatch struct {
	ID          string
	Title       string
	Status      string
	Type        string
	Description string
}

// getMinecartForWatch fetches and validates a minecart for watch/unwatch operations.
func getMinecartForWatch(townBeads, minecartID string) (*minecartForWatch, error) {
	showCmd := exec.Command("bd", "show", minecartID, "--json")
	showCmd.Dir = townBeads
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return nil, fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Type        string   `json:"issue_type"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &minecarts); err != nil {
		return nil, fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return nil, fmt.Errorf("minecart '%s' not found", minecartID)
	}

	c := minecarts[0]
	if !isMinecartIssue(c.Type, c.Labels) {
		return nil, fmt.Errorf("'%s' is not a minecart (type: %s)", minecartID, c.Type)
	}

	return &minecartForWatch{
		ID:          c.ID,
		Title:       c.Title,
		Status:      c.Status,
		Type:        c.Type,
		Description: c.Description,
	}, nil
}

// updateMinecartDescription updates a minecart's description via bd update.
func updateMinecartDescription(townBeads, minecartID, newDesc string) error {
	updateCmd := exec.Command("bd", "update", minecartID, "--description", newDesc)
	updateCmd.Dir = townBeads
	var stderr bytes.Buffer
	updateCmd.Stderr = &stderr

	if err := updateCmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("bd update: %s", errMsg)
		}
		return fmt.Errorf("bd update: %w", err)
	}
	return nil
}
