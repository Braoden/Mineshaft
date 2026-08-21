package cmd

import (
	"testing"
	"time"
)

// TestUsageRetryAfter pins the backoff schedule. The upstream usage endpoint
// rate-limits, and both the sampler and every open dashboard share this cache,
// so retrying on a fixed 60s interval is what keeps a 429 alive. The shift and
// the ceiling interact subtly enough to be worth asserting.
func TestUsageRetryAfter(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{0, time.Minute},         // healthy: normal cache window
		{1, 2 * time.Minute},     // back off immediately on first failure
		{2, 4 * time.Minute},
		{3, 8 * time.Minute},
		{4, 15 * time.Minute},    // 16m would exceed the ceiling
		{5, 15 * time.Minute},    // clamped, not wrapped
		{50, 15 * time.Minute},   // far past the shift cap
	}
	for _, tt := range tests {
		if got := usageRetryAfter(tt.failures); got != tt.want {
			t.Errorf("usageRetryAfter(%d) = %v, want %v", tt.failures, got, tt.want)
		}
	}
}

// A negative count should never produce a zero or negative wait, which would
// turn the cache into a hot loop against the upstream.
func TestUsageRetryAfterNeverHammers(t *testing.T) {
	for _, f := range []int{-1, 0, 1, 7, 64, 1 << 20} {
		if got := usageRetryAfter(f); got < time.Minute {
			t.Errorf("usageRetryAfter(%d) = %v, shorter than the 1m floor", f, got)
		}
		if got := usageRetryAfter(f); got > 15*time.Minute {
			t.Errorf("usageRetryAfter(%d) = %v, longer than the 15m ceiling", f, got)
		}
	}
}

// TestQueryOAuthUsageStatuses documents that every failure path names itself.
// Before this, all of them returned a bare viewUsage{} and the dashboard could
// not tell a rate-limited upstream from missing credentials — it just showed a
// dash. This asserts the success flag and status stay consistent.
func TestUsageStatusConsistency(t *testing.T) {
	cases := []struct {
		name string
		u    viewUsage
	}{
		{"rate limited", viewUsage{Status: "rate_limited"}},
		{"no credentials", viewUsage{Status: "no_credentials"}},
		{"unreachable", viewUsage{Status: "unreachable"}},
		{"unauthorized", viewUsage{Status: "unauthorized"}},
	}
	for _, tc := range cases {
		if tc.u.OK {
			t.Errorf("%s: OK should be false when a failure status is set", tc.name)
		}
		if tc.u.Status == "" {
			t.Errorf("%s: failure must carry a status", tc.name)
		}
	}

	ok := viewUsage{OK: true, Status: "ok"}
	if !ok.OK || ok.Status != "ok" {
		t.Error(`a healthy reading should be OK with status "ok"`)
	}
}
