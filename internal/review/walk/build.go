// Model building: from a git range to the walkthrough's story and commits.
package walk

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Gaurav-Gosain/tuios/internal/review"
)

// GuideStep is one step of an agent-authored guide: the story.json shape the
// rig used. The guide supplies the narrative; the diff supplies the truth.
type GuideStep struct {
	Key     string `json:"key"`
	Chapter string `json:"chapter"`
	Label   string `json:"label"`
	File    string `json:"file"`
	Lnum    int    `json:"lnum"`
	Endl    int    `json:"endl"`
	Section string `json:"section"`
	Desc    string `json:"desc"`
}

// BuildModel builds the walkthrough model for the range base..HEAD of the
// worktree at dir. guideJSON, when given, carries GuideStep entries; steps
// the diff cannot find keep their prose but mark NewInBase when the file is
// new, and are dropped when both look at nothing.
func BuildModel(ctx context.Context, dir, base, rigID string, guideJSON []byte) (*Model, error) {
	resolved, err := review.ResolveBase(ctx, dir, base, "", false)
	if err != nil {
		return nil, fmt.Errorf("resolve base: %w", err)
	}
	d, err := review.Build(ctx, review.Options{Dir: dir, Base: resolved, Context: 3})
	if err != nil {
		return nil, fmt.Errorf("build diff: %w", err)
	}
	m := &Model{
		Rig:     rigID,
		Base:    base,
		BaseSHA: d.BaseSHA,
		Diff:    d,
		Version: 1,
	}
	m.Head = strings.TrimSpace(runGit(ctx, dir, "log", "--format=%H", "-1"))
	m.Commits = buildCommits(ctx, dir, d.BaseSHA)

	if len(guideJSON) > 0 {
		steps, err := stepsFromGuide(d, guideJSON)
		if err != nil {
			return nil, err
		}
		m.Steps = steps
	}
	return m, nil
}

// AutoSteps builds steps without a guide: one per file, one hunk per stop.
func AutoSteps(d *review.Diff) []Step {
	var steps []Step
	for i := range d.Files {
		f := &d.Files[i]
		if len(f.Hunks) == 0 {
			continue
		}
		for j, h := range f.Hunks {
			steps = append(steps, Step{
				Key:      fmt.Sprintf("s%d_%d", i, j),
				Chapter:  fmt.Sprintf("file%d", i),
				Label:    fmt.Sprintf("%s hunk %d at +%d", f.Path, j+1, h.NewStart),
				File:     f.Path,
				From:     h.NewStart,
				To:       h.NewStart + h.NewLines - 1,
				NewInBase: f.Status == "A",
			})
		}
	}
	return steps
}

func stepsFromGuide(d *review.Diff, guideJSON []byte) ([]Step, error) {
	var guide []GuideStep
	if err := jsonUnmarshal(guideJSON, &guide); err != nil {
		return nil, fmt.Errorf("parse guide: %w", err)
	}
	steps := make([]Step, 0, len(guide))
	for _, g := range guide {
		s := Step{
			Key:      g.Key,
			Chapter:  g.Chapter,
			Label:    g.Label,
			File:     g.File,
			From:     g.Lnum,
			To:       g.Endl,
			Section:  g.Section,
			Prose:    g.Desc,
			NewInBase: fileStatus(d, g.File) == "A",
		}
		if s.From <= 0 {
			s.From = 1
		}
		if s.To < s.From {
			s.To = s.From
		}
		steps = append(steps, s)
	}
	return steps, nil
}

func fileStatus(d *review.Diff, path string) string {
	f := d.FileByPath(path)
	if f == nil {
		return ""
	}
	return f.Status
}

func buildCommits(ctx context.Context, dir, baseSHA string) []Commit {
	out := runGit(ctx, dir, "log", "--reverse", "--format=%H%x09%s", baseSHA+"..HEAD")
	var commits []Commit
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		sha, subj, _ := strings.Cut(line, "\t")
		commits = append(commits, Commit{SHA: sha, Subject: subj})
	}
	return commits
}

func runGit(ctx context.Context, dir string, args ...string) string {
	out, err := review.Git(ctx, dir, nil, args...)
	if err != nil {
		return ""
	}
	return out
}

func osWriteFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
