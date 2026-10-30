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
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	adopted  bool   // the surface pre-existed and was adopted
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
	BakeDoc     bool   // bake-and-replace per step, instead of bound-content patches
	Takeover    bool   // adopt a surface whose controller is still live
	DerivedFrom string // artifact-record link (default: the worktree name)
	LogPath     string
}

// pidFile is the single-writer lock: the rig identity maps to the pid of
// the controller that owns its surface and pane.
func pidFile(rigID string) string {
	return filepath.Join(os.TempDir(), "tui-review-"+rigID+".pid")
}

// claimOwnership refuses a second controller on the same review unless
// Takeover is set. The pid file plus a live-process check keeps a crashed
// controller from locking the review forever.
func claimOwnership(rigID string, takeover bool) error {
	pidPath := pidFile(rigID)
	if raw, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if n, _ := fmt.Sscanf(string(raw), "%d", &pid); n == 1 && processAlive(pid) {
			if !takeover {
				return fmt.Errorf("review %s is already walked by pid %d. "+
					"Close it there, or run again with --takeover", rigID, pid)
			}
			// taking over: the old controller loses the single-writer
			// guarantee, which is what the flag means.
		}
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("claim review %s: %w", rigID, err)
	}
	return nil
}

// releaseOwnership drops the claim if this process still holds it.
func releaseOwnership(rigID string) {
	pidPath := pidFile(rigID)
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	var pid int
	if n, _ := fmt.Sscanf(string(raw), "%d", &pid); n != 1 || pid != os.Getpid() {
		return
	}
	_ = os.Remove(pidPath)
}

// processAlive reports whether pid names a running process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
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

	if opt.Session == "" {
		// The CLI refuses an empty --session; the pane's own session is the
		// right default.
		opt.Session = os.Getenv("TUIOS_SESSION")
	}
	if err := c.preflight(opt); err != nil {
		return nil, err
	}
	if err := claimOwnership(opt.RigID, opt.Takeover); err != nil {
		return nil, err
	}
	defer releaseOwnership(opt.RigID)

	m, err := BuildModel(ctx, opt.Dir, opt.Base, opt.RigID, opt.GuideJSON)
	if err != nil {
		return nil, err
	}
	if opt.DerivedFrom != "" {
		m.DerivedFrom = opt.DerivedFrom
	}
	m.BoundContent = !opt.BakeDoc
	c.m = m
	c.st = DefaultState(m)
	c.logf("model: %d steps, %d commits, base_sha=%s", len(m.Steps), len(m.Commits), m.BaseSHA)

	if err := c.ensureSurface(opt); err != nil {
		return nil, err
	}
	// An adopted surface can carry a stale doc from an earlier review; the
	// bound-content walk patches state only, so install the structure once.
	if c.adopted && !opt.BakeDoc {
		c.replace()
	}
	if !opt.SurfaceOnly {
		c.ensureNvim()
	}
	c.showStep(0)
	go c.watchHead(ctx)
	c.loop(ctx, opt)
	return c, nil
}

// preflight fails fast when the controller runs inside a tuios pane whose
// grants cannot drive a review: the surface verbs and the pane management
// would otherwise surface a y/n prompt in the middle of the walkthrough.
// A controller from the person's shell (outside every pane) needs nothing.
func (c *Controller) preflight(opt Options) error {
	if os.Getenv("TUIOS_SESSION") == "" {
		return nil
	}
	held := map[string]bool{"admin": true, "write": true}
	if out := c.sh("tuios", "pane-grants", "--json"); strings.Contains(out, "{") {
		var grants struct {
			Grants []string `json:"grants"`
		}
		_ = json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &grants)
		held = map[string]bool{}
		for _, g := range grants.Grants {
			held[g] = true
		}
		c.logf("grants: %v", grants.Grants)
	}
	// admin covers write, and write covers the surface writes; admin also
	// covers the new-window the nvim pane needs.
	if !held["admin"] && !held["write"] {
		return fmt.Errorf("this pane's grants cannot drive a review (has %v). " +
			"Run 'tuios review walk' from your own shell, or hold admin: " +
			"ask for it with the Inbox, then rerun", held)
	}
	return nil
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
		c.adopted = true
		return nil
	}
	doc := c.render()
	out = c.sh("tuios", "call", "tml-open", mustJSON(map[string]any{
		"session": opt.Session, "doc": doc,
		"target": "pane", "name": opt.RigID, "workspace": workspaceParam(opt.Workspace),
	}))
	m := surfaceIDRe.FindString(out)
	if m == "" {
		return fmt.Errorf("tml-open produced no surface id: %s", strings.TrimSpace(out))
	}
	c.surface = m
	c.logf("created surface %s", c.surface)
	return nil
}

// workspaceParam sends numeric workspace selections as JSON numbers: the
// daemon's tml-open types the field as int and refuses a string.
func workspaceParam(ws string) any {
	if n, err := strconv.Atoi(strings.TrimSpace(ws)); err == nil {
		return n
	}
	return ws
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
