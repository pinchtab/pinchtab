package observe

import (
	"encoding/json"
	"fmt"
	"io"
)

func init() {
	RegisterFormat("har", func(creatorName, creatorVersion string) ExportEncoder {
		return &harEncoder{
			creator: harCreator{Name: creatorName, Version: creatorVersion},
		}
	})
}

// harEncoder writes HAR 1.2 JSON incrementally.
type harEncoder struct {
	creator    harCreator
	w          io.Writer
	entryCount int
	started    bool
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (e *harEncoder) ContentType() string   { return "application/har+json" }
func (e *harEncoder) FileExtension() string { return ".har" }

func (e *harEncoder) Start(w io.Writer) error {
	e.w = w
	e.started = true
	creatorJSON, err := json.Marshal(e.creator)
	if err != nil {
		return fmt.Errorf("marshal creator: %w", err)
	}
	_, err = io.WriteString(e.w, `{"log":{"version":"1.2","creator":`)
	if err != nil {
		return err
	}
	if _, err = e.w.Write(creatorJSON); err != nil {
		return err
	}
	_, err = io.WriteString(e.w, `,"entries":[`)
	return err
}

func (e *harEncoder) Encode(entry ExportEntry) error {
	if !e.started {
		return fmt.Errorf("encoder not started")
	}
	if e.entryCount > 0 {
		if _, err := io.WriteString(e.w, ","); err != nil {
			return err
		}
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := e.w.Write(data); err != nil {
		return err
	}
	e.entryCount++
	return nil
}

func (e *harEncoder) Finish() error {
	if !e.started {
		return fmt.Errorf("encoder not started")
	}
	_, err := io.WriteString(e.w, "]}}\n")
	return err
}
