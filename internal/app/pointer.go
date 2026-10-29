package app

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/debuglog"
)

// PointerShape represents a CSS cursor shape name for OSC 22.
type PointerShape string

const (
	PointerDefault    PointerShape = "default"
	PointerGrab       PointerShape = "grab"
	PointerGrabbing   PointerShape = "grabbing"
	PointerEWResize   PointerShape = "ew-resize"
	PointerNSResize   PointerShape = "ns-resize"
	PointerNWSEResize PointerShape = "nwse-resize"
	PointerNESWResize PointerShape = "nesw-resize"
	// PointerPointer is the hand a browser shows over a link, and it is what a
	// run of pane cells gets while the pointer is on one. It is the only shape
	// here that comes from content rather than from geometry.
	PointerPointer PointerShape = "pointer"
)

// SetPointerShape writes an OSC 22 sequence to change the mouse pointer,
// unless the model believes that shape is already on: the dedup that keeps a
// quiet pointer quiet. The heal is the one writer that goes through it.
func (m *OS) SetPointerShape(shape PointerShape) {
	if shape == m.currentPointer {
		return
	}
	if _, file, line, ok := runtime.Caller(1); ok {
		m.LogInfo("[POINTER] %s from %s:%d", shape, filepath.Base(file), line)
		debuglog.Internal{}.Printf("[POINTER] %s from %s:%d", shape, filepath.Base(file), line)
	}
	m.writePointerShape(shape)
}

// writePointerShape sends the OSC 22 with no dedup and records the shape as
// current. SetPointerShape and the heal are its only callers.
func (m *OS) writePointerShape(shape PointerShape) {
	m.currentPointer = shape
	m.pointerWrites++
	seq := fmt.Appendf(nil, "\x1b]22;%s\x1b\\", string(shape))
	m.lastPointerSeq = string(seq)
	m.writeHostSequence(seq)
}

// PointerWriteCount reports how many OSC 22 pointer writes this client has
// sent, deduped ones excluded. The flood and heal tests read it.
func (m *OS) PointerWriteCount() int { return m.pointerWrites }

// PointerShape reports the last shape written, for tests and diagnostics.
func (m *OS) PointerShape() PointerShape { return m.currentPointer }

// ResetPointerShape sets the pointer back to default.
//
// It writes unconditionally rather than through SetPointerShape's dedup: the
// last write can be lost while the host terminal is switching away or
// repainting, and a stale resize shape that tuios believes was already reset
// then sticks until real mouse motion changes it.
func (m *OS) ResetPointerShape() {
	m.currentPointer = ""
	m.SetPointerShape(PointerDefault)
}

// UpdatePointerForPosition sets the pointer shape based on what the mouse
// is hovering over: window borders, corners, separator lines, title bars.
func (m *OS) UpdatePointerForPosition(x, y int) {
	if m.Dragging || m.Resizing {
		return
	}
	m.SetPointerShape(m.pointerShapeForPosition(x, y))
}

