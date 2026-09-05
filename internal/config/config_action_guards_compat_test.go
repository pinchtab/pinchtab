package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileConfigAcceptsAndIgnoresRetiredEnableActionGuards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"security":{"enableActionGuards":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)

	result := readAndParseConfigFile()
	if result.ParseErr != nil || result.UnknownFields != nil {
		t.Fatalf("legacy config did not load cleanly: parse=%v unknown=%v", result.ParseErr, result.UnknownFields)
	}
	if result.FC == nil {
		t.Fatal("legacy config produced no file config")
	}
}
