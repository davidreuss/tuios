package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/overlay"
	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// sortSessions is a navigator listing with recency stamps to sort on:
// session one focused later than session zero, workspace one later than
// workspace zero, and pane one of each workspace later than pane zero.
func sortSessions() []navSession {
	return []navSession{
		{Name: "s0", Panes: []navPane{
			{ID: "a", Workspace: 0, LastViewed: 10, LastOutput: 100},
			{ID: "b", Workspace: 0, LastViewed: 20, LastOutput: 200},
			{ID: "c", Workspace: 1, LastViewed: 30, LastOutput: 300},
		}},
		{Name: "s1", Panes: []navPane{
			{ID: "d", Workspace: 0, LastViewed: 40, LastOutput: 400},
		}},
	}
}

// sortedSortOS is an OS whose navigator is open on sortSessions, in the
// given layout and sort.
func sortedSortOS(layout, sort string) *OS {
	m := &OS{}
	sessions := sortSessions()
	expanded := map[string]bool{}
	for i := range sessions {
		s := &sessions[i]
		expanded[s.key()] = true
		for _, ws := range s.workspaces() {
			expanded[s.key()+"\x00ws"+strconv.Itoa(ws)] = true
		}
	}
	m.navigator = navigatorState{
		open:     true,
		expanded: expanded,
		sessions: sessions,
		layout:   layout,
		sort:     sort,
	}
	return m
}

// paneIDs reads the pane a row names, for order assertions.
func paneIDs(m *OS, rows []navRow) []string {
	var out []string
	for _, r := range rows {
		if r.Kind != navRowPane {
			continue
		}
		out = append(out, m.navigator.sessions[r.Session].Panes[r.Pane].ID)
	}
	return out
}

func equalIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("panes %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("panes %v, want %v", got, want)
		}
	}
}

// How it could pass wrongly: the flat branch could sort only when the
// stamps happen to be set, or sort ascending. It must order most recently
// used first under viewed, and keep the position order under position.
func TestNavigatorSortOrdersFlatRows(t *testing.T) {
	m := sortedSortOS(config.NavigatorLayoutFlat, config.NavigatorSortViewed)
	equalIDs(t, paneIDs(m, m.navigatorRows()), []string{"d", "c", "b", "a"})

	m = sortedSortOS(config.NavigatorLayoutFlat, config.NavigatorSortActivity)
	equalIDs(t, paneIDs(m, m.navigatorRows()), []string{"d", "c", "b", "a"})

	m = sortedSortOS(config.NavigatorLayoutFlat, config.NavigatorSortPosition)
	equalIDs(t, paneIDs(m, m.navigatorRows()), []string{"a", "b", "c", "d"})
}

// The two sorts differ on a pane that wrote output but was never in front
// of the person: activity lists it by its output, viewed reads it as never.
func TestNavigatorSortSeparatesViewedFromActivity(t *testing.T) {
	m := &OS{}
	m.navigator = navigatorState{
		open:     true,
		expanded: map[string]bool{},
		sessions: []navSession{{Name: "s0", Panes: []navPane{
			{ID: "quiet", Workspace: 0, LastViewed: 5},
			{ID: "loud", Workspace: 0, LastOutput: 50},
		}}},
		layout: config.NavigatorLayoutFlat,
	}
	m.navigator.sort = config.NavigatorSortViewed
	equalIDs(t, paneIDs(m, m.navigatorRows()), []string{"quiet", "loud"})
	m.navigator.sort = config.NavigatorSortActivity
	equalIDs(t, paneIDs(m, m.navigatorRows()), []string{"loud", "quiet"})
}

