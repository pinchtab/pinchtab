//go:build windows

package actions

import "os"

// Windows has no uid or Unix mode bits to check, and its temp dir is per-user.
func refuseForeignOutputDir(string, os.FileInfo) error {
	return nil
}
