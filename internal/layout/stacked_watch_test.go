package layout

import (
	"fmt"
	"testing"
)

// The watch column: ordinary panes keep the accordion on the left, watch
// panes share a column on the right at equal heights, and with no watch panes
// (or nothing but watch panes) the layout is the plain accordion.

func TestStackedWatchLayoutWithoutWatchesIsTheAccordion(t *testing.T) {
	for _, n := range []int{1, 2, 4, 7} {
		isWatch := make([]bool, n)
		got := CalculateStackedWatchLayout(isWatch, n/2, 160, 48, 0, 1)
		want := CalculateStackedLayout(n, n/2, 160, 48, 0, 1)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("n=%d: watch layout without watch panes differs from the accordion\n got %v\nwant %v", n, got, want)
		}
	}
}

func TestStackedWatchLayoutAllWatchesIsAFullWidthAccordion(t *testing.T) {
	isWatch := []bool{true, true, true}
	got := CalculateStackedWatchLayout(isWatch, 1, 160, 48, 0, 1)
	want := CalculateStackedLayout(3, 1, 160, 48, 0, 1)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("all-watch layout differs from the accordion\n got %v\nwant %v", got, want)
	}
}

func TestStackedWatchLayoutSplitsIntoTwoColumns(t *testing.T) {
	// Panes 0 and 2 are ordinary, pane 1 is a watch, focus on the ordinary
	// pane at slot 0.
	isWatch := []bool{false, true, false}
	layouts := CalculateStackedWatchLayout(isWatch, 0, 180, 60, 0, 1)
	stackedDisjoint(t, layouts, 180, 60)

	if layouts[1].X <= layouts[0].X {
		t.Errorf("watch pane %v not to the right of ordinary pane %v", layouts[1], layouts[0])
	}
	// The watch column holds one pane here, so it is as tall as the region.
	if layouts[1].Height != 60 {
		t.Errorf("single watch pane is %d tall, want the region's 60", layouts[1].Height)
	}
	// The accordion in the ordinary column: focused pane 0 expanded, pane 2
	// collapsed to a bar. The expanded pane gives the bar and the gap up, so
	// it is two rows short of the region.
	if layouts[0].Height != 58 || layouts[2].Height != 1 {
		t.Errorf("accordion not respected in the ordinary column: pane 0 is %d tall, pane 2 is %d", layouts[0].Height, layouts[2].Height)
	}
}

func TestStackedWatchLayoutSplitsWatchesEvenlyDownwards(t *testing.T) {
	isWatch := []bool{false, true, true, true, true}
	layouts := CalculateStackedWatchLayout(isWatch, 0, 180, 61, 0, 1)
	stackedDisjoint(t, layouts, 180, 61)

	heights := []int{}
	for _, i := range []int{1, 2, 3, 4} {
		heights = append(heights, layouts[i].Height)
		if i > 1 && layouts[i].Y <= layouts[i-1].Y {
			t.Errorf("watch pane %d (y=%d) not below pane %d (y=%d)", i, layouts[i].Y, i-1, layouts[i-1].Y)
		}
	}
	lo, hi := heights[0], heights[0]
	for _, h := range heights {
		if h < lo {
			lo = h
		}
		if h > hi {
			hi = h
		}
	}
	if hi-lo > 1 {
		t.Errorf("watch panes differ in height by more than the rounding cell: %v", heights)
	}
}

func TestStackedWatchLayoutKeepsAReadableColumnWhenAWatchHoldsFocus(t *testing.T) {
	// Focus on the watch (slot 1): the ordinary column still expands its
	// first pane instead of collapsing into a wall of bars.
	isWatch := []bool{false, true, false}
	layouts := CalculateStackedWatchLayout(isWatch, 1, 180, 60, 0, 1)
	stackedDisjoint(t, layouts, 180, 60)
	if layouts[0].Height <= 1 {
		t.Errorf("ordinary column has no expanded pane while a watch holds the focus: %v", layouts)
	}
}
