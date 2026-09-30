package actions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func TestUploadDefaultPath(t *testing.T) {
	m := newMockServer()
	defer m.close()

	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	captureStdout(t, func() {
		Upload(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{f}, "#file", "")
	})

	if m.lastPath != "/upload" {
		t.Fatalf("expected /upload, got %s", m.lastPath)
	}
}

func TestUploadTabScopedPath(t *testing.T) {
	m := newMockServer()
	defer m.close()

	f := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	captureStdout(t, func() {
		Upload(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{f}, "#file", "tab1")
	})

	if m.lastPath != "/tabs/tab1/upload" {
		t.Fatalf("expected /tabs/tab1/upload, got %s", m.lastPath)
	}
}

// The CLI is the only place that still holds the name the user typed — the
// server receives bytes. Sending filepath.Base is what lets the page see
// data.csv instead of upload-0.bin, so it is asserted on the wire.
func TestUploadSendsTheBasenameOfEachFile(t *testing.T) {
	m := newMockServer()
	defer m.close()

	dir := t.TempDir()
	first := filepath.Join(dir, "quarterly report.csv")
	second := filepath.Join(dir, "notes.md")
	for _, f := range []string{first, second} {
		if err := os.WriteFile(f, []byte("x"), 0600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	captureStdout(t, func() {
		Upload(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{first, second}, "#file", "")
	})

	var body struct {
		Files     []string `json:"files"`
		FileNames []string `json:"fileNames"`
	}
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, m.lastBody)
	}
	want := []string{"quarterly report.csv", "notes.md"}
	if !reflect.DeepEqual(body.FileNames, want) {
		t.Errorf("fileNames = %v, want %v", body.FileNames, want)
	}
	// Index alignment is the contract the server reads them by, so the two
	// arrays have to stay the same length.
	if len(body.Files) != len(body.FileNames) {
		t.Errorf("files=%d fileNames=%d; the server pairs them by index", len(body.Files), len(body.FileNames))
	}
}

// Only the basename travels: the directory the file came from is the caller's
// filesystem layout, which the page has no business seeing and which would make
// the name a path the server then has to defend against.
func TestUploadSendsNoDirectoryComponent(t *testing.T) {
	m := newMockServer()
	defer m.close()

	nested := filepath.Join(t.TempDir(), "sub", "dir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(nested, "doc.txt")
	if err := os.WriteFile(f, []byte("hello"), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	captureStdout(t, func() {
		Upload(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, []string{f}, "#file", "")
	})

	var body struct {
		FileNames []string `json:"fileNames"`
	}
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, m.lastBody)
	}
	if len(body.FileNames) != 1 || body.FileNames[0] != "doc.txt" {
		t.Fatalf("fileNames = %v, want [doc.txt]", body.FileNames)
	}
	if strings.ContainsRune(body.FileNames[0], filepath.Separator) {
		t.Errorf("fileNames[0] = %q carries a directory component", body.FileNames[0])
	}
}
