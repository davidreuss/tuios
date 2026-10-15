package tuie2e

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuios/internal/overlay"
	"github.com/Gaurav-Gosain/tuitest"
)

func TestCurrentSessionStaysHighlighted(t *testing.T) {
	for _, side := range []string{"left", "right"} {
		for _, look := range []struct{ name, theme, bg string }{
			{name: "dark"},
			{name: "light", theme: "catppuccin_latte"},
			{name: "host", bg: "#5b5a60"},
		} {
			for _, depth := range chromeDepths {
				t.Run(side+"-"+look.name+"-"+depth.name, func(t *testing.T) {
					const cols, rows, width = 120, 32, 28
					cfg := fmt.Sprintf("[appearance]\ntheme = %q\ndockbar_position = 'top'\n"+
						"[appearance.sidebar]\nenabled = true\nposition = %q\nwidth = %d\n", look.theme, side, width)
					opts := startOpts{cols: cols, rows: rows, env: depth.env}
					if look.bg != "" {
						opts.wrap = []string{buildHostTerm(t), "run", "-bg", look.bg, "-fg", "#eeeeee", "-ansi", hostANSISpec(), "--"}
					}
					term, base := railClient(t, "alpha", cfg, opts)
					if out, err := tuiosCLI(t, base, "new", "bravo", "--detach"); err != nil {
						t.Fatalf("create bravo: %v\n%s", err, out)
					}

					// Restrict names to the rail so the dock cannot satisfy a check.
					nameAt := func(s tuitest.Screen, name string) (int, int) {
						start := 0
						if side == "right" {
							start = cols - width
						}
						for y := 1; y < rows-1; y++ {
							for x := start; x+len(name) <= start+width; x++ {
								match := true
								for i, ch := range name {
									match = match && s.Cell(x+i, y).Content == string(ch)
								}
								if match {
									return x, y
								}
							}
						}
						return -1, -1
					}
					if err := term.WaitFor(func(s tuitest.Screen) bool {
						_, a := nameAt(s, "alpha")
						_, b := nameAt(s, "bravo")
						return a >= 0 && b >= 0
					}, uiTimeout); err != nil {
						t.Fatalf("rail never listed both sessions: %v\n%s", err, term.Snapshot())
					}
					dir := artifactDir(t)
					host := hostPalette(t, look.theme)
					if look.bg != "" {
						host = hostGroundPalette("#eeeeee", look.bg)
					}
					frame := func(name string) tuitest.Screen {
						t.Helper()
						if err := term.WaitStable(uiTimeout); err != nil {
							t.Fatalf("%s did not settle: %v", name, err)
						}
						s := term.Screen()
						saveArtifact(t, term, dir, name)
						savePNG(t, s, host, dir, name)
						return s
					}
					cell := func(s tuitest.Screen, name string) tuitest.Cell {
						t.Helper()
						x, y := nameAt(s, name)
						if x < 0 {
							t.Fatalf("rail lost %s\n%s", name, s.Text())
						}
						return s.Cell(x, y)
					}
					waitCell := func(name, state string, accept func(tuitest.Cell) bool) {
						t.Helper()
						if err := term.WaitFor(func(s tuitest.Screen) bool {
							x, y := nameAt(s, name)
							return x >= 0 && accept(s.Cell(x, y))
						}, uiTimeout); err != nil {
							t.Fatalf("%s never reached %s: %v\n%s", name, state, err, term.SnapshotStyled())
						}
					}
					background := func(c tuitest.Cell) color.Color {
						v := resolveCell(c.Bg, host, host.BG)
						return color.RGBA{R: v.R, G: v.G, B: v.B, A: 0xff}
					}
					checkBand := func(s tuitest.Screen, name string, rest tuitest.Cell, minimum float64) {
						t.Helper()
						if depth.name == "16" {
							return
						}
						x, y := nameAt(s, name)
						bg := background(cell(s, name))
						if ratio := overlay.ContrastRatio(bg, background(rest)); ratio < minimum {
							t.Errorf("%s band contrast = %.2f:1, want at least %g:1", name, ratio, minimum)
						}
						start := 0
						if side == "right" {
							start = cols - width
						}
						for col := start; col < start+width; col++ {
							c := s.Cell(col, y)
							if c.Bg != cell(s, name).Bg || strings.TrimSpace(c.Content) == "" {
								continue
							}
							v := resolveCell(c.Fg, host, host.FG)
							fg := color.RGBA{R: v.R, G: v.G, B: v.B, A: 0xff}
							floor := overlay.MarkFloor
							if col >= x && col < x+len(name) {
								floor = overlay.ContrastFloor
							}
							if ratio := overlay.ContrastRatio(fg, bg); ratio < floor {
								t.Errorf("%s cell %d %q contrast = %.2f:1, want %.1f:1", name, col, c.Content, ratio, floor)
							}
						}
					}
					checkResting := func(s tuitest.Screen, active, other string, attention bool) {
						t.Helper()
						if strings.Contains(s.Text(), railPill) {
							t.Fatal("rail still owns the keyboard")
						}
						for name, bold := range map[string]bool{active: true, other: attention} {
							x, y := nameAt(s, name)
							if x < 0 {
								t.Fatalf("rail lost %s", name)
							}
							for i := range len(name) {
								if s.Cell(x+i, y).Bold != bold {
									t.Errorf("%s character %d bold = %v, want %v", name, i, !bold, bold)
								}
							}
						}
						a, b := cell(s, active), cell(s, other)
						checkBand(s, active, b, 1.08)
						if depth.name != "16" {
							ceiling := 1.6
							if ratio := overlay.ContrastRatio(background(a), background(b)); ratio > ceiling {
								t.Errorf("active band contrast = %.2f:1, want at most %g:1", ratio, ceiling)
							}
						}
						if depth.name == "16" {
							if !a.Underline || b.Underline || a.Reverse || b.Reverse {
								t.Errorf("resting session attributes: active=%+v other=%+v", a, b)
							}
						} else {
							if a.Bg == b.Bg {
								t.Errorf("active session has no persistent background: active=%+v other=%+v", a, b)
							}
							x, y := nameAt(s, active)
							if s.Cell(x+len(active)+2, y).Bg != a.Bg {
								t.Error("active background stops at the name instead of filling the row")
							}
						}
					}

					moveMouse(t, term, cols/2, rows/2)
					initial := frame("alpha-active")
					checkResting(initial, "alpha", "bravo", false)
					x, y := nameAt(initial, "bravo")
					moveMouse(t, term, x, y)
					waitCell("bravo", "hover", func(c tuitest.Cell) bool {
						if depth.name == "16" {
							return c.Underline
						}
						return c.Bg != cell(initial, "bravo").Bg
					})
					hovered := frame("bravo-hovered")
					checkBand(hovered, "bravo", cell(initial, "bravo"), 1.03)
					if depth.name != "16" {
						activeBG := background(cell(hovered, "alpha"))
						hoverBG := background(cell(hovered, "bravo"))
						restBG := background(cell(initial, "bravo"))
						activeContrast := overlay.ContrastRatio(activeBG, restBG)
						hoverContrast := overlay.ContrastRatio(hoverBG, restBG)
						if hoverContrast >= activeContrast {
							t.Error("hover must be dimmer than the active-session band")
						}
						ceiling := 1.2
						if depth.name == "256" {
							ceiling = 1.25
						}
						if ratio := overlay.ContrastRatio(activeBG, hoverBG); ratio < 1.05 || ratio > ceiling {
							t.Errorf("active/hover band contrast = %.2f:1, want 1.05:1 to %g:1", ratio, ceiling)
						}
					}
					if a := cell(hovered, "alpha"); !a.Bold || a.Bg != cell(initial, "alpha").Bg {
						t.Errorf("hovering bravo changed active alpha: %+v", a)
					}
					if b := cell(hovered, "bravo"); b.Bold || (depth.name == "16" && !b.Underline) ||
						(depth.name != "16" && b.Bg == cell(initial, "bravo").Bg) {
						t.Errorf("hover treatment lost: %+v", b)
					}
					x, y = nameAt(hovered, "alpha")
					moveMouse(t, term, x, y)
					waitCell("bravo", "hover moved to alpha", func(c tuitest.Cell) bool {
						return !c.Underline && c.Bg == cell(initial, "bravo").Bg
					})
					checkResting(frame("alpha-hovered"), "alpha", "bravo", false)
					moveMouse(t, term, cols/2, rows/2)
					waitCell("bravo", "hover cleared", func(c tuitest.Cell) bool {
						return !c.Underline && c.Bg == cell(initial, "bravo").Bg
					})
					checkResting(frame("hover-cleared"), "alpha", "bravo", false)

					sendKeys(t, term, "s")
					if err := term.WaitForText(railPill, uiTimeout); err != nil {
						t.Fatalf("rail did not take keyboard focus: %v", err)
					}
					sendKeys(t, term, "j")
					waitCell("bravo", "keyboard cursor", func(c tuitest.Cell) bool {
						if depth.name == "16" {
							return c.Reverse
						}
						return c.Bg != cell(initial, "bravo").Bg
					})
					focused := frame("bravo-cursor")
					a, b := cell(focused, "alpha"), cell(focused, "bravo")
					if !a.Bold || b.Bold || (depth.name == "16" && (!b.Reverse || a.Reverse)) ||
						(depth.name != "16" && a.Bg == b.Bg) {
						t.Errorf("keyboard cursor and active session are not distinct: active=%+v cursor=%+v", a, b)
					}
					checkBand(focused, "bravo", cell(initial, "bravo"), 2)
					if depth.name != "16" {
						if ratio := overlay.ContrastRatio(background(a), background(b)); ratio < 1.2 {
							t.Errorf("keyboard/active band contrast = %.2f:1, want at least 1.2:1", ratio)
						}
					}
					x, y = nameAt(focused, "bravo")
					moveMouse(t, term, x, y)
					if c := cell(frame("cursor-hovered"), "bravo"); c.Bg != b.Bg || c.Reverse != b.Reverse {
						t.Errorf("hover overrides keyboard cursor: before=%+v after=%+v", b, c)
					}
					moveMouse(t, term, cols/2, rows/2)
					sendKeys(t, term, tuitest.Enter)
					if err := term.WaitForText("Session: bravo", uiTimeout); err != nil {
						t.Fatalf("did not switch to bravo: %v\n%s", err, term.Snapshot())
					}
					sendKeys(t, term, tuitest.Esc)
					if err := term.WaitFor(func(s tuitest.Screen) bool {
						return !strings.Contains(s.Text(), railPill)
					}, uiTimeout); err != nil {
						t.Fatalf("rail kept keyboard focus: %v", err)
					}
					checkResting(frame("bravo-active"), "bravo", "alpha", false)

					if out, err := tuiosCLI(t, base, "set-agent-state", "needs_input", "-s", "alpha"); err != nil {
						t.Fatalf("set alpha attention: %v\n%s", err, out)
					}
					waitCell("alpha", "attention", func(c tuitest.Cell) bool { return c.Bold })
					checkResting(frame("alpha-attention"), "bravo", "alpha", true)
					alive(t, term, "after active-session styling")
				})
			}
		}
	}
}

