package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveImportSourcePath(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path required")
	}

	cleaned := filepath.Clean(sourcePath)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("source path invalid: %w", err)
		}
		cleaned = abs
	}

	roots, err := allowedImportRoots()
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		if pathWithinRoot(cleaned, root) {
			return cleaned, nil
		}
	}
	return "", fmt.Errorf("source path must be within %s", strings.Join(roots, " or "))
}

func allowedImportRoots() ([]string, error) {
	roots := []string{filepath.Clean(os.TempDir())}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	roots = append(roots, filepath.Clean(homeDir))
	return roots, nil
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
}