// The tree keeps its groups, and orders within and across them: panes by
// the chosen stamp, workspaces and sessions by their most recent pane.
func TestNavigatorSortOrdersTree(t *testing.T) {
	m := sortedSortOS(config.NavigatorLayoutTree, config.NavigatorSortViewed)
	rows := m.navigatorRows()
	// Session one first, then session zero: workspace one, then workspace
	// zero with its later pane before its earlier one.
	want := []struct {
		kind navRowKind
		id   string
	}{
		{navRowSession, "s1"},
		{navRowWorkspace, "0"},
		{navRowPane, "d"},
		{navRowSession, "s0"},
		{navRowWorkspace, "1"},
		{navRowPane, "c"},
		{navRowWorkspace, "0"},
		{navRowPane, "b"},
		{navRowPane, "a"},
	}
	got := 0
	for _, r := range rows {
		if r.Kind == navRowSession {
			if m.navigator.sessions[r.Session].Name != want[got].id {
				t.Fatalf("row %d is session %q, want %q", got, m.navigator.sessions[r.Session].Name, want[got].id)
			}
		} else if r.Kind == navRowPane {
			if m.navigator.sessions[r.Session].Panes[r.Pane].ID != want[got].id {
				t.Fatalf("row %d is pane %q, want %q", got, m.navigator.sessions[r.Session].Panes[r.Pane].ID, want[got].id)
			}
		} else if r.Kind == navRowWorkspace && strconv.Itoa(r.Workspace) != want[got].id {
			t.Fatalf("row %d is workspace %d, want %q", got, r.Workspace, want[got].id)
		}
		got++
	}
	if got != len(want) {
		t.Fatalf("saw %d rows, want %d", got, len(want))
	}
}

// How it could pass wrongly: the cycle could lose the per-open pick, wrap
// wrongly, or leave the sort state unset so the next open falls back.
func TestNavigatorCycleSortCyclesAndKeepsThePick(t *testing.T) {
	m := sortedSortOS(config.NavigatorLayoutTree, config.NavigatorSortPosition)
	m.navSortPick = ""
	for i, want := range []string{
		config.NavigatorSortViewed,
		config.NavigatorSortActivity,
		config.NavigatorSortPosition,
		config.NavigatorSortViewed,
	} {
		m.NavigatorCycleSort()
		if m.navigator.sort != want {
			t.Fatalf("cycle %d sorts by %q, want %q", i, m.navigator.sort, want)
		}
		if m.navSortPick != want {
			t.Fatalf("cycle %d picks %q, want %q", i, m.navSortPick, want)
		}
	}
	if got := m.navigatorSortChoice(); got != config.NavigatorSortViewed {
		t.Fatalf("choice after cycling is %q, want %q", got, config.NavigatorSortViewed)
	}
}

// markWindowViewed keeps the later stamp, whatever order the focus and the
// input arrive in, and ignores a window it cannot name.
func TestMarkWindowViewedKeepsTheLaterStamp(t *testing.T) {
	defer func(f func() int64) { windowViewedNow = f }(windowViewedNow)
	at := int64(7)
	windowViewedNow = func() int64 { return at }

	m := &OS{}
	m.markWindowViewed(nil)
	if len(m.WindowViewedAt) != 0 {
		t.Fatal("a nil window was stamped")
	}
	w := &terminal.Window{ID: "w1"}
	m.markWindowViewed(w)
	if m.WindowViewedAt["w1"] != 7 {
		t.Fatalf("stamp is %d, want 7", m.WindowViewedAt["w1"])
	}
	at = 3
	m.markWindowViewed(w)
	if m.WindowViewedAt["w1"] != 7 {
		t.Fatalf("an earlier stamp moved the value to %d", m.WindowViewedAt["w1"])
	}
	at = 9
	m.markWindowViewed(w)
	if m.WindowViewedAt["w1"] != 9 {
		t.Fatalf("stamp is %d, want 9", m.WindowViewedAt["w1"])
	}
}

