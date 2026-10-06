package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/tuios/internal/app"
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// TestRefocusClickRestatesThePointerShape is the regression test for the
// resize cursor seen after coming back to the window: a press is the first
// event a refocusing click delivers, and motion may not come until the pointer
// next moves, so the press restates the shape from its own position instead of
// leaving a shape left over from before the blur standing over content.
func TestRefocusClickRestatesThePointerShape(t *testing.T) {
	m := &app.OS{
		Settings:             config.Global,
		NumWorkspaces:        9,
		CurrentWorkspace:     1,
		WorkspaceFocus:       make(map[int]int),
		Width:                120,
		Height:               40,
		FocusedWindow:        0,
		PendingResizes:       make(map[string][2]int),
		WorkspaceHasCustom:   map[int]bool{},
		WorkspaceLayouts:     map[int][]app.WindowLayout{},
		WorkspaceMasterRatio: map[int]float64{},
	}
	ptyData := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ptyData:
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(done) })
	win := terminal.NewDaemonWindow("pointer-click-0001", "test", 0, 0, 60, 20, 0, "pty-pointer-click-0001", ptyData, config.DefaultScrollbackLines)
	if win == nil {
		t.Fatal("NewDaemonWindow returned nil")
	}
	t.Cleanup(win.Close)
	win.Workspace = 1
	m.Windows = []*terminal.Window{win}

	m.SetPointerShape(app.PointerNSResize)

	handleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 30, Y: 10}, m)

	// A press over floating content arms the move gesture, so the shape the
	// press states is grabbing; the assertion is that the stale resize shape
	// did not survive, whatever the press's own shape is.
	if got := m.PointerShapeNow(); got == app.PointerNSResize {
		t.Fatalf("a press over content left the pointer shape at %q; the stale shape survived the click", got)
	}
}
