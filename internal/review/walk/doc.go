// Doc rendering: the walkthrough surface as TML text. One function per tab,
// one assembly point - RenderDoc - that bakes the live state and the step's
// diff block into the document.
package walk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// tab titles are kept short: the tab strip must fit a 115-column pane.
var chapterShort = map[string]string{
	"ch1": "1", "ch2": "2", "ch3": "3", "ch4": "4",
	"ch5": "5", "ch6": "6", "ch7": "7",
}

// RenderDoc bakes m and st into the TML document text. The diff block of
// the current step is rendered raw (the diff component takes literal
// content); everything live rides in <state>.
func RenderDoc(m *Model, st State) string {
	var b strings.Builder
	b.WriteString("<tml version=\"1\" title=\"tui review\">\n")

	steps := m.Steps
	if st.StepIdx < 0 || st.StepIdx >= len(steps) {
		st.StepIdx = 0
	}
	if len(steps) == 0 {
		steps = []Step{{Key: "empty", Label: "no steps: the diff is empty or no guide was given"}}
	}
	step := steps[st.StepIdx]

	b.WriteString("<state>")
	b.Write(jsonState(m, st))
	b.WriteString("</state>\n")

	b.WriteString("  <col pad=\"1\">\n")

	// Guide tab: the walking view.
	b.WriteString("  <tab id=\"guide\" title=\"Guide\">\n    <col gap=\"1\" pad=\"1\">\n")
	if st.Stale {
		b.WriteString("      <text role=\"warning\"><b>stale: the tree moved under this walkthrough (model v" +
			itoa(m.Version) + ") - press Refresh to rebuild and resume</b></text>\n")
	}
	fmt.Fprintf(&b, "      <text>Now: <b>%s</b></text>\n", escapeXML(st.Now))
	fmt.Fprintf(&b, "      <text><b>%s</b></text>\n", escapeXML(st.Where))
	if st.Desc != "" {
		fmt.Fprintf(&b, "      <text>%s</text>\n", escapeXML(st.Desc))
	}
	b.WriteString("      <diff id=\"stepdiff\" lang=\"go\" layout=\"unified\">")
	if block := HunkLines(m.Diff, step); len(block) > 0 {
		b.WriteString(EscapeXML(strings.Join(block, "\n")))
	} else {
		b.WriteString("(no diff for this file in this range)")
	}
	b.WriteString("</diff>\n")
	b.WriteString("      <row gap=\"2\">\n")
	b.WriteString("        <button key=\"j\" kind=\"primary\" on-press=\"emit('next')\">▸ Next step</button>\n")
	b.WriteString("        <button key=\"k\" on-press=\"emit('prev')\">▸ Prev</button>\n")
	b.WriteString("        <button key=\"p\" on-press=\"emit('play')\">▸ Auto-play all</button>\n")
	b.WriteString("        <button key=\"v\" on-press=\"emit('whole')\">▸ Whole diff</button>\n")
	b.WriteString("        <button key=\"f\" on-press=\"emit('finish')\">▸ Finish</button>\n")
	b.WriteString("      </row>\n")
	b.WriteString("    </col>\n  </tab>\n")

	// Story tab: the full arc, click a row to jump.
	b.WriteString("  <tab id=\"story\" title=\"Story\">\n")
	b.WriteString("    <table rows=\"state.story\" on-select=\"emit('goto')\" select=\"single\">\n")
	b.WriteString("      <column field=\"mark\" title=\"\"/>\n")
	b.WriteString("      <column field=\"chapter\" title=\"chapter\"/>\n")
	b.WriteString("      <column field=\"label\" title=\"step\"/>\n")
	b.WriteString("      <column field=\"where\" title=\"file\" w=\"44\"/>\n")
	b.WriteString("    </table>\n  </tab>\n")

	// Commits tab: one row per commit, message text above the table.
	b.WriteString("  <tab id=\"commits\" title=\"Commits\">\n    <col gap=\"1\" pad=\"1\">\n")
	if st.Commit != "" {
		fmt.Fprintf(&b, "      <text>{state.commit}</text>\n")
	}
	b.WriteString("      <table rows=\"state.commits\" on-select=\"emit('commit')\" select=\"single\">\n")
	b.WriteString("        <column field=\"sha\" title=\"sha\" w=\"10\"/>\n")
	b.WriteString("        <column field=\"subject\" title=\"subject\" w=\"60\"/>\n")
	b.WriteString("        <column field=\"delta\" title=\"+/-\"/>\n")
	b.WriteString("      </table>\n")
	b.WriteString("    </col>\n  </tab>\n")

	// Notes tab: the person's annotations, anchored.
	b.WriteString("  <tab id=\"notes\" title=\"Notes\">\n    <col gap=\"1\" pad=\"1\">\n")
	b.WriteString("      <text>Visual-select lines in the review pane, press <b>leader n</b>, type the note. Notes anchor to the lines.</text>\n")
	b.WriteString("      <table rows=\"state.notes\" select=\"single\">\n")
	b.WriteString("        <column field=\"state\" title=\"\" w=\"8\"/>\n")
	b.WriteString("        <column field=\"file\" title=\"file\" w=\"30\"/>\n")
	b.WriteString("        <column field=\"lines\" title=\"lines\" w=\"12\"/>\n")
	b.WriteString("        <column field=\"comment\" title=\"note\"/>\n")
	b.WriteString("      </table>\n")
	b.WriteString("    </col>\n  </tab>\n")

	b.WriteString("  </col>\n</tml>\n")
	return b.String()
}

