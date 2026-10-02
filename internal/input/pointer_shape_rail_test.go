package input

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Gaurav-Gosain/tuios/internal/app"
	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// The rail consumes motion over its band, and that early return used to skip
// the pointer-shape updater, so a resize shape picked up on the pane edge
// beside the rail stuck while the pointer wandered the rail. Consuming the
// motion must still retire the shape.

type railHostWriter struct{ b strings.Builder }

func (h *railHostWriter) Write(p []byte) (int, error) { return h.b.Write(p) }

func TestRailMotionRetiresThePointerShape(t *testing.T) {
	withSidebarGlobals(t, "right")

	host := &railHostWriter{}
	win := &terminal.Window{ID: "w1", X: 0, Y: 0, Width: 120, Height: 39, Workspace: 1}
	o := app.NewOS(app.OSOptions{
		UserConfig:      config.DefaultConfig(),
		KeybindRegistry: config.NewKeybindRegistry(config.DefaultConfig()),
	})
	o.KittyPassthrough = app.NewKittyPassthroughWithOptions(app.KittyPassthroughOptions{Output: host})
	withSidebarGlobals(t, "right")
	o.Settings = config.Global
	o.Width, o.Height = 160, 40
	o.EffectiveWidth, o.EffectiveHeight = 160, 40
	o.Mode = app.WindowManagementMode
	o.Windows = []*terminal.Window{win}
	o.CurrentWorkspace, o.FocusedWindow = 1, 0
	_ = o.View()
	if !o.SidebarActive() {
		t.Fatal("the rail is not up, so nothing below tests what it says it tests")
	}

	// A pane corner puts the resize shape on the wire.
	o.UpdatePointerForPosition(0, 0)
	if got := host.b.String(); !strings.Contains(got, "]22;nwse-resize\x1b\\") {
		t.Fatalf("the pane corner offered %q, want nwse-resize on the wire", got)
	}
	host.b.Reset()

	// Motion onto the rail's band is consumed by the rail, and consuming it
	// must still retire the stale shape.
	_, _ = handleMouseMotion(tea.MouseMotionMsg{X: 150, Y: 10}, o)
	if got := host.b.String(); !strings.Contains(got, "]22;default\x1b\\") {
		t.Fatalf("rail motion left the shape as %q, want a default restatement", got)
	}
}
