// installed by tuios
// managed by tuios; `tuios integration install pi` overwrites this file and
// `tuios integration uninstall pi` removes it. Put your own extensions beside
// it instead of editing it.
// TUIOS_INTEGRATION_ID=pi
// TUIOS_INTEGRATION_VERSION=__TUIOS_VERSION__
// @ts-nocheck
//
// Reports Pi's turns to the tuios pane it runs in, through
// `tuios agent-hook pi`: session_start names the session, agent_start starts
// work and agent_settled ends it. tool_call and tool_result say what a
// working turn is doing, tool by tool, for the rail's "now" line and the
// activity log. ui_prompt_start says Pi is waiting on the person in a
// blocking prompt (a confirm, a choice, a line of input), and ui_prompt_end
// that the prompt was answered; both say whether a turn is running, so an
// answered prompt goes back to working or to rest. Only the TUI mode
// reports, since the print, JSON and RPC modes run with no terminal a
// pane could show.

import { spawn } from "node:child_process";

const TUIOS = __TUIOS_COMMAND__;

// firstText is the first text block of a tool result's content, for the
// error line a failed call reports.
function firstText(content) {
  if (!Array.isArray(content)) return "";
  for (const block of content) {
    if (block?.type === "text" && typeof block.text === "string" && block.text) return block.text;
  }
  return "";
}

function report(event, ctx, extra) {
  let sessionID = "";
  let sessionFile = "";
  try {
    const id = ctx?.sessionManager?.getSessionId?.();
    if (typeof id === "string") sessionID = id;
  } catch {}
  try {
    const file = ctx?.sessionManager?.getSessionFile?.();
    if (typeof file === "string") sessionFile = file;
  } catch {}
  const payload = JSON.stringify({
    hook_event_name: event,
    session_id: sessionID,
    transcript_path: sessionFile,
    ...extra,
  });
  try {
    const child = spawn(TUIOS, ["agent-hook", "pi", "--integration", "__TUIOS_VERSION__"], {
      stdio: ["pipe", "ignore", "ignore"],
      windowsHide: true,
    });
    child.on("error", () => {});
    child.stdin.on("error", () => {});
    child.stdin.end(payload);
  } catch {
    // A report that cannot be sent must never break Pi.
  }
}

export default function (pi) {
  if (process.env.TUIOS_ENV !== "1" && !process.env.TUIOS_AGENT) {
    return;
  }
  let tui = false;
  pi.on("session_start", (_event, ctx) => {
    tui = ctx?.mode === "tui";
    if (!tui) return;
    // A reload replaces the extension mid-turn without a new agent_start.
    report("session_start", ctx, { busy: ctx?.isIdle?.() === false });
  });
  pi.on("agent_start", (_event, ctx) => {
    if (tui) report("agent_start", ctx, {});
  });
  pi.on("agent_settled", (_event, ctx) => {
    if (tui && ctx?.isIdle?.() !== false) report("agent_settled", ctx, {});
  });
  pi.on("tool_call", (event, ctx) => {
    if (!tui) return;
    // A nested call belongs to the outer tool already named: a codemode
    // script running bash reads "codemode" on the row, not a flicker of
    // everything it runs.
    if (event?.parentToolCallId) return;
    report("tool_call", ctx, {
      tool_name: typeof event?.toolName === "string" ? event.toolName : "",
      tool_input: event?.input && typeof event.input === "object" ? event.input : {},
    });
  });
  pi.on("tool_result", (event, ctx) => {
    if (!tui) return;
    if (event?.parentToolCallId) return;
    const failed = event?.isError === true;
    report("tool_result", ctx, {
      tool_name: typeof event?.toolName === "string" ? event.toolName : "",
      tool_input: event?.input && typeof event.input === "object" ? event.input : {},
      is_error: failed,
      error: failed ? firstText(event?.content) : "",
    });
  });
  pi.on("ui_prompt_start", (event, ctx) => {
    if (!tui) return;
    report("ui_prompt_start", ctx, {
      kind: typeof event?.kind === "string" ? event.kind : "",
      title: typeof event?.title === "string" ? event.title : "",
      busy: ctx?.isIdle?.() === false,
    });
  });
  pi.on("ui_prompt_end", (event, ctx) => {
    if (!tui) return;
    report("ui_prompt_end", ctx, {
      kind: typeof event?.kind === "string" ? event.kind : "",
      busy: ctx?.isIdle?.() === false,
    });
  });
}
