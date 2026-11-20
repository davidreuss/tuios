package app

import (
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// The routing these tests pin is the one thing in the link feature that is a
// safety property rather than a convenience: a client running on a server must
// not open a browser there, because nobody is sitting at that machine.

// TestLinkFilePathAcceptsOnlyLocalFiles checks the parse that decides whether a
// link names a file on the machine the panes are on.
//
// The expected answers are written out from the URLs, not derived: a file URL
// with no host or with localhost is this machine's, one with another host is
// not, and an http URL is not a file at all.
//
// Negative control: with the host check dropped from localCwdPath, the
// other-host case comes back ok and this fails.
func TestLinkFilePathAcceptsOnlyLocalFiles(t *testing.T) {
	cases := []struct {
		url  string
		want string // "" means not a local file
	}{
		{"file:///etc/hosts", "/etc/hosts"},
		{"file://localhost/etc/hosts", "/etc/hosts"},
		{"file://someotherbox/etc/hosts", ""},
		{"https://example.com/a", ""},
		{"/etc/hosts", ""}, // a bare path is not a link
	}
	for _, c := range cases {
		got, ok := linkFilePath(c.url)
		if c.want == "" {
			if ok {
				t.Errorf("linkFilePath(%q) = %q, want no local file", c.url, got)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("linkFilePath(%q) = %q,%v, want %q,true", c.url, got, ok, c.want)
		}
	}
}

// TestRemoteClientCopiesRatherThanOpens is the footgun this feature was most
// likely to ship with.
//
// Under `tuios ssh` and `tuios-web` the client process runs on the server. A
// browser opened from there opens on the server's console, in front of nobody,
// and the person who clicked sees nothing happen. tuios has no way to run
// anything on the viewer's machine, so the honest action is the one that does
// reach it: OSC 52, which rides the same stream the frame does.
//
// The test asserts on the command rather than on a spawned process, because the
// bug it guards is that a process is spawned at all.
//
// Negative control: with the IsRemoteClient branch removed from OpenLink, a
// remote client returns a nil command here (having tried to spawn a browser)
// and this fails.
func TestRemoteClientCopiesRatherThanOpens(t *testing.T) {
	m := &OS{Settings: config.Global, RemoteClient: true}
	if cmd := m.OpenLink("https://example.com/a"); cmd == nil {
		t.Fatal("a remote client produced no clipboard write; the click did nothing at all")
	}
	if n := len(m.Notifications); n == 0 {
		t.Fatal("a remote client said nothing about what it did instead")
	}
}

// TestOnlyKnownSchemesReachTheDesktop is a safety property, not a convenience.
//
// A program in a pane chooses both halves of an OSC 8 link: the words on screen
// and the address behind them. The address is then handed to xdg-open, which
// resolves a scheme to whatever application the desktop registered for it, so an
// unfiltered address is a way for a pane to start an arbitrary application from
// one click on text it also wrote.
//
// Negative control, confirmed red: with the linkOpenableScheme test removed from
// OpenLink, every refused case below returns nil after trying to spawn a viewer.
func TestOnlyKnownSchemesReachTheDesktop(t *testing.T) {
	allowed := []string{
		"http://example.com/a",
		"https://example.com/a",
		"HTTPS://example.com/a", // the scheme is case-insensitive
		"mailto:someone@example.com",
		"ftp://example.com/a",
	}
	for _, u := range allowed {
		if !linkOpenableScheme(u) {
			t.Errorf("%q was refused; it is one of the schemes a terminal is asked for", u)
		}
	}

	// Each of these resolves to a registered application on some desktop, and
	// none of them is a link a pane's reader meant to click.
	refused := []string{
		"javascript:alert(1)",
		"data:text/html,<script>x</script>",
		"vscode://file/etc/passwd",
		"ssh://root@example.com",
		"smb://example.com/share",
		"example.com/a", // no scheme at all
		"",
	}
	for _, u := range refused {
		if linkOpenableScheme(u) {
			t.Errorf("%q was accepted for the desktop's own handler", u)
		}
	}

	// And the refusal is the clipboard, not silence. A click that appears to do
	// nothing is worse than one that says what it did instead.
	m := &OS{Settings: config.Global}
	if cmd := m.OpenLink("vscode://file/etc/passwd"); cmd == nil {
		t.Fatal("a refused scheme produced no clipboard fallback")
	}
	if len(m.Notifications) == 0 {
		t.Error("a refused scheme said nothing about what it did instead")
	}
}

// TestTuiosWindowLinksParse checks the shapes the tuios:// scheme answers:
// a window id focuses it, a /close suffix closes it, a /queue suffix with a
// text parameter queues the decoded text, and anything else is not an
// address tuios acts on.
//
// Negative control: with the host check dropped from parseTuiosLink, the
// tuios://session/... case comes back ok and this fails.
func TestTuiosWindowLinksParse(t *testing.T) {
	good := []struct {
		url     string
		id      string
		closing bool
		text    string
	}{
		{"tuios://window/4262c319", "4262c319", false, ""},
		{"tuios://window/4262c319/close", "4262c319", true, ""},
		{"tuios://window/a878cd43-0b70-4c1e-bd43-7b78de51cc43", "a878cd43-0b70-4c1e-bd43-7b78de51cc43", false, ""},
		{"tuios://window/4262c319/queue?text=run%20the%20tests", "4262c319", false, "run the tests"},
	}
	for _, c := range good {
		got, ok := parseTuiosLink(c.url)
		if !ok || got.id != c.id || got.close != c.closing || got.queue != c.text {
			t.Errorf("%q parsed as %+v, ok=%v; want id %q close=%v queue=%q", c.url, got, ok, c.id, c.closing, c.text)
		}
	}

	bad := []string{
		"tuios://window/",
		"tuios://window",
		"tuios://window//close",
		"tuios://session/4262c319",
		"tuios://pane/4262c319",
		"https://window/4262c319",
		// A queue link without a message queues nothing, and the empty text
		// is refused like any other shape tuios does not answer.
		"tuios://window/4262c319/queue",
		"tuios://window/4262c319/queue?text=",
		// Below the 8-character short id: a prefix that short names a window
		// one time in many, and the program printing the link chose it, not
		// the person clicking.
		"tuios://window/4262c31",
		"tuios://window/4262c31/close",
		"tuios://window/4262c31/queue?text=x",
		"tuios://window/a/close",
	}
	for _, u := range bad {
		if got, ok := parseTuiosLink(u); ok {
			t.Errorf("%q parsed as %+v; want refusal", u, got)
		}
	}
}

// TestTuiosWindowLinkResolution checks that a window id resolves to exactly
// one window, by full uuid or unique prefix of at least the short form, and
// that a prefix two windows share names nothing.
func TestTuiosWindowLinkResolution(t *testing.T) {
	m := &OS{Settings: config.Global}
	m.Windows = []*terminal.Window{
		{ID: "a878cd43-0b70-4c1e-bd43-7b78de51cc43"},
		{ID: "a878cd44-1111-2222-3333-444455556666"},
		{ID: "ffffffff-0000-0000-0000-000000000001"},
	}

	resolve := func(id string) (string, bool) {
		got := m.resolveWindowLink(id, nil)
		if len(got) != 1 {
			return "", false
		}
		return got[0].id, true
	}
	if id, ok := resolve("a878cd43-0b70-4c1e-bd43-7b78de51cc43"); !ok || id != m.Windows[0].ID {
		t.Errorf("full uuid resolved to %q, %v; want window 0", id, ok)
	}
	if id, ok := resolve("4262c319"); ok {
		t.Errorf("unknown prefix resolved to %q; want no window", id)
	}
	if id, ok := resolve("a878cd4"); ok {
		t.Errorf("ambiguous prefix resolved to %q; want no window", id)
	}
	if id, ok := resolve("ffff"); !ok || id != m.Windows[2].ID {
		t.Errorf("unique prefix resolved to %q, %v; want window 2", id, ok)
	}

	// Another session's panes resolve too, and a prefix shared across
	// sessions is as ambiguous as one shared within the attached session.
	m.SessionName = "home"
	listing := []session.SessionInfo{
		{Name: "home", Windows: []session.WindowSummary{{ID: "ffffffff-0000-0000-0000-000000000001"}}},
		{Name: "away", Windows: []session.WindowSummary{
			{ID: "bbbbbbbb-0000-0000-0000-000000000001"},
			{ID: "ffffffff-0000-0000-0000-000000000002"},
		}},
	}
	if got := m.resolveWindowLink("bbbbbbbb", listing); len(got) != 1 || got[0].session != "away" {
		t.Errorf("another session's pane resolved to %+v; want one match in away", got)
	}
	if got := m.resolveWindowLink("ffffffff", listing); len(got) != 2 {
		t.Errorf("prefix shared across sessions resolved to %+v; want both, so the click refuses", got)
	}
}

// TestTuiosWindowCloseConfirms checks that the close half of the scheme
// raises the confirmation naming the pane, and that the pane dies only when
// the dialog's destructive row runs.
func TestTuiosWindowCloseConfirms(t *testing.T) {
	m := &OS{Settings: config.Global}
	w := &terminal.Window{ID: "a878cd43-0b70-4c1e-bd43-7b78de51cc43"}
	w.SetTitle("build watcher")
	m.Windows = []*terminal.Window{w}

	m.OpenLink("tuios://window/a878cd43/close")
	if !m.ShowSessionClose || m.SessionCloseWindow != w.ID {
		t.Fatalf("a close link skipped the confirmation: shown=%v window=%q", m.ShowSessionClose, m.SessionCloseWindow)
	}
	if q := m.sessionCloseQuestion(); q != "Close pane 'build watcher'?" {
		t.Errorf("confirmation question was %q", q)
	}
	if len(m.Windows) != 1 {
		t.Fatal("the pane died before the dialog was answered")
	}

	m.SessionCloseActivate(SessionCloseRowCancel)
	if len(m.Windows) != 1 {
		t.Fatal("cancel closed the pane")
	}

	// A pane that appears ahead of the target while the dialog is open moves
	// it in m.Windows. The confirmation still closes the pane it named.
	m.OpenLink("tuios://window/a878cd43/close")
	other := &terminal.Window{ID: "ffffffff-0000-0000-0000-000000000001"}
	m.Windows = append([]*terminal.Window{other}, m.Windows...)
	m.SessionCloseActivate(SessionCloseRowClose)
	if len(m.Windows) != 1 || m.Windows[0] != other {
		t.Error("confirming the dialog did not close the pane it named")
	}
}

// TestLinkLabelHintForTuios checks the hint says what the click does to a
// tuios link, not "open": nothing opens, and the close half confirms.
func TestLinkLabelHintForTuios(t *testing.T) {
	s := config.Global
	s.LinkClick = config.LinkClickCtrl
	if got := linkLabelHintFor("tuios://window/a878cd43-0b70-4c1e-bd43-7b78de51cc43", &s); got != "ctrl+click jumps to that window" {
		t.Errorf("focus hint was %q", got)
	}
	if got := linkLabelHintFor("tuios://window/a878cd43-0b70-4c1e-bd43-7b78de51cc43/close", &s); got != "ctrl+click asks to close that window" {
		t.Errorf("close hint was %q", got)
	}
	if got := linkLabelHintFor("tuios://window/a878cd43-0b70-4c1e-bd43-7b78de51cc43/queue?text=hi", &s); got != "ctrl+click queues a message for that pane" {
		t.Errorf("queue hint was %q", got)
	}
	if got := linkLabelHintFor("https://example.com", &s); got != "ctrl+click to open" {
		t.Errorf("plain-URL hint was %q", got)
	}
}

// TestTuiosWindowLinkQueues checks the action half of the queue form: the
// click reaches the daemon's queue-prompt verb as the person, with the text
// the URL carried and the pane the id resolved to, and the answer comes back
// as a message that says what happened.
func TestTuiosWindowLinkQueues(t *testing.T) {
	m := inboxOS(t, zeroSettle())
	q := &queueFake{}
	m.SetInboxVerbCaller(q.call, func() string { return "nonce-1" })
	w := &terminal.Window{ID: "a878cd43-0b70-4c1e-bd43-7b78de51cc43"}
	m.Windows = []*terminal.Window{w}

	// The attached session's pane: no session named, the full id after the
	// prefix resolved, and the nonce of the person who clicked.
	runMsg(t, m, m.OpenLink("tuios://window/a878cd43/queue?text=run%20the%20tests"))
	p, ok := q.last("queue-prompt")
	if !ok {
		t.Fatal("the click never reached queue-prompt")
	}
	if p["text"] != "run the tests" || p["window"] != w.ID || p["human_nonce"] != "nonce-1" {
		t.Errorf("queue-prompt got %v", p)
	}
	if _, named := p["session"]; named {
		t.Errorf("queue-prompt was sent a session for the attached session's pane: %v", p)
	}
	if lastNotice(m).Type != "success" {
		t.Errorf("the click said nothing about queueing: %+v", lastNotice(m))
	}
	if len(m.Windows) != 1 || m.SessionName != "here" {
		t.Error("a queue link moved the person's session or its panes")
	}

	// A pane of another session is queued into by name, and the click does
	// not switch the person to that session first.
	q.calls = nil
	cmd := m.followWindowLink(mustParseQueue(t, "tuios://window/bbbbbbbb/queue?text=hi"),
		"tuios://window/bbbbbbbb/queue?text=hi",
		[]session.SessionInfo{{Name: "away", Windows: []session.WindowSummary{{ID: "bbbbbbbb-0000-0000-0000-000000000001"}}}})
	runMsg(t, m, cmd)
	if p, ok = q.last("queue-prompt"); !ok || p["session"] != "away" || p["window"] != "bbbbbbbb-0000-0000-0000-000000000001" {
		t.Errorf("queue-prompt for another session's pane got %v, ok=%v", p, ok)
	}
	if m.SessionName != "here" {
		t.Error("queueing into another session's pane switched the client there")
	}

	// A refusal from the daemon is said, not swallowed.
	q.calls = nil
	q.err = &session.VerbCallError{Code: session.ErrVerbQueueFull, Message: "the pane's queue is full"}
	runMsg(t, m, m.OpenLink("tuios://window/a878cd43/queue?text=hi"))
	if n := lastNotice(m); n.Type != "error" || !strings.Contains(n.Message, "queue is full") {
		t.Errorf("a refused queue said %q", n.Message)
	}
}

// mustParseQueue parses a queue link the tests above build, failing the test
// when the shape stops parsing.
func mustParseQueue(t *testing.T, rawURL string) tuiosLink {
	t.Helper()
	target, ok := parseTuiosLink(rawURL)
	if !ok {
		t.Fatalf("%q stopped parsing", rawURL)
	}
	return target
}
