package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/mineshaft/internal/config"
	"github.com/steveyegge/mineshaft/internal/feed"
	"github.com/steveyegge/mineshaft/internal/session"
	"github.com/steveyegge/mineshaft/internal/tmux"
	"github.com/steveyegge/mineshaft/internal/workspace"
)

//go:embed viewweb
var viewAssets embed.FS

var viewPort int

var viewCmd = &cobra.Command{
	Use:     "view",
	GroupID: GroupDiag,
	Short:   "Launch the Mineshaft web view",
	Long: `Start a local web server for the Mineshaft view.

Serves a real-time pixel-art visualization of the town: every agent is
a Clawd going about its work. Data comes from live tmux sessions and
the curated event feed.

Example:
  ms view                # Start on default port 8090
  ms view --port 3000    # Start on port 3000`,
	RunE: runView,
}

func init() {
	viewCmd.Flags().IntVar(&viewPort, "port", 8090, "HTTP port to listen on")
	rootCmd.AddCommand(viewCmd)
}

// viewAgent is one agent in the world snapshot sent to the frontend.
type viewAgent struct {
	ID      string `json:"id"`   // session name, e.g. hq-overseer
	Role    string `json:"role"` // overseer|supervisor|witness|refinery|crew|miner
	Rig     string `json:"rig,omitempty"`
	Name    string `json:"name"` // display name
	Running bool   `json:"running"`
}

// viewState is the full world snapshot.
type viewState struct {
	Town   string      `json:"town"`
	Rigs   []string    `json:"rigs"`
	Agents []viewAgent `json:"agents"`
}

// gatherViewState builds a snapshot of all known agents. The fixed roster
// (overseer, supervisor, per-rig witness/refinery) is always present with a
// running flag; ephemeral crew and miners appear only while their tmux
// sessions exist.
func gatherViewState(townRoot string) *viewState {
	t := tmux.NewTmux()
	st := &viewState{Town: filepath.Base(townRoot)}

	hasSession := func(name string) bool {
		ok, _ := t.HasSession(name)
		return ok
	}

	overseerSession := session.OverseerSessionName()
	supervisorSession := session.SupervisorSessionName()
	st.Agents = append(st.Agents,
		viewAgent{ID: overseerSession, Role: "overseer", Name: "Overseer", Running: hasSession(overseerSession)},
		viewAgent{ID: supervisorSession, Role: "supervisor", Name: "Supervisor", Running: hasSession(supervisorSession)},
	)

	rigsPath := filepath.Join(townRoot, "overseer", "rigs.json")
	if rigsConfig, err := config.LoadRigsConfig(rigsPath); err == nil {
		for name := range rigsConfig.Rigs {
			st.Rigs = append(st.Rigs, name)
		}
		sort.Strings(st.Rigs)
		for _, name := range st.Rigs {
			prefix := session.PrefixFor(name)
			witnessSession := session.WitnessSessionName(prefix)
			refinerySession := session.RefinerySessionName(prefix)
			st.Agents = append(st.Agents,
				viewAgent{ID: witnessSession, Role: "witness", Rig: name, Name: "witness", Running: hasSession(witnessSession)},
				viewAgent{ID: refinerySession, Role: "refinery", Rig: name, Name: "refinery", Running: hasSession(refinerySession)},
			)
		}
	}

	// Ephemeral agents: crew and miners with live sessions.
	if sessions, err := getAgentSessions(true); err == nil {
		for _, s := range sessions {
			switch s.Type {
			case AgentCrew:
				st.Agents = append(st.Agents, viewAgent{ID: s.Name, Role: "crew", Rig: s.Rig, Name: s.AgentName, Running: true})
			case AgentMiner:
				st.Agents = append(st.Agents, viewAgent{ID: s.Name, Role: "miner", Rig: s.Rig, Name: s.AgentName, Running: true})
			}
		}
	}

	return st
}

// viewUsage is the usage window snapshot for the day/night clock and the
// speed-dial page. Utilization/ResetsAt are the 5-hour window (field names
// are load-bearing: game.js day/night depends on them).
type viewUsage struct {
	OK              bool    `json:"ok"`
	Utilization     float64 `json:"utilization"` // 5h window percent used, 0-100
	ResetsAt        string  `json:"resets_at,omitempty"`
	WeekUtilization float64 `json:"week_utilization"` // 7-day window percent used, 0-100
	WeekResetsAt    string  `json:"week_resets_at,omitempty"`
	// Limits is the full per-limit breakdown as shown by Claude Code's /usage
	// (session, weekly_all, and model-scoped weekly limits).
	Limits []usageLimit `json:"limits,omitempty"`
}

