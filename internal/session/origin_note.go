package session

import "fmt"

// originNoteText is the line a spawned pane shows when another pane started
// it: the pane it came from, as a tuios://window link, so a watch or worker
// pane can walk back to the agent that made it. It resets the attributes the
// program may have set, starts on a fresh line, and is dim so it does not
// read as the program's own output.
func originNoteText(parentWindowID, label string) string {
	link := "\x1b]8;;tuios://window/" + parentWindowID + "\x1b\\" + label + "\x1b]8;;\x1b\\"
	return fmt.Sprintf("\x1b[0m\r\n\x1b[2m[spawned from %s]\x1b[0m\r\n", link)
}

// queueOriginNote puts the spawned-from line in the pane's stream. It takes
// the path program output takes, so the daemon's emulator, the catch-up ring
// and every subscriber get the same bytes at the same position. A pane whose
// stream is gone, or whose program has already ended, says nothing.
func (p *PTY) queueOriginNote(parentWindowID, label string) {
	if p == nil || p.ctx.Err() != nil {
		return
	}
	data := []byte(originNoteText(parentWindowID, label))
	p.streamMu.Lock()
	if p.vtClosed {
		p.streamMu.Unlock()
		return
	}
	p.outputMu.Lock()
	seq := p.appendToBuffer(data)
	p.outputMu.Unlock()
	p.broadcast(ptyChunk{data: data}, seq)
	select {
	case p.vtWriteChan <- vtChunk{data: data, seq: seq}:
	case <-p.ctx.Done():
	}
	p.streamMu.Unlock()
}
