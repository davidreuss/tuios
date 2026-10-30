// Package walk turns a reviewed change into a guided walkthrough: the story
// model, the per-step diff block, and the TML surface document that presents
// both. Pure functions: no tuios wiring, no daemon, no nvim - everything
// here is testable from a fixture.
package walk

import (
	"fmt"
	"strings"

	"github.com/Gaurav-Gosain/tuios/internal/review"
)

// Context is how many unchanged lines the step's diff block shows around the
// step's range.
const Context = 6

// Step is one guided stop: a file, a line range, a label and a prose line.
type Step struct {
	Key     string `json:"key"`
	Chapter string `json:"chapter"`
	// Label is the short step title shown in rails and buttons.
	Label string `json:"label"`
	File  string `json:"file"`
	From  int    `json:"from"`
	To    int    `json:"to"`
	// Prose explains what the change does at this stop. One paragraph.
	Prose string `json:"prose,omitempty"`
	// Section names the group of commits or steps this belongs to.
	Section string `json:"section,omitempty"`
	// NewInBase marks a file the branch creates: the whole file is the diff.
	NewInBase bool `json:"new_in_base,omitempty"`
}

// Commit is one commit of the walked range.
type Commit struct {
	SHA     string   `json:"sha"`
	Subject string   `json:"subject"`
	Files   []string `json:"files,omitempty"`
	Added   int      `json:"added"`
	Removed int      `json:"removed"`
}

// Model is the walkthrough content: the story steps and the commits, plus
// the base they are anchored to. Version bumps when the tree changes under
// a walked model.
type Model struct {
	Rig       string   `json:"rig"`
	Base      string   `json:"base"`
	BaseSHA   string   `json:"base_sha"`
	Head      string   `json:"head"`
	Version   int      `json:"version"`
	Steps     []Step   `json:"steps"`
	Commits   []Commit `json:"commits"`
	// Diff is the built change the steps walk through. It rides the model
	// in memory only; the serialized model keeps steps and commits.
	Diff *review.Diff `json:"-"`
	Prose     string   `json:"prose"`
	Decisions []string `json:"decisions,omitempty"`
}

// Mark is a story row's state on the surface.
type Mark string

const (
	MarkAhead   Mark = ""
	MarkWalked  Mark = "✓"
	MarkCurrent Mark = "▶"
)

// State is the live part of the surface: where the person is, and what the
// controller wants the doc to show. The controller bakes it into the doc on
// every render.
type State struct {
	Tab     string `json:"tab"`
	StepIdx int    `json:"step_idx"`
	Now     string `json:"now"`
	Where   string `json:"where"`
	Desc    string `json:"desc"`
	Commit  string `json:"commit"`
	Stale   bool   `json:"stale"`
	Done    int    `json:"done"`
	// Marks is one entry per step, parallel to Model.Steps.
	Marks []Mark `json:"marks"`
}

// DefaultState is the state a fresh walkthrough starts at.
func DefaultState(m *Model) State {
	return State{Tab: "guide", Marks: make([]Mark, len(m.Steps))}
}

// HunkLines returns the step's diff block: the hunk nearest the step's
// range, trimmed to the range with Context lines around it. The block keeps
// its unified-diff shape: header lines, the @@ line, then the kept lines in
// order.
func HunkLines(d *review.Diff, step Step) []string {
	if d == nil {
		return nil
	}
	f := d.FileByPath(step.File)
	if f == nil {
		return nil
	}
	if len(f.Hunks) == 0 {
		return nil
	}
	best, bestDist := 0, -1
	for i, h := range f.Hunks {
		dist := hunkDist(h, step.From, step.To)
		if bestDist < 0 || dist < bestDist {
			best, bestDist = i, dist
		}
	}
	h := f.Hunks[best]
	lo, hi := step.From-Context, step.To+Context
	var kept []string
	for _, l := range h.Lines {
		switch l.Op {
		case review.OpAdd, review.OpContext:
			if l.New >= lo && l.New <= hi {
				kept = append(kept, diffPrefix(l))
			}
		case review.OpDelete:
			// A removed line has no new-side line number; keep it when the
			// new-side window covers the position it sat at.
			if l.New >= lo && l.New <= hi {
				kept = append(kept, diffPrefix(l))
			}
		}
	}
	if len(kept) == 0 {
		for i, l := range h.Lines {
			if i >= 2*Context {
				break
			}
			kept = append(kept, diffPrefix(l))
		}
	}
	out := []string{
		fmt.Sprintf("diff --git a/%s b/%s", step.File, step.File),
		h.Header,
	}
	return append(out, kept...)
}

func diffPrefix(l review.Line) string {
	switch l.Op {
	case review.OpAdd:
		return "+" + l.Text
	case review.OpDelete:
		return "-" + l.Text
	default:
		return " " + l.Text
	}
}

func hunkDist(h review.Hunk, from, to int) int {
	start, end := h.NewStart, h.NewStart+h.NewLines-1
	if start <= to && end >= from {
		return 0
	}
	d1, d2 := from-end, start-from
	if d1 >= 0 && d2 >= 0 {
		return minInt(d1, d2)
	}
	if d1 >= 0 {
		return d1
	}
	return d2
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EscapeXML escapes text that goes into a TML raw-content block.
func EscapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
