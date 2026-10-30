// The controller's event loop and actions: walking, playing, jumping,
// stale watching, finishing.
package walk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"regexp"
	"strings"
	"time"
)

var surfaceIDRe = regexp.MustCompile(`tml-[a-z0-9]+`)

// mustJSON marshals v or panics - callers pass only maps of literals.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

type regexpRe struct{}

// render bakes the current model and state into the live doc.
func (c *Controller) render() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	rig := map[string]any{
		"id":            c.m.Rig,
		"base":          c.base,
		"model_version": c.m.Version,
		"head":          c.m.Head,
		"controller":    os.Getpid(),
	}
	step := Step{Key: "empty", Label: "no steps"}
	if len(c.m.Steps) > 0 {
		if c.st.StepIdx >= 0 && c.st.StepIdx < len(c.m.Steps) {
			step = c.m.Steps[c.st.StepIdx]
		} else {
			step = c.m.Steps[0]
		}
	}
	payload := map[string]any{
		"rig":     rig,
		"step":    map[string]any{"index": stStepIdx(c.st), "file": step.File, "from": step.From, "to": step.To, "prose": step.Prose},
		"tab":     c.st.Tab,
		"stale":   c.st.Stale,
		"now":     c.st.Now,
		"where":   c.st.Where,
		"desc":    c.st.Desc,
		"commit":  c.st.Commit,
		"done":    c.st.Done,
		"story":   ViewRows(c.m, c.st),
		"commits": CommitRows(c.m),
		"notes":   []any{},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte("{}")
	}
	doc := RenderDoc(c.m, c.st)
	open := strings.Index(doc, "<state>") + len("<state>")
	close := strings.Index(doc[open:], "</state>")
	var sb strings.Builder
	sb.WriteString(doc[:open])
	sb.Write(b)
	sb.WriteString(doc[open+close:])
	return sb.String()
}

func stStepIdx(st State) int { return st.StepIdx }

// replace bakes and swaps the doc in place. Same surface id, always.
func (c *Controller) replace() {
	doc := c.render()
	if err := os.WriteFile(c.liveFile, []byte(doc), 0o644); err != nil {
		c.logf("write live doc: %v", err)
		return
	}
	c.sh("tuios", "tml", "replace", c.surface, c.liveFile)
}

// patch applies a JSON patch to the surface state.
func (c *Controller) patch(ops string) {
	c.sh("tuios", "tml", "patch", c.surface, ops)
}

// showStep walks to step i: nvim jump + baked doc.
func (c *Controller) showStep(i int) {
	c.mu.Lock()
	if i < 0 {
		i = 0
	}
	step := Step{Key: "empty", Label: "no steps"}
	if len(c.m.Steps) > 0 {
		if i >= len(c.m.Steps) {
			i = 0
		}
		c.st.StepIdx = i
		step = c.m.Steps[i]
	}
	c.st.Now = fmt.Sprintf("%d/%d %s", i+1, len(c.m.Steps), step.Label)
	c.st.Where = fmt.Sprintf("%s L%d-%d", step.File, step.From, step.To)
	if step.NewInBase {
		c.st.Where += " (new in branch)"
	}
	c.st.Desc = strings.TrimSpace(step.Prose)
	if step.Section != "" {
		c.st.Desc = step.Section + ": " + c.st.Desc
	}
	if i < len(c.st.Marks) {
		c.st.Marks[i] = MarkCurrent
	}
	c.st.Done = c.countWalked()
	c.mu.Unlock()

	c.nvimLua(fmt.Sprintf("ReviewGo('%s', %d, %d, '%s', '%s')",
		step.File, step.From, step.To, c.base, escLua(c.st.Desc)))
	c.replace()
	c.logf("step %d: %s L%d-%d", i, step.File, step.From, step.To)
}

func (c *Controller) countWalked() int {
	n := 0
	for _, m := range c.st.Marks {
		if m == MarkWalked {
			n++
		}
	}
	return n
}

