package session

import (
	"testing"

	"tuios.dev/tml"
)

// TestRecordTMLClosedDerivedFrom verifies the close-out copy: a surface
// whose state carries a root-level derived_from hands it to the artifact
// record when it closes, from the live store and not the doc's seed state.
// The guided-review e2e runs a shimmed daemon, so this copy has no other
// coverage.
func TestRecordTMLClosedDerivedFrom(t *testing.T) {
	d := NewDaemon(&DaemonConfig{})
	const caller = "pane-1"

	// The log records a surface only for a pane that holds a window; the
	// add stands in for recordTMLOpened's guard, which is not the copy
	// under test.
	d.artifacts.add(&artifactRec{id: "tml-derived", kind: ArtifactTML, title: "review", window: caller})

	doc, problems, err := tml.Check([]byte(`<tml version="1" title="review" state='{"derived_from": "seed"}'><text>hi</text></tml>`), nil)
	if err != nil || len(problems) > 0 {
		t.Fatalf("the seed document does not validate: %v %v", err, problems)
	}
	s := &tmlSurface{id: "tml-derived", caller: caller, title: "review"}
	if s.store, err = tml.StoreFor(doc, nil); err != nil {
		t.Fatal(err)
	}
	s.doc = doc
	s.docJSON = tml.FormatJSON(doc, false)

	// The owner patches the live store after open: the seed value goes away.
	if _, err := s.store.Set("state.derived_from", "pr-595"); err != nil {
		t.Fatal(err)
	}
	s.reason = TMLClosedByPerson
	d.recordTMLClosed(s)

	recs := d.artifacts.byWindow[caller]
	if len(recs) != 1 {
		t.Fatalf("the pane has %d artifact records, want 1", len(recs))
	}
	if recs[0].derived_from != "pr-595" {
		t.Fatalf("derived_from = %q, want the live state's value", recs[0].derived_from)
	}
	if recs[0].open || recs[0].reason != TMLClosedByPerson {
		t.Errorf("the record is not a closed one: open=%v reason=%q", recs[0].open, recs[0].reason)
	}

	// A surface whose value is not a string does not crash the copy, and a
	// non-string leaves the field empty.
	if _, err := s.store.Set("state.derived_from", 42.0); err != nil {
		t.Fatal(err)
	}
	d.recordTMLClosed(s)
	if recs[0].derived_from != "" {
		t.Errorf("a non-string derived_from was copied: %q", recs[0].derived_from)
	}
}
