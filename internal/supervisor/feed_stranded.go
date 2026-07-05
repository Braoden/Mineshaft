package supervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/mineshaft/internal/beads"
	"github.com/steveyegge/mineshaft/internal/constants"
	"github.com/steveyegge/mineshaft/internal/util"
)

// Default parameters for feed-stranded rate limiting.
// Configurable via operational.supervisor.max_feeds_per_cycle and
// operational.supervisor.feed_cooldown in settings/config.json.
const (
	// DefaultMaxFeedsPerCycle is the maximum number of minecarts to feed in one invocation.
	// Prevents spawning too many dogs at once.
	DefaultMaxFeedsPerCycle = 3

	// DefaultFeedCooldown is the minimum time between feeding the same minecart.
	// Prevents re-dispatching a dog before the previous one finishes.
	DefaultFeedCooldown = 10 * time.Minute
)

// FeedStrandedState tracks feeding attempts per minecart.
// Persisted to supervisor/feed-stranded-state.json.
type FeedStrandedState struct {
	// Minecarts maps minecart ID to their feed tracking state.
	Minecarts map[string]*MinecartFeedState `json:"minecarts"`

	// LastUpdated is when this state was last written.
	LastUpdated time.Time `json:"last_updated"`
}

// MinecartFeedState tracks the feed history for a single minecart.
type MinecartFeedState struct {
	// MinecartID is the minecart identifier.
	MinecartID string `json:"minecart_id"`

	// FeedCount is total number of feed dispatches for this minecart.
	FeedCount int `json:"feed_count"`

	// LastFeedTime is when the last feed was dispatched.
	LastFeedTime time.Time `json:"last_feed_time,omitempty"`
}

// StrandedMinecart holds info about a stranded minecart from `gt minecart stranded --json`.
type StrandedMinecart struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	TrackedCount int      `json:"tracked_count"`
	ReadyCount   int      `json:"ready_count"`
	ReadyIssues  []string `json:"ready_issues"`
}

// FeedResult describes the outcome of a feed-stranded invocation.
type FeedResult struct {
	// Fed is the number of minecarts dispatched to dogs for feeding.
	Fed int `json:"fed"`

	// Closed is the number of empty minecarts auto-closed.
	Closed int `json:"closed"`

	// Skipped is the number of minecarts skipped (cooldown).
	Skipped int `json:"skipped"`

	// NeedsAttention is the number of minecarts with tracked issues but no ready
	// issues. These require agent judgment — Go surfaces the raw data but does
	// not classify or act on them.
	NeedsAttention int `json:"needs_attention"`

	// Errors is the number of minecarts that failed to process.
	Errors int `json:"errors"`

	// Details has per-minecart results.
	Details []FeedMinecartResult `json:"details"`
}

// FeedMinecartResult describes the outcome for a single minecart.
type FeedMinecartResult struct {
	MinecartID     string `json:"minecart_id"`
	Action       string `json:"action"` // "fed", "closed", "cooldown", "error", "limit", "needs_attention"
	Message      string `json:"message"`
	TrackedCount int    `json:"tracked_count,omitempty"` // Raw data for agent inspection
	ReadyCount   int    `json:"ready_count,omitempty"`   // Raw data for agent inspection
}

// FeedStrandedStateFile returns the path to the feed-stranded state file.
func FeedStrandedStateFile(townRoot string) string {
	return filepath.Join(townRoot, "supervisor", "feed-stranded-state.json")
}

// LoadFeedStrandedState loads the feed-stranded state from disk.
// Returns empty state if file doesn't exist.
func LoadFeedStrandedState(townRoot string) (*FeedStrandedState, error) {
	stateFile := FeedStrandedStateFile(townRoot)

	data, err := os.ReadFile(stateFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return &FeedStrandedState{
				Minecarts: make(map[string]*MinecartFeedState),
			}, nil
		}
		return nil, fmt.Errorf("reading feed-stranded state: %w", err)
	}

	var state FeedStrandedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing feed-stranded state: %w", err)
	}

	if state.Minecarts == nil {
		state.Minecarts = make(map[string]*MinecartFeedState)
	}

	return &state, nil
}