func escLua(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// loop reads the subscribe stream and dispatches events until ctx ends.
func (c *Controller) loop(ctx context.Context, opt Options) {
	cmd := exec.CommandContext(ctx, "tuios", "subscribe", "--session", opt.Session, "--types", "tml-event")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.logf("subscribe pipe: %v", err)
		return
	}
	if err := cmd.Start(); err != nil {
		c.logf("subscribe start: %v", err)
		return
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	c.logf("subscribed, surface=%s", c.surface)

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Tml  *struct {
				Surface string `json:"surface"`
				Name    string `json:"name"`
				Value   any    `json:"value"`
			} `json:"tml"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Type != "tml-event" || ev.Tml == nil {
			continue
		}
		if ev.Tml.Surface != "" && ev.Tml.Surface != c.surface {
			continue
		}
		c.handle(ev.Tml.Name, ev.Tml.Value)
	}
	c.logf("event loop ended: %v", sc.Err())
}

// handle dispatches one surface event. Every handler is recoverable: one
// bad event logs, none of them kills the loop.
func (c *Controller) handle(name string, value any) {
	defer func() {
		if r := recover(); r != nil {
			c.logf("handler %s panic: %v", name, r)
		}
	}()
	switch name {
	case "next", "prev":
		c.stopPlay()
		c.mu.Lock()
		i := c.st.StepIdx
		if name == "next" {
			i++
		} else {
			i--
		}
		c.mu.Unlock()
		c.showStep(i)
	case "goto":
		if i, ok := toInt(value); ok {
			c.showStep(i)
		}
	case "commit":
		sha, _ := value.(string)
		if sha == "" {
			return
		}
		c.patch(`[{"op":"replace","path":"/tab","value":"commits"}]`)
		c.nvimLua(fmt.Sprintf("ReviewCommit('%s')", escLua(sha)))
		msg := c.sh("git", "show", "-s", "--format=fuller", sha)
		c.mu.Lock()
		lines := strings.Split(strings.TrimSpace(msg), "\n")
		if len(lines) > 14 {
			lines = lines[:14]
		}
		c.st.Commit = strings.Join(lines, "\n")
		c.st.Where = "commit " + shortSHA(sha)
		c.mu.Unlock()
		c.replace()
		c.logf("commit view %s", sha)
	case "play":
		c.togglePlay()
	case "whole":
		c.nvimLua(fmt.Sprintf("vim.cmd('DiffviewOpen %s')", c.base))
		c.logf("whole diff")
	case "finish":
		c.finish()
	case "refresh":
		c.refresh()
	default:
		c.logf("unhandled event %s %v", name, value)
	}
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}

// togglePlay starts or stops auto-play. Any steering event stops it.
func (c *Controller) togglePlay() {
	c.mu.Lock()
	if c.playing {
		c.playing = false
		if c.playStop != nil {
			c.playStop()
		}
		c.mu.Unlock()
		c.logf("play cancelled by play press")
		return
	}
	c.playing = true
	c.playCtx, c.playStop = context.WithCancel(context.Background())
	ctx := c.playCtx
	c.mu.Unlock()
	c.logf("play started")
	go func() {
		for {
			c.mu.Lock()
			i := c.st.StepIdx + 1
			c.mu.Unlock()
			if i >= len(c.m.Steps) {
				break
			}
			select {
			case <-ctx.Done():
				c.logf("play stopped")
				return
			case <-time.After(2500 * time.Millisecond):
			}
			c.showStep(i)
		}
		c.mu.Lock()
		c.playing = false
		c.mu.Unlock()
		c.patch(`[{"op":"replace","path":"/now","value":"walkthrough complete"}]`)
		c.logf("play complete")
	}()
}

func (c *Controller) stopPlay() {
	c.mu.Lock()
	c.playing = false
	if c.playStop != nil {
		c.playStop()
	}
	c.mu.Unlock()
}

// refresh rebuilds the model against the moved HEAD and resumes at the
// step the person was on.
func (c *Controller) refresh() {
	c.mu.Lock()
	resume := c.st.StepIdx
	c.st.Stale = false
	c.mu.Unlock()
	guide := guideFromModel(c.m)
	m, err := BuildModel(context.Background(), c.dir, c.base, c.m.Rig, guide)
	if err != nil {
		c.logf("refresh: %v", err)
		return
	}
	c.mu.Lock()
	c.m = m
	if resume >= len(m.Steps) {
		resume = len(m.Steps) - 1
	}
	c.mu.Unlock()
	c.logf("refreshed to model v%d, resume at %d", m.Version, resume)
	c.showStep(resume)
}

// guideFromModel serializes the model's steps back into a guide so a
// refresh keeps the narrative.
func guideFromModel(m *Model) []byte {
	steps := make([]GuideStep, len(m.Steps))
	for i, s := range m.Steps {
		steps[i] = GuideStep{Key: s.Key, Chapter: s.Chapter, Label: s.Label,
			File: s.File, Lnum: s.From, Endl: s.To, Section: s.Section, Desc: s.Prose}
	}
	b, _ := json.Marshal(steps)
	return b
}

// watchHead flips the surface to stale when HEAD moves under the model.
func (c *Controller) watchHead(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		head := strings.TrimSpace(c.sh("git", "log", "--format=%H", "-1"))
		c.mu.Lock()
		moved := head != "" && head != c.m.Head && !c.st.Stale
		if moved {
			c.st.Stale = true
		}
		c.mu.Unlock()
		if moved {
			c.logf("HEAD moved to %s: stale", head)
			c.patch(`[{"op":"replace","path":"/stale","value":true}]`)
			c.replace()
		}
	}
}

// finish closes the review: the last doc registers as an artifact and the
// controller exits 0.
func (c *Controller) finish() {
	c.logf("finish")
	c.patch(`[{"op":"replace","path":"/now","value":"review complete"}]`)
	// Artifact registration lands with the cockpit integration (stage 2).
	os.Exit(0)
}