func TestRailContextTokenContrast(t *testing.T) {
	for _, depth := range chromeDepths[1:] {
		t.Run(depth.name, func(t *testing.T) {
			const token = "ctx 91%"
			const cfg = "[appearance]\ntheme = 'catppuccin_latte'\n[appearance.sidebar]\nenabled = true\nposition = 'left'\nwidth = 40\n"
			term, base := railClient(t, "tokens", cfg, startOpts{cols: 120, rows: 32, env: depth.env})
			for _, args := range [][]string{
				{"set-agent-state", "working", "-s", "tokens", "-w", "0", "--harness", "claude-code"},
				{"set-agent-meta", "-s", "tokens", "-w", "0", "--source", "statusline", "context=91%"},
			} {
				if out, err := tuiosCLI(t, base, args...); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			locate := func(s tuitest.Screen) (int, int) {
				for y := 1; y < 31; y++ {
					for x := 0; x+len(token) < 40; x++ {
						var text strings.Builder
						for i := range len(token) {
							text.WriteString(s.Cell(x+i, y).Content)
						}
						if text.String() == token {
							return x, y
						}
					}
				}
				return -1, -1
			}
			if err := term.WaitFor(func(s tuitest.Screen) bool { x, _ := locate(s); return x >= 0 }, uiTimeout); err != nil {
				t.Fatalf("context token missing: %v\n%s", err, term.Snapshot())
			}
			host, dir := hostPalette(t, "catppuccin_latte"), artifactDir(t)
			frame := func(name string) tuitest.Cell {
				t.Helper()
				if err := term.WaitStable(uiTimeout); err != nil {
					t.Fatal(err)
				}
				s := term.Screen()
				x, y := locate(s)
				if x < 0 {
					t.Fatalf("context token disappeared\n%s", s.Text())
				}
				saveArtifact(t, term, dir, name)
				savePNG(t, s, host, dir, name)
				for i := range len(token) {
					c := s.Cell(x+i, y)
					fg, bg := resolveCell(c.Fg, host, host.FG), resolveCell(c.Bg, host, host.BG)
					ink := color.RGBA{R: fg.R, G: fg.G, B: fg.B, A: 0xff}
					ground := color.RGBA{R: bg.R, G: bg.G, B: bg.B, A: 0xff}
					if ratio := overlay.ContrastRatio(ink, ground); ratio < overlay.MarkFloor {
						t.Errorf("%s token cell %d contrast = %.2f:1, want 3:1", name, i, ratio)
					}
				}
				return s.Cell(x, y)
			}
			moveMouse(t, term, 70, 20)
			rest := frame("context-resting")
			x, y := locate(term.Screen())
			moveMouse(t, term, x, y)
			if err := term.WaitFor(func(s tuitest.Screen) bool {
				x, y := locate(s)
				return x >= 0 && s.Cell(x, y).Bg != rest.Bg
			}, uiTimeout); err != nil {
				t.Fatalf("context hover never appeared: %v", err)
			}
			if c := frame("context-hovered"); c.Bg == rest.Bg {
				t.Fatal("hover did not fill context token row")
			}
			moveMouse(t, term, 70, 20)
			if err := term.WaitFor(func(s tuitest.Screen) bool {
				x, y := locate(s)
				return x >= 0 && s.Cell(x, y).Bg == rest.Bg
			}, uiTimeout); err != nil {
				t.Fatalf("context hover never cleared: %v", err)
			}
			sendKeys(t, term, "s")
			if err := term.WaitForText(railPill, uiTimeout); err != nil {
				t.Fatal(err)
			}
			focused := false
			for range 20 {
				if err := term.WaitStable(uiTimeout); err != nil {
					t.Fatal(err)
				}
				x, y := locate(term.Screen())
				if x >= 0 && term.Screen().Cell(x, y).Bg != rest.Bg {
					focused = true
					break
				}
				sendKeys(t, term, "j")
			}
			if !focused {
				t.Fatalf("keyboard never reached the context row\n%s", term.Snapshot())
			}
			if c := frame("context-focused"); c.Bg == rest.Bg {
				t.Fatal("keyboard did not fill context token row")
			}
			alive(t, term, "after context token styling")
		})
	}
}
