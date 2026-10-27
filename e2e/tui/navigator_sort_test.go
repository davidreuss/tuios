package tuie2e

import (
	"strings"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuitest"
)

// assertNavigatorOrder waits until the screen reads first before second.
// The navigator's session rows are what carry the names, so the order the
// two appear in is the order the sort in force chose.
func assertNavigatorOrder(t *testing.T, term *tuitest.Terminal, first, second string) {
	t.Helper()
	deadline := time.Now().Add(uiTimeout)
	for time.Now().Before(deadline) {
		text := term.Snapshot()
		i, j := strings.Index(text, first), strings.Index(text, second)
		if i >= 0 && j >= 0 && i < j {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("the rows never read %s before %s\n%s", first, second, term.Snapshot())
}

// TestNavigatorSortCycleReordersRows: s in the navigator steps the sort
// position, viewed, activity, and the rows follow. The person types in the
// home pane, so viewed keeps home first. The logs pane writes after that,
// so activity puts work first. A third s returns to the position order.
//
// How it could pass wrongly: the cycle could change a label without
// reordering, or reorder only the flat list. The tree rows are what assert
// here, and every mode is read off the same open panel.
func TestNavigatorSortCycleReordersRows(t *testing.T) {
	base, _ := navigatorSessions(t)
	term := attachIn(t, base, "home", startOpts{cols: 140, rows: 40})
	if err := term.WaitFor(func(s tuitest.Screen) bool { return countWindows(s) == 1 }, bootTimeout); err != nil {
		t.Fatalf("client never attached: %v\n%s", err, term.Snapshot())
	}
	toggleSidebarViaPalette(t, term)
	waitRailCurrent(t, term, "home")

	// The person types in the home pane: the only viewed stamp anywhere.
	sendKeys(t, term, "true")
	sendKeys(t, term, tuitest.Enter)
	// The logs pane writes after that: the newest output on any pane.
	printMarker(t, base, "work", "logs", navPrintMarker, navMarker)

	openNavigator(t, term, "home", "work")
	assertNavigatorOrder(t, term, "home", "work")

	sendKeys(t, term, "s")
	assertNavigatorOrder(t, term, "home", "work")
	saveFrame(t, term, "navigator-sort-viewed")

	sendKeys(t, term, "s")
	assertNavigatorOrder(t, term, "work", "home")
	saveFrame(t, term, "navigator-sort-activity")

	sendKeys(t, term, "s")
	assertNavigatorOrder(t, term, "home", "work")
	sendKeys(t, term, tuitest.Esc)
}