type usageLimit struct {
	Kind     string  `json:"kind"`     // session | weekly_all | weekly_scoped
	Percent  float64 `json:"percent"`  // 0-100
	Severity string  `json:"severity"` // normal | warning | critical
	ResetsAt string  `json:"resets_at,omitempty"`
	Scope    string  `json:"scope,omitempty"` // model display name for scoped limits
	IsActive bool    `json:"is_active"`
}

var usageCache struct {
	sync.Mutex
	data    viewUsage
	fetched time.Time
}

// fetchUsage returns the Claude 5h-window usage, cached for 60s (including
// failures, to avoid hammering the endpoint or the credentials file).
func fetchUsage() viewUsage {
	usageCache.Lock()
	defer usageCache.Unlock()
	if time.Since(usageCache.fetched) < time.Minute {
		return usageCache.data
	}
	usageCache.fetched = time.Now()
	usageCache.data = queryOAuthUsage()
	return usageCache.data
}

func queryOAuthUsage() viewUsage {
	home, err := os.UserHomeDir()
	if err != nil {
		return viewUsage{}
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return viewUsage{}
	}
	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(raw, &creds); err != nil || creds.ClaudeAiOauth.AccessToken == "" {
		return viewUsage{}
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return viewUsage{}
	}
	req.Header.Set("Authorization", "Bearer "+creds.ClaudeAiOauth.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return viewUsage{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return viewUsage{}
	}
	var body struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
		Limits []struct {
			Kind     string  `json:"kind"`
			Percent  float64 `json:"percent"`
			Severity string  `json:"severity"`
			ResetsAt string  `json:"resets_at"`
			IsActive bool    `json:"is_active"`
			Scope    *struct {
				Model *struct {
					DisplayName string `json:"display_name"`
				} `json:"model"`
			} `json:"scope"`
		} `json:"limits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return viewUsage{}
	}
	limits := make([]usageLimit, 0, len(body.Limits))
	for _, l := range body.Limits {
		lim := usageLimit{Kind: l.Kind, Percent: l.Percent, Severity: l.Severity, ResetsAt: l.ResetsAt, IsActive: l.IsActive}
		if l.Scope != nil && l.Scope.Model != nil {
			lim.Scope = l.Scope.Model.DisplayName
		}
		limits = append(limits, lim)
	}
	return viewUsage{
		OK:              true,
		Utilization:     body.FiveHour.Utilization,
		ResetsAt:        body.FiveHour.ResetsAt,
		WeekUtilization: body.SevenDay.Utilization,
		WeekResetsAt:    body.SevenDay.ResetsAt,
		Limits:          limits,
	}
}

func runView(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Mineshaft workspace: %w", err)
	}

	webRoot, err := fs.Sub(viewAssets, "viewweb")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServerFS(webRoot))
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(gatherViewState(townRoot)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(fetchUsage()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		serveViewEvents(w, r, townRoot)
	})

	listenAddr := fmt.Sprintf("127.0.0.1:%d", viewPort)
	fmt.Printf("  ms view at http://%s  •  ctrl+c to stop\n", listenAddr)

	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

// serveViewEvents streams world state and feed events over SSE.
//   - event: state  — full snapshot, sent immediately and then whenever it changes
//   - event: feed   — one curated feed event (raw JSONL line), tailed from .feed.jsonl
func serveViewEvents(w http.ResponseWriter, r *http.Request, townRoot string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	send := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	feedPath := filepath.Join(townRoot, feed.FeedFile)
	feedOffset := int64(0)
	if info, err := os.Stat(feedPath); err == nil {
		feedOffset = info.Size() // start at EOF: only stream new events
	}

	var lastState, lastUsage string
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		// State snapshot, only when changed.
		if data, err := json.Marshal(gatherViewState(townRoot)); err == nil {
			if s := string(data); s != lastState {
				lastState = s
				send("state", s)
			}
		}

		// Usage snapshot (fetch is cached for 60s), only when changed.
		if data, err := json.Marshal(fetchUsage()); err == nil {
			if s := string(data); s != lastUsage {
				lastUsage = s
				send("usage", s)
			}
		}

		// New feed lines.
		var lines []string
		lines, feedOffset = readNewLines(feedPath, feedOffset)
		for _, line := range lines {
			send("feed", line)
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// readNewLines returns complete lines appended to path since offset, and the
// new offset. Handles truncation (feed curator prunes the file) by rewinding
// to the start.
func readNewLines(path string, offset int64) ([]string, int64) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, offset
	}
	if info.Size() < offset {
		offset = 0 // file was truncated/pruned
	}
	if info.Size() == offset {
		return nil, offset
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, offset
	}

	// Only consume complete lines; leave a trailing partial write for next poll.
	end := strings.LastIndexByte(string(data), '\n')
	if end < 0 {
		return nil, offset
	}
	var lines []string
	for _, line := range strings.Split(string(data[:end]), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, offset + int64(end) + 1
}
