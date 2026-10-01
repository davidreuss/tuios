package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/Gaurav-Gosain/tuios/internal/sessiontree"
)

// railRowFor finds the drawn row naming a session, stripped of styling.
func railRowFor(t *testing.T, m *OS, tree sessiontree.Tree, name string) string {
	t.Helper()
	lines, _ := m.sidebarPanelLinesForTree(tree)
	for _, l := range lines {
		if plain := ansi.Strip(l); strings.Contains(plain, name) {
			return plain
		}
	}
	t.Fatalf("the rail has no row naming %q", name)
	return ""
}

// TestChordSwapsGutterForIndex: the switch numbers live in the gutter and
// only while a prefix chord is pending. An idle rail wears no numbers at
// all, and a resolved chord hands the cell back to the marks.
func TestChordSwapsGutterForIndex(t *testing.T) {
	withSessionColors(t, true)
	m, tree := sessionColorOS(t, 120, 40)

	idle := railRowFor(t, m, tree, "api")
	if strings.HasPrefix(strings.TrimSpace(idle), "2") {
		t.Errorf("an idle rail shows the switch number in the gutter: %q", idle)
	}

	m.PrefixActive = true
	pending := railRowFor(t, m, tree, "api")
	if !strings.HasPrefix(strings.TrimSpace(pending), "2") {
		t.Errorf("a pending chord left the switch number out of the gutter: %q", pending)
	}

	m.PrefixActive = false
	back := railRowFor(t, m, tree, "api")
	if back != idle {
		t.Errorf("a resolved chord did not hand the gutter back: %q, want %q", back, idle)
	}
}

// TestChordBustsTheRailCache: the gutter swap is drawn state, so a chord
// arming must change the signature the cache is keyed on or the rail serves
// the idle frame.
func TestChordBustsTheRailCache(t *testing.T) {
	withSessionColors(t, true)
	m, _ := sessionColorOS(t, 120, 40)

	base := m.sidebarSignature()
	m.PrefixActive = true
	if m.sidebarSignature() == base {
		t.Error("arming the prefix chord left the rail cache key unchanged")
	}
}
