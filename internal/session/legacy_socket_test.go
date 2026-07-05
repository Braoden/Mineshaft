package session

import (
	"fmt"
	"testing"
)

type mockLegacyTmux struct {
	sessions []string
	listErr  error
	killed   []string
	killErr  error
}

func (m *mockLegacyTmux) ListSessions() ([]string, error) {
	if len(m.killed) == 0 {
		return m.sessions, m.listErr
	}
	killed := make(map[string]bool, len(m.killed))
	for _, sess := range m.killed {
		killed[sess] = true
	}
	remaining := make([]string, 0, len(m.sessions))
	for _, sess := range m.sessions {
		if !killed[sess] {
			remaining = append(remaining, sess)
		}
	}
	return remaining, m.listErr
}

func (m *mockLegacyTmux) KillSessionWithProcesses(name string) error {
	if m.killErr != nil {
		return m.killErr
	}
	m.killed = append(m.killed, name)
	return nil
}

func setupLegacyHooks(t *testing.T, currentSocket string, mock *mockLegacyTmux) {
	t.Helper()

	origTmuxHook := legacyTmuxForTest
	origSocketHook := legacySocketForTest
	origRegistry := DefaultRegistry()
	t.Cleanup(func() {
		legacyTmuxForTest = origTmuxHook
		legacySocketForTest = origSocketHook
		SetDefaultRegistry(origRegistry)
	})

	legacySocketForTest = func() string { return currentSocket }
	legacyTmuxForTest = func(socket string) legacySocketTmux { return mock }

	r := NewPrefixRegistry()
	r.Register("ga", "mineshaft")
	SetDefaultRegistry(r)
}

func TestCleanupLegacyDefaultSocketSkipsWhenOnDefaultSocket(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyDefaultSocketSkipsWhenSocketIsDefault(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "default", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyDefaultSocketCleansMineshaftSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "hq-overseer"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 2 {
		t.Errorf("expected 2 cleaned, got %d", got)
	}
	if len(mock.killed) != 2 {
		t.Fatalf("expected 2 killed, got %d: %v", len(mock.killed), mock.killed)
	}
	want := map[string]bool{"ga-witness": true, "hq-overseer": true}
	for _, k := range mock.killed {
		if !want[k] {
			t.Errorf("unexpected kill: %s", k)
		}
	}
}

func TestCleanupLegacyDefaultSocketIgnoresNonMineshaftSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"personal-stuff", "hq-notes", "ga-witness"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 1 {
		t.Errorf("expected 1 cleaned, got %d", got)
	}
	if len(mock.killed) != 1 || mock.killed[0] != "ga-witness" {
		t.Errorf("expected only ga-witness killed, got %v", mock.killed)
	}
}

func TestCleanupLegacyDefaultSocketCleansSpecificTownSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"hq-supervisor", "hq-boot", "hq-dog-alpha", "hq-boss"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 3 {
		t.Errorf("expected 3 cleaned, got %d", got)
	}
	if len(mock.killed) != 3 {
		t.Fatalf("expected 3 killed, got %d: %v", len(mock.killed), mock.killed)
	}
	for _, killed := range mock.killed {
		if killed == "hq-boss" {
			t.Fatal("did not expect hq-boss to be killed")
		}
	}
}

func TestCleanupLegacyDefaultSocketNoDefaultServer(t *testing.T) {
	mock := &mockLegacyTmux{
		listErr: fmt.Errorf("no server running"),
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountLegacyDefaultSocketSkipsWhenOnDefault(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "", mock)

	got := CountLegacyDefaultSocketSessions()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountLegacyDefaultSocketCountsMineshaftOnly(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "personal", "hq-notes"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CountLegacyDefaultSocketSessions()
	if got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestCleanupLegacyBaseSocketSkipsWhenSameSocket(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "ms", mock)

	got := CleanupLegacyBaseSocket("/some/path/ms")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyBaseSocketCleansOldSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CleanupLegacyBaseSocket("/some/path/ms")
	if got != 1 {
		t.Errorf("expected 1 cleaned, got %d", got)
	}
	if len(mock.killed) != 1 || mock.killed[0] != "ga-witness" {
		t.Errorf("expected ga-witness killed, got %v", mock.killed)
	}
}

func TestCountLegacyBaseSocketSkipsWhenSame(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "ms", mock)

	got := CountLegacyBaseSocketSessions("/some/path/ms")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountLegacyBaseSocketCountsCorrectly(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "hq-supervisor", "random-thing"},
	}
	setupLegacyHooks(t, "ms-abc123", mock)

	got := CountLegacyBaseSocketSessions("/some/path/ms")
	if got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}
