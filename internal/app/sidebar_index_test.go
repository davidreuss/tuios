package app

import (
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/sessiontree"
	"github.com/charmbracelet/x/ansi"
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

// TestRailRowsWearTheirSwitchNumbers: a session row leads with the muted
// number switch_session_N opens. show_numbers defaults on, so the number is
// always drawn; with it off the rail is quiet and the number waits for the
// switch chord that gives it a question to answer.
func TestRailRowsWearTheirSwitchNumbers(t *testing.T) {
	withSessionColors(t, true)
	m, tree := sessionColorOS(t, 120, 40)

	idle := railRowFor(t, m, tree, "api")
	if !strings.Contains(idle, "2 api") {
		t.Errorf("the idle rail does not lead the api session with its switch number: %q", idle)
	}

	m.Settings.SidebarShowNumbers = false
	quiet := railRowFor(t, m, tree, "api")
	if strings.Contains(quiet, "2 api") {
		t.Errorf("the quiet rail shows the switch number the chord has not asked for: %q", quiet)
	}

	m.PrefixActive = true
	armed := railRowFor(t, m, tree, "api")
	if !strings.Contains(armed, "2 api") {
		t.Errorf("the armed rail does not lead the api session with its switch number: %q", armed)
	}
}

// TestTheRailCacheFollowsTheChord: the render cache's signature folds the
// chord state, so arming the prefix cannot serve rows drawn without it. The
// rows come from the cache's own build path, which in a test model knows only
// the attached session.
func TestTheRailCacheFollowsTheChord(t *testing.T) {
	withSessionColors(t, true)
	m, _ := sessionColorOS(t, 120, 40)
	m.Settings.SidebarShowNumbers = false
	_, _ = m.sidebarPanelLines()

	m.PrefixActive = true
	lines, _ := m.sidebarPanelLines()
	for _, l := range lines {
		if plain := ansi.Strip(l); strings.Contains(plain, "1 main") {
			return
		}
	}
	t.Error("arming the chord did not refresh the cached rail")
}
