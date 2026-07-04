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
			target:   "excavation/miners/toast",
			assignee: "excavation/miners/toast",
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
			target:   "excavation",
			assignee: "excavation/miners/toast",
			want:     true,
		},
		{
			name:      "self target matches self assignee",
			target:    ".",
			assignee:  "excavation/crew/alex",
			selfAgent: "excavation/crew/alex",
			want:      true,
		},
		{
			name:     "different target does not match",
			target:   "excavation/miners/other",
			assignee: "excavation/miners/toast",
			want:     false,
		},
		{
			name:     "rig target does not match non-miner assignee",
			target:   "excavation",
			assignee: "excavation/crew/alex",
			want:     false,
		},
		{
			name:     "empty assignee never matches",
			target:   "excavation/miners/toast",
			assignee: "",
			want:     false,
		},
		{
			name:      "empty target with empty selfAgent does not match",
			target:    "",
			assignee:  "excavation/miners/toast",
			selfAgent: "",
			want:      false,
		},
		{
			name:      "dot target with empty selfAgent does not match",
			target:    ".",
			assignee:  "excavation/miners/toast",
			selfAgent: "",
			want:      false,
		},
		{
			name:      "self target does not match different assignee",
			target:    ".",
			assignee:  "excavation/miners/toast",
			selfAgent: "excavation/crew/alex",
			want:      false,
		},
		// Shorthand and pool targets are intentionally NOT matched:
		// they have ambiguous resolution that requires filesystem/dispatcher context.
		{
			name:     "shorthand target does not match miner (ambiguous resolution)",
			target:   "excavation/toast",
			assignee: "excavation/miners/toast",
			want:     false,
		},
		{
			name:     "shorthand target does not match crew (ambiguous resolution)",
			target:   "excavation/alex",
			assignee: "excavation/crew/alex",
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
