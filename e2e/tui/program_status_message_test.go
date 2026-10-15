package tuie2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Gaurav-Gosain/tuitest"
)

// Read only rail cells so pane output cannot satisfy the text assertions.
// Check the stored report separately so missing OSC input cannot pass a hide check.
// Exercise foreign-session updates as well as the attached client's live records.
func TestProgramStatusFillsEmptyRailMessage(t *testing.T) {
	for _, location := range []string{"current", "foreign"} {
		t.Run(location, func(t *testing.T) {
			const width = 60
			const pane = "summary task"
			base := t.TempDir()
			killDaemon(t, base)
			useShippedLooks(base)
			writeConfig(t, base, "[appearance.sidebar]\nenabled = true\nposition = 'left'\nwidth = 60\nsections = 'agents'\n"+
				"[appearance.sidebar.agent_row]\ntokens = ['name', 'state', 'message']\n")
			cli := func(args ...string) {
				t.Helper()
				if out, err := tuiosCLI(t, base, args...); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			cli("new", "view", "--detach")
			target := "view"
			if location == "foreign" {
				target = "worker"
				cli("new", target, "--detach")
			}
			cli("set-window", "-s", target, "--name", pane)
			term := attachIn(t, base, "view", startOpts{cols: 160, rows: 45, shippedLooks: true})
			frames := artifactDir(t)
			hookState, hookMessage := "working", ""
			setHook := func(state, message string, extra ...string) {
				t.Helper()
				hookState, hookMessage = state, message
				args := []string{"set-agent-state", "-s", target, "-w", pane, state, "-m", message}
				cli(append(args, extra...)...)
			}
			step := 0
			report := func(state, title, message string) {
				t.Helper()
				step++
				marker := fmt.Sprintf("REPORT-%d", step)
				body := "state=" + state
				if state != "clear" {
					body += ":app=pi:title=" + b64e(title) + ":msg=" + b64e(message)
				}
				cli("send-text", "-s", target, "-w", pane,
					`printf '\033]7501;`+body+`\033\\'; echo `+splitMarker(marker)+"\n")
				waitCapture(t, base, target, pane, marker)
				st := waitPSState(t, base, target, pane, marker, func(s psState) bool {
					if state == "clear" {
						return len(s.Program) == 0
					}
					return len(s.Program) == 1 && s.Program[0].State == state &&
						s.Program[0].Title == title && s.Program[0].Msg == message
				})
				if st.Source != "report" || st.State != hookState || st.Message != hookMessage {
					t.Fatalf("native text changed hook ownership: %+v", st)
				}
			}
			rail := func(label, want string, absent ...string) {
				t.Helper()
				err := term.WaitFor(func(s tuitest.Screen) bool {
					text := railText(s, width, false)
					if !strings.Contains(text, want) || strings.Count(text, pane) != 1 {
						return false
					}
					for _, word := range absent {
						if strings.Contains(text, word) {
							return false
						}
					}
					return true
				}, uiTimeout)
				saveArtifact(t, term, frames, label)
				savePNG(t, term.Screen(), hostPalette(t, ""), frames, label)
				if err != nil {
					t.Fatalf("%s: rail wants %q without %q or a repeated name: %v\n%s", label, want, absent, err, term.Snapshot())
				}
			}

			setHook("working", "")
			report("working", "", "Compacting context")
			rail("1-native-message", "Compacting context")
			report("working", "", "Refreshing context")
			rail("2-native-update", "Refreshing context", "Compacting context")
			report("working", "", pane)
			rail("3-name-not-repeated", "working", "Refreshing context")
			report("working", pane, "Compacting context")
			rail("4-title-not-repeated", "Compacting context")
			report("working", "Context", "Compacting")
			rail("5-title-and-message", "Context: Compacting")
			report("working", "Context", "")
			rail("6-title-only", "Context", "Compacting")
			setHook("needs_input", "Hook approval", "--kind", "approval")
			report("working", "", "Native activity")
			rail("7-hook-message-wins", "Hook approval", "Native activity", "Context")
			setHook("working", "")
			report("blocked", "", "Native question")
			rail("8-hook-state-wins", "Native question", "Hook approval")
			report("clear", "", "")
			rail("9-clear", "working", "Native question")
		})
	}
}
