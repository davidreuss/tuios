// The controller: one long-lived process that owns the review surface and
// the nvim pane. It is the ONLY writer of the surface and the ONLY driver
// of nvim; the person and agents steer it through surface events and the
// CLI. Ported from the reviewed-by-hand rig (the python owner), with the
// traps it hit already handled: adopt-or-create by both titles, sign-count
// truth, cancellable auto-play, recoverable handlers.
package walk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Controller is one review session. Create with StartController.
type Controller struct {
	mu       sync.Mutex
	m        *Model
	st       State
	dir      string // worktree the review walks
	base     string
	sock     string // nvim --listen socket
	session  string // tuios session name
	surface  string // adopted surface id
	liveFile string // the baked doc the last replace used
	log      *os.File

	playing  bool
	playCtx  context.Context
	playStop context.CancelFunc
}

// Options configures StartController.
type Options struct {
	Dir         string // worktree under review
	Base        string // the base ref
	RigID       string // surface/rig identity
	Session     string // tuios session name
	Workspace   string // workspace to open the surface on
	GuideJSON   []byte // optional guide (story.json shape)
	SurfaceOnly bool   // skip the nvim pane entirely
	LogPath     string
}

// sh runs a command and returns stdout; errors are logged, not fatal -
// each call site decides what a failure means.
func (c *Controller) sh(args ...string) string {
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		c.logf("sh %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

func (c *Controller) logf(format string, args ...any) {
	if c.log == nil {
		return
	}
	fmt.Fprintf(c.log, time.Now().Format("15:04:05 ")+" "+format+"\n", args...)
}

// StartController builds the model, opens (or adopts) the surface, spawns
// the nvim pane, and runs the event loop until the context ends or the
// review finishes. It blocks; run it in a goroutine or the foreground.
func StartController(ctx context.Context, opt Options) (*Controller, error) {
	logFile, err := os.OpenFile(opt.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	c := &Controller{
		dir:      opt.Dir,
		base:     opt.Base,
		session:  opt.Session,
		sock:     filepath.Join(os.TempDir(), "tui-review-"+opt.RigID+".sock"),
		liveFile: filepath.Join(os.TempDir(), "tui-review-"+opt.RigID+"-live.tml"),
		log:      logFile,
	}
	c.logf("controller starting, base=%s", opt.Base)

	m, err := BuildModel(ctx, opt.Dir, opt.Base, opt.RigID, opt.GuideJSON)
	if err != nil {
		return nil, err
	}
	c.m = m
	c.st = DefaultState(m)
	c.logf("model: %d steps, %d commits, base_sha=%s", len(m.Steps), len(m.Commits), m.BaseSHA)

	if err := c.ensureSurface(opt); err != nil {
		return nil, err
	}
	if !opt.SurfaceOnly {
		c.ensureNvim()
	}
	c.showStep(0)
	go c.watchHead(ctx)
	c.loop(ctx, opt)
	return c, nil
}

// ensureSurface adopts a matching surface or creates one. The trap this
// encodes: tuios retitles surfaces from the doc's root attribute, so match
// BOTH the window name and the doc title.
func (c *Controller) ensureSurface(opt Options) error {
	out := c.sh("tuios", "tml", "list", "-s", opt.Session)
	start := strings.Index(out, "{")
	if start >= 0 {
		var listing struct {
			Surfaces []struct {
				Surface string `json:"surface"`
				Title   string `json:"title"`
			} `json:"surfaces"`
		}
		if err := json.Unmarshal([]byte(out[start:]), &listing); err == nil {
			for _, s := range listing.Surfaces {
				if s.Title == opt.RigID || s.Title == "tui review" {
					c.surface = s.Surface
					break
				}
			}
		}
	}
	if c.surface != "" {
		c.logf("adopted surface %s", c.surface)
		return nil
	}
	doc := c.render()
	out = c.sh("tuios", "call", "tml-open", mustJSON(map[string]any{
		"session": opt.Session, "doc": doc,
		"target": "pane", "name": opt.RigID, "workspace": opt.Workspace,
	}))
	m := surfaceIDRe.FindString(out)
	if m == "" {
		return fmt.Errorf("tml-open produced no surface id: %s", strings.TrimSpace(out))
	}
	c.surface = m
	c.logf("created surface %s", c.surface)
	return nil
}

// nvim driving: one --remote-expr per action, never remote-send.
func (c *Controller) nvimLua(lua string) {
	if c.nvimAlive() {
		c.runNvim(lua)
		return
	}
	c.ensureNvim()
	c.runNvim(lua)
}

func (c *Controller) runNvim(lua string) {
	c.sh("nvim", "--server", c.sock, "--remote-expr",
		"luaeval(\""+strings.ReplaceAll(lua, `"`, `\"`)+"\")")
}

func (c *Controller) nvimAlive() bool {
	cmd := exec.Command("nvim", "--server", c.sock, "--remote-expr", "1")
	return cmd.Run() == nil
}

func (c *Controller) ensureNvim() {
	if c.nvimAlive() {
		return
	}
	c.logf("nvim down: recreating")
	_ = os.Remove(c.sock)
	out := c.sh("tuios", "list-windows", "-s", c.session)
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "pr595-nvim") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if len(f) == 8 && isHex(f) {
				c.sh("tuios", "close-window", f, "-s", c.session)
				break
			}
		}
	}
	cmd := fmt.Sprintf("nvim --listen %s -c 'lua dofile(\"%s\")'", c.sock, c.bridgePath())
	c.sh("tuios", "new-window", "pr595-nvim", "-s", c.session,
		"--workspace", "8", "--cwd", c.dir, "--no-focus",
		"--", "zsh", "-c", cmd)
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		if c.nvimAlive() {
			break
		}
	}
	c.logf("nvim up")
}

// bridgePath writes the embedded bridge next to the controller's live file
// on first use: the nvim pane loads it from disk.
func (c *Controller) bridgePath() string {
	p := filepath.Join(os.TempDir(), "tui-review-bridge.lua")
	if _, err := os.Stat(p); err != nil {
		_ = os.WriteFile(p, []byte(BridgeLua), 0o644)
	}
	return p
}

func isHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
