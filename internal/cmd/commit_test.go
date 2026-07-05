package cmd

import "testing"

func TestIdentityToEmail(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		domain   string
		want     string
	}{
		{
			name:     "crew member",
			identity: "mineshaft/crew/jack",
			domain:   "mineshaft.local",
			want:     "mineshaft.crew.jack@mineshaft.local",
		},
		{
			name:     "miner",
			identity: "mineshaft/miners/max",
			domain:   "mineshaft.local",
			want:     "mineshaft.miners.max@mineshaft.local",
		},
		{
			name:     "witness",
			identity: "mineshaft/witness",
			domain:   "mineshaft.local",
			want:     "mineshaft.witness@mineshaft.local",
		},
		{
			name:     "refinery",
			identity: "mineshaft/refinery",
			domain:   "mineshaft.local",
			want:     "mineshaft.refinery@mineshaft.local",
		},
		{
			name:     "overseer with trailing slash",
			identity: "overseer/",
			domain:   "mineshaft.local",
			want:     "overseer@mineshaft.local",
		},
		{
			name:     "supervisor with trailing slash",
			identity: "supervisor/",
			domain:   "mineshaft.local",
			want:     "supervisor@mineshaft.local",
		},
		{
			name:     "custom domain",
			identity: "myrig/crew/alice",
			domain:   "example.com",
			want:     "myrig.crew.alice@example.com",
		},
		{
			name:     "deeply nested",
			identity: "rig/miners/nested/deep",
			domain:   "test.io",
			want:     "rig.miners.nested.deep@test.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identityToEmail(tt.identity, tt.domain)
			if got != tt.want {
				t.Errorf("identityToEmail(%q, %q) = %q, want %q",
					tt.identity, tt.domain, got, tt.want)
			}
		})
	}
}
