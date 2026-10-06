package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestBlurRetiresThePointerShape is the regression test for the resize cursor
// that survived a trip through another application. The pointer shape is hover
// geometry inside this window, and the terminal paints the last shape it was
// sent until a new one arrives, so a shape not retired on blur came back as
// arrows over pane content when the window was refocused.
func TestBlurRetiresThePointerShape(t *testing.T) {
	m := newTestOS(newTestWindow(t, "pointer-blur-0001", 60, 20))
	m.currentPointer = PointerNSResize

	if _, _ = m.Update(tea.BlurMsg{}); m.currentPointer != PointerDefault {
		t.Fatalf("after blur the pointer shape is %q, want default", m.currentPointer)
	}
}
