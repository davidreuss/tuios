package app

import (
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/config"
)

// The back stack: a walk home and back again, through the one handler every
// listing lands in.
func TestFileBackCapturesTheWalk(t *testing.T) {
	m := &OS{Settings: config.Global}
	m.filesView.Gen = 1
	m.filesView.Origin = "win1"

	// First listing: the starting directory, nothing behind it yet.
	m.HandleFileList(fileListMsg{Gen: 1, Dir: "/home/me/a"})
	if m.FileBackDir() != "" {
		t.Fatalf("back offered %q with no step behind it", m.FileBackDir())
	}

	// A step in: back must offer where the walk started.
	m.filesView.Gen = 2 // requestFileList bumps the generation for each ask
	m.HandleFileList(fileListMsg{Gen: 2, Dir: "/home/me/a/sub"})
	if got := m.FileBackDir(); got != "/home/me/a" {
		t.Fatalf("back offered %q, want the directory just left", got)
	}

	// Taking it: the listing goes back, and the stack holds the way forward.
	cmd := m.FileViewBack()
	if cmd == nil {
		t.Fatal("FileViewBack returned no command")
	}
	if m.filesView.Want != "/home/me/a" {
		t.Fatalf("back asked for %q", m.filesView.Want)
	}
}
