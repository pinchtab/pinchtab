package cloak

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WindowsFingerprintFonts lists font-family substrings expected on a
// Windows-fingerprint host; any one match passes the check.
var WindowsFingerprintFonts = []string{
	"arial",
	"times new roman",
	"calibri",
	"segoe ui",
}

// LinuxFontDirs are scanned when fc-list is missing.
var LinuxFontDirs = []string{
	"/usr/share/fonts",
	"/usr/local/share/fonts",
	"/var/lib/fonts",
}

type WindowsFontProbe struct {
	Matched  []string
	Source   string
	Expected []string
}

// ProbeWindowsFingerprintFonts checks for Windows fingerprint font
// availability via fc-list or filesystem scan.
func ProbeWindowsFingerprintFonts(ctx context.Context) WindowsFontProbe {
	if matched, err := probeFontsViaFcList(ctx); err == nil {
		return WindowsFontProbe{
			Matched:  matched,
			Source:   "fc-list",
			Expected: append([]string(nil), WindowsFingerprintFonts...),
		}
	}
	return WindowsFontProbe{
		Matched:  probeFontsViaFilesystem(LinuxFontDirs),
		Source:   "filesystem",
		Expected: append([]string(nil), WindowsFingerprintFonts...),
	}
}

func probeFontsViaFcList(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("fc-list"); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "fc-list", ":", "family").Output()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(string(out))
	var found []string
	for _, target := range WindowsFingerprintFonts {
		if strings.Contains(lower, target) {
			found = append(found, target)
		}
	}
	return found, nil
}

func probeFontsViaFilesystem(dirs []string) []string {
	var found []string
	seen := map[string]struct{}{}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, _ os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			base := strings.ToLower(filepath.Base(path))
			for _, target := range WindowsFingerprintFonts {
				// fc-list searches by family; filenames typically collapse
				// "Times New Roman" -> "times" and "Segoe UI" -> "segoeui",
				// so match the first whitespace-delimited token.
				token := strings.Fields(target)[0]
				if !strings.Contains(base, token) {
					continue
				}
				if _, ok := seen[target]; ok {
					continue
				}
				seen[target] = struct{}{}
				found = append(found, target)
			}
			return nil
		})
	}
	return found
}
