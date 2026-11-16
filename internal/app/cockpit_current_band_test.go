package app

import (
	"image/color"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/overlay"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
	uv "github.com/charmbracelet/ultraviolet"
)

// currentMarkOS is a client with two working agents, the first one the pane
// the person is looking at, the cockpit drawn at 256 colours so the rows
// carry grounds a cell can be read for.
func currentMarkOS(t *testing.T) *OS {
	t.Helper()
	prev := overlay.CurrentDepth()
	overlay.SetDepth(overlay.Depth256)
	t.Cleanup(func() { overlay.SetDepth(prev) })
	config.Global.SidebarEnabled = true
	t.Cleanup(func() { config.Global.SidebarEnabled = false })
	now := time.Now().Add(-5 * time.Minute).UnixNano()
	m := &OS{
		Settings: config.Global,
		Windows: []*terminal.Window{
			{ID: "w0", CustomName: "here", Workspace: 1, AgentState: "working", AgentHarness: "claude-code", AgentStateAt: now},
			{ID: "w1", CustomName: "there", Workspace: 1, AgentState: "working", AgentHarness: "claude-code", AgentStateAt: now},
		},
		Width: 160, Height: 60, SessionName: "s", CurrentWorkspace: 1, FocusedWindow: 0,
	}
	m.OpenInbox("")
	m.Inbox.agents.on = true
	return m
}

// currentMarkAgents fills the cockpit's agent list by hand: one entry the
// pane the person is looking at, one resting.
func currentMarkAgents(m *OS) {
	m.Inbox.ck.agents = []sidebarAgentEntry{
		{Title: "here", SessionID: "s", State: "working", Focused: true},
		{Title: "there", SessionID: "s", State: "working"},
	}
}

// ckCellBg is the ground a canvas cell was painted on, nil for the canvas's
// own.
func ckCellBg(c *ckCanvas, x, y int) color.Color {
	if cell := c.buf.CellAt(x, y); cell != nil {
		return cell.Style.Bg
	}
	return nil
}

// ckCellFg is the ink a canvas cell was painted with.
func ckCellFg(c *ckCanvas, x, y int) color.Color {
	if cell := c.buf.CellAt(x, y); cell != nil {
		return cell.Style.Fg
	}
	return nil
}

// TestCockpitCurrentRowBand pins the fleet's "you are here" band: the row
// of the pane the person is looking at wears it, and no other row does.
func TestCockpitCurrentRowBand(t *testing.T) {
	m := currentMarkOS(t)
	currentMarkAgents(m)
	l := m.cockpitLook()
	c := new(ckCanvas).reset(60, 3, l.canvas)
	m.drawFleetRow(c, l, ckRow{kind: ckRowAgent, agent: 0, key: "k0", lines: 1, group: ckWorking}, false, false, 0, 0, 60, 0)
	m.drawFleetRow(c, l, ckRow{kind: ckRowAgent, agent: 1, key: "k1", lines: 1, group: ckWorking}, false, false, 0, 1, 60, 0)

	if bg := ckCellBg(c, 40, 0); !sameColor(bg, l.active) {
		t.Errorf("the current pane's row has no band: bg %v, want %v", bg, l.active)
	}
	if bg := ckCellBg(c, 40, 1); !sameColor(bg, l.panel) {
		t.Errorf("a resting row wears the current pane's band: bg %v, want %v", bg, l.panel)
	}
}

