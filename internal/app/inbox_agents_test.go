package app

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// TestAgentsSortBreaksTiesOnIdentity: every sort order ties when two rows
// share its key, and a stable sort then keeps the tree's order, which moves
// as sessions and panes arrive in steps. Two rebuilds of the same rows a
// frame apart drew two different lists, which the frame stream showed as a
// strobe. The tie-break is the row's identity, so the order comes out by
// session then window whatever order the input arrived in, the same fix the
// rail's agents section took.
func TestAgentsSortBreaksTiesOnIdentity(t *testing.T) {
	ids := [][2]string{
		{"sess-c", "w2"}, {"sess-a", "w9"}, {"sess-b", "w1"},
		{"sess-a", "w1"}, {"sess-b", "w10"}, {"sess-c", "w1"},
	}
	want := slices.Clone(ids)
	slices.SortFunc(want, func(a, b [2]string) int {
		if c := strings.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return strings.Compare(a[1], b[1])
	})

	now := time.Now().Add(-time.Minute).UnixNano()
	m := &OS{}
	for mode := range agentsSortCount {
		entries := make([]sidebarAgentEntry, len(ids))
		for i, id := range ids {
			entries[i] = sidebarAgentEntry{
				SessionID: id[0], WindowID: id[1],
				Title: "same title", SessionLabel: "same session",
				State: "working", StateAt: now,
			}
		}
		// Churn the input the way the tree does between two rebuilds, so a
		// pass that inherits the input order fails the test.
		for i := range entries {
			j := (i*7 + 3) % len(entries)
			entries[i], entries[j] = entries[j], entries[i]
		}
		m.Inbox.agents.sort = mode
		got := m.agentsSort(entries)
		for i, e := range got {
			if e.SessionID != want[i][0] || e.WindowID != want[i][1] {
				t.Errorf("sort %d row %d = (%s, %s), want (%s, %s)",
					mode, i, e.SessionID, e.WindowID, want[i][0], want[i][1])
			}
		}
	}
}
