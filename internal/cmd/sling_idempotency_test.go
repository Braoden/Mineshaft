package cmd

import "testing"

func TestMatchesSlingTarget(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		assignee  string
		selfAgent string
		want      bool
	}{
		{
			name:     "exact match",
			target:   "mineshaft/miners/toast",
			assignee: "mineshaft/miners/toast",
			want:     true,
		},
		{
			name:     "target with trailing slash matches overseer assignee",
			target:   "overseer",
			assignee: "overseer/",
			want:     true,
		},
		{
			name:     "rig namespace target matches existing miner assignment",
			target:   "mineshaft",
			assignee: "mineshaft/miners/toast",
			want:     true,
		},
		{
			name:      "self target matches self assignee",
			target:    ".",
			assignee:  "mineshaft/crew/alex",
			selfAgent: "mineshaft/crew/alex",
			want:      true,
		},
		{
			name:     "different target does not match",
			target:   "mineshaft/miners/other",
			assignee: "mineshaft/miners/toast",
			want:     false,
		},
		{
			name:     "rig target does not match non-miner assignee",
			target:   "mineshaft",
			assignee: "mineshaft/crew/alex",
			want:     false,
		},
		{
			name:     "empty assignee never matches",
			target:   "mineshaft/miners/toast",
			assignee: "",
			want:     false,
		},
		{
			name:      "empty target with empty selfAgent does not match",
			target:    "",
			assignee:  "mineshaft/miners/toast",
			selfAgent: "",
			want:      false,
		},
		{
			name:      "dot target with empty selfAgent does not match",
			target:    ".",
			assignee:  "mineshaft/miners/toast",
			selfAgent: "",
			want:      false,
		},
		{
			name:      "self target does not match different assignee",
			target:    ".",
			assignee:  "mineshaft/miners/toast",
			selfAgent: "mineshaft/crew/alex",
			want:      false,
		},
		// Shorthand and pool targets are intentionally NOT matched:
		// they have ambiguous resolution that requires filesystem/dispatcher context.
		{
			name:     "shorthand target does not match miner (ambiguous resolution)",
			target:   "mineshaft/toast",
			assignee: "mineshaft/miners/toast",
			want:     false,
		},
		{
			name:     "shorthand target does not match crew (ambiguous resolution)",
			target:   "mineshaft/alex",
			assignee: "mineshaft/crew/alex",
			want:     false,
		},
		{
			name:     "dog pool target does not match specific dog (pool dispatch)",
			target:   "supervisor/dogs",
			assignee: "supervisor/dogs/alpha",
			want:     false,
		},
		{
			name:     "exact dog path still matches",
			target:   "supervisor/dogs/alpha",
			assignee: "supervisor/dogs/alpha",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesSlingTarget(tt.target, tt.assignee, tt.selfAgent)
			if got != tt.want {
				t.Fatalf("matchesSlingTarget(%q, %q, %q) = %v, want %v",
					tt.target, tt.assignee, tt.selfAgent, got, tt.want)
			}
		})
	}
}
