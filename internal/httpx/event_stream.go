package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type EventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func NewEventStream(w http.ResponseWriter, flusher http.Flusher) *EventStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &EventStream{w: w, flusher: flusher}
}

func (s *EventStream) Event(event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Raw(event, data)
}

func (s *EventStream) Raw(event string, data []byte) error {
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

func (s *EventStream) Keepalive() error {
	if _, err := fmt.Fprintf(s.w, ": keepalive\n\n"); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
