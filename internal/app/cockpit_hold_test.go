package app

import (
	"testing"
	"time"
)

// TestCockpitHoldFlapNeverMovesBands feeds one agent a state that flaps
// working to settled to working with a period longer than ckHoldUrgent and
// shorter than ckHoldCalm, and asserts its row never moves bands. This is
// the shape a working agent makes between turns, the one that strobed the
// list.
func TestCockpitHoldFlapNeverMovesBands(t *testing.T) {
	m := &OS{}
	now := time.Now()
	if got := m.ckHoldGroup("a", ckWorking, now); got != ckWorking {
		t.Fatalf("a new row landed in group %d, want %d", got, ckWorking)
	}
	for i := 1; i <= 200; i++ {
		want := ckSettled
		if i%2 == 0 {
			want = ckWorking
		}
		now = now.Add(time.Second)
		if got := m.ckHoldGroup("a", want, now); got != ckWorking {
			t.Fatalf("flap %d drew the row in group %d, want %d", i, got, ckWorking)
		}
	}
}

// TestCockpitHoldCalmMoveCompletes checks the other side: a state that
// rests past ckHoldCalm does move bands, but not a moment sooner.
func TestCockpitHoldCalmMoveCompletes(t *testing.T) {
	m := &OS{}
	now := time.Now()
	m.ckHoldGroup("a", ckWorking, now)
	m.ckHoldGroup("a", ckSettled, now.Add(100*time.Millisecond))
	if got := m.ckHoldGroup("a", ckSettled, now.Add(4900*time.Millisecond)); got != ckWorking {
		t.Fatalf("a settled state held %.1fs drew group %d, want %d still", 4.8, got, ckWorking)
	}
	if got := m.ckHoldGroup("a", ckSettled, now.Add(5200*time.Millisecond)); got != ckSettled {
		t.Fatalf("a settled state held %.1fs drew group %d, want %d", 5.1, got, ckSettled)
	}
}

// TestCockpitHoldUrgentMoveCompletes checks that a move into Needs you
// surfaces at once, not on the calm hold.
func TestCockpitHoldUrgentMoveCompletes(t *testing.T) {
	m := &OS{}
	now := time.Now()
	m.ckHoldGroup("a", ckWorking, now)
	m.ckHoldGroup("a", ckNeeds, now.Add(100*time.Millisecond))
	if got := m.ckHoldGroup("a", ckNeeds, now.Add(200*time.Millisecond)); got != ckWorking {
		t.Fatalf("a needs-you state held %v drew group %d, want %d still", 200*time.Millisecond, got, ckWorking)
	}
	if got := m.ckHoldGroup("a", ckNeeds, now.Add(700*time.Millisecond)); got != ckNeeds {
		t.Fatalf("a needs-you state held %v drew group %d, want %d", 600*time.Millisecond, got, ckNeeds)
	}
}

// TestCockpitHoldLeavingNeedsYouWaits checks the demotion: an agent that
// stopped needing you keeps its row there until the calm hold passes, so a
// missed answer does not slide away mid-read.
func TestCockpitHoldLeavingNeedsYouWaits(t *testing.T) {
	m := &OS{}
	now := time.Now()
	m.ckHoldGroup("a", ckNeeds, now)
	if got := m.ckHoldGroup("a", ckWorking, now.Add(1*time.Second)); got != ckNeeds {
		t.Fatalf("a working state held %v drew group %d, want %d still", time.Second, got, ckNeeds)
	}
	if got := m.ckHoldGroup("a", ckWorking, now.Add(6*time.Second)); got != ckWorking {
		t.Fatalf("a working state held %v drew group %d, want %d", 6*time.Second, got, ckWorking)
	}
}