// SaveFeedStrandedState saves the feed-stranded state to disk.
func SaveFeedStrandedState(townRoot string, state *FeedStrandedState) error {
	stateFile := FeedStrandedStateFile(townRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating supervisor directory: %w", err)
	}

	state.LastUpdated = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling feed-stranded state: %w", err)
	}

	return os.WriteFile(stateFile, data, 0600)
}

// GetMinecartState returns the feed state for a minecart, creating if needed.
func (s *FeedStrandedState) GetMinecartState(minecartID string) *MinecartFeedState {
	if s.Minecarts == nil {
		s.Minecarts = make(map[string]*MinecartFeedState)
	}

	state, ok := s.Minecarts[minecartID]
	if !ok {
		state = &MinecartFeedState{MinecartID: minecartID}
		s.Minecarts[minecartID] = state
	}
	return state
}

// IsInCooldown returns true if the minecart was recently fed.
func (s *MinecartFeedState) IsInCooldown(cooldown time.Duration) bool {
	if s.LastFeedTime.IsZero() {
		return false
	}
	return time.Since(s.LastFeedTime) < cooldown
}

// CooldownRemaining returns how long until cooldown expires.
func (s *MinecartFeedState) CooldownRemaining(cooldown time.Duration) time.Duration {
	if s.LastFeedTime.IsZero() {
		return 0
	}
	remaining := cooldown - time.Since(s.LastFeedTime)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// RecordFeed records a feed dispatch for the minecart.
func (s *MinecartFeedState) RecordFeed() {
	s.FeedCount++
	s.LastFeedTime = time.Now().UTC()
}

// FindStrandedMinecarts runs `gt minecart stranded --json` and parses the output.
func FindStrandedMinecarts(townRoot string) ([]StrandedMinecart, error) {
	cmd := exec.Command("gt", "minecart", "stranded", "--json")
	cmd.Dir = townRoot
	cmd.Env = supervisorReadOnlyRoutingEnv(townRoot)
	util.SetDetachedProcessGroup(cmd)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running gt minecart stranded: %w", err)
	}

	var stranded []StrandedMinecart
	if err := json.Unmarshal(output, &stranded); err != nil {
		return nil, fmt.Errorf("parsing stranded minecarts: %w", err)
	}

	return stranded, nil
}

