package townlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatLogLine(t *testing.T) {
	ts := time.Date(2025, 12, 26, 15, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		event    Event
		contains []string
	}{
		{
			name: "spawn event",
			event: Event{
				Timestamp: ts,
				Type:      EventSpawn,
				Agent:     "mineshaft/crew/max",
				Context:   "ms-xyz",
			},
			contains: []string{"2025-12-26 15:30:45", "[spawn]", "mineshaft/crew/max", "spawned for ms-xyz"},
		},
		{
			name: "nudge event",
			event: Event{
				Timestamp: ts,
				Type:      EventNudge,
				Agent:     "mineshaft/crew/max",
				Context:   "start work",
			},
			contains: []string{"[nudge]", "mineshaft/crew/max", "nudged with"},
		},
		{
			name: "done event",
			event: Event{
				Timestamp: ts,
				Type:      EventDone,
				Agent:     "mineshaft/crew/max",
				Context:   "ms-abc",
			},
			contains: []string{"[done]", "completed ms-abc"},
		},
		{
			name: "crash event",
			event: Event{
				Timestamp: ts,
				Type:      EventCrash,
				Agent:     "mineshaft/miners/Toast",
				Context:   "signal 9",
			},
			contains: []string{"[crash]", "exited unexpectedly", "signal 9"},
		},
		{
			name: "kill event",
			event: Event{
				Timestamp: ts,
				Type:      EventKill,
				Agent:     "mineshaft/miners/Toast",
				Context:   "ms stop",
			},
			contains: []string{"[kill]", "killed", "ms stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := formatLogLine(tt.event)
			for _, want := range tt.contains {
				if !strings.Contains(line, want) {
					t.Errorf("formatLogLine() = %q, want it to contain %q", line, want)
				}
			}
		})
	}
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		check   func(Event) bool
	}{
		{
			name: "valid spawn line",
			line: "2025-12-26 15:30:45 [spawn] mineshaft/crew/max spawned for ms-xyz",
			check: func(e Event) bool {
				return e.Type == EventSpawn && e.Agent == "mineshaft/crew/max"
			},
		},
		{
			name: "valid nudge line",
			line: "2025-12-26 15:31:02 [nudge] mineshaft/crew/max nudged with \"start\"",
			check: func(e Event) bool {
				return e.Type == EventNudge && e.Agent == "mineshaft/crew/max"
			},
		},
		{
			name:    "too short",
			line:    "short",
			wantErr: true,
		},
		{
			name:    "missing bracket",
			line:    "2025-12-26 15:30:45 spawn mineshaft/crew/max",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := parseLogLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseLogLine() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("parseLogLine() unexpected error: %v", err)
				return
			}
			if tt.check != nil && !tt.check(event) {
				t.Errorf("parseLogLine() check failed for event: %+v", event)
			}
		})
	}
}

func TestLoggerLogEvent(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "townlog-test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := NewLogger(tmpDir)

	// Log an event
	err = logger.Log(EventSpawn, "mineshaft/crew/max", "ms-xyz")
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	// Verify log file was created
	logPath := filepath.Join(tmpDir, "logs", "town.log")
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}

	if !strings.Contains(string(content), "[spawn]") {
		t.Errorf("log file should contain [spawn], got: %s", content)
	}
	if !strings.Contains(string(content), "mineshaft/crew/max") {
		t.Errorf("log file should contain agent name, got: %s", content)
	}
}

func TestFilterEvents(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Timestamp: now.Add(-2 * time.Hour), Type: EventSpawn, Agent: "mineshaft/crew/max", Context: "ms-1"},
		{Timestamp: now.Add(-1 * time.Hour), Type: EventNudge, Agent: "mineshaft/crew/max", Context: "hi"},
		{Timestamp: now.Add(-30 * time.Minute), Type: EventDone, Agent: "mineshaft/miners/Toast", Context: "ms-2"},
		{Timestamp: now.Add(-10 * time.Minute), Type: EventSpawn, Agent: "wyvern/crew/joe", Context: "ms-3"},
	}

	tests := []struct {
		name      string
		filter    Filter
		wantCount int
	}{
		{
			name:      "no filter",
			filter:    Filter{},
			wantCount: 4,
		},
		{
			name:      "filter by type",
			filter:    Filter{Type: EventSpawn},
			wantCount: 2,
		},
		{
			name:      "filter by agent prefix",
			filter:    Filter{Agent: "mineshaft/"},
			wantCount: 3,
		},
		{
			name:      "filter by time",
			filter:    Filter{Since: now.Add(-45 * time.Minute)},
			wantCount: 2,
		},
		{
			name:      "combined filters",
			filter:    Filter{Type: EventSpawn, Agent: "mineshaft/"},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterEvents(events, tt.filter)
			if len(result) != tt.wantCount {
				t.Errorf("FilterEvents() got %d events, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10c", 10, "exactly10c"},
		{"this is a longer string", 10, "this is..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestEventHandoffNoPersist_Format(t *testing.T) {
	e := Event{
		Type:    EventHandoffNoPersist,
		Agent:   "mineshaft/crew/max",
		Context: "session cycling — error: connection refused",
	}
	line := formatLogLine(e)
	if !strings.Contains(line, "[handoff-NOPERSIST]") {
		t.Errorf("expected [handoff-NOPERSIST] in log line, got: %s", line)
	}
	if !strings.Contains(line, "handoff FAILED") {
		t.Errorf("expected 'handoff FAILED' in log line, got: %s", line)
	}
}

func TestEventHandoffNoPersist_NoContext(t *testing.T) {
	e := Event{
		Type:  EventHandoffNoPersist,
		Agent: "overseer",
	}
	line := formatLogLine(e)
	if !strings.Contains(line, "handoff FAILED (Dolt persistence)") {
		t.Errorf("expected default detail, got: %s", line)
	}
}

func TestEventHandoffNoPersist_ParseRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "town.log")
	logger := &Logger{logPath: logPath}

	if err := logger.Log(EventHandoffNoPersist, "mineshaft/crew/max", "test failure"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, "[handoff-NOPERSIST]") {
		t.Errorf("expected [handoff-NOPERSIST] in written log, got: %s", line)
	}
}
