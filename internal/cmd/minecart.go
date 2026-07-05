package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/config"
	minecartops "github.com/steveyegge/mineshaft/internal/minecart"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/style"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/tui/minecart"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

var minecartIDEntropy io.Reader = rand.Reader

// generateShortID generates a collision-resistant minecart ID suffix using base36.
// 5 chars of base36 gives ~60M possible values (36^5 = 60,466,176).
// Birthday paradox: ~1% collision at ~1,100 IDs — safe for minecart volumes. (#2063)
func generateShortID() string {
	return generateShortIDFromReader(minecartIDEntropy)
}

func generateShortIDFromReader(r io.Reader) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 5)
	_, _ = io.ReadFull(r, b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// looksLikeIssueID checks if a string looks like a beads issue ID.
// Issue IDs have the format: prefix-id (e.g., gt-abc, bd-xyz, hq-123).
func looksLikeIssueID(s string) bool {
	// Check registry prefixes and legacy fallbacks via centralized helper
	if session.HasKnownPrefix(s) {
		return true
	}
	// Pattern check: 2-3 lowercase letters followed by hyphen.
	// Covers unregistered short rig prefixes (e.g., nx, rpk).
	// Longer prefixes (4+ chars like nrpk) are caught by HasKnownPrefix
	// via the registry — no need to heuristic-match them here.
	hyphenIdx := strings.Index(s, "-")
	if hyphenIdx >= 2 && hyphenIdx <= 3 && len(s) > hyphenIdx+1 {
		prefix := s[:hyphenIdx]
		for _, c := range prefix {
			if c < 'a' || c > 'z' {
				return false
			}
		}
		return true
	}
	return false
}

// Minecart command flags
var (
	minecartMolecule     string
	minecartNotify       string
	minecartOwner        string
	minecartOwned        bool
	minecartMerge        string
	minecartBaseBranch   string
	minecartStatusJSON   bool
	minecartListJSON     bool
	minecartListStatus   string
	minecartListAll      bool
	minecartListTree     bool
	minecartInteractive  bool
	minecartStrandedJSON bool
	minecartCloseReason  string
	minecartCloseNotify  string
	minecartCloseForce   bool
	minecartCheckDryRun  bool
	minecartLandForce    bool
	minecartLandKeep     bool
	minecartLandDryRun   bool
	minecartFromEpic     string
)

const (
	minecartStatusOpen           = "open"
	minecartStatusClosed         = "closed"
	minecartStatusStagedReady    = "staged_ready"
	minecartStatusStagedWarnings = "staged_warnings"

	// trackedStatusUnknown is the sentinel for a tracked dependency whose
	// status could not be resolved — typically a cross-rig bead whose rig DB
	// is missing, parked, or unroutable from the minecart owner's cwd. Distinct
	// from "open" so auto-close does not mistake it for pending work and
	// `gt minecart status` can label it clearly. (gt-bs6 / GH#2786)
	trackedStatusUnknown = "unknown"
)

func normalizeMinecartStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func ensureKnownMinecartStatus(status string) error {
	switch normalizeMinecartStatus(status) {
	case minecartStatusOpen, minecartStatusClosed, minecartStatusStagedReady, minecartStatusStagedWarnings:
		return nil
	default:
		return fmt.Errorf(
			"unsupported minecart status %q (expected %q, %q, %q, or %q)",
			status,
			minecartStatusOpen,
			minecartStatusClosed,
			minecartStatusStagedReady,
			minecartStatusStagedWarnings,
		)
	}
}

// isStagedStatus reports whether the given normalized status is a staged status.
func isStagedStatus(status string) bool {
	return strings.HasPrefix(status, "staged_")
}

func validateMinecartStatusTransition(currentStatus, targetStatus string) error {
	current := normalizeMinecartStatus(currentStatus)
	target := normalizeMinecartStatus(targetStatus)

	if err := ensureKnownMinecartStatus(current); err != nil {
		return err
	}
	if err := ensureKnownMinecartStatus(target); err != nil {
		return err
	}
	if current == target {
		return nil
	}

	// Original open ↔ closed transitions.
	if (current == minecartStatusOpen && target == minecartStatusClosed) ||
		(current == minecartStatusClosed && target == minecartStatusOpen) {
		return nil
	}

	// Staged → open (launch) and staged → closed (cancel) are allowed.
	if isStagedStatus(current) && (target == minecartStatusOpen || target == minecartStatusClosed) {
		return nil
	}

	// Staged ↔ staged transitions (re-stage with different result).
	if isStagedStatus(current) && isStagedStatus(target) {
		return nil
	}

	// REJECT: open → staged_* and closed → staged_* are not allowed.
	// (Falls through to the error below.)

	return fmt.Errorf("illegal minecart status transition %q -> %q", currentStatus, targetStatus)
}

var minecartCmd = &cobra.Command{
	Use:         "minecart",
	GroupID:     GroupWork,
	Annotations: map[string]string{AnnotationMinerSafe: "true"},
	Short:       "Track batches of work across rigs",
	RunE: func(cmd *cobra.Command, args []string) error {
		if minecartInteractive {
			return runMinecartTUI()
		}
		return requireSubcommand(cmd, args)
	},
	Long: `Manage minecarts - the primary unit for tracking batched work.

A minecart is a persistent tracking unit that monitors related issues across
rigs. When you kick off work (even a single issue), a minecart tracks it so
you can see when it lands and what was included.

WHAT IS A MINECART:
  - Persistent tracking unit with an ID (hq-*)
  - Tracks issues across rigs (frontend+backend, beads+mineshaft, etc.)
  - Auto-closes when all tracked issues complete → notifies subscribers
  - Can be reopened by adding more issues

WHAT IS A SWARM:
  - Ephemeral: "the workers currently assigned to a minecart's issues"
  - No separate ID - uses the minecart ID
  - Dissolves when work completes

TRACKING SEMANTICS:
  - 'tracks' relation is non-blocking (tracked issues don't block minecart)
  - Cross-prefix capable (minecart in hq-* tracks issues in gt-*, bd-*)
  - Landed: all tracked issues closed → notification sent to subscribers

COMMANDS:
  create    Create a minecart tracking specified issues
  add       Add issues to an existing minecart (reopens if closed)
  close     Close a minecart (verifies all items done, or use --force)
  land      Land an owned minecart (cleanup worktrees, close minecart)
  status    Show minecart progress, tracked issues, and active workers
  list      List minecarts (the dashboard view)
  watch     Subscribe to minecart completion notifications
  unwatch   Unsubscribe from minecart completion notifications`,
}

var minecartCreateCmd = &cobra.Command{
	Use:   "create <name> [issues...]",
	Short: "Create a new minecart",
	Long: `Create a new minecart that tracks the specified issues.

The minecart is created in town-level beads (hq-* prefix) and can track
issues across any rig.

The --owner flag specifies who requested the minecart (receives completion
notification by default). If not specified, defaults to created_by.
The --notify flag adds additional subscribers beyond the owner.

The --merge flag sets the merge strategy for all work in the minecart:
  direct  Push branch directly to main (no MR, no refinery)
  mr      Create merge-request bead, refinery processes (default)
  local   Keep on feature branch (for upstream PRs, human review)

Examples:
  gt minecart create "Deploy v2.0" gt-abc bd-xyz
  gt minecart create "Release prep" gt-abc --notify           # defaults to overseer/
  gt minecart create "Release prep" gt-abc --notify ops/      # notify ops/
  gt minecart create "Feature rollout" gt-a gt-b --owner overseer/ --notify ops/
  gt minecart create "Feature rollout" gt-a gt-b gt-c --molecule mol-release
  gt minecart create --owned "Manual deploy" gt-abc           # caller-managed lifecycle
  gt minecart create "Quick fix" gt-abc --merge=direct        # bypass refinery

  # Auto-discover issues from an epic's children:
  gt minecart create --from-epic gt-epic-abc
  gt minecart create --from-epic gt-epic-abc --owned --merge=direct`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE:         runMinecartCreate,
}

var minecartStatusCmd = &cobra.Command{
	Use:   "status [minecart-id]",
	Short: "Show minecart status",
	Long: `Show detailed status for a minecart.

Displays minecart metadata, tracked issues, and completion progress.
Without an ID, shows status of all active minecarts.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartStatus,
}

var minecartListCmd = &cobra.Command{
	Use:   "list",
	Short: "List minecarts",
	Long: `List minecarts, showing open minecarts by default.

Examples:
  gt minecart list              # Open minecarts only (default)
  gt minecart list --all        # All minecarts (open + closed)
  gt minecart list --status=closed  # Recently landed
  gt minecart list --tree       # Show minecart + child status tree
  gt minecart list --json`,
	SilenceUsage: true,
	RunE:         runMinecartList,
}

var minecartAddCmd = &cobra.Command{
	Use:   "add <minecart-id> <issue-id> [issue-id...]",
	Short: "Add issues to an existing minecart",
	Long: `Add issues to an existing minecart.

If the minecart is closed, it will be automatically reopened.

Examples:
  gt minecart add hq-cv-abc gt-new-issue
  gt minecart add hq-cv-abc gt-issue1 gt-issue2 gt-issue3`,
	Args:         cobra.MinimumNArgs(2),
	SilenceUsage: true,
	RunE:         runMinecartAdd,
}

var minecartCheckCmd = &cobra.Command{
	Use:   "check [minecart-id]",
	Short: "Check and auto-close completed minecarts",
	Long: `Check minecarts and auto-close any where all tracked issues are complete.

Without arguments, checks all open minecarts. With a minecart ID, checks only that minecart.

This handles cross-rig minecart completion: minecarts in town beads tracking issues
in rig beads won't auto-close via bd close alone. This command bridges that gap.

Can be run manually or by supervisor patrol to ensure minecarts close promptly.

Examples:
  gt minecart check              # Check all open minecarts
  gt minecart check hq-cv-abc    # Check specific minecart
  gt minecart check --dry-run    # Preview what would close without acting`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartCheck,
}

var minecartStrandedCmd = &cobra.Command{
	Use:   "stranded",
	Short: "Find stranded minecarts (ready work, stuck, or empty) needing attention",
	Long: `Find minecarts that have ready issues but no workers processing them,
stuck minecarts (tracked issues but none ready), or empty minecarts that need cleanup.

A minecart is "stranded" when:
- Minecart is open AND either:
  - Has tracked issues that are ready but unassigned, OR
  - Has tracked issues but none are ready (stuck — waiting on dependencies/workers), OR
  - Has 0 tracked issues (empty — needs auto-close via minecart check)

Use this to detect minecarts that need feeding or cleanup. The Supervisor patrol
runs this periodically and dispatches dogs to feed stranded minecarts.

Examples:
  gt minecart stranded              # Show stranded minecarts
  gt minecart stranded --json       # Machine-readable output for automation`,
	SilenceUsage: true,
	RunE:         runMinecartStranded,
}

var minecartCloseCmd = &cobra.Command{
	Use:   "close <minecart-id>",
	Short: "Close a minecart",
	Long: `Close a minecart, optionally with a reason.

By default, verifies that all tracked issues are closed before allowing the
close. Use --force to close regardless of tracked issue status.

The close is idempotent - closing an already-closed minecart is a no-op.

Examples:
  gt minecart close hq-cv-abc                           # Close (all items must be done)
  gt minecart close hq-cv-abc --force                   # Force close abandoned minecart
  gt minecart close hq-cv-abc --reason="no longer needed" --force
  gt minecart close hq-cv-xyz --notify overseer/`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartClose,
}

var minecartLandCmd = &cobra.Command{
	Use:   "land <minecart-id>",
	Short: "Land an owned minecart (cleanup worktrees, close minecart)",
	Long: `Land an owned minecart, performing caller-side cleanup.

This is the caller-managed equivalent of the witness/refinery merge pipeline.
Use this to explicitly land a minecart when you're satisfied with the results.

The command:
  1. Verifies the minecart has the gt:owned label (refuses non-owned minecarts)
  2. Checks all tracked issues are done/closed (use --force to override)
  3. Cleans up miner worktrees associated with the minecart's tracked issues
  4. Closes the minecart bead with reason "Landed by owner"
  5. Sends completion notifications to owner/notify addresses

Use 'gt minecart close' instead for non-owned minecarts.

Examples:
  gt minecart land hq-cv-abc                  # Land owned minecart
  gt minecart land hq-cv-abc --force          # Land even with open issues
  gt minecart land hq-cv-abc --keep-worktrees # Skip worktree cleanup
  gt minecart land hq-cv-abc --dry-run        # Preview what would happen`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runMinecartLand,
}

func init() {
	// Create flags
	minecartCreateCmd.Flags().StringVar(&minecartMolecule, "molecule", "", "Associated molecule ID")
	minecartCreateCmd.Flags().StringVar(&minecartOwner, "owner", "", "Owner who requested minecart (gets completion notification)")
	minecartCreateCmd.Flags().StringVar(&minecartNotify, "notify", "", "Additional address to notify on completion (default: overseer/ if flag used without value)")
	minecartCreateCmd.Flags().Lookup("notify").NoOptDefVal = "overseer/"
	minecartCreateCmd.Flags().BoolVar(&minecartOwned, "owned", false, "Mark minecart as caller-managed lifecycle (no automatic witness/refinery registration)")
	minecartCreateCmd.Flags().StringVar(&minecartMerge, "merge", "", "Merge strategy: direct (push to main), mr (merge queue, default), local (keep on branch)")
	minecartCreateCmd.Flags().StringVar(&minecartBaseBranch, "base-branch", "", "Target branch for miners (e.g., 'feat/extraction-review')")
	minecartCreateCmd.Flags().StringVar(&minecartFromEpic, "from-epic", "", "Auto-discover tracked issues from an epic's slingable children")

	// Status flags
	minecartStatusCmd.Flags().BoolVar(&minecartStatusJSON, "json", false, "Output as JSON")

	// List flags
	minecartListCmd.Flags().BoolVar(&minecartListJSON, "json", false, "Output as JSON")
	minecartListCmd.Flags().StringVar(&minecartListStatus, "status", "", "Filter by status (open, closed)")
	minecartListCmd.Flags().BoolVar(&minecartListAll, "all", false, "Show all minecarts (open and closed)")
	minecartListCmd.Flags().BoolVar(&minecartListTree, "tree", false, "Show minecart + child status tree")

	// Interactive TUI flag (on parent command)
	minecartCmd.Flags().BoolVarP(&minecartInteractive, "interactive", "i", false, "Interactive tree view")

	// Check flags
	minecartCheckCmd.Flags().BoolVar(&minecartCheckDryRun, "dry-run", false, "Preview what would close without acting")

	// Stranded flags
	minecartStrandedCmd.Flags().BoolVar(&minecartStrandedJSON, "json", false, "Output as JSON")

	// Close flags
	minecartCloseCmd.Flags().StringVar(&minecartCloseReason, "reason", "", "Reason for closing the minecart")
	minecartCloseCmd.Flags().StringVar(&minecartCloseNotify, "notify", "", "Agent to notify on close (e.g., overseer/)")
	minecartCloseCmd.Flags().BoolVarP(&minecartCloseForce, "force", "f", false, "Close even if tracked issues are still open")

	// Land flags
	minecartLandCmd.Flags().BoolVarP(&minecartLandForce, "force", "f", false, "Land even if tracked issues are not all closed")
	minecartLandCmd.Flags().BoolVar(&minecartLandKeep, "keep-worktrees", false, "Skip worktree cleanup")
	minecartLandCmd.Flags().BoolVar(&minecartLandDryRun, "dry-run", false, "Show what would happen without acting")

	// Add subcommands
	minecartCmd.AddCommand(minecartCreateCmd)
	minecartCmd.AddCommand(minecartStatusCmd)
	minecartCmd.AddCommand(minecartListCmd)
	minecartCmd.AddCommand(minecartAddCmd)
	minecartCmd.AddCommand(minecartCheckCmd)
	minecartCmd.AddCommand(minecartStrandedCmd)
	minecartCmd.AddCommand(minecartCloseCmd)
	minecartCmd.AddCommand(minecartLandCmd)
	minecartCmd.AddCommand(minecartStageCmd)
	minecartCmd.AddCommand(minecartLaunchCmd)

	rootCmd.AddCommand(minecartCmd)
}

// getTownBeadsDir returns the town root directory for bd commands.
// Minecart commands run bd from town root (not .beads/) so bd discovers
// the correct database via its own workspace detection.
func getTownBeadsDir() (string, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", fmt.Errorf("not in a Mineshaft workspace: %w", err)
	}
	return townRoot, nil
}

