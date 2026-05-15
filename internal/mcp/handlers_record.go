package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// maxRecordFileBytes caps the output file written by record_stop to prevent
// unbounded disk writes (matches the server-side maxOutputBytes).
const maxRecordFileBytes = 256 << 20 // 256 MiB

func handleRecordStart(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		file, err := r.RequireString("file")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		ext := filepath.Ext(file)
		var format string
		switch ext {
		case ".gif":
			format = "gif"
		case ".webm":
			format = "webm"
		case ".mp4":
			format = "mp4"
		default:
			return mcp.NewToolResultError(fmt.Sprintf("unsupported format %q — use .gif, .webm, or .mp4", ext)), nil
		}

		if _, err := safeRecordPath(file); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid output path: %v", err)), nil
		}

		payload := map[string]any{"format": format}
		if fps, ok := optInt(r, "fps"); ok {
			payload["fps"] = fps
		}
		if quality, ok := optInt(r, "quality"); ok {
			payload["quality"] = quality
		}
		if scale, ok := optFloat(r, "scale"); ok {
			payload["scale"] = scale
		}
		if tabID := optString(r, "tabId"); tabID != "" {
			payload["tabId"] = tabID
		}

		body, code, err := c.Post(ctx, "/record/start", payload)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}

func handleRecordStop(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		file := optString(r, "file")
		if file == "" {
			return mcp.NewToolResultError("file parameter is required"), nil
		}

		dest, pathErr := safeRecordPath(file)
		if pathErr != nil {
			_, _, stopErr := c.Post(ctx, "/record/stop", map[string]any{"discard": true})
			msg := fmt.Sprintf("invalid output path: %v — recording discarded", pathErr)
			if stopErr != nil {
				msg += fmt.Sprintf(" (also failed to discard recording: %v)", stopErr)
			}
			return mcp.NewToolResultError(msg), nil
		}

		// Server encodes to a controlled recordings directory; we move the
		// file to the caller's desired destination after encoding completes.
		body, code, err := c.Post(ctx, "/record/stop", map[string]any{})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if code >= 400 {
			return resultFromBytes(body, code)
		}

		var stopResp struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(body, &stopResp) != nil || stopResp.Path == "" {
			return resultFromBytes(body, code)
		}

		// Poll until encoding finishes, then move the file.
		serverPath := stopResp.Path
		if err := pollRecordingFinished(ctx, c); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf(
				"encoding did not complete: %v — partial file may be at %s", err, serverPath)), nil
		}

		if err := moveFile(serverPath, dest); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf(
				"encoded to %s but failed to move to %s: %v", serverPath, dest, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Recording saved to %s", dest)), nil
	}
}

// pollRecordingFinished polls /record/status until state is "finished" or "idle".
func pollRecordingFinished(ctx context.Context, c *Client) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			body, _, err := c.Get(ctx, "/record/status", nil)
			if err != nil {
				continue
			}
			var status struct {
				State string `json:"state"`
				Error string `json:"error"`
			}
			if json.Unmarshal(body, &status) != nil {
				continue
			}
			switch status.State {
			case "finished":
				if status.Error != "" {
					return fmt.Errorf("encode failed: %s", status.Error)
				}
				return nil
			case "idle":
				return nil
			}
		}
	}
}

// moveFile moves src to dst using rename (fast, same filesystem) or
// falls back to copy+remove for cross-filesystem moves. The destination
// is created with O_EXCL to prevent overwrites or symlink following.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-filesystem: copy with exclusive creation.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	_ = os.Remove(src)
	return nil
}

// safeRecordPath validates a recording output path to prevent arbitrary file
// overwrites. It requires an absolute path with a supported extension, rejects
// symlinks and path traversal, and refuses to overwrite existing files.
func safeRecordPath(file string) (string, error) {
	cleaned := filepath.Clean(file)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("file must be an absolute path, got %q", file)
	}

	ext := filepath.Ext(cleaned)
	switch ext {
	case ".gif", ".webm", ".mp4":
	default:
		return "", fmt.Errorf("unsupported extension %q — use .gif, .webm, or .mp4", ext)
	}

	dir := filepath.Dir(cleaned)
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("output directory: %w", err)
	}
	if dirInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("output directory %q is a symlink", dir)
	}
	if !dirInfo.IsDir() {
		return "", fmt.Errorf("output directory %q is not a directory", dir)
	}

	if info, err := os.Lstat(cleaned); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing to follow symlink at %q", cleaned)
		}
		return "", fmt.Errorf("file already exists at %q — remove it first or choose another path", cleaned)
	}

	return cleaned, nil
}

// streamToFile writes from r to path using O_CREATE|O_EXCL (no overwrite),
// capped at maxRecordFileBytes. Returns the number of bytes written.
func streamToFile(path string, r io.Reader) (int64, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return 0, err
	}

	n, copyErr := io.Copy(f, io.LimitReader(r, maxRecordFileBytes+1))
	if closeErr := f.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(path)
		return 0, copyErr
	}
	if n > maxRecordFileBytes {
		_ = os.Remove(path)
		return 0, fmt.Errorf("recording exceeds %d MiB limit", maxRecordFileBytes>>20)
	}
	return n, nil
}

func handleRecordStatus(c *Client) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		body, code, err := c.Get(ctx, "/record/status", nil)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return resultFromBytes(body, code)
	}
}
