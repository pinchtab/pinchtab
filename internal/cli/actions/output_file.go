package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/fileout"
)

// autoNamedOutputPath builds the absolute path for an output the user did not name. It
// lives under the OS temp dir rather than the cwd, which is usually a work tree.
func autoNamedOutputPath(prefix, ext string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(os.TempDir(), "pinchtab"))
	if err != nil {
		return "", fmt.Errorf("resolve auto-named output dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create auto-named output dir: %w", err)
	}
	if err := refuseSharedOutputDir(dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, prefix+"-"+time.Now().Format("20060102-150405")+ext), nil
}

var errUnsafeOutputDir = errors.New("unsafe auto-named output dir")

// refuseSharedOutputDir guards the auto-named dir, which on Linux is /tmp/pinchtab and
// shared by every user: MkdirAll accepts a dir another user created or pointed elsewhere.
func refuseSharedOutputDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect auto-named output dir: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w %s: it is a symlink", errUnsafeOutputDir, dir)
	}
	return refuseForeignOutputDir(dir, info)
}

// writeOutputFile writes buf to path and returns the path actually used.
//
// An auto-named path is reserved with an exclusive create, so a second run in the same
// second gets capture-<ts>-1.jpg rather than destroying the first file. The returned
// path is what the caller must print: it may carry a suffix the caller did not build,
// and printing the built name instead trades a silent overwrite for a confidently
// reported filename that does not exist.
//
// An explicit path is written as the user typed it, overwrite included — that is their
// instruction, not a collision.
func writeOutputFile(path string, autoNamed bool, buf []byte) (string, error) {
	if autoNamed {
		return fileout.WriteUniquePath(path, buf)
	}
	return path, os.WriteFile(path, buf, 0600)
}

// capture's lowercase "saved" and record's "Saved → %s" are deliberately NOT routed
// here. They are different sentences for different shapes — capture prints a save line
// alongside the snapshot that is half of what it returned, and record has no byte count
// because it moves an already-encoded file — so folding them in would change their
// output rather than remove a duplicate.
func printSaved(path string, size int) {
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Saved %s (%d bytes)", path, size)))
}
