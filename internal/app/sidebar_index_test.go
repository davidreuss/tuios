package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/sessiontree"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
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

// TestRailRowsWearTheirSwitchNumbers: with show_numbers on, a session row
// leads with the muted number switch_session_N opens.
func TestRailRowsWearTheirSwitchNumbers(t *testing.T) {
	withSessionColors(t, true)
	withSidebarNumbers(t)
	m, tree := sessionColorOS(t, 120, 40)

	row := railRowFor(t, m, tree, "api")
	if !strings.Contains(row, "2 api") {
		t.Errorf("the rail does not lead the api session with its switch number: %q", row)
	}
}

// TestRailRowsStayQuietWithoutShowNumbers: the numbers are opt-in, and the
// default rail carries only the names.
func TestRailRowsStayQuietWithoutShowNumbers(t *testing.T) {
	withSessionColors(t, true)
	m, tree := sessionColorOS(t, 120, 40)

	row := railRowFor(t, m, tree, "api")
	if strings.Contains(row, "2 api") {
		t.Errorf("the default rail shows a switch number with show_numbers off: %q", row)
	}
}

// withSidebarNumbers turns show_numbers on for one test and puts it back.
func withSidebarNumbers(t *testing.T) {
	t.Helper()
	prev := config.Global.SidebarShowNumbers
	config.Global.SidebarShowNumbers = true
	t.Cleanup(func() { config.Global.SidebarShowNumbers = prev })
}

// TestRailNumbersReserveTheirColumn: with show_numbers on and a rail whose
// sessions run past nine, the tenth session's row reserves the number's two
// cells blank instead of dropping them, so every name starts on one column
// and a worktree group's tree marks hold a single depth instead of drifting
// with each row's number.
func TestRailNumbersReserveTheirColumn(t *testing.T) {
	withSessionColors(t, true)
	withSidebarNumbers(t)
	m := newNarrowOS(t, 120, 60)
	m.CurrentWorkspace = 1
	m.SessionName = "main"
	m.Windows = []*terminal.Window{
		{ID: "aaaaaaaa1111", CustomName: "nvim", Width: 40, Height: 20, Workspace: 1},
	}
	m.FocusedWindow = 0
	m.DaemonClient = &session.TUIClient{}
	m.IsDaemonSession = true
	withSidebar(t, true, "left", config.SidebarDefaultWidth)
	m.Settings = config.Global
	m.SidebarOrder = nil

	inputs := []sessiontree.SessionInput{
		{Name: "main", Attached: true, IsCurrent: true, CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "aaaaaaaa1111", Title: "nvim", Focused: true, Workspace: 1},
		}},
	}
	// One repository with eleven worktree sessions: their switch numbers run
	// 2..12, so the group straddles the 9 boundary the numbers stop at.
	for i := 1; i <= 11; i++ {
		inputs = append(inputs, sessiontree.SessionInput{
			Name:     fmt.Sprintf("ld-%02d", i),
			Worktree: &sessiontree.WorktreeRef{Repo: "lets-deploy", Branch: fmt.Sprintf("branch-%02d", i)},
		})
	}
	tree := sessiontree.Build(inputs)

	lines, _ := m.sidebarPanelLinesForTree(tree)
	cols := map[int]int{}
	for _, l := range lines {
		plain := ansi.Strip(l)
		if i := strings.Index(plain, "├─"); i >= 0 {
			cols[i]++
		}
	}
	if len(cols) == 0 {
		t.Fatalf("the rail drew no worktree tree marks:\n%s", strings.Join(lines, "\n"))
	}
	if len(cols) > 1 {
		t.Errorf("the rail drew worktree tree marks at %d depths (%v); want one column", len(cols), cols)
	}
	for _, l := range lines {
		plain := ansi.Strip(l)
		if strings.Contains(plain, "branch-08") {
			if !strings.Contains(plain, "9 ") {
				t.Errorf("the ninth session's row lost its switch number: %q", plain)
			}
		}
		if strings.Contains(plain, "branch-09") {
			if strings.Contains(plain, "10 ") {
				t.Errorf("the tenth session's row wears a number switch_session_10 does not exist for: %q", plain)
			}
			mark := strings.Index(plain, "├─")
			nine := ""
			for _, other := range lines {
				if s := ansi.Strip(other); strings.Contains(s, "branch-08") {
					nine = s
				}
			}
			if nine != "" && strings.Index(nine, "├─") != mark {
				t.Errorf("tree marks drifted: ninth row %q vs tenth row %q", nine, plain)
			}
		}
	}
}
