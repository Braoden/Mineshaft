package minecart

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMinecartsWriteConcurrentWithView verifies that updating m.minecarts
// concurrently with View() does not trigger data races.
func TestMinecartsWriteConcurrentWithView(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: simulate fetchMinecartsMsg updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.mu.Lock()
			m.minecarts = []MinecartItem{
				{ID: "hq-abc", Title: "Test Minecart", Status: "open",
					Issues:   []IssueItem{{ID: "ms-xyz", Title: "Fix bug", Status: "open"}},
					Progress: "0/1", Expanded: true},
			}
			m.mu.Unlock()
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestToggleExpandConcurrentWithView verifies that toggling minecart expansion
// while View() renders does not race.
func TestToggleExpandConcurrentWithView(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	// Pre-populate minecarts
	m.minecarts = []MinecartItem{
		{ID: "hq-abc", Title: "Minecart 1", Status: "open",
			Issues:   []IssueItem{{ID: "ms-1", Title: "Issue 1", Status: "open"}},
			Progress: "0/1", Expanded: false},
		{ID: "hq-def", Title: "Minecart 2", Status: "open",
			Issues:   []IssueItem{{ID: "ms-2", Title: "Issue 2", Status: "open"}},
			Progress: "0/1", Expanded: false},
	}

	var wg sync.WaitGroup

	// Writer goroutine: toggle expansion
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.mu.Lock()
			m.toggleExpandLocked()
			m.mu.Unlock()
		}
	}()

	// Reader goroutine: render
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestCursorToMinecartIndexLocked verifies correct cursor-to-minecart mapping.
func TestCursorToMinecartIndexLocked(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.minecarts = []MinecartItem{
		{ID: "hq-abc", Title: "C1", Status: "open",
			Issues: []IssueItem{
				{ID: "ms-1", Title: "I1", Status: "open"},
				{ID: "ms-2", Title: "I2", Status: "closed"},
			},
			Expanded: true},
		{ID: "hq-def", Title: "C2", Status: "open",
			Issues: []IssueItem{
				{ID: "ms-3", Title: "I3", Status: "open"},
			},
			Expanded: false},
	}

	tests := []struct {
		cursor    int
		wantConv  int
		wantIssue int
	}{
		{0, 0, -1},  // First minecart header
		{1, 0, 0},   // First issue of first minecart
		{2, 0, 1},   // Second issue of first minecart
		{3, 1, -1},  // Second minecart header (collapsed)
		{4, -1, -1}, // Beyond last item
	}

	for _, tc := range tests {
		m.cursor = tc.cursor
		m.mu.RLock()
		ci, ii := m.cursorToMinecartIndexLocked()
		m.mu.RUnlock()

		if ci != tc.wantConv || ii != tc.wantIssue {
			t.Errorf("cursor=%d: got (%d, %d), want (%d, %d)",
				tc.cursor, ci, ii, tc.wantConv, tc.wantIssue)
		}
	}
}

// TestMaxCursorLocked verifies correct max cursor calculation.
func TestMaxCursorLocked(t *testing.T) {
	m := New("/tmp/fake-beads")

	// Empty
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 0 {
		t.Errorf("empty: maxCursor = %d, want 0", got)
	}
	m.mu.RUnlock()

	// One minecart, collapsed
	m.minecarts = []MinecartItem{
		{ID: "hq-abc", Issues: []IssueItem{{ID: "ms-1"}}, Expanded: false},
	}
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 0 {
		t.Errorf("1 collapsed: maxCursor = %d, want 0", got)
	}
	m.mu.RUnlock()

	// One minecart, expanded with 2 issues
	m.minecarts[0].Expanded = true
	m.minecarts[0].Issues = append(m.minecarts[0].Issues, IssueItem{ID: "ms-2"})
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 2 {
		t.Errorf("1 expanded w/2 issues: maxCursor = %d, want 2", got)
	}
	m.mu.RUnlock()
}

// TestViewConcurrentWithWindowResize verifies that View and WindowSizeMsg
// updates can run concurrently without data races on width/height/help.
func TestViewConcurrentWithWindowResize(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.minecarts = []MinecartItem{
		{ID: "hq-abc", Title: "Test", Status: "open",
			Issues: []IssueItem{{ID: "ms-1", Title: "Issue", Status: "open"}},
			Progress: "0/1", Expanded: true},
	}
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: send WindowSizeMsg via Update
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(tea.WindowSizeMsg{Width: 80 + i, Height: 40 + i})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestViewConcurrentWithCursorNavigation verifies that View and cursor
// key handlers can run concurrently without data races.
func TestViewConcurrentWithCursorNavigation(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.minecarts = []MinecartItem{
		{ID: "hq-abc", Title: "C1", Status: "open",
			Issues: []IssueItem{
				{ID: "ms-1", Title: "I1", Status: "open"},
				{ID: "ms-2", Title: "I2", Status: "open"},
			},
			Progress: "0/2", Expanded: true},
		{ID: "hq-def", Title: "C2", Status: "open",
			Issues: []IssueItem{{ID: "ms-3", Title: "I3", Status: "open"}},
			Progress: "0/1", Expanded: true},
	}
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: navigate up/down and toggle help
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
			m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestViewConcurrentWithFetchMinecarts verifies that View and fetchMinecartsMsg
// via Update can run concurrently without data races.
func TestViewConcurrentWithFetchMinecarts(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: send fetchMinecartsMsg via Update
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(fetchMinecartsMsg{
				minecarts: []MinecartItem{
					{ID: "hq-abc", Title: "Test", Status: "open",
						Issues:   []IssueItem{{ID: "ms-1", Title: "I1", Status: "open"}},
						Progress: "0/1", Expanded: true},
				},
			})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}
