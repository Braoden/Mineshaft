package cli

import (
	"sync"
	"testing"
)

func TestName_DefaultIsGt(t *testing.T) {
	// Reset singleton for test isolation
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("MS_COMMAND", "")

	got := Name()
	if got != "ms" {
		t.Errorf("Name() = %q, want %q", got, "ms")
	}
}

func TestName_RespectsGT_COMMAND(t *testing.T) {
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("MS_COMMAND", "mineshaft")

	got := Name()
	if got != "mineshaft" {
		t.Errorf("Name() = %q, want %q", got, "mineshaft")
	}
}

func TestName_OnceSemantics(t *testing.T) {
	nameOnce = sync.Once{}
	name = ""
	t.Setenv("MS_COMMAND", "first")

	first := Name()
	if first != "first" {
		t.Fatalf("Name() = %q, want %q", first, "first")
	}

	// Changing env after first call should have no effect (sync.Once)
	t.Setenv("MS_COMMAND", "second")
	second := Name()
	if second != "first" {
		t.Errorf("Name() returned %q after env change, want %q (sync.Once should cache)", second, "first")
	}
}
