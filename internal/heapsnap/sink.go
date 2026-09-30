package heapsnap

import (
	"errors"
	"fmt"
	"io"
)

var ErrTooLarge = errors.New("heap snapshot exceeds the size cap")

type Sink struct {
	w     io.Writer
	max   int64
	bytes int64
	err   error
}

func NewSink(w io.Writer, maxBytes int64) *Sink {
	return &Sink{w: w, max: maxBytes}
}

func (s *Sink) WriteChunk(chunk string) error {
	if s.err != nil {
		return s.err
	}
	if s.max > 0 && s.bytes+int64(len(chunk)) > s.max {
		s.err = fmt.Errorf("%w (%d bytes)", ErrTooLarge, s.max)
		return s.err
	}
	n, err := io.WriteString(s.w, chunk)
	s.bytes += int64(n)
	if err != nil {
		s.err = fmt.Errorf("write heap snapshot chunk: %w", err)
	}
	return s.err
}

func (s *Sink) Bytes() int64 { return s.bytes }