// TestCockpitCurrentRowBesideTheCursor pins the two states apart on one
// row and on neighbouring rows. On the row the cursor is on, the cursor's
// ground wins and the bar is the cursor's. On the current pane's row
// beside it, the band stands and the bar is brighter than the cursor's, so
// the two never blur.
func TestCockpitCurrentRowBesideTheCursor(t *testing.T) {
	m := currentMarkOS(t)
	m.Inbox.ck.agents = []sidebarAgentEntry{
		{Title: "there", SessionID: "s", State: "working"},
		{Title: "here", SessionID: "s", State: "working", Focused: true},
	}
	l := m.cockpitLook()
	pal := l.pal
	c := new(ckCanvas).reset(60, 3, l.canvas)
	m.drawFleetRow(c, l, ckRow{kind: ckRowAgent, agent: 0, key: "k0", lines: 1, group: ckWorking}, true, false, 0, 0, 60, 0)
	m.drawFleetRow(c, l, ckRow{kind: ckRowAgent, agent: 1, key: "k1", lines: 1, group: ckWorking}, false, false, 0, 1, 60, 0)

	if bg := ckCellBg(c, 40, 0); !sameColor(bg, l.cursor) {
		t.Errorf("the cursor's ground does not win on its own row: bg %v, want %v", bg, l.cursor)
	}
	if cell := c.buf.CellAt(0, 0); cell == nil || cell.Content != "▌" || !sameColor(cell.Style.Fg, pal.Accent) {
		t.Errorf("the cursor's bar is not the cursor's: %+v", cell)
	}
	if fg := ckCellFg(c, 8, 0); fg == nil {
		t.Error("the name on the cursor row over the current pane has no ink")
	}

	if bg := ckCellBg(c, 40, 1); !sameColor(bg, l.active) {
		t.Errorf("the current pane's row beside the cursor lost its band: bg %v, want %v", bg, l.active)
	}
	if cell := c.buf.CellAt(0, 1); cell == nil || cell.Content != "▌" || !sameColor(cell.Style.Fg, pal.AccentBright) {
		t.Errorf("the current pane's bar is not brighter than the cursor's: %+v", cell)
	}
	if sameColor(l.active, l.cursor) {
		t.Errorf("the band %v blurs into the cursor's ground", l.active)
	}
}

// TestCockpitFocusHeaderCarriesTheMark pins the reading view's half of the
// mark: the focus header of the pane the person is looking at wears the
// same band its row does, with the bright bold name, and the header of
// another agent rests.
func TestCockpitFocusHeaderCarriesTheMark(t *testing.T) {
	m := currentMarkOS(t)
	l := m.cockpitLook()

	selectAgent := func(focused bool) {
		t.Helper()
		for _, r := range m.cockpitRows() {
			if r.kind == ckRowAgent && m.Inbox.ck.agents[r.agent].Focused == focused {
				m.cockpitSelectKey(r.key)
				return
			}
		}
		t.Fatalf("no %s agent row", map[bool]string{true: "current", false: "resting"}[focused])
	}

	selectAgent(true)
	c := new(ckCanvas).reset(100, 40, l.canvas)
	m.drawFocus(c, l, 0, 0, 100, 38, 0, time.Now())
	if bg := ckCellBg(c, 60, 0); !sameColor(bg, l.active) {
		t.Errorf("the focus header of the current pane has no band: bg %v, want %v", bg, l.active)
	}
	pal := l.pal
	bright := overlay.Readable(pal.AccentBright, l.active)
	if fg := ckCellFg(c, 0, 0); !sameColor(fg, bright) {
		t.Errorf("the focus header's name is not the bright ink on the band: fg %v, want %v", fg, bright)
	}
	if cell := c.buf.CellAt(0, 0); cell == nil || cell.Style.Attrs&uv.AttrBold == 0 {
		t.Errorf("the focus header's name is not bold: %+v", cell)
	}

	selectAgent(false)
	c = new(ckCanvas).reset(100, 40, l.canvas)
	m.drawFocus(c, l, 0, 0, 100, 38, 0, time.Now())
	if bg := ckCellBg(c, 60, 0); sameColor(bg, l.active) {
		t.Errorf("the focus header of a resting agent wears the band: bg %v", bg)
	}
	resting := overlay.Readable(pal.Fg, l.active)
	if fg := ckCellFg(c, 0, 0); !sameColor(fg, resting) || sameColor(fg, bright) {
		t.Errorf("a resting agent's header name is not the plain ink: fg %v, want %v", fg, resting)
	}
}