// runBdJSON runs a bd command, captures stdout as JSON output. If the command
// fails, the error includes bd's stderr for diagnostics instead of a bare
// "exit status 1". BEADS_DIR is stripped from the subprocess environment to
// prevent stale overrides from interfering with bd's workspace detection.
func runBdJSON(dir string, args ...string) ([]byte, error) {
	return runBdJSONWithOptions(dir, false, false, args...)
}

func runBdJSONAllowStale(dir string, args ...string) ([]byte, error) {
	return runBdJSONWithOptions(dir, true, false, args...)
}

func runBdJSONWithAutoCommit(dir string, args ...string) ([]byte, error) {
	return runBdJSONWithOptions(dir, false, true, args...)
}

func runBdJSONWithOptions(dir string, allowStale, autoCommit bool, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	bdc := BdCmd(args...).Dir(dir).StripBeadsDir().Stderr(&stderr)
	if allowStale {
		bdc.AllowStale()
	}
	if autoCommit {
		bdc.WithAutoCommit()
	}
	cmd := bdc.Build()
	cmd.Dir = dir
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		if errMsg := strings.TrimSpace(stderr.String()); errMsg != "" {
			return nil, fmt.Errorf("bd %s: %s", args[0], errMsg)
		}
		return nil, fmt.Errorf("bd %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

// bdDepListRawIDs queries the raw dependencies table via bd sql to get
// dependency target IDs. Unlike bd dep list, this does NOT join with the
// issues table, so it works for cross-database dependencies where the
// target issues live in a different Dolt database. See GH #2624.
//
// dir should be the town beads directory (.beads) for HQ queries.
// direction is "down" (issue_id → depends_on_id) or "up" (depends_on_id → issue_id).
// depType filters by dependency type (e.g., "tracks", "blocks"); empty means all types.
//
// Returns deduplicated, unwrapped issue IDs (external:prefix:id → id).
func bdDepListRawIDs(dir, issueID, direction, depType string) ([]string, error) {
	// Bead IDs are system-generated alphanumeric strings with hyphens, dots,
	// and underscores — validate to prevent injection before interpolating below.
	if !isValidBeadID(issueID) {
		return nil, fmt.Errorf("invalid bead ID: %q", issueID)
	}

	var parseKey string
	if direction == "up" {
		parseKey = "issue_id"
	} else {
		parseKey = "depends_on_id"
	}
	if depType != "" && !isValidBeadID(depType) {
		return nil, fmt.Errorf("invalid dep type: %q", depType)
	}

	if ids, err := bdDepListRawIDsViaDolt(dir, issueID, direction, depType); err == nil {
		return ids, nil
	}

	var lastErr error
	for _, legacy := range []bool{false, true} {
		query := rawDepSQLLiteral(issueID, direction, depType, legacy)
		out, err := runBdJSONWithAutoCommit(dir, "sql", query, "--json")
		if err != nil {
			lastErr = err
			continue
		}
		ids, err := parseRawDepRows(out, parseKey)
		if err != nil {
			return nil, fmt.Errorf("parsing dep sql for %s: %w", issueID, err)
		}
		return ids, nil
	}
	return nil, fmt.Errorf("bd sql for deps of %s: %w", issueID, lastErr)
}

func bdDepListRawIDsViaDolt(dir, issueID, direction, depType string) ([]string, error) {
	beadsDir := beads.ResolveBeadsDir(dir)
	cfg, ok := readBeadsRuntimeConfig(beadsDir)
	if !ok || cfg.Database == "" || cfg.Port == 0 {
		return nil, fmt.Errorf("missing server metadata for %s", beadsDir)
	}
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	dsn := fmt.Sprintf("root@tcp(%s)/%s?parseTime=true", net.JoinHostPort(host, strconv.Itoa(cfg.Port)), url.PathEscape(cfg.Database))
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	typedQuery, typedArgs := rawDepSQLArgs(issueID, direction, depType, false)
	ids, err := queryRawDepIDs(ctx, db, typedQuery, typedArgs)
	if err == nil {
		return ids, nil
	}
	legacyQuery, legacyArgs := rawDepSQLArgs(issueID, direction, depType, true)
	return queryRawDepIDs(ctx, db, legacyQuery, legacyArgs)
}

func rawDepSQLArgs(issueID, direction, depType string, legacy bool) (string, []any) {
	var query string
	var args []any
	if direction == "up" {
		if legacy {
			query = "SELECT issue_id FROM dependencies WHERE depends_on_id = ?"
			args = append(args, issueID)
		} else {
			query = "SELECT issue_id FROM dependencies WHERE (depends_on_issue_id = ? OR depends_on_wisp_id = ? OR depends_on_external LIKE ? ESCAPE '!')"
			args = append(args, issueID, issueID, "%:"+strings.ReplaceAll(issueID, "_", "!_"))
		}
	} else if legacy {
		query = "SELECT depends_on_id FROM dependencies WHERE issue_id = ?"
		args = append(args, issueID)
	} else {
		query = "SELECT COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external) AS depends_on_id FROM dependencies WHERE issue_id = ?"
		args = append(args, issueID)
	}
	if depType != "" {
		query += " AND type = ?"
		args = append(args, depType)
	}
	return query, args
}

func rawDepSQLLiteral(issueID, direction, depType string, legacy bool) string {
	query, args := rawDepSQLArgs(issueID, direction, depType, legacy)
	for _, arg := range args {
		query = strings.Replace(query, "?", "'"+arg.(string)+"'", 1)
	}
	return query
}

func queryRawDepIDs(ctx context.Context, db *sql.DB, query string, args []any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var ids []string
	for rows.Next() {
		var rawID sql.NullString
		if err := rows.Scan(&rawID); err != nil {
			return nil, err
		}
		if !rawID.Valid {
			continue
		}
		id := beads.ExtractIssueID(rawID.String)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func parseRawDepRows(out []byte, parseKey string) ([]string, error) {
	var rows []map[string]string
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(rows))
	var ids []string
	for _, row := range rows {
		id := beads.ExtractIssueID(row[parseKey])
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func sqlExternalDepTargetClause(issueID string) string {
	// Use an escape character that is not valid in bead IDs so underscores stay literal.
	escapedID := strings.ReplaceAll(issueID, "_", "!_")
	return fmt.Sprintf("depends_on_external LIKE '%%:%s' ESCAPE '!'", escapedID)
}

// isValidBeadID checks that a string is safe for SQL interpolation in dep queries.
// Bead IDs contain only alphanumeric chars, hyphens, dots, and underscores.
func isValidBeadID(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

// collectEpicChildren does a BFS walk of an epic's parent-child hierarchy and
// returns all slingable leaf descendants (task, bug, feature, chore).
func collectEpicChildren(epicID string) ([]string, error) {
	epic, err := bdShow(epicID)
	if err != nil {
		return nil, fmt.Errorf("epic '%s' not found: %w", epicID, err)
	}
	if epic.IssueType != "epic" {
		return nil, fmt.Errorf("'%s' is not an epic (type: %s); --from-epic only works with epic beads", epicID, epic.IssueType)
	}

	var issueIDs []string
	visited := make(map[string]bool)
	queue := []string{epicID}
	visited[epicID] = true

	for len(queue) > 0 {
		parentID := queue[0]
		queue = queue[1:]

		children, err := bdListChildren(parentID)
		if err != nil {
			style.PrintWarning("couldn't list children of %s: %v", parentID, err)
			continue
		}

		for _, child := range children {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true

			if minecartops.IsSlingableType(child.IssueType) {
				issueIDs = append(issueIDs, child.ID)
			} else {
				// Non-slingable types (sub-epics, decisions) — recurse to find slingable descendants
				queue = append(queue, child.ID)
			}
		}
	}

	if len(issueIDs) == 0 {
		return nil, fmt.Errorf("epic '%s' has no slingable children (task, bug, feature, chore)", epicID)
	}
	return issueIDs, nil
}

func runMinecartCreate(cmd *cobra.Command, args []string) error {
	// Validate --merge flag if provided
	if minecartMerge != "" {
		switch minecartMerge {
		case "direct", "mr", "local":
			// Valid
		default:
			return fmt.Errorf("invalid --merge value %q: must be direct, mr, or local", minecartMerge)
		}
	}

	var name string
	var trackedIssues []string

	if minecartFromEpic != "" {
		// --from-epic mode: auto-discover children
		epicIssues, err := collectEpicChildren(minecartFromEpic)
		if err != nil {
			return err
		}
		trackedIssues = epicIssues

		// Use epic title as minecart name unless a name arg was provided
		if len(args) > 0 {
			name = args[0]
		} else {
			if epic, err := bdShow(minecartFromEpic); err == nil {
				name = epic.Title
			} else {
				name = fmt.Sprintf("From epic %s", minecartFromEpic)
			}
		}
	} else {
		// Standard mode: explicit issue list
		if len(args) == 0 {
			return fmt.Errorf("at least one argument is required\nUsage: gt minecart create <name> <issue-id> [issue-id...]\n       gt minecart create --from-epic <epic-id>")
		}
		name = args[0]
		trackedIssues = args[1:]

		// If first arg looks like an issue ID (has beads prefix), treat all args as issues
		// and auto-generate a name from the first issue's title
		if looksLikeIssueID(name) {
			trackedIssues = args
			if details := getIssueDetails(args[0]); details != nil && details.Title != "" {
				name = details.Title
			} else {
				name = fmt.Sprintf("Tracking %s", args[0])
			}
		}

		if len(trackedIssues) == 0 {
			return fmt.Errorf("at least one issue ID is required\nUsage: gt minecart create <name> <issue-id> [issue-id...]")
		}
	}

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Resolve the actual .beads directory (follows redirects) before calling
	// EnsureCustomTypes/Statuses, which expect a .beads path, not a workspace root.
	resolvedBeads := beads.ResolveBeadsDir(townBeads)

	// Ensure custom types (including 'minecart') are registered in town beads.
	// This handles cases where install didn't complete or beads was initialized manually.
	if err := beads.EnsureCustomTypes(resolvedBeads); err != nil {
		return fmt.Errorf("ensuring custom types: %w", err)
	}

	// Ensure custom statuses (staged_ready, staged_warnings) are registered.
	if err := beads.EnsureCustomStatuses(resolvedBeads); err != nil {
		return fmt.Errorf("ensuring custom statuses: %w", err)
	}

	// Create minecart issue in town beads
	description := fmt.Sprintf("Minecart tracking %d issues", len(trackedIssues))

	// Default owner to creator identity if not specified
	owner := minecartOwner
	if owner == "" {
		owner = detectSender()
	}
	minecartFieldValues := &beads.MinecartFields{
		Owner:      owner,
		Notify:     minecartNotify,
		Merge:      minecartMerge,
		Molecule:   minecartMolecule,
		BaseBranch: minecartBaseBranch,
	}
	description = beads.SetMinecartFields(&beads.Issue{Description: description}, minecartFieldValues)

	// Guard against flag-like minecart names (gt-e0kx5)
	if beads.IsFlagLikeTitle(name) {
		return fmt.Errorf("refusing to create minecart: name %q looks like a CLI flag", name)
	}

	// Generate minecart ID with cv- prefix
	minecartID := fmt.Sprintf("hq-cv-%s", generateShortID())

	createArgs := []string{
		"create",
		"--type=task",
		"--id=" + minecartID,
		"--title=" + name,
		"--description=" + description,
		"--labels=" + minecartLabels(minecartOwned),
		"--json",
	}
	if beads.NeedsForceForID(minecartID) {
		createArgs = append(createArgs, "--force")
	}

	var stderr bytes.Buffer
	if err := BdCmd(createArgs...).
		WithAutoCommit().
		Dir(townBeads).
		Stderr(&stderr).
		Run(); err != nil {
		return fmt.Errorf("creating minecart: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	// Notify address is stored in description (line 166-168) and read from there

	// Add 'tracks' relations for each tracked issue
	trackedCount := 0
	for _, issueID := range trackedIssues {
		if err := addTrackingRelationFn(townBeads, minecartID, issueID); err != nil {
			style.PrintWarning("couldn't track %s: %s", issueID, err)
		} else {
			trackedCount++
		}
	}

	// Output
	fmt.Printf("%s Created minecart 🚚 %s\n\n", style.Bold.Render("✓"), minecartID)
	fmt.Printf("  Name:     %s\n", name)
	if minecartFromEpic != "" {
		fmt.Printf("  Epic:     %s\n", minecartFromEpic)
	}
	fmt.Printf("  Tracking: %d issues\n", trackedCount)
	if minecartFromEpic == "" && len(trackedIssues) > 0 {
		fmt.Printf("  Issues:   %s\n", strings.Join(trackedIssues, ", "))
	}
	if owner != "" {
		fmt.Printf("  Owner:    %s\n", owner)
	}
	if minecartNotify != "" {
		fmt.Printf("  Notify:   %s\n", minecartNotify)
	}
	if minecartMerge != "" {
		fmt.Printf("  Merge:    %s\n", minecartMerge)
	}
	if minecartMolecule != "" {
		fmt.Printf("  Molecule: %s\n", minecartMolecule)
	}
	if minecartBaseBranch != "" {
		fmt.Printf("  Base:     %s\n", minecartBaseBranch)
	}
	if minecartOwned {
		fmt.Printf("  Owned:    %s\n", style.Warning.Render("caller-managed lifecycle"))
	}

	if minecartOwned {
		fmt.Printf("\n  %s\n", style.Dim.Render("Owned minecart: caller manages lifecycle via gt minecart land"))
	} else {
		fmt.Printf("\n  %s\n", style.Dim.Render("Minecart auto-closes when all tracked issues complete"))
	}

	return nil
}

func runMinecartAdd(cmd *cobra.Command, args []string) error {
	minecartID := args[0]
	issuesToAdd := args[1:]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// Validate minecart exists and get its status
	showOut, err := BdCmd("show", minecartID, "--json").
		Dir(townBeads).
		Stderr(io.Discard).
		Output()
	if err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Type        string   `json:"issue_type"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(showOut, &minecarts); err != nil {
		return fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	minecart := minecarts[0]

	// Verify it's actually a minecart type
	if !isMinecartIssue(minecart.Type, minecart.Labels) {
		return fmt.Errorf("'%s' is not a minecart (type: %s)", minecartID, minecart.Type)
	}
	if err := ensureKnownMinecartStatus(minecart.Status); err != nil {
		return fmt.Errorf("minecart '%s' has invalid lifecycle state: %w", minecartID, err)
	}

	// If minecart is closed, reopen it
	reopened := false
	if normalizeMinecartStatus(minecart.Status) == minecartStatusClosed {
		// closed→open is always valid; ensureKnownMinecartStatus above guarantees
		// the current status is known, so no additional transition check needed.
		if err := BdCmd("update", minecartID, "--status=open").
			Dir(townBeads).
			WithAutoCommit().
			Run(); err != nil {
			return fmt.Errorf("couldn't reopen minecart: %w", err)
		}
		if fields := beads.ParseMinecartFields(&beads.Issue{Description: minecart.Description}); fields != nil && fields.CompletionNotifiedAt != "" {
			fields.CompletionNotifiedAt = ""
			newDesc := beads.SetMinecartFields(&beads.Issue{Description: minecart.Description}, fields)
			if err := BdCmd("update", minecartID, "--description="+newDesc).
				Dir(townBeads).
				WithAutoCommit().
				Run(); err != nil {
				return fmt.Errorf("couldn't clear minecart completion notification state: %w", err)
			}
		}
		if err := persistTownBeadsJSONL(townBeads); err != nil {
			return fmt.Errorf("couldn't persist reopened minecart to JSONL: %w", err)
		}
		reopened = true
		fmt.Printf("%s Reopened minecart %s\n", style.Bold.Render("↺"), minecartID)
	}

	// Add 'tracks' relations for each issue
	addedCount := 0
	for _, issueID := range issuesToAdd {
		if err := addTrackingRelationFn(townBeads, minecartID, issueID); err != nil {
			style.PrintWarning("couldn't add %s: %s", issueID, err)
		} else {
			addedCount++
		}
	}

	// Output
	if reopened {
		fmt.Println()
	}
	fmt.Printf("%s Added %d issue(s) to minecart 🚚 %s\n", style.Bold.Render("✓"), addedCount, minecartID)
	if addedCount > 0 {
		fmt.Printf("  Issues: %s\n", strings.Join(issuesToAdd[:addedCount], ", "))
	}

	return nil
}

func runMinecartCheck(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// If a specific minecart ID is provided, check only that minecart
	if len(args) == 1 {
		minecartID := args[0]
		return checkSingleMinecart(townBeads, minecartID, minecartCheckDryRun)
	}

	// Check all open minecarts
	closed, err := checkAndCloseCompletedMinecarts(townBeads, minecartCheckDryRun)
	if err != nil {
		return err
	}

	if len(closed) == 0 {
		fmt.Println("No minecarts ready to close.")
	} else {
		if minecartCheckDryRun {
			fmt.Printf("%s Would auto-close %d minecart(s):\n", style.Warning.Render("⚠"), len(closed))
		} else {
			fmt.Printf("%s Auto-closed %d minecart(s):\n", style.Bold.Render("✓"), len(closed))
		}
		for _, c := range closed {
			fmt.Printf("  🚚 %s: %s\n", c.ID, c.Title)
		}
	}

	return nil
}

// closeMinecartIfComplete checks whether all tracked issues in a minecart are resolved
// and closes the minecart if so. Returns (true, nil) if the minecart was closed or
// would be closed (dry-run), (false, nil) if not ready, or (false, err) on failure.
func closeMinecartIfComplete(townBeads, minecartID, title string, tracked []trackedIssueInfo, dryRun bool) (bool, error) {
	// If no tracked issues were resolved, skip auto-close. A 0/0 result means
	// cross-rig tracking resolution failed — not that all issues are done.
	// Treating 0/0 as "complete" caused false 🚚 Minecart landed notifications. (GH#3xxx)
	if len(tracked) == 0 {
		return false, nil
	}

	allClosed := true
	openCount := 0
	unknownCount := 0
	for _, t := range tracked {
		switch t.Status {
		case "closed", "tombstone":
			// counted as complete
		case trackedStatusUnknown:
			// Cross-rig DB unreachable — can't verify completion. Leave minecart
			// open, treat as Info (not a minecart-level failure). (gt-bs6)
			allClosed = false
			unknownCount++
		default:
			allClosed = false
			openCount++
		}
	}

	if !allClosed {
		switch {
		case unknownCount > 0 && openCount > 0:
			fmt.Printf("%s Minecart %s has %d open, %d unknown (cross-rig unreachable) issue(s) remaining\n",
				style.Dim.Render("○"), minecartID, openCount, unknownCount)
		case unknownCount > 0:
			fmt.Printf("%s Minecart %s has %d tracked issue(s) with unknown status (cross-rig unreachable)\n",
				style.Dim.Render("○"), minecartID, unknownCount)
		default:
			fmt.Printf("%s Minecart %s has %d open issue(s) remaining\n", style.Dim.Render("○"), minecartID, openCount)
		}
		return false, nil
	}

	if dryRun {
		fmt.Printf("%s Would auto-close minecart 🚚 %s: %s\n", style.Warning.Render("⚠"), minecartID, title)
		return true, nil
	}

	reason := "All tracked issues completed"
	closeArgs := []string{"close", minecartID, "-r", reason}
	if err := runTownMutationAndExport(townBeads, closeArgs...); err != nil {
		return false, fmt.Errorf("closing minecart: %w", err)
	}

	fmt.Printf("%s Auto-closed minecart 🚚 %s: %s\n", style.Bold.Render("✓"), minecartID, title)
	notifyMinecartCompletion(townBeads, minecartID, title)
	return true, nil
}

// checkSingleMinecart checks a specific minecart and closes it if all tracked issues are complete.
func checkSingleMinecart(townBeads, minecartID string, dryRun bool) error {
	stdout, err := runBdJSON(townBeads, "show", minecartID, "--json")
	if err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Type        string   `json:"issue_type"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout, &minecarts); err != nil {
		return fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	minecart := minecarts[0]

	// Verify it's actually a minecart type
	if !isMinecartIssue(minecart.Type, minecart.Labels) {
		return fmt.Errorf("'%s' is not a minecart (type: %s)", minecartID, minecart.Type)
	}
	if err := ensureKnownMinecartStatus(minecart.Status); err != nil {
		return fmt.Errorf("minecart '%s' has invalid lifecycle state: %w", minecartID, err)
	}

	// Check if minecart is already closed
	if normalizeMinecartStatus(minecart.Status) == minecartStatusClosed {
		fmt.Printf("%s Minecart %s is already closed\n", style.Dim.Render("○"), minecartID)
		return persistAndNotifyMinecartCompletion(townBeads, minecartID, minecart.Title)
	}

	// Get tracked issues
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		return fmt.Errorf("checking minecart %s: %w", minecartID, err)
	}

	_, err = closeMinecartIfComplete(townBeads, minecartID, minecart.Title, tracked, dryRun)
	return err
}

func runMinecartClose(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	stdout, err := runBdJSON(townBeads, "show", minecartID, "--json")
	if err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Type        string   `json:"issue_type"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout, &minecarts); err != nil {
		return fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	minecart := minecarts[0]

	// Verify it's actually a minecart type
	if !isMinecartIssue(minecart.Type, minecart.Labels) {
		return fmt.Errorf("'%s' is not a minecart (type: %s)", minecartID, minecart.Type)
	}
	if err := ensureKnownMinecartStatus(minecart.Status); err != nil {
		return fmt.Errorf("minecart '%s' has invalid lifecycle state: %w", minecartID, err)
	}

	// Idempotent: if already closed, just report it
	if normalizeMinecartStatus(minecart.Status) == minecartStatusClosed {
		fmt.Printf("%s Minecart %s is already closed\n", style.Dim.Render("○"), minecartID)
		return persistAndNotifyMinecartCompletion(townBeads, minecartID, minecart.Title)
	}
	if err := validateMinecartStatusTransition(minecart.Status, minecartStatusClosed); err != nil {
		return fmt.Errorf("can't close minecart '%s': %w", minecartID, err)
	}

	// Verify all tracked issues are done (unless --force)
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		// If we can't check tracked issues, require --force
		if !minecartCloseForce {
			return fmt.Errorf("couldn't verify tracked issues: %w\n  Use --force to close anyway", err)
		}
		style.PrintWarning("couldn't verify tracked issues: %v", err)
	}

	if len(tracked) > 0 && !minecartCloseForce {
		var openIssues []trackedIssueInfo
		for _, t := range tracked {
			if t.Status != "closed" && t.Status != "tombstone" {
				openIssues = append(openIssues, t)
			}
		}

		if len(openIssues) > 0 {
			fmt.Printf("%s Minecart %s has %d open issue(s):\n\n", style.Warning.Render("⚠"), minecartID, len(openIssues))
			for _, t := range openIssues {
				status := "○"
				if t.Status == "in_progress" || t.Status == "hooked" {
					status = "▶"
				}
				fmt.Printf("    %s %s: %s [%s]\n", status, t.ID, t.Title, t.Status)
			}
			fmt.Printf("\n  Use %s to close anyway.\n", style.Bold.Render("--force"))
			return fmt.Errorf("minecart has %d open issue(s)", len(openIssues))
		}
	}

	// Build close reason
	reason := minecartCloseReason
	if reason == "" {
		if minecartCloseForce {
			reason = "Force closed"
		} else {
			reason = "All tracked issues completed"
		}
	}

	// Close the minecart
	closeArgs := []string{"close", minecartID, "-r", reason}
	if err := runTownMutationAndExport(townBeads, closeArgs...); err != nil {
		return fmt.Errorf("closing minecart: %w", err)
	}

	fmt.Printf("%s Closed minecart 🚚 %s: %s\n", style.Bold.Render("✓"), minecartID, minecart.Title)
	if minecartCloseReason != "" {
		fmt.Printf("  Reason: %s\n", minecartCloseReason)
	}

	// Report cleanup summary
	if len(tracked) > 0 {
		closedCount := 0
		openCount := 0
		for _, t := range tracked {
			if t.Status == "closed" || t.Status == "tombstone" {
				closedCount++
			} else {
				openCount++
			}
		}
		fmt.Printf("  Tracked: %d issue(s) (%d closed", len(tracked), closedCount)
		if openCount > 0 {
			fmt.Printf(", %d still open", openCount)
		}
		fmt.Println(")")
	}

	// Report molecule if present
	minecartFields := beads.ParseMinecartFields(&beads.Issue{Description: minecart.Description})
	if minecartFields != nil && minecartFields.Molecule != "" {
		fmt.Printf("  Molecule: %s (not auto-detached)\n", minecartFields.Molecule)
	}

	// Send notification if --notify flag provided
	if minecartCloseNotify != "" {
		sendCloseNotification(minecartCloseNotify, minecartID, minecart.Title, reason)
	} else {
		// Check if minecart has a notify address in description
		notifyMinecartCompletion(townBeads, minecartID, minecart.Title)
	}

	return nil
}

// sendCloseNotification sends a notification about minecart closure.
func sendCloseNotification(addr, minecartID, title, reason string) {
	subject := fmt.Sprintf("🚚 Minecart closed: %s", title)
	body := fmt.Sprintf("Minecart %s has been closed.\n\nReason: %s", minecartID, reason)

	mailArgs := []string{"mail", "send", addr, "-s", subject, "-m", body}
	mailCmd := exec.Command("gt", mailArgs...)
	if err := mailCmd.Run(); err != nil {
		style.PrintWarning("couldn't send notification: %v", err)
	} else {
		fmt.Printf("  Notified: %s\n", addr)
	}
}

func runMinecartLand(cmd *cobra.Command, args []string) error {
	minecartID := args[0]

	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	stdout, err := runBdJSON(townBeads, "show", minecartID, "--json")
	if err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Type        string   `json:"issue_type"`
		Description string   `json:"description"`
		Labels      []string `json:"labels,omitempty"`
	}
	if err := json.Unmarshal(stdout, &minecarts); err != nil {
		return fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	minecart := minecarts[0]

	// Verify it's a minecart type
	if !isMinecartIssue(minecart.Type, minecart.Labels) {
		return fmt.Errorf("'%s' is not a minecart (type: %s)", minecartID, minecart.Type)
	}

	// Verify the minecart is owned
	if !hasLabel(minecart.Labels, "gt:owned") {
		return fmt.Errorf("minecart '%s' is not an owned minecart\n  Only minecarts created with --owned can be landed.\n  Use %s instead for non-owned minecarts.",
			minecartID, style.Bold.Render("gt minecart close"))
	}

	// Check if already closed
	if err := ensureKnownMinecartStatus(minecart.Status); err != nil {
		return fmt.Errorf("minecart '%s' has invalid lifecycle state: %w", minecartID, err)
	}
	if normalizeMinecartStatus(minecart.Status) == minecartStatusClosed {
		fmt.Printf("%s Minecart %s is already closed\n", style.Dim.Render("○"), minecartID)
		return persistAndNotifyMinecartCompletion(townBeads, minecartID, minecart.Title)
	}

	// Get tracked issues
	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		if !minecartLandForce {
			return fmt.Errorf("couldn't verify tracked issues: %w\n  Use --force to land anyway", err)
		}
		style.PrintWarning("couldn't verify tracked issues: %v", err)
	}

	// Check if all tracked issues are done
	var openIssues []trackedIssueInfo
	for _, t := range tracked {
		if t.Status != "closed" && t.Status != "tombstone" {
			openIssues = append(openIssues, t)
		}
	}

	if len(openIssues) > 0 && !minecartLandForce {
		fmt.Printf("%s Minecart %s has %d open issue(s):\n\n", style.Warning.Render("⚠"), minecartID, len(openIssues))
		for _, t := range openIssues {
			status := "○"
			if t.Status == "in_progress" || t.Status == "hooked" {
				status = "▶"
			}
			fmt.Printf("    %s %s: %s [%s]\n", status, t.ID, t.Title, t.Status)
		}
		fmt.Printf("\n  Use %s to land anyway.\n", style.Bold.Render("--force"))
		return fmt.Errorf("minecart has %d open issue(s)", len(openIssues))
	}

	if minecartLandDryRun {
		fmt.Printf("%s Dry run — would land minecart 🚚 %s: %s\n\n", style.Warning.Render("⚠"), minecartID, minecart.Title)
		fmt.Printf("  Tracked: %d issue(s) (%d closed, %d open)\n", len(tracked), len(tracked)-len(openIssues), len(openIssues))
		if !minecartLandKeep {
			worktrees := findMinecartWorktrees(tracked)
			fmt.Printf("  Worktrees to clean: %d\n", len(worktrees))
			for _, wt := range worktrees {
				fmt.Printf("    • %s (%s)\n", wt.minerName, wt.rigName)
			}
		} else {
			fmt.Printf("  Worktrees: skipped (--keep-worktrees)\n")
		}
		fmt.Printf("  Close reason: Landed by owner\n")
		return nil
	}

	// Phase 1: Clean up miner worktrees
	if !minecartLandKeep {
		worktrees := findMinecartWorktrees(tracked)
		if len(worktrees) > 0 {
			fmt.Printf("  Cleaning up %d worktree(s)...\n", len(worktrees))
			for _, wt := range worktrees {
				if err := removeMinerWorktree(wt); err != nil {
					style.PrintWarning("couldn't remove worktree %s/%s: %v", wt.rigName, wt.minerName, err)
				} else {
					fmt.Printf("    %s %s/%s\n", style.Dim.Render("✓"), wt.rigName, wt.minerName)
				}
			}
		}
	}

	// Phase 2: Close the minecart
	reason := "Landed by owner"
	closeArgs := []string{"close", minecartID, "-r", reason}
	if err := runTownMutationAndExport(townBeads, closeArgs...); err != nil {
		return fmt.Errorf("closing minecart: %w", err)
	}

	fmt.Printf("\n%s Landed minecart 🚚 %s: %s\n", style.Bold.Render("✓"), minecartID, minecart.Title)
	fmt.Printf("  Reason: %s\n", reason)
	if len(tracked) > 0 {
		closedCount := len(tracked) - len(openIssues)
		fmt.Printf("  Tracked: %d issue(s) (%d closed", len(tracked), closedCount)
		if len(openIssues) > 0 {
			fmt.Printf(", %d still open", len(openIssues))
		}
		fmt.Println(")")
	}

	// Phase 3: Send completion notifications
	notifyMinecartCompletion(townBeads, minecartID, minecart.Title)

	return nil
}

// minecartWorktreeInfo holds info about a miner worktree to clean up.
type minecartWorktreeInfo struct {
	rigName     string // e.g., "mineshaft"
	minerName string // e.g., "rictus"
	townRoot    string // workspace root
}

// findMinecartWorktrees discovers miner worktrees associated with a minecart's tracked issues.
// It matches tracked issue assignees to miner worktrees across all rigs.
func findMinecartWorktrees(tracked []trackedIssueInfo) []minecartWorktreeInfo {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return nil
	}

	rigsConfigPath := filepath.Join(townRoot, "overseer", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil
	}

	// Collect all assignees from tracked issues
	assignees := make(map[string]bool)
	for _, t := range tracked {
		if t.Assignee != "" {
			assignees[t.Assignee] = true
		}
	}

	if len(assignees) == 0 {
		return nil
	}

	var worktrees []minecartWorktreeInfo

	for rigName := range rigsConfig.Rigs {
		rigPath := filepath.Join(townRoot, rigName)
		minersDir := filepath.Join(rigPath, "miners")

		entries, err := os.ReadDir(minersDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			// Check if this miner's assignee matches any tracked issue assignee
			// Assignees have format: rig/miners/name
			minerAssignee := fmt.Sprintf("%s/miners/%s", rigName, entry.Name())
			if assignees[minerAssignee] {
				worktrees = append(worktrees, minecartWorktreeInfo{
					rigName:     rigName,
					minerName: entry.Name(),
					townRoot:    townRoot,
				})
			}
		}
	}

	return worktrees
}

// removeMinerWorktree removes a miner worktree via gt miner remove.
func removeMinerWorktree(wt minecartWorktreeInfo) error {
	// gt miner remove accepts rig/miner format
	target := fmt.Sprintf("%s/%s", wt.rigName, wt.minerName)
	cmd := exec.Command("gt", "miner", "remove", target, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}
	return nil
}

// strandedMinecartInfo holds info about a stranded minecart.
type strandedMinecartInfo struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	TrackedCount int      `json:"tracked_count"`
	ReadyCount   int      `json:"ready_count"`
	ReadyIssues  []string `json:"ready_issues"`
	CreatedAt    string   `json:"created_at,omitempty"`
	BaseBranch   string   `json:"base_branch,omitempty"`
}

// readyIssueInfo holds info about a ready (stranded) issue.
type readyIssueInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority string `json:"priority"`
}

func runMinecartStranded(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	stranded, err := findStrandedMinecarts(townBeads)
	if err != nil {
		return err
	}

	if minecartStrandedJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stranded)
	}

	if len(stranded) == 0 {
		fmt.Println("No stranded minecarts found.")
		return nil
	}

	fmt.Printf("%s Found %d stranded minecart(s):\n\n", style.Warning.Render("⚠"), len(stranded))
	for _, s := range stranded {
		fmt.Printf("  🚚 %s: %s\n", s.ID, s.Title)
		if s.ReadyCount == 0 && s.TrackedCount == 0 {
			fmt.Printf("     Empty minecart (0 tracked issues) — needs cleanup\n")
		} else if s.ReadyCount == 0 && s.TrackedCount > 0 {
			fmt.Printf("     %d tracked issues, 0 ready — needs agent review\n", s.TrackedCount)
		} else {
			fmt.Printf("     Ready issues: %d (of %d tracked)\n", s.ReadyCount, s.TrackedCount)
			for _, issueID := range s.ReadyIssues {
				fmt.Printf("       • %s\n", issueID)
			}
		}
		fmt.Println()
	}

	// Separate feed advice, needs-attention minecarts, and cleanup advice.
	var feedable, needsAttention, empty []strandedMinecartInfo
	for _, s := range stranded {
		if s.ReadyCount > 0 {
			feedable = append(feedable, s)
		} else if s.TrackedCount > 0 {
			needsAttention = append(needsAttention, s)
		} else {
			empty = append(empty, s)
		}
	}

	if len(feedable) > 0 {
		fmt.Println("To feed stranded minecarts, run:")
		for _, s := range feedable {
			fmt.Printf("  gt sling mol-minecart-feed supervisor/dogs --var minecart=%s\n", s.ID)
		}
	}
	if len(needsAttention) > 0 {
		if len(feedable) > 0 {
			fmt.Println()
		}
		fmt.Println("Needs agent review (tracked issues exist but none are ready):")
		for _, s := range needsAttention {
			fmt.Printf("  🚚 %s (%d tracked, 0 ready)\n", s.ID, s.TrackedCount)
		}
	}
	if len(empty) > 0 {
		if len(feedable) > 0 || len(needsAttention) > 0 {
			fmt.Println()
		}
		fmt.Println("To close empty minecarts, run:")
		for _, s := range empty {
			fmt.Printf("  gt minecart check %s\n", s.ID)
		}
	}
	fmt.Println()
	fmt.Println(style.Dim.Render("  Note: Pool dispatch auto-creates dogs if pool is under capacity."))

	return nil
}

// findStrandedMinecarts finds minecarts with ready work but no workers,
// or empty minecarts (0 tracked issues) that need cleanup.
func findStrandedMinecarts(townBeads string) ([]strandedMinecartInfo, error) {
	stranded := []strandedMinecartInfo{} // Initialize as empty slice for proper JSON encoding

	minecarts, err := listMinecartIssues(townBeads, "open", false)
	if err != nil {
		return nil, fmt.Errorf("listing minecarts: %w", err)
	}

	// Check each minecart for stranded state
	for _, minecart := range minecarts {
		// Extract base_branch from minecart description fields
		var baseBranch string
		if cf := beads.ParseMinecartFields(&beads.Issue{Description: minecart.Description}); cf != nil {
			baseBranch = cf.BaseBranch
		}

		tracked, err := getTrackedIssues(townBeads, minecart.ID)
		if err != nil {
			// Write to stderr explicitly — stdout may be consumed as JSON
			// by the daemon's JSON parser (fixes #2142).
			fmt.Fprintf(os.Stderr, "⚠ Warning: skipping minecart %s: %v\n", minecart.ID, err)
			continue
		}
		// Empty minecarts (0 tracked issues) are stranded — they need
		// attention (auto-close via minecart check or manual cleanup).
		if len(tracked) == 0 {
			stranded = append(stranded, strandedMinecartInfo{
				ID:           minecart.ID,
				Title:        minecart.Title,
				TrackedCount: 0,
				ReadyCount:   0,
				ReadyIssues:  []string{},
				CreatedAt:    minecart.CreatedAt,
				BaseBranch:   baseBranch,
			})
			continue
		}

		// Find ready issues (open, not blocked, no live assignee, slingable).
		// Town-level beads (hq- prefix with path=".") are excluded because
		// they can't be dispatched via gt sling -- they're handled by the supervisor.
		// Non-slingable types (epics, minecarts, etc.) are also excluded.

		// Batch-check scheduling status for all tracked issues (single DB query).
		var trackedIDs []string
		for _, t := range tracked {
			trackedIDs = append(trackedIDs, t.ID)
		}
		scheduledSet := areScheduled(trackedIDs)

		var readyIssues []string
		for _, t := range tracked {
			if isReadyIssue(t, scheduledSet) {
				if !isSlingableBead(townBeads, t.ID) {
					continue
				}
				if !minecartops.IsSlingableType(t.IssueType) {
					continue
				}
				readyIssues = append(readyIssues, t.ID)
			}
		}

		if len(readyIssues) > 0 {
			stranded = append(stranded, strandedMinecartInfo{
				ID:           minecart.ID,
				Title:        minecart.Title,
				TrackedCount: len(tracked),
				ReadyCount:   len(readyIssues),
				ReadyIssues:  readyIssues,
				CreatedAt:    minecart.CreatedAt,
				BaseBranch:   baseBranch,
			})
		} else {
			// Has tracked issues but none are ready — include in stranded
			// list so callers can distinguish from truly empty minecarts.
			stranded = append(stranded, strandedMinecartInfo{
				ID:           minecart.ID,
				Title:        minecart.Title,
				TrackedCount: len(tracked),
				ReadyCount:   0,
				ReadyIssues:  []string{},
				CreatedAt:    minecart.CreatedAt,
				BaseBranch:   baseBranch,
			})
		}
	}

	return stranded, nil
}

// isReadyIssue checks if an issue is ready for dispatch (stranded).
// An issue is ready if:
// - status = "open" AND (no assignee OR assignee session is dead)
// - OR status = "in_progress"/"hooked" AND assignee session is dead (orphaned molecule)
// - AND not blocked (cross-rig-aware from issue details)
// scheduledSet is a pre-computed set of bead IDs with open sling contexts (from areScheduled).
func isReadyIssue(t trackedIssueInfo, scheduledSet map[string]bool) bool {
	status := strings.TrimSpace(t.Status)

	// Unresolved issues are not safe to dispatch.
	if status == "" || status == trackedStatusUnknown {
		return false
	}

	// Closed issues are never ready
	if status == "closed" || status == "tombstone" {
		return false
	}

	// Must not be blocked
	if t.Blocked {
		return false
	}

	// Scheduled beads are not stranded — they're waiting for dispatch capacity.
	if scheduledSet[t.ID] {
		return false
	}

	// Open issues with no assignee are trivially ready
	if status == "open" && t.Assignee == "" {
		return true
	}

	// For issues with an assignee (or non-open status with molecule attached),
	// check if the worker session is still alive
	if t.Assignee == "" {
		// Non-open status but no assignee is an edge case (shouldn't happen
		// normally, but could occur if molecule detached improperly)
		return true
	}

	// Has assignee - check if session is alive
	// Use the shared assigneeToSessionName from rig.go
	sessionName, _ := assigneeToSessionName(t.Assignee)
	if sessionName == "" {
		return true // Can't determine session = treat as ready
	}

	// Check if tmux session exists
	checkCmd := tmux.BuildCommand("has-session", "-t", sessionName)
	if err := checkCmd.Run(); err != nil {
		// Session doesn't exist = orphaned molecule or dead worker
		// This is the key fix: issues with in_progress/hooked status but
		// dead workers are now correctly detected as stranded
		return true
	}

	return false // Session exists = worker is active
}

// isSlingableBead reports whether a bead can be dispatched via gt sling.
// Town-level beads (hq- prefix with path=".") and beads with unknown
// prefixes are not slingable — they're handled by the supervisor/overseer.
func isSlingableBead(townRoot, beadID string) bool {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return true // No prefix info, assume slingable
	}
	return beads.GetRigNameForPrefix(townRoot, prefix) != ""
}

// checkAndCloseCompletedMinecarts finds open minecarts where all tracked issues are closed
// and auto-closes them. Returns the list of minecarts that were closed (or would be closed in dry-run mode).
// If dryRun is true, no changes are made and the function returns what would have been closed.
func checkAndCloseCompletedMinecarts(townBeads string, dryRun bool) ([]struct{ ID, Title string }, error) {
	var closed []struct{ ID, Title string }

	minecarts, err := listMinecartIssues(townBeads, "open", false)
	if err != nil {
		return nil, fmt.Errorf("listing minecarts: %w", err)
	}

	// Check each minecart
	for _, minecart := range minecarts {
		if err := ensureKnownMinecartStatus(minecart.Status); err != nil {
			style.PrintWarning("skipping minecart %s: invalid lifecycle state: %v", minecart.ID, err)
			continue
		}
		tracked, err := getTrackedIssues(townBeads, minecart.ID)
		if err != nil {
			style.PrintWarning("skipping minecart %s: %v", minecart.ID, err)
			continue
		}
		ready, err := closeMinecartIfComplete(townBeads, minecart.ID, minecart.Title, tracked, dryRun)
		if err != nil {
			style.PrintWarning("couldn't close minecart %s: %v", minecart.ID, err)
			continue
		}
		if ready {
			closed = append(closed, struct{ ID, Title string }{minecart.ID, minecart.Title})
		}
	}

	return closed, nil
}

// persistTownBeadsJSONL writes the current town Beads state to the JSONL file
// used by bd's fallback import path. Minecart close/check suppresses bd's implicit
// auto-export for normal command hygiene, but close state must survive a later
// Dolt rebuild from .beads/issues.jsonl.
func persistTownBeadsJSONL(townBeads string) error {
	beadsDir := beads.ResolveBeadsDir(townBeads)
	if beadsDir == "" {
		return fmt.Errorf("could not resolve town .beads directory")
	}
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	return BdCmd("export", "-o", issuesPath).Dir(townBeads).Run()
}

func runTownMutationAndExport(townBeads string, args ...string) error {
	if err := BdCmd(args...).Dir(townBeads).WithAutoCommit().Run(); err != nil {
		return err
	}
	return persistTownBeadsJSONL(townBeads)
}

func persistAndNotifyMinecartCompletion(townBeads, minecartID, title string) error {
	if err := persistTownBeadsJSONL(townBeads); err != nil {
		return fmt.Errorf("persisting minecart close to JSONL: %w", err)
	}
	notifyMinecartCompletion(townBeads, minecartID, title)
	return nil
}

// notifyMinecartCompletion sends notifications to owner, any notify addresses, and overseer/.
func notifyMinecartCompletion(townBeads, minecartID, title string) {
	stdout, err := runBdJSON(townBeads, "show", minecartID, "--json")
	if err != nil {
		return
	}

	var minecarts []struct {
		Description string `json:"description"`
		CreatedAt   string `json:"created_at"`
	}
	if err := json.Unmarshal(stdout, &minecarts); err != nil || len(minecarts) == 0 {
		return
	}

	// ZFC: Use typed accessor instead of parsing description text
	fields := beads.ParseMinecartFields(&beads.Issue{Description: minecarts[0].Description})
	if fields == nil {
		fields = &beads.MinecartFields{}
	}
	if fields.CompletionNotifiedAt != "" {
		return
	}

	// Compute duration since minecart was created.
	var durationStr string
	if t, err := time.Parse(time.RFC3339, minecarts[0].CreatedAt); err == nil {
		d := time.Since(t).Round(time.Minute)
		durationStr = formatWorkerAge(d)
	}

	// Count tracked issues (best-effort; 0 on error is fine for display).
	trackedIDs, _ := bdDepListRawIDs(townBeads, minecartID, "down", "tracks")
	issueCount := len(trackedIDs)

	// Build enriched body for overseer notification.
	overseerBody := fmt.Sprintf("Minecart %s has completed. All tracked issues are now closed.", minecartID)
	if issueCount > 0 || durationStr != "" {
		overseerBody += "\n"
		if issueCount > 0 {
			overseerBody += fmt.Sprintf("\nIssues: %d", issueCount)
		}
		if durationStr != "" {
			overseerBody += fmt.Sprintf("\nDuration: %s", durationStr)
		}
	}

	// Track notified addresses to avoid duplicate overseer/ notification.
	notifiedAddrs := make(map[string]bool)

	for _, addr := range fields.NotificationAddresses() {
		notifiedAddrs[addr] = true
		mailArgs := []string{"mail", "send", addr,
			"-s", fmt.Sprintf("🚚 Minecart landed: %s", title),
			"-m", fmt.Sprintf("Minecart %s has completed.\n\nAll tracked issues are now closed.", minecartID)}
		mailCmd := exec.Command("gt", mailArgs...)
		if err := mailCmd.Run(); err != nil {
			style.PrintWarning("could not notify %s: %v", addr, err)
		}
	}

	// Send nudge notifications to nudge watchers.
	for _, addr := range fields.NudgeNotificationAddresses() {
		nudgeMsg := fmt.Sprintf("🚚 Minecart landed: %s — Minecart %s has completed. All tracked issues are now closed.", title, minecartID)
		nudgeCmd := exec.Command("gt", "nudge", addr, "-m", nudgeMsg)
		if err := nudgeCmd.Run(); err != nil {
			style.PrintWarning("could not nudge %s: %v", addr, err)
		}
	}

	// Always notify overseer/ for strategic visibility, unless already notified above.
	if !notifiedAddrs["overseer/"] {
		mailArgs := []string{"mail", "send", "overseer/",
			"-s", fmt.Sprintf("Minecart complete: %s", title),
			"-m", overseerBody}
		mailCmd := exec.Command("gt", mailArgs...)
		if err := mailCmd.Run(); err != nil {
			style.PrintWarning("could not notify overseer/ of minecart completion: %v", err)
		}
	}

	// Push notification to active Overseer session if configured.
	notifyOverseerSession(townBeads, minecartID, title)

	fields.CompletionNotifiedAt = time.Now().UTC().Format(time.RFC3339)
	newDesc := beads.SetMinecartFields(&beads.Issue{Description: minecarts[0].Description}, fields)
	if err := runTownMutationAndExport(townBeads, "update", minecartID, "--description="+newDesc); err != nil {
		style.PrintWarning("could not record minecart completion notification state for %s: %v", minecartID, err)
		return
	}
}

// notifyOverseerSession pushes a minecart completion notification into the active
// Overseer session via nudge, if minecart.notify_on_complete is enabled.
func notifyOverseerSession(townBeads, minecartID, title string) {
	settingsPath := config.TownSettingsPath(townBeads)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return
	}
	if settings.Minecart == nil || !settings.Minecart.NotifyOnComplete {
		return
	}

	nudgeMsg := fmt.Sprintf("🚚 Minecart landed: %s — Minecart %s has completed. All tracked issues are now closed.", title, minecartID)
	nudgeCmd := exec.Command("gt", "nudge", "overseer", "-m", nudgeMsg)
	if err := nudgeCmd.Run(); err != nil {
		style.PrintWarning("could not nudge Overseer session: %v", err)
	}
}

func runMinecartStatus(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	// If no ID provided, show all active minecarts
	if len(args) == 0 {
		return showAllMinecartStatus(townBeads)
	}

	minecartID := args[0]

	// Check if it's a numeric shortcut (e.g., "1" instead of "hq-cv-xyz")
	if n, err := strconv.Atoi(minecartID); err == nil && n > 0 {
		resolved, err := resolveMinecartNumber(townBeads, n)
		if err != nil {
			return err
		}
		minecartID = resolved
	}

	// Get minecart details
	showOut, err := runBdJSON(townBeads, "show", minecartID, "--json")
	if err != nil {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	// Parse minecart data
	var minecarts []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		Description string   `json:"description"`
		CreatedAt   string   `json:"created_at"`
		ClosedAt    string   `json:"closed_at,omitempty"`
		DependsOn   []string `json:"depends_on,omitempty"`
		Labels      []string `json:"labels,omitempty"`
	}
	if err := json.Unmarshal(showOut, &minecarts); err != nil {
		return fmt.Errorf("parsing minecart data: %w", err)
	}

	if len(minecarts) == 0 {
		return fmt.Errorf("minecart '%s' not found", minecartID)
	}

	minecart := minecarts[0]

	// Check if minecart is owned (caller-managed lifecycle)
	isOwned := hasLabel(minecart.Labels, "gt:owned")

	tracked, err := getTrackedIssues(townBeads, minecartID)
	if err != nil {
		return fmt.Errorf("getting tracked issues for %s: %w", minecartID, err)
	}

	// Count completed
	completed := 0
	for _, t := range tracked {
		if t.Status == "closed" {
			completed++
		}
	}

	if minecartStatusJSON {
		lifecycle := "system-managed"
		if isOwned {
			lifecycle = "caller-managed"
		}
		type jsonStatus struct {
			ID            string             `json:"id"`
			Title         string             `json:"title"`
			Status        string             `json:"status"`
			Owned         bool               `json:"owned"`
			Lifecycle     string             `json:"lifecycle"`
			MergeStrategy string             `json:"merge_strategy,omitempty"`
			Tracked       []trackedIssueInfo `json:"tracked"`
			Completed     int                `json:"completed"`
			Total         int                `json:"total"`
		}
		out := jsonStatus{
			ID:            minecart.ID,
			Title:         minecart.Title,
			Status:        minecart.Status,
			Owned:         isOwned,
			Lifecycle:     lifecycle,
			MergeStrategy: minecartMergeFromFields(minecart.Description),
			Tracked:       tracked,
			Completed:     completed,
			Total:         len(tracked),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Human-readable output
	fmt.Printf("🚚 %s %s\n\n", style.Bold.Render(minecart.ID+":"), minecart.Title)
	fmt.Printf("  Status:    %s\n", formatMinecartStatus(minecart.Status))
	fmt.Printf("  Owned:     %s\n", formatYesNo(isOwned))
	if isOwned {
		fmt.Printf("  Lifecycle: %s\n", style.Warning.Render("caller-managed"))
	} else {
		fmt.Printf("  Lifecycle: %s\n", "system-managed")
	}
	merge := minecartMergeFromFields(minecart.Description)
	if merge != "" {
		fmt.Printf("  Merge:     %s\n", merge)
	}
	fmt.Printf("  Progress:  %d/%d completed\n", completed, len(tracked))
	fmt.Printf("  Created:   %s\n", minecart.CreatedAt)
	if minecart.ClosedAt != "" {
		fmt.Printf("  Closed:    %s\n", minecart.ClosedAt)
	}

	if len(tracked) > 0 {
		fmt.Printf("\n  %s\n", style.Bold.Render("Tracked Issues:"))
		for _, t := range tracked {
			// Status symbol: ✓ closed, ▶ in_progress/hooked, ? unknown (cross-rig unreachable), ○ other
			status := "○"
			switch t.Status {
			case "closed":
				status = "✓"
			case "in_progress", "hooked":
				status = "▶"
			case trackedStatusUnknown:
				status = "?"
			}

			// Show assignee in brackets (extract short name from path like mineshaft/miners/goose -> goose)
			bracketContent := t.IssueType
			if t.Assignee != "" {
				parts := strings.Split(t.Assignee, "/")
				bracketContent = parts[len(parts)-1] // Last part of path
			} else if bracketContent == "" {
				bracketContent = "unassigned"
			}

			line := fmt.Sprintf("    %s %s: %s [%s]", status, t.ID, t.Title, bracketContent)
			if t.Worker != "" {
				workerDisplay := "@" + t.Worker
				if t.WorkerAge != "" {
					workerDisplay += fmt.Sprintf(" (%s)", t.WorkerAge)
				}
				line += fmt.Sprintf("  %s", style.Dim.Render(workerDisplay))
			}
			fmt.Println(line)
		}
	}

	// Hint for owned minecarts when all issues are complete
	if isOwned && completed == len(tracked) && len(tracked) > 0 && normalizeMinecartStatus(minecart.Status) == minecartStatusOpen {
		fmt.Printf("\n  %s\n", style.Dim.Render("All issues complete. Land with: gt minecart land "+minecartID))
	}

	return nil
}

func showAllMinecartStatus(townBeads string) error {
	minecarts, err := listMinecartIssues(townBeads, "open", false)
	if err != nil {
		return fmt.Errorf("listing minecarts: %w", err)
	}

	if len(minecarts) == 0 {
		fmt.Println("No active minecarts.")
		fmt.Println("Create a minecart with: gt minecart create <name> [issues...]")
		return nil
	}

	if minecartStatusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(minecarts)
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Active Minecarts"))
	for _, c := range minecarts {
		ownedTag := ""
		if hasLabel(c.Labels, "gt:owned") {
			ownedTag = " " + style.Warning.Render("[owned]")
		}
		fmt.Printf("  🚚 %s: %s%s\n", c.ID, c.Title, ownedTag)
	}
	fmt.Printf("\nUse 'gt minecart status <id>' for detailed status.\n")

	return nil
}

func runMinecartList(cmd *cobra.Command, args []string) error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	minecarts, err := listMinecartIssues(townBeads, minecartListStatus, minecartListAll)
	if err != nil {
		return fmt.Errorf("listing minecarts: %w", err)
	}

	if minecartListJSON {
		// Enrich each minecart with tracked issues and completion counts
		type minecartListEntry struct {
			ID        string             `json:"id"`
			Title     string             `json:"title"`
			Status    string             `json:"status"`
			CreatedAt string             `json:"created_at"`
			Tracked   []trackedIssueInfo `json:"tracked"`
			Completed int                `json:"completed"`
			Total     int                `json:"total"`
		}
		enriched := make([]minecartListEntry, 0, len(minecarts))
		for _, c := range minecarts {
			tracked, err := getTrackedIssues(townBeads, c.ID)
			if err != nil {
				style.PrintWarning("skipping minecart %s: %v", c.ID, err)
				continue
			}
			if tracked == nil {
				tracked = []trackedIssueInfo{} // Ensure JSON [] not null
			}
			completed := 0
			for _, t := range tracked {
				if t.Status == "closed" {
					completed++
				}
			}
			enriched = append(enriched, minecartListEntry{
				ID:        c.ID,
				Title:     c.Title,
				Status:    c.Status,
				CreatedAt: c.CreatedAt,
				Tracked:   tracked,
				Completed: completed,
				Total:     len(tracked),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(enriched)
	}

	if len(minecarts) == 0 {
		fmt.Println("No minecarts found.")
		fmt.Println("Create a minecart with: gt minecart create <name> [issues...]")
		return nil
	}

	// Tree view: show minecarts with their child issues
	if minecartListTree {
		return printMinecartTree(townBeads, minecarts)
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Minecarts"))
	for i, c := range minecarts {
		status := formatMinecartStatus(c.Status)
		ownedTag := ""
		if hasLabel(c.Labels, "gt:owned") {
			ownedTag = " " + style.Warning.Render("[owned]")
		}
		fmt.Printf("  %d. 🚚 %s: %s %s%s\n", i+1, c.ID, c.Title, status, ownedTag)
	}
	fmt.Printf("\nUse 'gt minecart status <id>' or 'gt minecart status <n>' for detailed view.\n")

	return nil
}

// printMinecartTree displays minecarts with their child issues in a tree format.
func printMinecartTree(townBeads string, minecarts []minecartListIssue) error {
	for _, c := range minecarts {
		// Get tracked issues for this minecart
		tracked, err := getTrackedIssues(townBeads, c.ID)
		if err != nil {
			style.PrintWarning("skipping minecart %s: %v", c.ID, err)
			continue
		}

		// Count completed
		completed := 0
		for _, t := range tracked {
			if t.Status == "closed" {
				completed++
			}
		}

		// Print minecart header with progress
		total := len(tracked)
		progress := ""
		if total > 0 {
			progress = fmt.Sprintf(" (%d/%d)", completed, total)
		}
		ownedTag := ""
		if hasLabel(c.Labels, "gt:owned") {
			ownedTag = " " + style.Warning.Render("[owned]")
		}
		fmt.Printf("🚚 %s: %s%s%s\n", c.ID, c.Title, progress, ownedTag)

		// Print tracked issues as tree children
		for i, t := range tracked {
			// Determine tree connector
			isLast := i == len(tracked)-1
			connector := "├──"
			if isLast {
				connector = "└──"
			}

			// Status symbol: ✓ closed, ▶ in_progress/hooked, ○ other
			status := "○"
			switch t.Status {
			case "closed":
				status = "✓"
			case "in_progress", "hooked":
				status = "▶"
			}

			fmt.Printf("%s %s %s: %s\n", connector, status, t.ID, t.Title)
		}

		// Add blank line between minecarts
		fmt.Println()
	}

	return nil
}

// hasLabel checks if a label exists in a list of labels.
func hasLabel(labels []string, target string) bool { //nolint:unparam // target is always "gt:owned" today but the API is intentionally general
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

type minecartListIssue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"created_at"`
	Description string   `json:"description"`
	IssueType   string   `json:"issue_type"`
	Labels      []string `json:"labels"`
}

func isMinecartIssue(issueType string, labels []string) bool {
	return issueType == "minecart" || hasLabel(labels, "gt:minecart")
}

func minecartLabels(owned bool) string {
	if owned {
		return "gt:minecart,gt:owned"
	}
	return "gt:minecart"
}

func listMinecartIssues(townBeads, status string, all bool, extraLabels ...string) ([]minecartListIssue, error) {
	args := []string{"list", "--label=gt:minecart", "--json", "--limit=0"}
	for _, label := range extraLabels {
		args = append(args, "--label="+label)
	}
	if status != "" {
		args = append(args, "--status="+status)
	} else if all {
		args = append(args, "--all")
	}

	args = beads.InjectFlatForListJSON(args)
	minecarts, err := readMinecartIssues(townBeads, args...)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(minecarts))
	for _, minecart := range minecarts {
		seen[minecart.ID] = true
	}

	legacyArgs := []string{"list", "--json", "--limit=0"}
	if status != "" {
		legacyArgs = append(legacyArgs, "--status="+status)
	} else if all {
		legacyArgs = append(legacyArgs, "--all")
	}
	legacyArgs = beads.InjectFlatForListJSON(legacyArgs)
	legacy, err := readMinecartIssues(townBeads, legacyArgs...)
	if err != nil {
		return nil, err
	}
	for _, issue := range legacy {
		if seen[issue.ID] || issue.IssueType != "minecart" || !hasAllLabels(issue.Labels, extraLabels) {
			continue
		}
		minecarts = append(minecarts, issue)
		seen[issue.ID] = true
	}
	return minecarts, nil
}

func readMinecartIssues(townBeads string, args ...string) ([]minecartListIssue, error) {
	out, err := runBdJSON(townBeads, args...)
	if err != nil {
		return nil, err
	}
	var issues []minecartListIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

func hasAllLabels(labels, required []string) bool {
	for _, label := range required {
		if !hasLabel(labels, label) {
			return false
		}
	}
	return true
}

// minecartMergeFromFields extracts the merge strategy from a minecart description
// using the typed MinecartFields accessor.
// Returns the strategy string ("direct", "mr", "local") or empty string if not set.
func minecartMergeFromFields(description string) string {
	fields := beads.ParseMinecartFields(&beads.Issue{Description: description})
	if fields == nil {
		return ""
	}
	return fields.Merge
}

// formatYesNo returns "yes" or "no" for a boolean value.
func formatYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func formatMinecartStatus(status string) string {
	switch status {
	case "open":
		return style.Warning.Render("●")
	case "closed":
		return style.Success.Render("✓")
	case "in_progress":
		return style.Info.Render("→")
	default:
		return status
	}
}

// trackedIssueInfo holds info about an issue being tracked by a minecart.
type trackedIssueInfo struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Type      string   `json:"dependency_type"`
	IssueType string   `json:"issue_type"`
	Blocked   bool     `json:"blocked,omitempty"`    // True if issue currently has blockers
	Assignee  string   `json:"assignee,omitempty"`   // Assigned agent (e.g., mineshaft/miners/goose)
	Labels    []string `json:"labels,omitempty"`     // Bead labels (propagated from trackedDependency)
	Worker    string   `json:"worker,omitempty"`     // Worker currently assigned (e.g., mineshaft/nux)
	WorkerAge string   `json:"worker_age,omitempty"` // How long worker has been on this issue
}

// trackedDependency is dep-list data enriched with fresh issue details.
type trackedDependency struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	IssueType      string   `json:"issue_type"`
	Assignee       string   `json:"assignee"`
	DependencyType string   `json:"dependency_type"`
	Labels         []string `json:"labels"`
	Blocked        bool     `json:"-"`
}

func applyFreshIssueDetails(dep *trackedDependency, details *issueDetails) {
	dep.Status = strings.TrimSpace(details.Status)
	if dep.Status == "" {
		dep.Status = trackedStatusUnknown
	}
	dep.Blocked = details.IsBlocked()
	if dep.Title == "" {
		dep.Title = details.Title
	}
	if dep.Assignee == "" {
		dep.Assignee = details.Assignee
	}
	if dep.IssueType == "" {
		dep.IssueType = details.IssueType
	}
	// Always refresh labels unconditionally — bd dep list may return stale
	// labels from dependency records, but bd show returns current bead labels.
	// This ensures isReadyIssue sees accurate queue labels (gt:queued,
	// gt:queue-dispatched) for cross-rig beads. Assigning even when fresh
	// labels are empty clears stale queue labels that would otherwise
	// suppress stranded issue detection.
	dep.Labels = details.Labels
}

// getTrackedIssues gets issues tracked by a minecart with fresh cross-rig details.
// Returns issue details including status, type, and worker info.
//
// Prefers raw SQL query against the dependencies table (bdDepListRawIDs) which
// avoids the JOIN with the issues table that silently drops cross-database
// dependencies (see GH #2624, #2832). Falls back to bd dep list and bd show
// for older bd versions that don't support bd sql.
// Then fetches fresh issue details via bd show with prefix routing.
func getTrackedIssues(townBeads, minecartID string) ([]trackedIssueInfo, error) {
	// Prefer raw SQL — works for cross-database deps where tracked beads
	// live in different Dolt databases. Falls back to bd dep list if bd sql
	// is not available (older bd versions).
	trackedIDs, err := bdDepListRawIDs(townBeads, minecartID, "down", "tracks")
	if err != nil {
		// bd sql not supported (older bd) — fall back to bd dep list.
		trackedIDs, err = bdDepListTracked(townBeads, minecartID)
		if err != nil {
			return nil, fmt.Errorf("querying tracked issues for %s: %w", minecartID, err)
		}
	}

	// Fallback: when dep queries return empty (common for cross-database deps
	// on older bd where the JOIN fails), try parsing from bd show output.
	if len(trackedIDs) == 0 {
		trackedIDs, err = bdShowTrackedDeps(townBeads, minecartID)
		if err != nil {
			return nil, fmt.Errorf("fallback show for tracked deps of %s: %w", minecartID, err)
		}
	}

	if len(trackedIDs) == 0 {
		return nil, nil
	}

	// Fetch fresh issue details via bd show (uses prefix routing for cross-rig).
	freshDetails := getIssueDetailsBatch(trackedIDs)

	// Build tracked dependency structs from fresh details. When fresh details
	// are missing (cross-rig DB unreachable, missing, parked, or unroutable
	// from town root), mark the dep with trackedStatusUnknown so callers can
	// distinguish it from a legitimately open bead. (gt-bs6)
	var deps []trackedDependency
	for _, id := range trackedIDs {
		dep := trackedDependency{
			ID:             id,
			DependencyType: "tracks",
		}
		if details, ok := freshDetails[id]; ok {
			applyFreshIssueDetails(&dep, details)
		} else {
			dep.Status = trackedStatusUnknown
		}
		deps = append(deps, dep)
	}

	// Collect non-closed issue IDs for worker lookup
	openIssueIDs := make([]string, 0, len(deps))
	for _, dep := range deps {
		if dep.Status != "closed" {
			openIssueIDs = append(openIssueIDs, dep.ID)
		}
	}
	workersMap := getWorkersForIssues(openIssueIDs)

	// Build result
	var tracked []trackedIssueInfo
	for _, dep := range deps {
		info := trackedIssueInfo{
			ID:        dep.ID,
			Title:     dep.Title,
			Status:    dep.Status,
			Type:      dep.DependencyType,
			IssueType: dep.IssueType,
			Blocked:   dep.Blocked,
			Assignee:  dep.Assignee,
			Labels:    dep.Labels,
		}

		// Add worker info if available
		if worker, ok := workersMap[dep.ID]; ok {
			info.Worker = worker.Worker
			info.WorkerAge = worker.Age
		}

		tracked = append(tracked, info)
	}

	return tracked, nil
}

// bdDepListTracked runs `bd dep list <minecartID> --direction=down --type=tracks --json`
// and returns the tracked issue IDs (unwrapped from external: prefixes).
// Uses --allow-stale for consistency with sling's other bd calls (verifyBeadExists,
// bdShowBead) — without it, a jsonl write that straddles a second boundary causes
// "database out of sync" errors in CI and fast-turnaround production workflows.
func bdDepListTracked(dir, minecartID string) ([]string, error) {
	out, err := runBdJSONAllowStale(dir, "dep", "list", minecartID, "--direction=down", "--type=tracks", "--json")
	if err != nil {
		return nil, err
	}

	var results []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parsing dep list for %s: %w", minecartID, err)
	}

	seen := make(map[string]bool, len(results))
	var ids []string
	for _, r := range results {
		id := beads.ExtractIssueID(r.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// bdShowTrackedDeps falls back to `bd show <minecartID> --json` and extracts
// tracked dependency IDs from the minecart's dependencies array.
// This handles cross-database dependencies where bd dep list returns empty.
func bdShowTrackedDeps(dir, minecartID string) ([]string, error) {
	out, err := runBdJSON(dir, "show", minecartID, "--json")
	if err != nil {
		return nil, err
	}

	var results []struct {
		Dependencies []issueDependency `json:"dependencies"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parsing show for %s: %w", minecartID, err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var ids []string
	for _, dep := range results[0].Dependencies {
		if dep.DependencyType != "tracks" {
			continue
		}
		id := beads.ExtractIssueID(dep.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

type issueDependency struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	DependencyType string `json:"dependency_type"`
}

// issueDetails holds basic issue info.
type issueDetails struct {
	ID             string
	Title          string
	Status         string
	IssueType      string
	Assignee       string
	Labels         []string
	BlockedBy      []string
	BlockedByCount int
	Dependencies   []issueDependency
}

func (d issueDetails) IsBlocked() bool {
	if d.BlockedByCount > 0 || len(d.BlockedBy) > 0 {
		return true
	}

	// bd show can omit blocked_by_count; fall back to live dependency edges.
	for _, dep := range d.Dependencies {
		if dep.DependencyType == "blocks" && dep.Status != "closed" && dep.Status != "tombstone" {
			return true
		}
	}

	return false
}

// getIssueDetailsBatch fetches details through the central routed beads lookup.
// Returns a map from issue ID to details. Missing/invalid issues are omitted from the map.
func getIssueDetailsBatch(issueIDs []string) map[string]*issueDetails {
	result := make(map[string]*issueDetails, len(issueIDs))
	if len(issueIDs) == 0 {
		return result
	}

	client := minecartIssueClient()
	if client == nil {
		return result
	}

	issues, err := client.ShowMultiple(issueIDs)
	for id, issue := range issues {
		if details := issueToDetails(issue); details != nil {
			result[id] = details
		}
	}
	if err == nil {
		return result
	}

	// If a grouped batch fails because one ID is missing or stale, keep the
	// previous best-effort behavior and recover any IDs that still resolve.
	for _, id := range issueIDs {
		if result[id] != nil {
			continue
		}
		if details := getIssueDetailsWithClient(client, id); details != nil {
			result[id] = details
		}
	}

	return result
}

// getIssueDetails fetches issue details through the central routed beads lookup.
func getIssueDetails(issueID string) *issueDetails {
	client := minecartIssueClient()
	if client == nil {
		return nil
	}
	return getIssueDetailsWithClient(client, issueID)
}

func minecartIssueClient() *beads.Beads {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil
	}
	return beads.New(townRoot)
}

func getIssueDetailsWithClient(client *beads.Beads, issueID string) *issueDetails {
	issue, err := client.Show(issueID)
	if err != nil {
		return nil
	}
	return issueToDetails(issue)
}

func issueToDetails(issue *beads.Issue) *issueDetails {
	if issue == nil {
		return nil
	}

	deps := make([]issueDependency, 0, len(issue.Dependencies))
	for _, dep := range issue.Dependencies {
		deps = append(deps, issueDependency{
			ID:             dep.ID,
			Status:         dep.Status,
			DependencyType: dep.DependencyType,
		})
	}

	return &issueDetails{
		ID:             issue.ID,
		Title:          issue.Title,
		Status:         issue.Status,
		IssueType:      issue.Type,
		Assignee:       issue.Assignee,
		Labels:         issue.Labels,
		BlockedBy:      issue.BlockedBy,
		BlockedByCount: issue.BlockedByCount,
		Dependencies:   deps,
	}
}

// workerInfo holds info about a worker assigned to an issue.
type workerInfo struct {
	Worker string // Agent identity (e.g., mineshaft/nux)
	Age    string // How long assigned (e.g., "12m")
}

// getWorkersForIssues finds workers currently assigned to the given issues.
// Returns a map from issue ID to worker info.
//
// Optimized to batch queries per rig (O(R) instead of O(N×R)) and
// parallelize across rigs.
func getWorkersForIssues(issueIDs []string) map[string]*workerInfo {
	result := make(map[string]*workerInfo)
	if len(issueIDs) == 0 {
		return result
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return result
	}

	// Build a set of target issue IDs for fast lookup
	targetIDs := make(map[string]bool, len(issueIDs))
	for _, id := range issueIDs {
		targetIDs[id] = true
	}

	// Discover rigs with beads directories
	rigDirs, _ := filepath.Glob(filepath.Join(townRoot, "*", "miners"))
	var beadsDirs []string
	for _, minersDir := range rigDirs {
		rigDir := filepath.Dir(minersDir)
		beadsDir := filepath.Join(rigDir, "overseer", "rig", ".beads")
		if info, err := os.Stat(beadsDir); err == nil && info.IsDir() {
			beadsDirs = append(beadsDirs, filepath.Join(rigDir, "overseer", "rig"))
		}
	}

	if len(beadsDirs) == 0 {
		return result
	}

	// Query all rigs in parallel using bd list
	type rigResult struct {
		agents []struct {
			ID           string `json:"id"`
			HookBead     string `json:"hook_bead"`
			LastActivity string `json:"last_activity"`
		}
	}

	resultChan := make(chan rigResult, len(beadsDirs))
	var wg sync.WaitGroup

	for _, dir := range beadsDirs {
		wg.Add(1)
		go func(workDir string) {
			defer wg.Done()

			out, err := BdCmd("list", "--label=gt:agent", "--status=open", "--json", "--limit=0", "--flat").
				Dir(workDir).
				StripBeadsDir().
				Stderr(io.Discard).
				Output()
			if err != nil {
				resultChan <- rigResult{}
				return
			}

			var rr rigResult
			if err := json.Unmarshal(out, &rr.agents); err != nil {
				resultChan <- rigResult{}
				return
			}
			resultChan <- rr
		}(dir)
	}

	// Wait for all queries to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results from all rigs, filtering by target issue IDs
	for rr := range resultChan {
		for _, agent := range rr.agents {
			// Only include agents working on issues we care about
			if !targetIDs[agent.HookBead] {
				continue
			}

			// Skip if we already found a worker for this issue
			if _, ok := result[agent.HookBead]; ok {
				continue
			}

			// Parse agent ID to get worker identity
			workerID := parseWorkerFromAgentBead(agent.ID)
			if workerID == "" {
				continue
			}

			// Calculate age from last_activity
			age := ""
			if agent.LastActivity != "" {
				if t, err := time.Parse(time.RFC3339, agent.LastActivity); err == nil {
					age = formatWorkerAge(time.Since(t))
				}
			}

			result[agent.HookBead] = &workerInfo{
				Worker: workerID,
				Age:    age,
			}
		}
	}

	return result
}

// parseWorkerFromAgentBead extracts worker identity from agent bead ID.
// Input: "gt-mineshaft-miner-nux" -> Output: "mineshaft/miner/nux"
// Input: "gt-beads-crew-amber" -> Output: "beads/crew/amber"
func parseWorkerFromAgentBead(agentID string) string {
	rig, role, name, ok := beads.ParseAgentBeadID(agentID)
	if !ok {
		return ""
	}

	// Build path from parsed components
	if rig == "" {
		// Town-level
		if name != "" {
			return role + "/" + name
		}
		return role
	}
	if name != "" {
		return rig + "/" + role + "/" + name
	}
	return rig + "/" + role
}

// formatWorkerAge formats a duration as a short string (e.g., "5m", "2h", "1d")
func formatWorkerAge(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// runMinecartTUI launches the interactive minecart TUI.
func runMinecartTUI() error {
	townBeads, err := getTownBeadsDir()
	if err != nil {
		return err
	}

	m := minecart.New(townBeads)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// resolveMinecartNumber converts a numeric shortcut (1, 2, 3...) to a minecart ID.
// Numbers correspond to the order shown in 'gt minecart list'.
func resolveMinecartNumber(townBeads string, n int) (string, error) {
	minecarts, err := listMinecartIssues(townBeads, "", false)
	if err != nil {
		return "", fmt.Errorf("listing minecarts: %w", err)
	}

	if n < 1 || n > len(minecarts) {
		return "", fmt.Errorf("minecart %d not found (have %d minecarts)", n, len(minecarts))
	}

	return minecarts[n-1].ID, nil
}
