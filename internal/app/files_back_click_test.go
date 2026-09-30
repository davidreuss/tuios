package app

import (
	"strings"
	"testing"
)

// TestRailFilesBackTokenClicks walks the listing by mouse and takes the back
// token by mouse, the one route that cannot be tested below the render: the
// token is only clickable where the draw published it.
func TestRailFilesBackTokenClicks(t *testing.T) {
	dir := fileViewTree(t)
	m := filesOS(t, dir, "")

	var entry sidebarRowHit
	for _, h := range m.SidebarHits {
		if h.Kind == sidebarRowFileEntry && h.WindowID == "apple" {
			entry = h
		}
	}
	if entry.Kind != sidebarRowFileEntry {
		t.Fatalf("the rail published no row for \"apple\": %v", entryNames(m))
	}
	if !m.SidebarClick(entry.X0+2, entry.Y0, false) {
		t.Fatal("the rail refused a press on the folder row")
	}
	cmd := m.TakeSidebarCmd()
	if cmd == nil {
		t.Fatal("entering the folder scheduled no read")
	}
	msg, ok := cmd().(fileListMsg)
	if !ok {
		t.Fatalf("the read answered with %T, not a listing", msg)
	}
	m.HandleFileList(msg)

	before := m.FileBackDir()
	if before != dir {
		t.Fatalf("the stack offers %q before the render, want %q", before, dir)
	}

	lines := railLines(t, m)
	var back sidebarRowHit
	found := false
	for _, h := range m.SidebarHits {
		if h.Kind == sidebarRowFileBack {
			back, found = h, true
		}
	}
	if !found {
		for i, ln := range lines {
			t.Logf("row %2d: %q", i, ln)
		}
		t.Fatalf("a walk with a step behind it published no back token (stack offers %q, origin %q)", m.FileBackDir(), m.filesView.Origin)
	}
	if back.Y0 >= len(lines) {
		t.Fatalf("the back token's row %d is off the drawn rail", back.Y0)
	}
	cells := []rune(lines[back.Y0])
	if back.X1 > len(cells) || back.X0 >= back.X1 {
		t.Fatalf("the back token's span [%d,%d) does not fit the row %q", back.X0, back.X1, lines[back.Y0])
	}
	drawn := string(cells[back.X0:back.X1])
	if !strings.HasPrefix(strings.TrimSpace(drawn), "‹") && !strings.HasPrefix(strings.TrimSpace(drawn), "<") {
		t.Fatalf("the columns the back token claims hold %q, not the token", drawn)
	}

	if !m.SidebarClick(back.X0, back.Y0, false) {
		t.Fatal("the rail refused a press on the back token")
	}
	cmd = m.TakeSidebarCmd()
	if cmd == nil {
		t.Fatal("a press on the back token scheduled no read")
	}
	msg, ok = cmd().(fileListMsg)
	if !ok {
		t.Fatalf("the back read answered with %T, not a listing", msg)
	}
	m.HandleFileList(msg)
	if m.filesView.Dir != dir {
		t.Fatalf("back landed on %q, want the directory the walk started from", m.filesView.Dir)
	}
}
