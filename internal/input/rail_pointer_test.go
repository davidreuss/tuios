package input

import (
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/app"
	"github.com/Gaurav-Gosain/tuios/internal/config"
)

// TestSidebarEdgeResizeOwnsThePointer: the rail's width drag is a pointer
// gesture like the pane ones. The press puts the ew-resize cursor on, and the
// release takes it off, so the mode never inherits the shape a pane edge
// decided and never leaves one behind. The drag consumes motion without
// consulting the shape updater, so without a shape at the press the cursor
// stays at whatever the pointer last crossed.
func TestSidebarEdgeResizeOwnsThePointer(t *testing.T) {
	m := hoverOS(t)
	_ = frameLines(m)
	edgeX := config.Global.SidebarWidth - 1

	m = pressed(m, edgeX, 5)
	if !m.SidebarEdgeActive() {
		t.Fatal("a press on the rail's edge rule did not arm the width resize")
	}
	if got := m.PointerShape(); got != app.PointerEWResize {
		t.Errorf("pressing the rail's edge left the pointer as %q, want ew-resize", got)
	}

	m = dragged(m, edgeX-8, 5)
	if !m.SidebarEdgeActive() {
		t.Fatal("a held drag did not keep the width resize armed")
	}
	if got := m.PointerShape(); got != app.PointerEWResize {
		t.Errorf("dragging the rail's edge changed the pointer to %q, want ew-resize held", got)
	}

	m = released(m, edgeX-8, 5)
	if m.SidebarEdgeActive() {
		t.Error("the rail's width drag outlived the button that started it")
	}
	if got := m.PointerShape(); got != app.PointerDefault {
		t.Errorf("releasing the rail's edge left the pointer as %q, want default", got)
	}
}
