package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pinchtab/pinchtab/internal/fileout"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// imageContentType maps an image format to its MIME type (jpeg default).
func imageContentType(format string) string {
	if format == "png" {
		return "image/png"
	}
	return "image/jpeg"
}

// imageExt maps an image format to its file extension (.jpg default).
func imageExt(format string) string {
	if format == "png" {
		return ".png"
	}
	return ".jpg"
}

// writeRawImage writes raw binary output with the given content type, logging
// (but not surfacing) a write error under logLabel.
func writeRawImage(w http.ResponseWriter, buf []byte, contentType, logLabel string) {
	w.Header().Set("Content-Type", contentType)
	if _, err := w.Write(buf); err != nil {
		slog.Error(logLabel, "err", err)
	}
}

// The no-overwrite rule itself lives in internal/fileout, a leaf both this package and
// internal/cli/actions can import — the CLI sites had the same defect and a second copy
// of the loop is the drift worth avoiding. This keeps the name every call site in this
// package already uses, and delegates. A caller that reserves a name without writing
// the bytes calls fileout.ReserveUnique directly: routing that through a local handle
// is what let this package lose the removal on a failed close.
func writeUniqueFile(dir, base, ext string, buf []byte) (string, error) {
	return fileout.WriteUnique(dir, base, ext, buf)
}

// exportTimestamp is the second-resolution stamp every auto-named export shares. It is
// no longer load-bearing for uniqueness — the exclusive create is — so it stays at second
// granularity to keep filenames readable.
func exportTimestamp() string {
	return time.Now().Format("20060102-150405")
}

// saveBinaryToStateDir writes buf to StateDir/<subdir>/<prefix>-<ts><ext> using
// the standard binary-export modes (dir 0750, file 0600) and returns the path
// and timestamp. This is the single persistence policy shared by the PDF,
// screenshot, and capture handlers' default-location output.
//
// The returned path is authoritative: on a same-second collision it carries a suffix,
// so callers must report this value rather than rebuilding the name from the timestamp.
func saveBinaryToStateDir(stateDir, subdir, prefix, ext string, buf []byte) (filePath, timestamp string, err error) {
	dir := filepath.Join(stateDir, subdir)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", "", err
	}
	timestamp = exportTimestamp()
	filePath, err = writeUniqueFile(dir, prefix+"-"+timestamp, ext, buf)
	if err != nil {
		return "", "", err
	}
	return filePath, timestamp, nil
}

// callerFilePath resolves a caller-supplied file name under base, refusing
// anything that resolves to base itself since a file cannot be written there.
func callerFilePath(base, userPath string) (string, error) {
	absPath, err := httpx.SafeCreatePath(base, userPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if absBase, _ := filepath.Abs(base); absPath == absBase {
		return "", fmt.Errorf("invalid path: %q names the directory, not a file", userPath)
	}
	return absPath, nil
}

// saveToCallerPath writes buf to the caller-named path under stateDir with the
// binary-export modes. Unlike the auto-named exports it overwrites: a caller
// who names a file is entitled to replace it.
func saveToCallerPath(stateDir, userPath string, buf []byte) (absPath string, status int, err error) {
	absPath, err = callerFilePath(stateDir, userPath)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0750); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(absPath, buf, 0600); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("write file: %w", err)
	}
	return absPath, http.StatusOK, nil
}
