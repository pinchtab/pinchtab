//go:build windows

package actions

import "os"

func refuseForeignOutputDir(string, os.FileInfo) error {
	return nil
}
