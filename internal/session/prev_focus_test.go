package session

import (
	"testing"
	"time"
)

// The previous-pane record answers the last_pane chord: flip back to the pane
// focus came from. Daemon-owned focus moves record it beside the focus they
// move, so a jump routed from one client can be flipped back from another,
// and a merge keeps the fresher of two records.

func TestFocusDaemonWindowRecordsPrevFocus(t *testing.T) {
	sess := newTestSession(t)
	a, err := sess.AddDaemonWindow("shell", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow: %v", err)
	}
	b, err := sess.AddDaemonWindow("shell", nil)
	if err != nil {
		t.Fatalf("AddDaemonWindow: %v", err)
	}

	st := sess.GetState()
	if st.FocusedWindowID != b.ID {
		t.Fatalf("FocusedWindowID = %q, want the new pane %q", st.FocusedWindowID, b.ID)
	}
	if st.PrevFocusedID != a.ID || st.PrevFocusedAt == 0 {
		t.Fatalf("PrevFocusedID = %q at %d, want %q at a time", st.PrevFocusedID, st.PrevFocusedAt, a.ID)
	}

	if err := sess.FocusDaemonWindow(a.ID); err != nil {
		t.Fatalf("FocusDaemonWindow: %v", err)
	}
	st = sess.GetState()
	if st.FocusedWindowID != a.ID || st.PrevFocusedID != b.ID || st.PrevFocusedAt == 0 {
		t.Fatalf("after the jump back: focused %q prev %q at %d, want focused %q prev %q at a time",
			st.FocusedWindowID, st.PrevFocusedID, st.PrevFocusedAt, a.ID, b.ID)
	}
}

func TestOlderClientSyncKeepsDaemonPrevFocus(t *testing.T) {
	sess := newTestSession(t)
	if err := sess.mutateState(func(state *SessionState) error {
		state.PrevFocusedID = "daemon-prev"
		state.PrevFocusedAt = time.Now().UnixNano()
		return nil
	}); err != nil {
		t.Fatalf("seeding prev focus: %v", err)
	}

	push := clientSnapshot(sess)
	push.PrevFocusedID, push.PrevFocusedAt = "", 0
	if !sess.UpdateState(push) {
		t.Fatal("old client push was unexpectedly treated as stale")
	}
	if got := sess.GetState().PrevFocusedID; got != "daemon-prev" {
		t.Fatalf("PrevFocusedID after old client push = %q, want daemon-prev", got)
	}
}

func TestNewerClientPrevFocusWins(t *testing.T) {
	sess := newTestSession(t)
	if err := sess.mutateState(func(state *SessionState) error {
		state.PrevFocusedID = "older"
		state.PrevFocusedAt = time.Now().Add(-time.Minute).UnixNano()
		return nil
	}); err != nil {
		t.Fatalf("seeding prev focus: %v", err)
	}

	push := clientSnapshot(sess)
	push.PrevFocusedID = "newer"
	push.PrevFocusedAt = time.Now().UnixNano()
	if !sess.UpdateState(push) {
		t.Fatal("client push was unexpectedly treated as stale")
	}
	if got := sess.GetState().PrevFocusedID; got != "newer" {
		t.Fatalf("PrevFocusedID after client push = %q, want newer", got)
	}
}

func TestReconcileStaleTakesDaemonPrevFocus(t *testing.T) {
	canonical := &SessionState{PrevFocusedID: "d", PrevFocusedAt: 2}
	incoming := &SessionState{PrevFocusedID: "c", PrevFocusedAt: 1}
	reconcileStale(incoming, canonical, func(string) bool { return true })
	if incoming.PrevFocusedID != "d" || incoming.PrevFocusedAt != 2 {
		t.Fatalf("PrevFocusedID after reconcile = %q at %d, want d at 2",
			incoming.PrevFocusedID, incoming.PrevFocusedAt)
	}
}
