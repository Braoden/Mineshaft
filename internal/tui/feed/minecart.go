package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/util"
)

// minecartIDPattern validates minecart IDs.
var minecartIDPattern = regexp.MustCompile(`^hq-[a-zA-Z0-9-]+$`)

// Minecart represents a minecart's status for the dashboard
type Minecart struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Completed int       `json:"completed"`
	Total     int       `json:"total"`
	CreatedAt time.Time `json:"created_at"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
}

// MQEntry represents a single merge request in the merge queue
type MQEntry struct {
	ID      string // Bead ID (e.g., "ms-mr-abc")
	Branch  string // Source branch name
	Status  string // queued, merging, merged, failed
	Miner string // Miner that submitted (e.g., "nux")
	Rig     string // Which rig this MR belongs to
}

// MinecartState holds all minecart data for the panel
type MinecartState struct {
	InProgress []Minecart
	Landed     []Minecart
	MQEntries  []MQEntry
	LastUpdate time.Time
}

// FetchMinecarts retrieves minecart status from town-level beads
func FetchMinecarts(townRoot string) (*MinecartState, error) {
	townBeads := filepath.Join(townRoot, ".beads")

	state := &MinecartState{
		InProgress: make([]Minecart, 0),
		Landed:     make([]Minecart, 0),
		LastUpdate: time.Now(),
	}

	// Fetch open minecarts
	openMinecarts, err := listMinecarts(townBeads, "open")
	if err != nil {
		// Not a fatal error - just return empty state
		return state, nil
	}

	for _, c := range openMinecarts {
		// Get detailed status for each minecart
		minecart := enrichMinecart(townBeads, c)
		state.InProgress = append(state.InProgress, minecart)
	}

	// Fetch recently closed minecarts (landed in last 24h)
	closedMinecarts, err := listMinecarts(townBeads, "closed")
	if err == nil {
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, c := range closedMinecarts {
			minecart := enrichMinecart(townBeads, c)
			if !minecart.ClosedAt.IsZero() && minecart.ClosedAt.After(cutoff) {
				state.Landed = append(state.Landed, minecart)
			}
		}
	}

	// Sort: in-progress by created (oldest first), landed by closed (newest first)
	sort.Slice(state.InProgress, func(i, j int) bool {
		return state.InProgress[i].CreatedAt.Before(state.InProgress[j].CreatedAt)
	})
	sort.Slice(state.Landed, func(i, j int) bool {
		return state.Landed[i].ClosedAt.After(state.Landed[j].ClosedAt)
	})

	// Fetch merge queue entries from all rigs
	state.MQEntries = fetchMQEntries(townRoot)

	return state, nil
}

// listMinecarts returns minecarts with the given status
func listMinecarts(beadsDir, status string) ([]minecartListItem, error) {
	listArgs := []string{"list", "--status=" + status, "--json", "--limit=0"}

	ctx, cancel := context.WithTimeout(context.Background(), constants.BdSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", listArgs...) //nolint:gosec // G204: args are constructed internally
	util.SetDetachedProcessGroup(cmd)
	cmd.Dir = beadsDir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var rawItems []minecartListItem
	if err := json.Unmarshal(stdout.Bytes(), &rawItems); err != nil {
		return nil, err
	}

	items := make([]minecartListItem, 0, len(rawItems))
	for _, item := range rawItems {
		if item.IssueType == "minecart" || feedMinecartHasLabel(item.Labels, "ms:minecart") {
			items = append(items, item)
		}
	}
	return items, nil
}

type minecartListItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
	ClosedAt  string   `json:"closed_at,omitempty"`
	IssueType string   `json:"issue_type"`
	Labels    []string `json:"labels"`
}

func feedMinecartHasLabel(labels []string, target string) bool {
	for _, label := range labels {
		if label == target {
			return true
		}
	}
	return false
}

// enrichMinecart adds tracked issue counts to a minecart
func enrichMinecart(beadsDir string, item minecartListItem) Minecart {
	minecart := Minecart{
		ID:     item.ID,
		Title:  item.Title,
		Status: item.Status,
	}

	// Parse timestamps
	if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
		minecart.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04", item.CreatedAt); err == nil {
		minecart.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, item.ClosedAt); err == nil {
		minecart.ClosedAt = t
	} else if t, err := time.Parse("2006-01-02 15:04", item.ClosedAt); err == nil {
		minecart.ClosedAt = t
	}

	// Get tracked issues and their status
	tracked := getTrackedIssueStatus(beadsDir, item.ID)
	minecart.Total = len(tracked)
	for _, t := range tracked {
		if t.Status == "closed" {
			minecart.Completed++
		}
	}

	return minecart
}

// Minecart panel styles
var (
	MinecartPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDim).
				Padding(0, 1)

	MinecartTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary)

	MinecartSectionStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Bold(true)

	MinecartIDStyle = lipgloss.NewStyle().
			Foreground(colorHighlight)

	MinecartNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	MinecartProgressStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	MinecartLandedStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	MinecartAgeStyle = lipgloss.NewStyle().
			Foreground(colorDim)
)

// renderMinecartPanel renders the minecart status panel
func (m *Model) renderMinecartPanel() string {
	style := MinecartPanelStyle
	if m.focusedPanel == PanelMinecart {
		style = FocusedBorderStyle
	}
	// Add title before content
	title := MinecartTitleStyle.Render("🚚 Minecarts")
	content := title + "\n" + m.minecartViewport.View()
	return style.Width(m.width - 2).Render(content)
}

// renderMinecarts renders the minecart panel content
// renderMinecarts renders the minecart status content.
// Caller must hold m.mu.
func (m *Model) renderMinecarts() string {
	if m.minecartState == nil {
		return AgentIdleStyle.Render("Loading minecarts...")
	}

	var lines []string

	// In Progress section
	lines = append(lines, MinecartSectionStyle.Render("IN PROGRESS"))
	if len(m.minecartState.InProgress) == 0 {
		lines = append(lines, "  "+AgentIdleStyle.Render("No active minecarts"))
	} else {
		for _, c := range m.minecartState.InProgress {
			lines = append(lines, renderMinecartLine(c, false))
		}
	}

	lines = append(lines, "")

	// Recently Landed section
	lines = append(lines, MinecartSectionStyle.Render("RECENTLY LANDED (24h)"))
	if len(m.minecartState.Landed) == 0 {
		lines = append(lines, "  "+AgentIdleStyle.Render("No recent landings"))
	} else {
		for _, c := range m.minecartState.Landed {
			lines = append(lines, renderMinecartLine(c, true))
		}
	}

	// Merge Queue section
	lines = append(lines, "")
	lines = append(lines, MQTitleStyle.Render("⚙ Merge Queue"))
	if len(m.minecartState.MQEntries) == 0 {
		lines = append(lines, "  "+AgentIdleStyle.Render("No pending merges"))
	} else {
		for _, entry := range m.minecartState.MQEntries {
			lines = append(lines, renderMQLine(entry))
		}
	}

	return strings.Join(lines, "\n")
}

// renderMinecartLine renders a single minecart status line
func renderMinecartLine(c Minecart, landed bool) string {
	// Format: "  hq-xyz  Title       2/4 ●●○○" or "  hq-xyz  Title       ✓ 2h ago"
	id := MinecartIDStyle.Render(c.ID)

	// Truncate title if too long (rune-safe to avoid splitting multi-byte UTF-8)
	title := c.Title
	if utf8.RuneCountInString(title) > 20 {
		runes := []rune(title)
		title = string(runes[:17]) + "..."
	}
	title = MinecartNameStyle.Render(title)

	if landed {
		// Show checkmark and time since landing
		age := formatAge(time.Since(c.ClosedAt))
		status := MinecartLandedStyle.Render("✓") + " " + MinecartAgeStyle.Render(age+" ago")
		return fmt.Sprintf("  %s  %-20s  %s", id, title, status)
	}

	// Show progress bar
	progress := renderProgressBar(c.Completed, c.Total)
	count := MinecartProgressStyle.Render(fmt.Sprintf("%d/%d", c.Completed, c.Total))
	return fmt.Sprintf("  %s  %-20s  %s %s", id, title, count, progress)
}

// renderMQLine renders a single merge queue entry
func renderMQLine(entry MQEntry) string {
	// Format: "  ⚙ miner/nux  branch-name       merging"
	var statusStyle lipgloss.Style
	var statusIcon string
	switch entry.Status {
	case "merging":
		statusStyle = MQStatusMerging
		statusIcon = "⚙"
	case "queued":
		statusStyle = MQStatusQueued
		statusIcon = "○"
	case "merged":
		statusStyle = MQStatusMerged
		statusIcon = "✓"
	case "failed":
		statusStyle = MQStatusFailed
		statusIcon = "✗"
	default:
		statusStyle = MQStatusQueued
		statusIcon = "?"
	}

	// Truncate branch name if too long (rune-safe)
	branch := entry.Branch
	if utf8.RuneCountInString(branch) > 30 {
		runes := []rune(branch)
		branch = string(runes[:27]) + "..."
	}

	// Build the line
	status := statusStyle.Render(statusIcon + " " + entry.Status)
	branchPart := MQBranchStyle.Render(branch)

	minerPart := ""
	if entry.Miner != "" {
		minerPart = MQMinerStyle.Render(entry.Miner)
	}

	return fmt.Sprintf("  %s  %-30s  %s", status, branchPart, minerPart)
}

// MQ panel styles
var (
	MQTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	MQStatusQueued = lipgloss.NewStyle().
			Foreground(colorDim)

	MQStatusMerging = lipgloss.NewStyle().
			Foreground(colorPrimary)

	MQStatusMerged = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	MQStatusFailed = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	MQBranchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	MQMinerStyle = lipgloss.NewStyle().
			Foreground(colorAccent)
)

// mqListItem represents a raw MR bead from bd list --json output
type mqListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedBy string `json:"created_by,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
}

