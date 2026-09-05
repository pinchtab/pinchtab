package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// bundleStampFile is written by scripts/build-dashboard.sh beside the bundle it
// built and embedded with it, so the binary carries the identity of the source
// its dashboard came from.
const bundleStampFile = "dashboard/bundle.stamp"

// BundleNotBuilt is what /health reports when no bundle is embedded; the UI
// route serves its explicit "not built" page in that case.
const BundleNotBuilt = "not-built"

// bundleInputsFile declares, once, the paths under dashboard/ that decide the
// bundle's contents; scripts/build-dashboard.sh hashes the same file's lines.
const bundleInputsFile = "bundle-inputs.txt"

// loadBundleInputs reads the declaration. A missing or empty list is an error
// rather than an empty hash: an empty list makes every stamp equal and turns
// the staleness guard green for every tree.
func loadBundleInputs(dashboardDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dashboardDir, bundleInputsFile))
	if err != nil {
		return nil, fmt.Errorf("bundle inputs: %w", err)
	}
	var inputs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		inputs = append(inputs, line)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("bundle inputs: %s declares no inputs", filepath.Join(dashboardDir, bundleInputsFile))
	}
	return inputs, nil
}

// BundleStamp is the source hash the embedded bundle was built from, or
// BundleNotBuilt when nothing is embedded.
func BundleStamp() string {
	data, err := dashboardFS.ReadFile(bundleStampFile)
	if err != nil {
		return BundleNotBuilt
	}
	return strings.TrimSpace(string(data))
}

// SourceStamp hashes the bundle inputs under dashboardDir: one line per file,
// sorted by slash path, "path NUL sha256(content) LF", then sha256 over the lines.
// Dotfiles are skipped so an editor's droppings do not differ between machines.
func SourceStamp(dashboardDir string) (string, error) {
	bundleInputs, err := loadBundleInputs(dashboardDir)
	if err != nil {
		return "", err
	}
	// A declared path that does not exist is skipped, not refused: the build
	// script skips it too, and the two stamps must agree or a typo in the
	// declaration reads as staleness with a remedy that cannot clear it. The
	// walk floor below names the skipped paths when nothing was left to hash.
	var files, missing []string
	for _, input := range bundleInputs {
		root := filepath.Join(dashboardDir, input)
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, input)
				continue
			}
			return "", err
		}
		if !info.IsDir() {
			files = append(files, input)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dashboardDir, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no bundle inputs under %s: every declared input is missing (%s)", dashboardDir, strings.Join(missing, ", "))
	}
	sort.Strings(files)
	lines := sha256.New()
	for _, rel := range files {
		f, err := os.Open(filepath.Join(dashboardDir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		content := sha256.New()
		_, err = io.Copy(content, f)
		_ = f.Close()
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(lines, "%s\x00%s\n", rel, hex.EncodeToString(content.Sum(nil)))
	}
	return hex.EncodeToString(lines.Sum(nil)), nil
}

// BundleStale reports whether an embedded stamp disagrees with the source under
// dashboardDir. An absent bundle is never stale: the UI already says so itself.
func BundleStale(stamp, dashboardDir string) (bool, error) {
	if stamp == "" || stamp == BundleNotBuilt {
		return false, nil
	}
	current, err := SourceStamp(dashboardDir)
	if err != nil {
		return false, err
	}
	return current != stamp, nil
}
