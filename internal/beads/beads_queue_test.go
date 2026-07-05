package beads

import (
	"strings"
	"testing"
)

func TestMatchClaimPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		identity string
		want     bool
	}{
		// Wildcard matches anyone
		{
			name:     "wildcard matches anyone",
			pattern:  "*",
			identity: "mineshaft/crew/max",
			want:     true,
		},
		{
			name:     "wildcard matches town-level agent",
			pattern:  "*",
			identity: "overseer/",
			want:     true,
		},

		// Exact match
		{
			name:     "exact match",
			pattern:  "mineshaft/crew/max",
			identity: "mineshaft/crew/max",
			want:     true,
		},
		{
			name:     "exact match fails on different identity",
			pattern:  "mineshaft/crew/max",
			identity: "mineshaft/crew/nux",
			want:     false,
		},

		// Suffix wildcard
		{
			name:     "suffix wildcard matches",
			pattern:  "mineshaft/miners/*",
			identity: "mineshaft/miners/capable",
			want:     true,
		},
		{
			name:     "suffix wildcard matches different name",
			pattern:  "mineshaft/miners/*",
			identity: "mineshaft/miners/nux",
			want:     true,
		},
		{
			name:     "suffix wildcard doesn't match nested path",
			pattern:  "mineshaft/miners/*",
			identity: "mineshaft/miners/sub/capable",
			want:     false,
		},
		{
			name:     "suffix wildcard doesn't match different rig",
			pattern:  "mineshaft/miners/*",
			identity: "bartertown/miners/capable",
			want:     false,
		},

		// Prefix wildcard
		{
			name:     "prefix wildcard matches",
			pattern:  "*/witness",
			identity: "mineshaft/witness",
			want:     true,
		},
		{
			name:     "prefix wildcard matches different rig",
			pattern:  "*/witness",
			identity: "bartertown/witness",
			want:     true,
		},
		{
			name:     "prefix wildcard doesn't match different role",
			pattern:  "*/witness",
			identity: "mineshaft/refinery",
			want:     false,
		},

		// Crew patterns
		{
			name:     "crew wildcard",
			pattern:  "mineshaft/crew/*",
			identity: "mineshaft/crew/max",
			want:     true,
		},
		{
			name:     "crew wildcard matches any crew member",
			pattern:  "mineshaft/crew/*",
			identity: "mineshaft/crew/jack",
			want:     true,
		},

		// Edge cases
		{
			name:     "empty identity doesn't match",
			pattern:  "*",
			identity: "",
			want:     true, // * matches anything
		},
		{
			name:     "empty pattern doesn't match",
			pattern:  "",
			identity: "mineshaft/crew/max",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchClaimPattern(tt.pattern, tt.identity)
			if got != tt.want {
				t.Errorf("MatchClaimPattern(%q, %q) = %v, want %v",
					tt.pattern, tt.identity, got, tt.want)
			}
		})
	}
}

func TestFormatQueueDescription(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		fields *QueueFields
		want   []string // Lines that should be present
	}{
		{
			name:  "basic queue",
			title: "Queue: work-requests",
			fields: &QueueFields{
				Name:         "work-requests",
				ClaimPattern: "mineshaft/crew/*",
				Status:       QueueStatusActive,
			},
			want: []string{
				"Queue: work-requests",
				"name: work-requests",
				"claim_pattern: mineshaft/crew/*",
				"status: active",
			},
		},
		{
			name:  "queue with default claim pattern",
			title: "Queue: public",
			fields: &QueueFields{
				Name:   "public",
				Status: QueueStatusActive,
			},
			want: []string{
				"name: public",
				"claim_pattern: *", // Default
				"status: active",
			},
		},
		{
			name:  "queue with counts",
			title: "Queue: processing",
			fields: &QueueFields{
				Name:            "processing",
				ClaimPattern:    "*/refinery",
				Status:          QueueStatusActive,
				AvailableCount:  5,
				ProcessingCount: 2,
				CompletedCount:  10,
				FailedCount:     1,
			},
			want: []string{
				"name: processing",
				"claim_pattern: */refinery",
				"available_count: 5",
				"processing_count: 2",
				"completed_count: 10",
				"failed_count: 1",
			},
		},
		{
			name:   "nil fields",
			title:  "Just Title",
			fields: nil,
			want:   []string{"Just Title"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatQueueDescription(tt.title, tt.fields)
			for _, line := range tt.want {
				if !strings.Contains(got, line) {
					t.Errorf("FormatQueueDescription() missing line %q in:\n%s", line, got)
				}
			}
		})
	}
}

func TestParseQueueFields(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantName    string
		wantPattern string
		wantStatus  string
	}{
		{
			name: "basic queue",
			description: `Queue: work-requests

name: work-requests
claim_pattern: mineshaft/crew/*
status: active`,
			wantName:    "work-requests",
			wantPattern: "mineshaft/crew/*",
			wantStatus:  QueueStatusActive,
		},
		{
			name: "queue with defaults",
			description: `Queue: minimal

name: minimal`,
			wantName:    "minimal",
			wantPattern: "*", // Default
			wantStatus:  QueueStatusActive,
		},
		{
			name:        "empty description",
			description: "",
			wantName:    "",
			wantPattern: "*", // Default
			wantStatus:  QueueStatusActive,
		},
		{
			name: "queue with counts",
			description: `Queue: processing

name: processing
claim_pattern: */refinery
status: paused
available_count: 5
processing_count: 2`,
			wantName:    "processing",
			wantPattern: "*/refinery",
			wantStatus:  QueueStatusPaused,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQueueFields(tt.description)
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.ClaimPattern != tt.wantPattern {
				t.Errorf("ClaimPattern = %q, want %q", got.ClaimPattern, tt.wantPattern)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestQueueBeadID(t *testing.T) {
	tests := []struct {
		name        string
		queueName   string
		isTownLevel bool
		want        string
	}{
		{
			name:        "town-level queue",
			queueName:   "dispatch",
			isTownLevel: true,
			want:        "hq-q-dispatch",
		},
		{
			name:        "rig-level queue",
			queueName:   "merge",
			isTownLevel: false,
			want:        "gt-q-merge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QueueBeadID(tt.queueName, tt.isTownLevel)
			if got != tt.want {
				t.Errorf("QueueBeadID(%q, %v) = %q, want %q",
					tt.queueName, tt.isTownLevel, got, tt.want)
			}
		})
	}
}