// pointerShapeForPosition is the hover decision without the write: the shape
// the pointer at x, y should carry, from geometry alone. The heal reads it
// through UpdatePointerForPosition, which owns the drag guard.
func (m *OS) pointerShapeForPosition(x, y int) PointerShape {
	// Check dock area
	topMargin := m.viewReserve().Top
	if m.Settings.DockbarPosition == "top" && y < topMargin {
		return PointerDefault
	}
	if m.Settings.DockbarPosition == "bottom" && y >= topMargin+m.ViewUsableHeight() {
		return PointerDefault
	}

	// In tiled mode with shared borders, check separator lines
	if m.panesBorderless() {
		// The same lines the separator overlay draws, so the resize cursor is
		// only offered over a divider that is really there.
		for _, s := range m.separatorSplits() {
			if s.Vertical && x == s.Pos && y >= s.From && y <= s.To {
				return PointerEWResize
			}
			// The strip's horizontal dividers stand between windows stacked
			// in one column, which always share its height evenly: a drag
			// there does nothing, so it offers no resize pointer.
			if !s.Vertical && !m.UseScrollingLayout && y == s.Pos && x >= s.From && x <= s.To {
				return PointerNSResize
			}
		}
	}

	// Find window under cursor (topmost by Z)
	topIdx := -1
	topZ := -1
	for i, win := range m.Windows {
		if win.Workspace != m.CurrentWorkspace || win.Minimized {
			continue
		}
		if x >= win.X && x < win.X+win.Width && y >= win.Y && y < win.Y+win.Height {
			if z := HitZ(win); z > topZ {
				topZ = z
				topIdx = i
			}
		}
	}

	if topIdx == -1 {
		return PointerDefault
	}

	win := m.Windows[topIdx]

	// A pane with no border of its own has no edge to offer, whatever the
	// setting says: BorderOffset is the pane's own answer, and the divider
	// between borderless panes was already handled above.
	borderOff := win.BorderOffset()
	if borderOff == 0 {
		return PointerDefault
	}

	onLeft := x == win.X
	onRight := x == win.X+win.Width-1
	onTop := y == win.Y
	onBottom := y == win.Y+win.Height-1

	// Corners → diagonal resize
	if (onLeft && onTop) || (onRight && onBottom) {
		return PointerNWSEResize
	}
	if (onRight && onTop) || (onLeft && onBottom) {
		return PointerNESWResize
	}

	// Vertical edges → horizontal resize
	if onLeft || onRight {
		return PointerEWResize
	}

	// Top border → grab (title bar)
	if onTop {
		return PointerGrab
	}

	// Bottom border → vertical resize
	if onBottom {
		return PointerNSResize
	}

	return PointerDefault
}

// pointerHealInterval is how often the heal rewrites the shape the model
// believes in.
const pointerHealInterval = time.Second

// pointerHealWindow is how long after the last pointer event the heal keeps
// ticking. A stale shape matters while the pointer is in play; once it has
// rested this long, any lost write has long been healed, and the tick chain
// dies so an idle client sleeps until a message arrives. The next motion
// re-arms it, and the heal then runs within one tick of that motion.
const pointerHealWindow = 5 * pointerHealInterval

// HealPointerShape re-evaluates the pointer shape at the last seen position
// and writes it even when the dedup believes it is already current.
//
// A shape write the host lost stays lost behind SetPointerShape's dedup:
// every later motion that would restate the shape is skipped, because the
// model believes it is already on. The stuck shape then outlives the pointer
// that drew it until the pointer crosses chrome that sets a different one.
// Rewriting the believed shape once a tick heals any lost write inside a
// second, whatever lost it.
func (m *OS) HealPointerShape() {
	if m.MouseOff || m.Dragging || m.Resizing || m.ScrollbarDragging {
		return
	}
	x, y := m.PointerSeen()
	if x <= 0 && y <= 0 {
		return
	}
	before := m.currentPointer
	m.UpdatePointerForPosition(x, y)
	m.TrackLinkPointer(x, y)
	if m.currentPointer == "" || m.currentPointer != before {
		return
	}
	m.LogInfo("[POINTER] heal %s", m.currentPointer)
	debuglog.Internal{}.Printf("[POINTER] heal %s", m.currentPointer)
	m.writePointerShape(m.currentPointer)
}

// pointerHealTick runs the heal when it is due and arms the next one. It runs
// before the tick's idle gate on purpose: a host that lost the shape write is
// exactly the state in which nothing else is due, so the heal is the one job
// that must not idle out while the pointer is in play. When the pointer has
// rested past the heal window, the chain dies and the client sleeps.
func (m *OS) pointerHealTick(now time.Time) {
	if m.MouseOff || m.pointerSeenAt.IsZero() || now.Sub(m.pointerSeenAt) > pointerHealWindow {
		return
	}
	if !m.pointerHealAt.IsZero() && now.Before(m.pointerHealAt) {
		return
	}
	m.HealPointerShape()
	m.pointerHealAt = now.Add(pointerHealInterval)
}