// seedWindowViewedAt adopts a restored stamp but never pulls one backwards.
// The footer names the active sort, so the s key says what it does before
// it is pressed. While a search is up the hint is gone, since s is not
// handled there.
func TestNavigatorSortHintNamesTheActiveSort(t *testing.T) {
	m := sortedSortOS(config.NavigatorLayoutFlat, config.NavigatorSortViewed)
	want := map[string]string{
		config.NavigatorSortPosition: "sort: position",
		config.NavigatorSortViewed:   "sort: viewed",
		config.NavigatorSortActivity: "sort: activity",
	}
	for sort, label := range want {
		m.navigator.sort = sort
		hint := ""
		for _, h := range m.navigatorHints() {
			if h.Key == "s" {
				hint = h.Label
			}
		}
		if hint != label {
			t.Fatalf("sort %s hint label is %q, want %q", sort, hint, label)
		}
	}
	m.navigator.searching = true
	for _, h := range m.navigatorHints() {
		if h.Key == "s" {
			t.Fatal("the s hint is shown while a search is up")
		}
	}
}

// The Panes box note pairs the layout with the sort, so the frame says what
// the list is ordered by. A search replaces the pair with results.
func TestNavigatorListNotePairsLayoutWithSort(t *testing.T) {
	m := sortedSortOS(config.NavigatorLayoutTree, config.NavigatorSortActivity)
	if got := m.navigatorListNote(); got != "tree · activity" {
		t.Fatalf("note is %q, want %q", got, "tree · activity")
	}
	m.navigator.sort = config.NavigatorSortPosition
	if got := m.navigatorListNote(); got != "tree · position" {
		t.Fatalf("note is %q, want %q", got, "tree · position")
	}
	m.navigator.query = "web"
	if got := m.navigatorListNote(); got != "results" {
		t.Fatalf("note is %q, want %q", got, "results")
	}
}

func TestSeedWindowViewedNeverGoesBackwards(t *testing.T) {
	m := &OS{}
	m.WindowViewedAt = map[string]int64{"w1": 9}
	m.seedWindowViewedAt(&session.WindowState{ID: "w1", LastViewed: 5})
	if m.WindowViewedAt["w1"] != 9 {
		t.Fatalf("seed moved the stamp to %d", m.WindowViewedAt["w1"])
	}
	m.seedWindowViewedAt(&session.WindowState{ID: "w1", LastViewed: 12})
	if m.WindowViewedAt["w1"] != 12 {
		t.Fatalf("stamp is %d, want 12", m.WindowViewedAt["w1"])
	}
}

func TestNavAgoFormatsRelativeAges(t *testing.T) {
	now := int64(1_000_000_000_000_000_000) // unix nano
	cases := []struct {
		age  time.Duration
		want string
	}{
		{30 * time.Second, "now"},
		{2 * time.Minute, "2m"},
		{59 * time.Minute, "59m"},
		{5 * time.Hour, "5h"},
		{3 * 24 * time.Hour, "3d"},
		{2 * 7 * 24 * time.Hour, "2w"},
		{3 * 30 * 24 * time.Hour, "3mo"},
	}
	for _, c := range cases {
		if got := navAgo(now-int64(c.age), now); got != c.want {
			t.Fatalf("navAgo(%d) is %q, want %q", c.age, got, c.want)
		}
	}
	if got := navAgo(0, now); got != "" {
		t.Fatalf("a zero stamp is %q, want none", got)
	}
	if got := navAgo(now+int64(5*time.Minute), now); got != "now" {
		t.Fatalf("a stamp ahead of now is %q, want now", got)
	}
}