// fetchMQEntries queries all rigs for merge-request beads
func fetchMQEntries(townRoot string) []MQEntry {
	// Load rigs config to discover rigs
	rigsConfigPath := constants.OverseerRigsPath(townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil
	}

	var entries []MQEntry
	for rigName := range rigsConfig.Rigs {
		rigPath := filepath.Join(townRoot, rigName)
		// Check rig directory exists
		if _, err := os.Stat(rigPath); err != nil {
			continue
		}

		// Fetch open and in-progress MRs
		for _, status := range []string{"open", "in_progress"} {
			items := listMQBeads(rigPath, status)
			for _, item := range items {
				entry := mqItemToEntry(item, rigName)
				entries = append(entries, entry)
			}
		}
	}

	// Sort: in-progress (merging) first, then open (queued)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Status != entries[j].Status {
			return entries[i].Status == "merging"
		}
		return entries[i].ID < entries[j].ID
	})

	return entries
}

// listMQBeads queries bd for merge-request beads with given status
func listMQBeads(rigPath, status string) []mqListItem {
	ctx, cancel := context.WithTimeout(context.Background(), constants.BdSubprocessTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", "list",
		"--label=ms:merge-request",
		"--status="+status,
		"--json",
	)
	util.SetDetachedProcessGroup(cmd)
	cmd.Dir = rigPath
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var items []mqListItem
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		return nil
	}
	return items
}