// ViewRows flattens the model for the Story table: one row per step.
func ViewRows(m *Model, st State) []map[string]string {
	rows := make([]map[string]string, len(m.Steps))
	for i, s := range m.Steps {
		mark := ""
		switch {
		case i == st.StepIdx:
			mark = string(MarkCurrent)
		case st.Marks != nil && i < len(st.Marks) && st.Marks[i] == MarkWalked:
			mark = string(MarkWalked)
		}
		where := fmt.Sprintf("%s L%d-%d", s.File, s.From, s.To)
		if s.NewInBase {
			where += " (new in branch)"
		}
		rows[i] = map[string]string{
			"mark":    mark,
			"chapter": chapterLabel(s.Chapter),
			"label":   s.Label,
			"where":   where,
		}
	}
	return rows
}

// CommitRows flattens the commits for the Commits table.
func CommitRows(m *Model) []map[string]string {
	rows := make([]map[string]string, len(m.Commits))
	for i, c := range m.Commits {
		rows[i] = map[string]string{
			"sha":     shortSHA(c.SHA),
			"subject": c.Subject,
			"delta":   fmt.Sprintf("+%d/-%d", c.Added, c.Removed),
		}
	}
	return rows
}

func chapterLabel(ch string) string {
	if s, ok := chapterShort[ch]; ok {
		return s
	}
	return ch
}

func shortSHA(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// JSONState writes the doc's <state> payload: the rig identity, the current
// step, and the view rows the tables bind to.
func jsonState(m *Model, st State) []byte {
	rig := map[string]any{
		"id":            m.Rig,
		"base":          m.Base,
		"model_version": m.Version,
		"head":          m.Head,
	}
	if len(m.Steps) == 0 {
		m.Steps = []Step{{Key: "empty", Label: "no steps"}}
	}
	s := m.Steps[st.StepIdx]
	payload := map[string]any{
		"rig":   rig,
		"derived_from": m.DerivedFrom,
		"step":  map[string]any{"index": st.StepIdx, "file": s.File, "from": s.From, "to": s.To, "prose": s.Prose},
		"tab":   st.Tab,
		"stale": st.Stale,
		"now":   st.Now,
		"where": st.Where,
		"desc":  st.Desc,
		"commit": st.Commit,
		"done":  st.Done,
		"story": ViewRows(m, st),
		"commits": CommitRows(m),
		"notes": []any{},
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return []byte("{}")
	}
	return out
}