// The age rides on the row only while a recency sort is in force: it names
// the stamp the rows are ordered by, in a column at the row's right edge.
// Position, the default, shows no age, and a row with no stamp leaves the
// column empty rather than spending words on it.
func TestNavigatorPaneRowsCarryAgeUnderRecencySort(t *testing.T) {
	now := time.Now().UnixNano()
	m := sortedSortOS(config.NavigatorLayoutFlat, config.NavigatorSortViewed)
	s := &m.navigator.sessions[0]
	s.Panes[0].LastViewed = now - 5*int64(time.Minute) // pane a: newest, first
	s.Panes[1].LastViewed = 0                          // pane b: never viewed
	s.Panes[1].LastOutput = 0
	rowOf := func(id string) navRow {
		for _, r := range m.navigatorRows() {
			if r.Kind == navRowPane && m.navigator.sessions[r.Session].Panes[r.Pane].ID == id {
				return r
			}
		}
		t.Fatalf("pane %s is not listed", id)
		return navRow{}
	}
	lines := m.navigatorItem(rowOf("a"), overlay.RowState{}, 120, overlay.Palette{})
	if !strings.Contains(lines[0], "5m") {
		t.Fatalf("viewed sort row is %q, want the 5m age on it", lines[0])
	}
	lines = m.navigatorItem(rowOf("b"), overlay.RowState{}, 120, overlay.Palette{})
	if strings.Contains(lines[0], "never") || strings.Contains(lines[0], "quiet") {
		t.Fatalf("unstamped row is %q, want an empty age column", lines[0])
	}
	m.navigator.sort = config.NavigatorSortPosition
	lines = m.navigatorItem(rowOf("a"), overlay.RowState{}, 120, overlay.Palette{})
	if strings.Contains(lines[0], "5m") {
		t.Fatalf("position sort row is %q, want no age on it", lines[0])
	}
}

// Session and workspace rows read the stamp of their most recent pane, so
// the age column runs down the whole list, not just the pane rows.
func TestNavigatorGroupRowsCarryAgeColumn(t *testing.T) {
	now := time.Now().UnixNano()
	m := sortedSortOS(config.NavigatorLayoutTree, config.NavigatorSortViewed)
	s := &m.navigator.sessions[0]
	for i := range s.Panes {
		s.Panes[i].LastViewed = now - 2*int64(time.Hour)
	}
	session := m.navigatorItem(navRow{Kind: navRowSession, Session: 0}, overlay.RowState{}, 120, overlay.Palette{})
	if !strings.Contains(session[0], "2h") {
		t.Fatalf("session row is %q, want the 2h age on it", session[0])
	}
	ws := m.navigatorItem(navRow{Kind: navRowWorkspace, Session: 0, Workspace: 0}, overlay.RowState{}, 120, overlay.Palette{})
	if !strings.Contains(ws[0], "2h") {
		t.Fatalf("workspace row is %q, want the 2h age on it", ws[0])
	}
	m.navigator.sort = config.NavigatorSortPosition
	session = m.navigatorItem(navRow{Kind: navRowSession, Session: 0}, overlay.RowState{}, 120, overlay.Palette{})
	if strings.Contains(session[0], "2h") {
		t.Fatalf("position sort session row is %q, want no age on it", session[0])
	}
}

// The age column hangs off the row's right edge, after the count, so the
// counts line up beside the ages instead of the ages wandering with the
// width of the count.
func TestNavigatorAgeColumnSitsAtTheRightEdge(t *testing.T) {
	now := time.Now().UnixNano()
	m := sortedSortOS(config.NavigatorLayoutTree, config.NavigatorSortViewed)
	s := &m.navigator.sessions[0]
	for i := range s.Panes {
		s.Panes[i].LastViewed = now - 2*int64(time.Hour)
	}
	line := func(kind navRowKind, ws int) string {
		return m.navigatorItem(navRow{Kind: kind, Session: 0, Workspace: ws}, overlay.RowState{}, 120, overlay.Palette{})[0]
	}
	for name, l := range map[string]string{
		"session":   line(navRowSession, 0),
		"workspace": line(navRowWorkspace, 0),
		"pane":      line(navRowPane, 0),
	} {
		runes := []rune(l)
		tail := strings.TrimSpace(string(runes[max(0, len(runes)-10):]))
		if !strings.HasSuffix(tail, "2h") {
			t.Fatalf("%s row ends %q, want the 2h age at the right edge", name, tail)
		}
	}
}
