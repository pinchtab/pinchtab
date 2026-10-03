//go:build !windows

package actions

import (
	"fmt"
	"os"
	"syscall"
)

func refuseForeignOutputDir(dir string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w %s: owner unknown", errUnsafeOutputDir, dir)
	}
	if uid := os.Getuid(); int(stat.Uid) != uid {
		return fmt.Errorf("%w %s: owned by uid %d, not %d", errUnsafeOutputDir, dir, stat.Uid, uid)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%w %s: mode %#o allows group or other access", errUnsafeOutputDir, dir, perm)
	}
	return nil
}
