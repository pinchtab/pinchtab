package bridge

import (
	"encoding/json"
	"testing"
)

func TestInstanceMarshalJSONDerivesModeFromHeadless(t *testing.T) {
	data, err := json.Marshal(Instance{Headless: true})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded["mode"]; got != "headless" {
		t.Fatalf("mode = %v, want headless", got)
	}
	if got := decoded["headless"]; got != true {
		t.Fatalf("headless = %v, want true", got)
	}
}

func TestInstanceMarshalJSONPreservesExplicitMode(t *testing.T) {
	data, err := json.Marshal(Instance{Mode: "headed", Headless: false})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got := decoded["mode"]; got != "headed" {
		t.Fatalf("mode = %v, want headed", got)
	}
	if got := decoded["headless"]; got != false {
		t.Fatalf("headless = %v, want false", got)
	}
}
