package beads

import "testing"

func TestNeedsForceForID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{id: "", want: false},
		{id: "hq-overseer", want: false},
		{id: "ms-abc123", want: false},
		{id: "hq-overseer-role", want: true},
		{id: "st-stockdrop-miner-nux", want: true},
		{id: "hq-cv-abc", want: true},
	}

	for _, tc := range tests {
		if got := NeedsForceForID(tc.id); got != tc.want {
			t.Fatalf("NeedsForceForID(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}
