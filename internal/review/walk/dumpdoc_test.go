package walk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/testutil"
)

// Writes a rendered doc from the real worktree so `tuios tml validate`
// can name the daemon's problems.
func TestDumpRenderedDoc(t *testing.T) {
	dir := "/Users/dreuss/.local/share/tuios/worktrees/tuios/dreuss.tui-review"
	m, err := BuildModel(t.Context(), dir, "aaaa128cd", "walk-doc", nil)
	if err != nil {
		t.Skipf("build: %v", err)
	}
	m.BoundContent = false
	t.Logf("diff files=%d commits=%d", len(m.Diff.Files), len(m.Commits))
	for i, c := range m.Commits {
		if i < 3 {
			t.Logf("commit %s +%d/-%d files=%v", c.SHA[:8], c.Added, c.Removed, c.Files)
		}
	}
	doc := RenderDoc(m, DefaultState(m))
	out := filepath.Join(os.TempDir(), "walk-doc.tml")
	if err := os.WriteFile(out, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", out)
	_ = testutil.GitRepo
}
