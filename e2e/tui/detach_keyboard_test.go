package tuie2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuitest"
)

// A flags query sent while detaching can be answered after the client has
// stopped reading. The outer shell then receives ESC[?7u as input. Require a
// settled negotiation before detach so startup cannot account for the query.
// The dock case starts with flags 7 and catches a fix that always requests 31.
func TestDetachDoesNotQueryKeyboardFlags(t *testing.T) {
	for _, route := range []string{"leader", "menu", "dock"} {
		t.Run(route, func(t *testing.T) {
			base := t.TempDir()
			writeConfig(t, base, "[startup]\nopen_default_window = true\nstart_in_terminal_mode = true\n")
			stream := &hostStream{}
			term := startIn(t, base, startOpts{
				cols: 120, rows: 40, out: stream,
				args: []string{"new", "detach-keyboard", "--ascii-only"},
			})
			dir := artifactDir(t)
			t.Cleanup(func() {
				if err := os.WriteFile(filepath.Join(dir, "host-stream.bin"), stream.bytes(), 0o644); err != nil {
					t.Errorf("save host stream: %v", err)
				}
			})
			if err := term.WaitForText("$ ", bootTimeout); err != nil {
				t.Fatalf("shell never appeared: %v\n%s", err, term.Snapshot())
			}
			runInShell(t, term, `printf '\033[>3u'; echo KEY''READY`, "KEYREADY", shellTimeout)

			waitFlags := func(flags int, after int) {
				t.Helper()
				want := []byte(fmt.Sprintf("\x1b[=%d;1u\x1b[?u", flags))
				if err := term.WaitFor(func(tuitest.Screen) bool {
					return bytes.Contains(stream.bytes()[after:], want)
				}, uiTimeout); err != nil {
					t.Fatalf("host never requested flags %d: %v", flags, err)
				}
				sendRaw(t, term, hostAnswer(flags))
				time.Sleep(insertGuard)
			}
			waitFlags(7, 0)
			if route != "dock" {
				before := len(stream.bytes())
				sendRaw(t, term, "\x1b[98;5u", "\x1b[98;5:3u")
				waitFlags(31, before)
				if route == "menu" {
					sendRaw(t, term, "\x1b[113u")
					if err := term.WaitForText("Detach", uiTimeout); err != nil {
						t.Fatalf("quit menu never appeared: %v\n%s", err, term.Snapshot())
					}
				}
			}
			saveArtifact(t, term, dir, "before-detach")
			before := len(stream.bytes())
			if route == "dock" {
				x := -1
				for col := 119; col >= 108; col-- {
					if term.Screen().Cell(col, 39).Rune == 'd' {
						x = col
						break
					}
				}
				if x < 0 {
					t.Fatalf("dock leave button missing\n%s", term.Snapshot())
				}
				mousePress(t, term, x, 39, tuitest.MouseLeft, 0)
			} else {
				sendRaw(t, term, "\x1b[100u")
			}
			if code := waitExit(t, term, "after detach"); code != 0 {
				t.Fatalf("detach exited %d", code)
			}
			ending := stream.bytes()[before:]
			if bytes.Contains(ending, []byte("\x1b[?u")) {
				t.Errorf("detach sent a keyboard query after its input reader was about to stop: %q", ending)
			}
			if !bytes.Contains(ending, []byte("\x1b[=0;1u")) {
				t.Error("detach did not reset host keyboard flags")
			}
			if !sessionListed(t, base, "detach-keyboard") {
				t.Error("detach killed the session")
			}
		})
	}
}
