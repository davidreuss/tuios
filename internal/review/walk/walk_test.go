package walk

import (
	"context"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/testutil"
)

// fixture walks a tiny two-commit repo: a base commit with loader.go and
// main.go, then a branch commit changing both.
func fixture(t *testing.T) (*Model, string) {
	t.Helper()
	repo := testutil.GitRepo(t)
	testutil.Git(t, repo, "config", "user.email", "t@t")
	testutil.Git(t, repo, "config", "user.name", "t")
	writeFile(t, repo+"/loader.go", "package main\n\nfunc Load() {\n\t// base\n}\n")
	writeFile(t, repo+"/main.go", "package main\n\nfunc main() {\n\tLoad()\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "base")
	testutil.Git(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, repo+"/loader.go", "package main\n\nfunc Load(dir string) {\n\t// walks the dir now\n\tfor range 3 {\n\t}\n}\n")
	writeFile(t, repo+"/new.go", "package main\n\n// a brand new file\nfunc Extra() int {\n\treturn 7\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "feat: load a dir")

	guide := []GuideStep{{
		Key: "s1", Chapter: "ch1", Label: "Load takes a dir",
		File: "loader.go", Lnum: 3, Endl: 6, Desc: "Load walks a directory now.",
	}, {
		Key: "s2", Chapter: "ch1", Label: "Extra arrives",
		File: "new.go", Lnum: 3, Endl: 5, Desc: "A brand new helper.",
	}}
	b, err := jsonMarshal(guide)
	if err != nil {
		t.Fatal(err)
	}
	m, err := BuildModel(context.Background(), repo, "main", "rig-1", b)
	if err != nil {
		t.Fatal(err)
	}
	return m, repo
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := osWriteFile(path, body); err != nil {
		t.Fatal(err)
	}
}

func TestBuildModelAnchorsToTheMergeBase(t *testing.T) {
	m, _ := fixture(t)
	if m.BaseSHA == "" || m.BaseSHA == m.Head {
		t.Fatalf("BaseSHA = %q, Head = %q: want the base below HEAD", m.BaseSHA, m.Head)
	}
	if len(m.Commits) != 1 || m.Commits[0].Subject != "feat: load a dir" {
		t.Fatalf("commits = %+v", m.Commits)
	}
	if len(m.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(m.Steps))
	}
	if !m.Steps[1].NewInBase {
		t.Errorf("new.go step should mark NewInBase")
	}
	if m.Steps[0].NewInBase {
		t.Errorf("loader.go step should not mark NewInBase")
	}
}

func TestHunkLinesTrimsToTheRange(t *testing.T) {
	m, _ := fixture(t)
	block := HunkLines(m.Diff, m.Steps[0])
	if len(block) == 0 {
		t.Fatal("no diff block for the loader step")
	}
	joined := strings.Join(block, "\n")
	if !strings.Contains(joined, "+func Load(dir string)") {
		t.Errorf("block misses the changed line: %q", joined)
	}
	if strings.Contains(joined, "main()") {
		t.Errorf("block leaked main.go content: %q", joined)
	}
	for _, l := range block {
		if len(l) > 0 && l[0] != '+' && l[0] != '-' && l[0] != ' ' && !strings.HasPrefix(l, "diff --git") && !strings.HasPrefix(l, "@@") {
			t.Errorf("line lost its diff prefix: %q", l)
		}
	}
}

func TestHunkLinesPicksTheNearestHunk(t *testing.T) {
	m, _ := fixture(t)
	// A step pointing far from the only hunk still lands on it.
	s := m.Steps[0]
	s.From, s.To = 40, 44
	block := HunkLines(m.Diff, s)
	if len(block) == 0 {
		t.Fatal("nearest-hunk fallback returned nothing")
	}
	if !strings.Contains(strings.Join(block, "\n"), "+func Load") {
		t.Errorf("fallback picked the wrong hunk: %q", strings.Join(block, "\n"))
	}
}

func TestRenderDocBakesStateAndDiff(t *testing.T) {
	m, _ := fixture(t)
	st := DefaultState(m)
	st.StepIdx = 1
	st.Marks[0] = MarkWalked
	st.Now = "2/2 Extra arrives"
	st.Where = "new.go L3-5 (new in branch)"
	st.Desc = "A brand new helper."
	doc := RenderDoc(m, st)

	for _, want := range []string{
		`<tab id="guide" title="Guide">`,
		`<tab id="story" title="Story">`,
		`<tab id="commits" title="Commits">`,
		`<tab id="notes" title="Notes">`,
		"rows=\"state.story\"",
		"rows=\"state.commits\"",
		"▸ Next step",
		"+func Extra() int",
		"new.go L3-5 (new in branch)",
		"feat: load a dir",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("doc misses %q", want)
		}
	}
	// No visual-mode default: attention is the box, not a selection dial.
	if strings.Contains(doc, "state.marks\"") {
		t.Errorf("story table binds the wrong state path")
	}
}

func TestRenderDocEscapesTheDiff(t *testing.T) {
	_, repo := fixture(t)
	writeFile(t, repo+"/new.go", "package main\n\nfunc Extra() int {\n\tif 1 < 2 && 3 > 2 {\n\t\treturn 7\n\t}\n\treturn 0\n}\n")
	testutil.Git(t, repo, "add", ".")
	testutil.Git(t, repo, "commit", "-q", "-m", "feat: comparisons")
	guide := []GuideStep{{
		Key: "s1", Chapter: "ch1", Label: "comparisons",
		File: "new.go", Lnum: 3, Endl: 6,
	}}
	gb, err := jsonMarshal(guide)
	if err != nil {
		t.Fatal(err)
	}
	m, err := BuildModel(context.Background(), repo, "main", "rig-1", gb)
	if err != nil {
		t.Fatal(err)
	}
	doc := RenderDoc(m, DefaultState(m))
	if strings.Contains(doc, "1 < 2 && 3 > 2") {
		t.Errorf("raw < and > leaked into the doc XML")
	}
	if !strings.Contains(doc, "1 &lt; 2 &amp;&amp; 3 &gt; 2") {
		t.Errorf("escaped comparison missing from the doc")
	}
}
