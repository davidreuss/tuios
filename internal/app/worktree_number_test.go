package app

import (
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/sessiontree"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

func TestWorktreeGroupedRowsWearTheirNumbers(t *testing.T) {
	withSessionColors(t, true)
	m := newNarrowOS(t, 120, 40)
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

	repo := "tuios"
	tree := sessiontree.Build([]sessiontree.SessionInput{
		{Name: "main", Attached: true, IsCurrent: true, CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "aaaaaaaa1111", Title: "nvim", Focused: true, Workspace: 1},
		}},
		{Name: "fix-rail", Worktree: &sessiontree.WorktreeRef{Repo: repo, Branch: "fix-rail"}, CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "bbbbbbbb2222", Title: "sh", Workspace: 1},
		}},
		{Name: "api", CurrentWorkspace: 1, Windows: []sessiontree.WindowInput{
			{ID: "dddddddd4444", Title: "server", Workspace: 1},
		}},
	})

	m.PrefixActive = true
	for _, name := range []string{"main", "fix-rail", "api"} {
		row := railRowFor(t, m, tree, name)
		if !strings.Contains(row, "1 ") && !strings.Contains(row, "2 ") && !strings.Contains(row, "3 ") {
			t.Errorf("the %q row has no switch number: %q", name, row)
		}
	}
}
