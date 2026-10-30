// End-to-end: the real controller against shimmed tuios and nvim binaries
// and a fixture git repo. Freezes the rig's hard-won behavior: adopt over
// create, baked docs per step, stale flip on a HEAD move, recoverable
// handlers.
package walk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/testutil"
)

// shimBin writes script shims for tuios and nvim and returns a PATH that
// finds them first. Every shim appends its invocation to calls.txt; tuios
// answers from canned responses.
func shimBin(t *testing.T) (dir string) {
	t.Helper()
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "tuios"), `#!/bin/sh
echo "$@" >> "${TEST_STATE}/calls.txt"
case "$1 $2" in
  "tml list")
    echo "{\"surfaces\":[{\"surface\":\"tml-test0001\",\"title\":\"tui review\"}]}" ;;
  "tml replace") echo "{\"type\":\"tml_replaced\"}" ;;
  "tml patch")   echo "{\"type\":\"tml_patched\"}" ;;
  "call tml-open") echo "tml-test0001" ;;
  "subscribe")
    cat "${TEST_STATE}/events.txt" 2>/dev/null || true
    sleep 30 ;;
  *) ;;
esac
`)
	writeFile(t, filepath.Join(bin, "nvim"), `#!/bin/sh
echo "$@" >> "${TEST_STATE}/calls.txt"
exit 0
`)
	for _, name := range []string{"tuios", "nvim"} {
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func TestControllerWalksFeedsAndStales(t *testing.T) {
	repo := testutil.GitRepo(t)
	testutil.Git(t, repo, "config", "user.email", "t@t")
	testutil.Git(t, repo, "config", "user.name", "t")
	writeFile(t, repo+"/thing.go", "package main\n\nfunc Thing() {\n\t// base\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "base")
	testutil.Git(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, repo+"/thing.go", "package main\n\nfunc Thing(id string) {\n\t// now takes an id\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "feat: thing takes an id")

	guide := []GuideStep{{Key: "s1", Chapter: "ch1", Label: "thing takes an id",
		File: "thing.go", Lnum: 3, Endl: 5, Desc: "Thing now carries an id."}}
	gb, err := jsonMarshal(guide)
	if err != nil {
		t.Fatal(err)
	}

	state := t.TempDir()
	t.Setenv("TEST_STATE", state)
	t.Setenv("PATH", shimBin(t)+":"+os.Getenv("PATH"))
	events := filepath.Join(state, "events.txt")
	ev := map[string]any{
		"type": "tml-event", "tml": map[string]any{"surface": "tml-test0001", "name": "next"},
	}
	b, _ := json.Marshal(ev)
	if err := os.WriteFile(events, []byte(string(b)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := StartController(ctx, Options{
			Dir: repo, Base: "main", RigID: "walk-test", Session: "s",
			DerivedFrom: "pr-595", BakeDoc: true,
			GuideJSON: gb, LogPath: filepath.Join(state, "ctl.log"),
		})
		done <- err
	}()

	// The walkthrough: two replaces - the open step, then the walked step.
	deadline := time.Now().Add(15 * time.Second)
	walked := false
	for time.Now().Before(deadline) {
		calls, _ := os.ReadFile(filepath.Join(state, "calls.txt"))
		if strings.Count(string(calls), "tml replace") >= 2 {
			walked = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Errorf("controller: %v", err)
	}
	if !walked {
		log, _ := os.ReadFile(filepath.Join(state, "ctl.log"))
		t.Fatalf("controller never baked the walked step; ctl log:\n%s", log)
	}

	calls, _ := os.ReadFile(filepath.Join(state, "calls.txt"))
	s := string(calls)
	if !strings.Contains(s, "subscribe") {
		t.Errorf("controller never subscribed to the event stream")
	}
	// Adopt over create: the fixture tml list already shows a surface, so
	// tml-open must never run.
	if strings.Contains(s, "tml-open") {
		t.Errorf("controller created a surface although one existed to adopt")
	}
	// The baked doc carries the walked state.
	live, err := os.ReadFile(filepath.Join(os.TempDir(), "tui-review-walk-test-live.tml"))
	if err != nil {
		t.Fatalf("no live doc: %v", err)
	}
	d := string(live)
	for _, want := range []string{"1/1 thing takes an id", "thing.go L3-5", "+func Thing(id string)", `"derived_from":"pr-595"`} {
		if !strings.Contains(d, want) {
			t.Errorf("live doc misses %q", want)
		}
	}
}

func TestStaleFlipsOnHeadMove(t *testing.T) {
	repo := testutil.GitRepo(t)
	testutil.Git(t, repo, "config", "user.email", "t@t")
	testutil.Git(t, repo, "config", "user.name", "t")
	writeFile(t, repo+"/a.go", "package a\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "base")
	testutil.Git(t, repo, "checkout", "-q", "-b", "feature")

	head := strings.TrimSpace(testutil.Git(t, repo, "log", "--format=%H", "-1"))

	state := t.TempDir()
	t.Setenv("TEST_STATE", state)
	t.Setenv("PATH", shimBin(t)+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(state, "events.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		_, _ = StartController(ctx, Options{
			Dir: repo, Base: "main", RigID: "stale-test", Session: "s",
			LogPath: filepath.Join(state, "stale-ctl.log"),
		})
	}()

	// Move HEAD under the walking controller.
	testutil.Git(t, repo, "commit", "-q", "--allow-empty", "-m", "the agent pushed a fix")

	deadline := time.Now().Add(12 * time.Second)
	staled := false
	for time.Now().Before(deadline) {
		log, _ := os.ReadFile(filepath.Join(state, "stale-ctl.log"))
		if strings.Contains(string(log), "stale") {
			staled = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	if !staled {
		dump, _ := os.ReadFile(filepath.Join(state, "stale-ctl.log"))
		t.Fatalf("controller never noticed HEAD move (head was %s); ctl log:\n%s", head, dump)
	}
}

// The single-writer lock: a live controller refuses a second one, --takeover
// overrides, and a crashed controller's stale pid file unlocks.
func TestOwnershipRefusesAndTakesOver(t *testing.T) {
	const rig = "ownership-test"
	t.Cleanup(func() { releaseOwnership(rig) })

	if err := claimOwnership(rig, false); err != nil {
		t.Fatalf("fresh claim: %v", err)
	}
	if err := claimOwnership(rig, false); err == nil ||
		!strings.Contains(err.Error(), "--takeover") {
		t.Fatalf("second claim: err=%v, want a refusal naming --takeover", err)
	}
	if err := claimOwnership(rig, true); err != nil {
		t.Fatalf("takeover claim: %v", err)
	}

	// A pid that can never exist (macOS pids stay below 99998) stands for
	// a crashed controller: the next claim must win without --takeover.
	if err := os.WriteFile(pidFile(rig), []byte("999999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := claimOwnership(rig, false); err != nil {
		t.Fatalf("claim over a dead pid: %v", err)
	}
}

// Bound content (the default): the walk moves through ONE state patch, the
// daemon re-renders the bound diff. Exactly one replace installs the doc on
// adoption; the step never rewrites it.
func TestControllerWalksBoundContent(t *testing.T) {
	repo := testutil.GitRepo(t)
	testutil.Git(t, repo, "config", "user.email", "t@t")
	testutil.Git(t, repo, "config", "user.name", "t")
	writeFile(t, repo+"/thing.go", "package main\n\nfunc Thing() {\n\t// base\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "base")
	testutil.Git(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, repo+"/thing.go", "package main\n\nfunc Thing(id string) {\n\t// now takes an id\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "feat: thing takes an id")

	guide := []GuideStep{{Key: "s1", Chapter: "ch1", Label: "thing takes an id",
		File: "thing.go", Lnum: 3, Endl: 5, Desc: "Thing now carries an id."}}
	guideJSON, err := jsonMarshal(guide)
	if err != nil {
		t.Fatal(err)
	}

	state := t.TempDir()
	t.Setenv("TEST_STATE", state)
	t.Setenv("PATH", shimBin(t)+":"+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(state, "events.txt"),
		[]byte(`{"type":"tml-event","tml":{"surface":"tml-test0001","name":"value","value":"next"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The live file persists across runs; drop it so the later read only
	// ever sees THIS controller's bake.
	_ = os.Remove(filepath.Join(os.TempDir(), "tui-review-bound-live.tml"))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := StartController(ctx, Options{
			Dir: repo, Base: "main", RigID: "bound", Session: "s",
			DerivedFrom: "pr-595", GuideJSON: guideJSON,
			LogPath: filepath.Join(state, "ctl.log"),
		})
		done <- err
	}()

	deadline := time.Now().Add(15 * time.Second)
	patched := false
	for time.Now().Before(deadline) {
		calls, _ := os.ReadFile(filepath.Join(state, "calls.txt"))
		s := string(calls)
		if strings.Contains(s, "tml patch") {
			patched = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil && err != context.Canceled {
		t.Errorf("controller: %v", err)
	}
	if !patched {
		log, _ := os.ReadFile(filepath.Join(state, "ctl.log"))
		t.Fatalf("bound walk never patched the step; ctl log:\n%s", log)
	}

	calls, _ := os.ReadFile(filepath.Join(state, "calls.txt"))
	s := string(calls)
	if n := strings.Count(s, "tml replace"); n != 1 {
		t.Errorf("bound walk ran %d replaces, want 1 (the adoption install)", n)
	}
	if !strings.Contains(s, `"\"hunk\"`) && !strings.Contains(s, "hunk") {
		t.Errorf("the walk patch carries no hunk")
	}
	if !strings.Contains(s, "+func Thing(id string)") {
		t.Errorf("the walk patch carries no diff text")
	}
}
