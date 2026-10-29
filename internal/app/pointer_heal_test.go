package app

import (
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// pointerOS builds a model with one bordered pane whose content box starts at
// x=32: the pane's own edge at x=31 is a divider a hover can put a resize
// shape on, and x=40 inside it is plain content.
func pointerOS(t *testing.T) *OS {
	t.Helper()
	cfg := config.DefaultConfig()
	m := NewOS(OSOptions{UserConfig: cfg, KeybindRegistry: config.NewKeybindRegistry(cfg)})
	m.Width, m.Height = 120, 40
	m.EffectiveWidth, m.EffectiveHeight = 120, 40
	m.Windows = []*terminal.Window{
		{ID: "aaaaaaaa1111", CustomName: "editor", X: 31, Y: 1, Width: 40, Height: 20, Workspace: 1},
	}
	m.CurrentWorkspace, m.FocusedWindow = 1, 0
	return m
}

// TestHealRewritesThroughTheDedup pins the self-heal: a shape write the host
// lost leaves the model believing the shape is current, and every SetPointer
// after that is deduped away. The heal must write again anyway.
func TestHealRewritesThroughTheDedup(t *testing.T) {
	m := pointerOS(t)
	m.pointerSeenX, m.pointerSeenY = 40, 10
	m.SetPointerShape(PointerDefault)
	if got := m.PointerWriteCount(); got != 1 {
		t.Fatalf("the first shape write count is %d, want 1", got)
	}

	// The dedup suppresses an identical shape: this is the state after a lost
	// write, where only the model's belief moved on.
	m.SetPointerShape(PointerDefault)
	if got := m.PointerWriteCount(); got != 1 {
		t.Fatalf("the dedup wrote again: count %d, want 1", got)
	}

	m.HealPointerShape()
	if got := m.PointerWriteCount(); got != 2 {
		t.Fatalf("the heal did not write through the dedup: count %d, want 2", got)
	}
}

// TestHealRestoresTheStuckDivider pins the reported symptom: the pointer
// crossed a divider, the reset write was lost, and the shape sticks on
// ew-resize while the pointer rests over plain pane content. The heal sees
// the geometry and writes default.
func TestHealRestoresTheStuckDivider(t *testing.T) {
	m := pointerOS(t)
	// The host's stale truth: a resize shape from the pane edge at x=31.
	m.SetPointerShape(PointerEWResize)
	// The model's belief: the repair write went out and was lost, so it says
	// default while the host still shows the resize shape.
	m.SetPointerShape(PointerDefault)
	m.pointerSeenX, m.pointerSeenY = 40, 10

	m.HealPointerShape()

	if got := m.PointerShape(); got != PointerDefault {
		t.Fatalf("the heal changed the believed shape to %q, want default", got)
	}
	if got := m.PointerWriteCount(); got != 3 {
		t.Fatalf("the heal did not rewrite the believed shape: count %d, want 3", got)
	}
}

// TestHealEvaluatesGeometry pins that the heal writes the shape the position
// asks for, not the one the model already had: a pointer resting on the
// pane's edge is healed to ew-resize even from a believed default.
func TestHealEvaluatesGeometry(t *testing.T) {
	m := pointerOS(t)
	m.SetPointerShape(PointerDefault)
	m.pointerSeenX, m.pointerSeenY = 31, 10

	m.HealPointerShape()

	if got := m.PointerShape(); got != PointerEWResize {
		t.Fatalf("a pointer on the pane's edge healed to %q, want ew-resize", got)
	}
	if got := m.PointerWriteCount(); got != 2 {
		t.Fatalf("the heal skipped the write it existed for: count %d, want 2", got)
	}
}

// TestHealStaysOutOfGesturesAndWithMouseOff pins the heal's guards: no write
// while a gesture owns the pointer, and none at all when the client stopped
// asking for motion.
func TestHealStaysOutOfGesturesAndWithMouseOff(t *testing.T) {
	m := pointerOS(t)
	m.SetPointerShape(PointerDefault)
	m.pointerSeenX, m.pointerSeenY = 40, 10

	for _, tc := range []struct {
		name string
		set  func()
	}{
		{"dragging", func() { m.Dragging = true }},
		{"resizing", func() { m.Resizing = true }},
		{"mouse off", func() { m.MouseOff = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := m.PointerWriteCount()
			tc.set()
			defer func() { tc.set = func() {}; m.Dragging, m.Resizing, m.MouseOff = false, false, false }()
			m.HealPointerShape()
			if got := m.PointerWriteCount(); got != count {
				t.Errorf("%s: the heal wrote, count %d, want %d", tc.name, got, count)
			}
		})
	}
}

// TestPointerHealTickArmsFromTheFirstTick pins the scheduler: the first tick
// heals at once, and the due time then moves a full interval out, so a quiet
// pointer costs one write a second and no more.
func TestPointerHealTickArmsFromTheFirstTick(t *testing.T) {
	m := pointerOS(t)
	m.pointerSeenX, m.pointerSeenY = 40, 10
	m.pointerSeenAt = time.Now().Add(-time.Millisecond)
	now := time.Now()
	m.pointerHealTick(now)
	if got := m.PointerWriteCount(); got != 1 {
		t.Fatalf("the first heal did not run: count %d, want 1", got)
	}
	if !m.pointerHealAt.Equal(now.Add(pointerHealInterval)) {
		t.Fatalf("the next heal is due at %v, want %v", m.pointerHealAt, now.Add(pointerHealInterval))
	}

	m.pointerHealTick(now.Add(time.Millisecond))
	if got := m.PointerWriteCount(); got != 1 {
		t.Fatalf("a tick before the due time healed again: count %d, want 1", got)
	}
}

// TestPointerHealChainDiesWhenThePointerRests pins the sleep contract: a
// pointer that has not moved for the heal window stops the chain, so an idle
// client has no tick due and sleeps until a message arrives.
func TestPointerHealChainDiesWhenThePointerRests(t *testing.T) {
	m := pointerOS(t)
	now := time.Now()
	m.pointerSeenX, m.pointerSeenY = 40, 10
	m.pointerSeenAt = now.Add(-pointerHealWindow - time.Second)

	m.pointerHealTick(now)
	if got := m.PointerWriteCount(); got != 0 {
		t.Fatalf("a rested pointer healed: count %d, want 0", got)
	}
	if _, due := m.nextWake(now); due {
		t.Fatal("a rested pointer still has a tick due; the client must sleep")
	}
}

// TestHealWritesTheOsc pins the wire form the heal sends, because a wrong
// escape sequence is a bug no shape assertion can see. writePointerShape
// records the exact bytes it sent.
func TestHealWritesTheOsc(t *testing.T) {
	m := pointerOS(t)
	m.pointerSeenX, m.pointerSeenY = 40, 10
	m.SetPointerShape(PointerDefault)
	if got := m.lastPointerSeq; got != "\x1b]22;default\x1b\\" {
		t.Fatalf("the plain write sent %q, want an OSC 22 default", got)
	}

	m.HealPointerShape()
	if got := m.lastPointerSeq; got != "\x1b]22;default\x1b\\" {
		t.Fatalf("the heal sent %q, want an OSC 22 default", got)
	}
}
