package e2e

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

type progressLine struct {
	w       io.Writer
	isTTY   bool
	width   int
	lastLen int
	mu      sync.Mutex
}

func newProgressLine(w io.Writer) *progressLine {
	p := &progressLine{w: w}
	if f, ok := w.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			p.isTTY = true
			if w, _, err := term.GetSize(fd); err == nil && w > 0 {
				p.width = w
			} else {
				p.width = 80
			}
		}
	}
	return p
}

func (p *progressLine) Update(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isTTY {
		return
	}

	line := msg
	if p.width > 0 && len(line) > p.width-1 {
		line = line[:p.width-4] + "..."
	}

	padLen := p.lastLen - len(line)
	pad := ""
	if padLen > 0 {
		pad = strings.Repeat(" ", padLen)
	}
	_, _ = fmt.Fprintf(p.w, "\r%s%s", line, pad)
	p.lastLen = len(line)
}

func (p *progressLine) Complete(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isTTY {
		padLen := p.lastLen - len(msg)
		pad := ""
		if padLen > 0 {
			pad = strings.Repeat(" ", padLen)
		}
		_, _ = fmt.Fprintf(p.w, "\r%s%s\n", msg, pad)
	} else {
		_, _ = fmt.Fprintf(p.w, "%s\n", msg)
	}
	p.lastLen = 0
}

func (p *progressLine) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isTTY || p.lastLen == 0 {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastLen))
	p.lastLen = 0
}
