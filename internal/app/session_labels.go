package app

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/Gaurav-Gosain/tuios/internal/session"
)

// sessionIndexSuffix is the " [n]" tail of a session whose identity name ends
// in digits. Sessions are named session-N, so renaming hides the number a
// person needs to tell sessions apart; the suffix puts it back. Display only.
func sessionIndexSuffix(name string) string {
	i := len(name)
	for i > 0 && name[i-1] >= '0' && name[i-1] <= '9' {
		i--
	}
	if i == len(name) || i == 0 {
		return ""
	}
	return " [" + name[i:] + "]"
}

// withSessionIndex appends the identity index to a display title. A title
// already carrying the suffix, and a title that is the identity name itself,
// stay untouched: the latter would read "session-1 [1]".
func withSessionIndex(name, title string) string {
	suffix := sessionIndexSuffix(name)
	if suffix == "" || title == "" || title == name || strings.HasSuffix(title, suffix) {
		return title
	}
	return title + suffix
}

// adoptSessionLabels copies the daemon-owned labels off a state push. They are
// daemon-exclusive: the client never sends them back (BuildSessionState omits
// them and the merge treats an empty incoming value as "not sent"), so this is
// a read-only adoption and a client can never clear a label by syncing.
func (m *OS) adoptSessionLabels(state *session.SessionState) {
	m.SessionDisplayName = state.DisplayName
	m.SessionAccent = state.Accent
	m.SessionRestored = state.Restored
	m.SessionGlobal = state.Global
	// The worktree record travels with the state, so the attached row is drawn
	// from the same push every other label on it comes from. A copy, because
	// the state snapshot is the daemon's and this outlives the call.
	m.SessionWorktree = nil
	if state.Worktree != nil {
		wt := *state.Worktree
		m.SessionWorktree = &wt
	}
	// A drag in flight owns the arrangement until the pointer comes up. Adopting
	// mid-drag would snap the pills back under the pointer on any push that
	// happened to land, and the push that matters is this client's own.
	if !m.dockWorkspaceDrag.Dragging {
		m.WorkspaceOrder = slices.Clone(state.WorkspaceOrder)
	}
	if len(state.WorkspaceNames) == 0 {
		m.WorkspaceNames = nil
		return
	}
	m.WorkspaceNames = maps.Clone(state.WorkspaceNames)
}

// SessionLabel is what to show for a session: its display name when it has one,
// otherwise the identity name. Never use it as a key.
func (m *OS) SessionLabel(name string) string {
	if name == m.SessionName && m.SessionDisplayName != "" {
		return withSessionIndex(name, m.SessionDisplayName)
	}
	if m.DaemonClient != nil {
		if display, _ := m.DaemonClient.SessionLabel(name); display != "" {
			return withSessionIndex(name, display)
		}
	}
	return name
}

// sessionTitle is the one name a session is shown by in every view: the rail's
// title for it (sessiontree.BuildSession), which is its display name, else the
// directory of its focused pane when tuios made the name up, else the name.
// The rail said "demo" while the Inbox, the peek, the dock's alerts, the
// palette and the close dialog said "session-0" for the same session.
//
// It is for showing only. Never use it as a key: the daemon addresses a
// session by its name.
func (m *OS) sessionTitle(name string) string {
	if display := m.SessionLabel(name); display != name && display != "" {
		return display
	}
	if dir, _ := m.sessionPlace(name); dir != "" {
		return dir
	}
	return name
}

// sessionPlace is where a session is, as the rail shows it: the directory label
// for its focused pane and the git branch there, both from the daemon's listing
// and both empty until the listing has said. The directory is offered only for
// a name tuios generated, since "session-3" says nothing and a name a person
// chose says more than a directory. The branch follows either.
func (m *OS) sessionPlace(name string) (dir, branch string) {
	if m.DaemonClient == nil {
		return "", ""
	}
	dir, branch = m.DaemonClient.SessionPlace(name)
	if !session.IsGeneratedSessionName(name) {
		dir = ""
	}
	return dir, branch
}

// WorkspaceLabel is what to show for a workspace of the attached session. An
// unnamed workspace reads back as its number, which is both its identity and
// the label it has always shown.
func (m *OS) WorkspaceLabel(ws int) string {
	if name := m.WorkspaceNames[ws]; name != "" {
		return name
	}
	return strconv.Itoa(ws)
}
