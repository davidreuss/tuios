package session

import (
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/worktree"
)

// shipGateOutbound builds the outbound call a gate test needs: a pane of the
// session and a push target, with every field the confirm token hashes.
func shipGateOutbound(t *testing.T, d *Daemon, sess *Session) shipOutbound {
	t.Helper()
	if _, err := sess.AddDaemonWindow("Window", nil); err != nil {
		t.Fatalf("AddDaemonWindow: %v", err)
	}
	wins := sess.GetState().Windows
	if len(wins) == 0 {
		t.Fatal("the test session holds no window")
	}
	return shipOutbound{
		sess:   sess,
		target: wins[0],
		repo:   reviewRepo{root: t.TempDir()},
		branch: "feat/retry",
		commit: "abc1234",
		push:   worktree.PushTarget{Remote: "origin", URL: "github.com/o/r.git"},
	}
}

func TestShipOutboundGateDefaultAsksThePerson(t *testing.T) {
	d, sess := newTestDaemonSession(t)
	o := shipGateOutbound(t, d, sess)
	p := shipPushParams{Wait: 30}
	p.Confirm = o.token("ship-push", p)

	approved, verr := d.shipOutboundGate(&connState{paneOnly: true, done: make(chan struct{})}, "ship-push", "tuios ship push", o, p)
	if verr == nil || verr.Code != ErrVerbNotReady {
		t.Fatalf("the default config did not ask the person: approved %q, verr %+v", approved, verr)
	}
	if approved != "" {
		t.Errorf("approved_by = %q, want empty while the question is open", approved)
	}
}

func TestShipOutboundGateNeverShipsWithoutTheAsk(t *testing.T) {
	d, sess := newTestDaemonSession(t)
	d.shipOutbound = config.ShipOutboundNever
	o := shipGateOutbound(t, d, sess)
	p := shipPushParams{Wait: 30}
	p.Confirm = o.token("ship-push", p)

	approved, verr := d.shipOutboundGate(&connState{paneOnly: true, done: make(chan struct{})}, "ship-push", "tuios ship push", o, p)
	if verr != nil {
		t.Fatalf("ship_outbound = never still refused the push: %+v", verr)
	}
	if approved != "" {
		t.Errorf("approved_by = %q, want empty: the caller is not the person", approved)
	}
}

func TestShipOutboundGateViaLinkRefused(t *testing.T) {
	for _, mode := range []string{"", config.ShipOutboundInbox, config.ShipOutboundNever} {
		t.Run("mode="+mode, func(t *testing.T) {
			d, sess := newTestDaemonSession(t)
			d.shipOutbound = mode
			o := shipGateOutbound(t, d, sess)
			p := shipPushParams{Wait: 30}
			p.Confirm = o.token("ship-push", p)

			_, verr := d.shipOutboundGate(&connState{viaLink: true, done: make(chan struct{})}, "ship-push", "tuios ship push", o, p)
			if verr == nil || verr.Code != ErrVerbForbidden {
				t.Fatalf("a push over a link was not refused under ship_outbound %q: %+v", mode, verr)
			}
		})
	}
}

func TestShipOutboundGateStillNeedsConfirmToken(t *testing.T) {
	d, sess := newTestDaemonSession(t)
	d.shipOutbound = config.ShipOutboundNever
	o := shipGateOutbound(t, d, sess)

	_, verr := d.shipOutboundGate(&connState{paneOnly: true, done: make(chan struct{})}, "ship-push", "tuios ship push", o, shipPushParams{Wait: 30})
	if verr == nil || verr.Code != ErrVerbConfirmRequired {
		t.Fatalf("ship_outbound = never did not keep the confirm token check: %+v", verr)
	}
}
