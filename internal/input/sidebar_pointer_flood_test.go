package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Gaurav-Gosain/tuios/internal/app"
)

// TestSidebarBandMotionDoesNotFloodThePointer pins the de-flood: motion over
// the sidebar band fires per event, and the reset it carries is deduped, so a
// sweep across the band costs one OSC 22 write and not one per event. The
// flood was the source of the lost writes that left a stale shape stuck.
func TestSidebarBandMotionDoesNotFloodThePointer(t *testing.T) {
	m := hoverOS(t)
	writes := m.PointerWriteCount()

	for x := 0; x < 20; x++ {
		next, _ := m.Update(tea.MouseMotionMsg{X: x, Y: 5})
		m = next.(*app.OS)
	}
	if got := m.PointerWriteCount() - writes; got != 1 {
		t.Fatalf("20 motions over the band sent %d pointer writes, want 1", got)
	}
	if got := m.PointerShape(); got != app.PointerDefault {
		t.Fatalf("motions over the band left the pointer as %q, want default", got)
	}
}
