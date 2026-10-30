# Driving a TML surface: the agent contract

An agent that opens a surface owns it. The person clicks, the clicks come
back as events, the agent answers with state patches. This topic is the
contract a driver is written against. `tml` covers the language; `events`
covers the socket underneath.

## Opening, and who may write

`tml-open` hands back the surface id in its reply, at the top level:

```json
{"type": "tml_opened", "surface": "tml-4f2a...", "window_id": "w1"}
```

A pane may change only the surfaces it opened. A second writer in another
pane is refused by the daemon, so one surface has one writer by
construction. The person's own commands may change any surface; that is by
design, not an invitation to drive someone else's surface from a script.

Adopt a surface that already exists only by a state field the opening
agent wrote for the purpose, such as `state.rig.base`. Never match on the
title: the daemon retitles a surface from its root `<tml title="...">`
attribute, so a title a driver remembers is gone after one restart.

## Events: the only return channel

Subscribe once and read JSON lines:

```sh
tuios subscribe --surface tml-4f2a... --types tml-event
```

`--surface` narrows the stream to one surface's `tml-event` events and
opts the subscription into the type. `tuios call subscribe` cannot work:
a stream holds the connection open, and `call` waits for a reply and
exits. `tml-open` with `events: true` streams the same events on its own
connection after the reply.

Where the surface id lives:

- In a verb reply (`tml_opened`, `tml_closed`, `tml_result`): at the top
  level, because the reply has one subject.
- In a stream event: at `obj.tml.surface`, because a stream event carries
  `type`, `session`, `window`, `seq` and `boot_id` at the top level and
  its payload in one typed field. The same envelope holds for every event
  type; it does not change per type.

Each event carries `obj.tml.version`, the surface version at the click.
Ignore an event whose version is behind the state you have written since:
the click landed on a document you already replaced.

An `emit('name', value)` action arrives as:

```json
{"seq": 44, "type": "tml-event", "tml": {"surface": "tml-4f2a...", "name": "next", "value": null, "version": 3}}
```

A close arrives the same way with `name` of `close` and a `reason`:
`closed` (the person's own close action, `value` is the answer),
`program` (a `tml-close` verb) or `dismissed` (esc, the window closed,
the session ended).

A table's `on-select` emits the selected row object in `value`; a row
that must carry its own index for the owner puts it in the row.

## Answering

`tml-replace` swaps the whole document, id and pane kept. `tml-patch`
applies a JSON Patch to the state and redraws only the boxes that read
the changed paths. Prefer patches; a replace for every change costs a
full redraw and re-escaping on both sides.

Raw content goes through the state, not through the document. A
`code`, `diff` or `markdown` node with `content="{state.hunk}"` renders
the state value directly: patch `state.hunk` and the box follows, no
XML-escaping in the owner, no document rebuild. The expression machinery
caps any bound string at 64 KiB per render, the same bound every bound
text has.

## Resuming

Keep the `seq` of the last event and its `boot_id`. On reconnect:

```sh
tuios subscribe --surface tml-4f2a... --types tml-event --after-seq 43 --boot-id 9f2c41d07a3e8b65
```

The daemon replays what it still holds, then streams live. When it cannot
replay exactly, the first line is a gap marker with a reason: `evicted`
(the ring moved past it), `boot_changed` (the daemon restarted), or
`overflow` (you read too slowly). After a gap, read current state back
(`tml-get`) instead of trusting that you saw every click.

## What a clickable looks like

A TUI has no hover. Every actionable control carries the `▸` glyph in its
label, and nothing else does. A surface that follows the convention needs
no explanation; one that does not teaches the person to guess.