// mqItemToEntry converts a raw MQ bead to an MQEntry with display-friendly fields
func mqItemToEntry(item mqListItem, rigName string) MQEntry {
	entry := MQEntry{
		ID:  item.ID,
		Rig: rigName,
	}

	// Map bead status to display status
	switch item.Status {
	case "in_progress":
		entry.Status = "merging"
	case "open":
		entry.Status = "queued"
	case "closed":
		entry.Status = "merged"
	default:
		entry.Status = item.Status
	}

	// Extract branch name from title (MR beads typically titled with branch name)
	entry.Branch = item.Title
	if entry.Branch == "" {
		entry.Branch = item.ID
	}

	// Extract miner name from assignee or created_by
	miner := item.Assignee
	if miner == "" {
		miner = item.CreatedBy
	}
	// Shorten: "mineshaft/miners/nux" -> "nux", "mineshaft/nux" -> "nux"
	if parts := strings.Split(miner, "/"); len(parts) > 0 {
		miner = parts[len(parts)-1]
	}
	entry.Miner = miner

	return entry
}

// renderProgressBar creates a simple progress bar: ●●○○
func renderProgressBar(completed, total int) string {
	if total == 0 {
		return ""
	}

	// Cap at 5 dots for display
	displayTotal := total
	if displayTotal > 5 {
		displayTotal = 5
	}

	filled := (completed * displayTotal) / total
	if filled > displayTotal {
		filled = displayTotal
	}

	bar := strings.Repeat("●", filled) + strings.Repeat("○", displayTotal-filled)
	return MinecartProgressStyle.Render(bar)
}
