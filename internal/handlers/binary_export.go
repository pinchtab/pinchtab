package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
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

// saveBinaryToStateDir writes buf to StateDir/<subdir>/<prefix>-<ts><ext> using
// the standard binary-export modes (dir 0750, file 0600) and returns the path
// and timestamp. This is the single persistence policy shared by the PDF,
// screenshot, and capture handlers' default-location output.
func saveBinaryToStateDir(stateDir, subdir, prefix, ext string, buf []byte) (filePath, timestamp string, err error) {
	dir := filepath.Join(stateDir, subdir)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", "", err
	}
	timestamp = time.Now().Format("20060102-150405")
	filePath = filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext))
	if err := os.WriteFile(filePath, buf, 0600); err != nil {
		return "", "", err
	}
	return filePath, timestamp, nil
}
