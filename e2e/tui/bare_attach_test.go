package tuie2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuitest"
)

// Bare attach with no name must land on the session a client was last
// watching, not the one agents and scripts were last typing into.
//
// The shape that decides it: a client watches session A and detaches; work
// then happens in session B, whose keystrokes make it the most recently
// active. A bare attach now has two candidate answers. Activity picks B, and
// B is wrong: nobody was looking at it. Presence picks A, which is where the
// person left their screen.
//
// What would pass a weaker test and fail this one: a bare attach that falls
// back to the default session when no activity ranking exists, a client that
// re-attaches to its own last session by accident of the harness reusing the
// connection, or a presence record a detach forgets to write. The listing is
// read from the daemon, so only a real attach to A can satisfy it.
func TestBareAttachReturnsToTheWatchedSession(t *testing.T) {
	base := t.TempDir()
	killDaemon(t, base)
	for _, name := range []string{"e2e-watched", "e2e-busy"} {
		if out, err := tuiosCLI(t, base, "new", name, "--detach"); err != nil {
			t.Fatalf("create %s: %v\n%s", name, err, out)
		}
	}

	watcher := startIn(t, base, startOpts{args: []string{"attach", "e2e-watched"}})
	if err := watcher.WaitFor(func(s tuitest.Screen) bool { return countWindows(s) == 1 }, bootTimeout); err != nil {
		t.Fatalf("the watcher never attached: %v\n%s", err, watcher.Snapshot())
	}
	if err := watcher.SendKeys(tuitest.Ctrl('b'), "d"); err != nil {
		t.Fatalf("detach the watcher: %v", err)
	}
	waitExit(t, watcher, "after the watcher's detach")

	busy := startIn(t, base, startOpts{args: []string{"attach", "e2e-busy"}})
	if err := busy.WaitFor(func(s tuitest.Screen) bool { return countWindows(s) == 1 }, bootTimeout); err != nil {
		t.Fatalf("the busy client never attached: %v\n%s", err, busy.Snapshot())
	}
	// Keystrokes in e2e-busy make it the most recently active session, which
	// is the wrong answer the presence rule has to beat. Wait for the echo so
	// the activity has landed in the daemon before the bare attach asks.
	if err := busy.SendKeys("echo BUSYMARK-tuios-e2e", tuitest.Enter); err != nil {
		t.Fatalf("type into the busy session: %v", err)
	}
	if err := busy.WaitForText("BUSYMARK-tuios-e2e", shellTimeout); err != nil {
		t.Fatalf("the busy session never took the keystrokes: %v\n%s", err, busy.Snapshot())
	}
	time.Sleep(500 * time.Millisecond)

	bare := startIn(t, base, startOpts{args: []string{"attach"}})
	if err := bare.WaitFor(func(s tuitest.Screen) bool { return countWindows(s) == 1 }, bootTimeout); err != nil {
		t.Fatalf("the bare attach never came up: %v\n%s", err, bare.Snapshot())
	}

	type clientRow struct {
		Session string `json:"session"`
	}
	out, err := tuiosCLI(t, base, "list-clients", "--json")
	if err != nil {
		t.Fatalf("list clients: %v\n%s", err, out)
	}
	var rows []clientRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode clients: %v\n%s", err, out)
	}
	watched := false
	busyAttached := false
	for _, row := range rows {
		if row.Session == "e2e-watched" {
			watched = true
		}
		if row.Session == "e2e-busy" {
			busyAttached = true
		}
	}
	if !busyAttached {
		t.Fatalf("the busy client dropped out before the bare attach\n%s", out)
	}
	if !watched {
		t.Fatalf("the bare attach did not land on the watched session\nclients:\n%s", out)
	}
}
