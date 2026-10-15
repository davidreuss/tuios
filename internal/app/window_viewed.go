package app

import (
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// windowViewedNow is the clock markWindowViewed reads. A test replaces it.
var windowViewedNow = func() int64 { return time.Now().UnixNano() }

// markWindowViewed records that this client's user had the pane in front of
// them now: they focused it, or typed into it from their own terminal. Input
// that arrives through another client, a tape or an agent never counts,
// because only the client can tell whose keystrokes they are. The stamp
// feeds the navigator's viewed sort through WindowState.LastViewed.
func (m *OS) markWindowViewed(w *terminal.Window) {
	if w == nil || w.ID == "" {
		return
	}
	at := windowViewedNow()
	if m.WindowViewedAt == nil {
		m.WindowViewedAt = make(map[string]int64, 1)
	}
	if at > m.WindowViewedAt[w.ID] {
		m.WindowViewedAt[w.ID] = at
	}
}

// seedWindowViewedAt adopts a recency stamp from restored or sync state. The
// daemon keeps the later of the two sides, so a stamp here can never pull
// the value backwards.
func (m *OS) seedWindowViewedAt(ws *session.WindowState) {
	if ws == nil || ws.LastViewed <= m.WindowViewedAt[ws.ID] {
		return
	}
	if m.WindowViewedAt == nil {
		m.WindowViewedAt = make(map[string]int64, 1)
	}
	m.WindowViewedAt[ws.ID] = ws.LastViewed
}