// FeedStranded detects stranded minecarts and takes mechanical actions where safe.
// Empty minecarts (0 tracked) are auto-closed. Feedable minecarts get a dog dispatched.
// Minecarts with tracked-but-not-ready issues are surfaced as "needs_attention" with
// raw data (tracked_count, ready_count) for the supervisor agent to inspect and decide.
// Rate limits by maxPerCycle and per-minecart cooldown.
func FeedStranded(townRoot string, maxPerCycle int, cooldown time.Duration) *FeedResult {
	result := &FeedResult{}

	if maxPerCycle <= 0 {
		maxPerCycle = DefaultMaxFeedsPerCycle
	}
	if cooldown <= 0 {
		cooldown = DefaultFeedCooldown
	}

	// Find stranded minecarts
	stranded, err := FindStrandedMinecarts(townRoot)
	if err != nil {
		result.Errors++
		result.Details = append(result.Details, FeedMinecartResult{
			Action:  "error",
			Message: fmt.Sprintf("failed to find stranded minecarts: %v", err),
		})
		return result
	}

	if len(stranded) == 0 {
		return result
	}

	// Load state for cooldown tracking
	state, err := LoadFeedStrandedState(townRoot)
	if err != nil {
		result.Errors++
		result.Details = append(result.Details, FeedMinecartResult{
			Action:  "error",
			Message: fmt.Sprintf("failed to load feed state: %v", err),
		})
		return result
	}

	fedCount := 0

	for _, minecart := range stranded {
		// Handle minecarts with no ready issues.
		if minecart.ReadyCount == 0 {
			// Minecart has tracked issues but none are ready — surface raw data
			// for the supervisor agent to inspect. Go does not classify WHY issues
			// aren't ready (dependency resolution, external block, etc.).
			if minecart.TrackedCount > 0 {
				result.NeedsAttention++
				result.Details = append(result.Details, FeedMinecartResult{
					MinecartID:     minecart.ID,
					Action:       "needs_attention",
					Message:      fmt.Sprintf("%d tracked issues, 0 ready — requires agent review", minecart.TrackedCount),
					TrackedCount: minecart.TrackedCount,
					ReadyCount:   0,
				})
				continue
			}

			// Truly empty minecart (0 tracked issues) — auto-close
			if err := closeEmptyMinecart(townRoot, minecart.ID); err != nil {
				result.Errors++
				result.Details = append(result.Details, FeedMinecartResult{
					MinecartID: minecart.ID,
					Action:   "error",
					Message:  fmt.Sprintf("failed to auto-close empty minecart: %v", err),
				})
			} else {
				result.Closed++
				result.Details = append(result.Details, FeedMinecartResult{
					MinecartID: minecart.ID,
					Action:   "closed",
					Message:  "auto-closed empty minecart (0 tracked issues)",
				})
			}
			continue
		}

		// Rate limit: check per-cycle cap
		if fedCount >= maxPerCycle {
			result.Details = append(result.Details, FeedMinecartResult{
				MinecartID: minecart.ID,
				Action:   "limit",
				Message:  fmt.Sprintf("skipped: per-cycle limit reached (%d/%d)", fedCount, maxPerCycle),
			})
			continue
		}

		// Rate limit: check per-minecart cooldown
		minecartState := state.GetMinecartState(minecart.ID)
		if minecartState.IsInCooldown(cooldown) {
			remaining := minecartState.CooldownRemaining(cooldown)
			result.Skipped++
			result.Details = append(result.Details, FeedMinecartResult{
				MinecartID: minecart.ID,
				Action:   "cooldown",
				Message:  fmt.Sprintf("in cooldown (remaining: %s)", remaining.Round(time.Second)),
			})
			continue
		}

		// Dispatch dog to feed the minecart
		if err := dispatchFeedDog(townRoot, minecart.ID); err != nil {
			result.Errors++
			result.Details = append(result.Details, FeedMinecartResult{
				MinecartID: minecart.ID,
				Action:   "error",
				Message:  fmt.Sprintf("failed to dispatch feed dog: %v", err),
			})
			continue
		}

		minecartState.RecordFeed()
		fedCount++
		result.Fed++
		result.Details = append(result.Details, FeedMinecartResult{
			MinecartID: minecart.ID,
			Action:   "fed",
			Message:  fmt.Sprintf("dispatched dog to feed (%d ready issues)", minecart.ReadyCount),
		})
	}

	// Save state
	if err := SaveFeedStrandedState(townRoot, state); err != nil {
		result.Details = append(result.Details, FeedMinecartResult{
			Action:  "error",
			Message: fmt.Sprintf("warning: failed to save feed state: %v", err),
		})
	}

	return result
}

// closeEmptyMinecart runs `gt minecart check <id>` to auto-close an empty minecart.
func closeEmptyMinecart(townRoot, minecartID string) error {
	cmd := exec.Command("gt", "minecart", "check", minecartID)
	cmd.Dir = townRoot
	cmd.Env = supervisorMutationRoutingEnv(townRoot)
	util.SetDetachedProcessGroup(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dispatchFeedDog dispatches a dog to feed a stranded minecart via gt sling.
func dispatchFeedDog(townRoot, minecartID string) error {
	cmd := exec.Command("gt", "sling", constants.MolMinecartFeed, "supervisor/dogs",
		"--var", fmt.Sprintf("minecart=%s", minecartID))
	cmd.Dir = townRoot
	cmd.Env = supervisorMutationRoutingEnv(townRoot)
	util.SetDetachedProcessGroup(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// PruneFeedStrandedState removes entries for minecarts that are no longer open.
// Call periodically to prevent unbounded state growth.
func PruneFeedStrandedState(townRoot string) (int, error) {
	state, err := LoadFeedStrandedState(townRoot)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for minecartID := range state.Minecarts {
		status := getMinecartStatus(townRoot, minecartID)
		if status == "closed" || status == "" {
			delete(state.Minecarts, minecartID)
			pruned++
		}
	}

	if pruned > 0 {
		if err := SaveFeedStrandedState(townRoot, state); err != nil {
			return pruned, err
		}
	}

	return pruned, nil
}

// getMinecartStatus returns the current status of a minecart bead.
func getMinecartStatus(townRoot, minecartID string) string {
	cmd := beads.Command(townRoot, townBeadsDir(townRoot), beads.ReadOnlyRouting, "show", minecartID, "--json")

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &issues); err != nil || len(issues) == 0 {
		return ""
	}
	return issues[0].Status
}
