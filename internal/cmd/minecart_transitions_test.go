package cmd

import (
	"strings"
	"testing"
)

func TestEnsureKnownMinecartStatus(t *testing.T) {
	t.Parallel()

	if err := ensureKnownMinecartStatus("open"); err != nil {
		t.Fatalf("expected open to be accepted: %v", err)
	}
	if err := ensureKnownMinecartStatus(" closed "); err != nil {
		t.Fatalf("expected closed to be accepted: %v", err)
	}
	if err := ensureKnownMinecartStatus("in_progress"); err == nil {
		t.Fatal("expected unknown status to be rejected")
	}
}

func TestValidateMinecartStatusTransition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		target  string
		wantErr bool
	}{
		{name: "open to closed", current: "open", target: "closed", wantErr: false},
		{name: "closed to open", current: "closed", target: "open", wantErr: false},
		{name: "same open", current: "open", target: "open", wantErr: false},
		{name: "same closed", current: "closed", target: "closed", wantErr: false},
		{name: "unknown current", current: "in_progress", target: "closed", wantErr: true},
		{name: "unknown target", current: "open", target: "archived", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateMinecartStatusTransition(tc.current, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for transition %q -> %q", tc.current, tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %q -> %q to pass, got %v", tc.current, tc.target, err)
			}
		})
	}
}

func TestEnsureKnownMinecartStatus_Staged(t *testing.T) {
	t.Parallel()

	// staged_ready should be accepted
	if err := ensureKnownMinecartStatus("staged_ready"); err != nil {
		t.Fatalf("expected staged_ready to be accepted: %v", err)
	}

	// staged_warnings should be accepted
	if err := ensureKnownMinecartStatus("staged_warnings"); err != nil {
		t.Fatalf("expected staged_warnings to be accepted: %v", err)
	}

	// staged_unknown should be rejected
	if err := ensureKnownMinecartStatus("staged_unknown"); err == nil {
		t.Fatal("expected staged_unknown to be rejected")
	}

	// STAGED_READY (uppercase) should be accepted via normalization
	if err := ensureKnownMinecartStatus("STAGED_READY"); err != nil {
		t.Fatalf("expected STAGED_READY to be accepted (normalization): %v", err)
	}

	// Verify error message includes all valid statuses
	err := ensureKnownMinecartStatus("bogus")
	if err == nil {
		t.Fatal("expected bogus to be rejected")
	}
	msg := err.Error()
	for _, want := range []string{"open", "closed", "staged_ready", "staged_warnings"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q should mention %q", msg, want)
		}
	}
}

func TestValidateMinecartStatusTransition_Staged(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		target  string
		wantErr bool
	}{
		// staged → open (launch)
		{name: "staged_ready to open", current: "staged_ready", target: "open", wantErr: false},
		{name: "staged_warnings to open", current: "staged_warnings", target: "open", wantErr: false},

		// staged → closed (cancel)
		{name: "staged_ready to closed", current: "staged_ready", target: "closed", wantErr: false},
		{name: "staged_warnings to closed", current: "staged_warnings", target: "closed", wantErr: false},

		// staged identity
		{name: "staged_ready to staged_ready", current: "staged_ready", target: "staged_ready", wantErr: false},
		{name: "staged_warnings to staged_warnings", current: "staged_warnings", target: "staged_warnings", wantErr: false},

		// staged ↔ staged (re-stage)
		{name: "staged_ready to staged_warnings", current: "staged_ready", target: "staged_warnings", wantErr: false},
		{name: "staged_warnings to staged_ready", current: "staged_warnings", target: "staged_ready", wantErr: false},

		// REJECTED: open → staged_*
		{name: "open to staged_ready rejected", current: "open", target: "staged_ready", wantErr: true},
		{name: "open to staged_warnings rejected", current: "open", target: "staged_warnings", wantErr: true},

		// REJECTED: closed → staged_*
		{name: "closed to staged_ready rejected", current: "closed", target: "staged_ready", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateMinecartStatusTransition(tc.current, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for transition %q -> %q", tc.current, tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %q -> %q to pass, got %v", tc.current, tc.target, err)
			}
		})
	}
}
