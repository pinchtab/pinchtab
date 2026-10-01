// Package fileout owns the no-overwrite rule for auto-named output files.
//
// Every auto-naming writer in this repo builds a name from a SECOND-resolution
// timestamp, so two outputs in the same second land on one path and the first is
// destroyed while the caller still reports that path and its byte count. Widening
// the timestamp only shrinks the window — two concurrent writers can share a
// millisecond — so the guarantee has to come from the create call itself.
//
// It lives here rather than in internal/handlers because the rule has two consumers
// that cannot import each other: the HTTP handlers and internal/cli/actions. A leaf
// depending only on the standard library is what both can reach. None of the existing
// shared packages owns filesystem output, and putting file creation in a sanitiser or
// an id generator would trade one small package for a responsibility violation.
//
// The boundary is generated default names only. A path the user typed is written as
// they typed it, overwrite included — that is their instruction, not a collision.
package fileout

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxUniqueNameAttempts bounds the suffix search. A directory holding this many files
// named for the same second is a runaway caller, not a collision to resolve.
const MaxUniqueNameAttempts = 512

// CreateUnique reserves a path under dir by CREATING it exclusively, bumping a numeric
// suffix until the create succeeds: base+ext, then base-1+ext, base-2+ext…
// It returns the open handle, so the name cannot be taken between the check and the
// write — which is the whole point, and why this is not a stat-then-write.
//
// The caller owns the base name, including its timestamp format, so this closes the
// collision without renaming anything a user might already be globbing for. The
// returned path is authoritative: it may carry a suffix the caller did not ask for,
// and reporting the name that was built instead trades a silent overwrite for a
// confidently printed filename that does not exist.
func CreateUnique(dir, base, ext string) (*os.File, string, error) {
	for attempt := range MaxUniqueNameAttempts {
		name := base + ext
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d%s", base, attempt, ext)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			return f, path, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not find a free name for %s%s in %s after %d attempts", base, ext, dir, MaxUniqueNameAttempts)
}

// WriteUnique creates a fresh file under dir and writes buf to it, returning the path
// actually used.
// A failed write removes the file it created. CreateUnique reserves the name by
// creating it, so returning an error while leaving it behind hands the caller a
// 0-byte file under a name that reads like a real output — the same defect as an
// abandoned reservation, one layer down.
func WriteUnique(dir, base, ext string, buf []byte) (string, error) {
	return writeUnique(dir, base, ext, buf, createUniqueFile)
}

// uniqueFile is what writeUnique needs of the handle its creator hands back: the two
// calls whose failure it must clean up after. An interface rather than *os.File
// because the two failures are not equally reachable through a real file — see
// writeUnique.
type uniqueFile interface {
	io.Writer
	io.Closer
}

// createUniqueFile adapts CreateUnique to that seam. It exists so CreateUnique keeps
// its concrete *os.File signature for every caller that wants the real handle, and
// returns an explicit nil on error rather than a typed nil wearing the interface.
func createUniqueFile(dir, base, ext string) (uniqueFile, string, error) {
	f, path, err := CreateUnique(dir, base, ext)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

// writeUnique takes its creator as a parameter so a test can hand it a handle that
// fails, and assert the file it created is gone. A real write or close failure needs a
// full disk or a revoked handle, neither of which a test can arrange portably, and an
// unpinned removal is an unproven one.
//
// The creator returns an INTERFACE because the two failures are reachable by different
// means. A closed *os.File fails at the write — os.File.Write validates the descriptor
// before it looks at the buffer, zero-length buffer included — so it can never reach
// the close branch below. That branch's production case is the opposite shape: a write
// that succeeds and a close that fails, which is delayed allocation on a full
// filesystem, or NFS flushing at close. Only a stub can be both, so the seam admits
// one.
func writeUnique(dir, base, ext string, buf []byte, create func(string, string, string) (uniqueFile, string, error)) (string, error) {
	f, path, err := create(dir, base, ext)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// WriteUniquePath is WriteUnique for a caller holding a whole path rather than its
// three parts, which is the shape every CLI site has.
func WriteUniquePath(path string, buf []byte) (string, error) {
	dir, base, ext := splitPath(path)
	return WriteUnique(dir, base, ext, buf)
}

// ReserveUnique claims a path for a caller that will not write the bytes itself — one
// moving an already-written file into place with os.Rename, or handing the name to an
// encoder that writes over the placeholder. The placeholder is created exclusively and
// closed, and a rename then replaces it atomically on POSIX, so the reservation is what
// stops a second process taking the name in between.
//
// A failed close removes the placeholder, for the reason WriteUnique removes a failed
// write: the alternative is an error alongside a 0-byte file wearing the output's name.
//
// A reservation is a side effect: every path that then gives up must Remove the
// returned path, or an abandoned run litters an empty file under a name that reads
// like a real output.
func ReserveUnique(dir, base, ext string) (string, error) {
	return reserveUnique(dir, base, ext, CreateUnique)
}

// reserveUnique takes its creator as a parameter for the reason writeUnique does: a
// close failure on a freshly created file needs a full or revoked filesystem, and an
// unpinned removal is an unproven one.
func reserveUnique(dir, base, ext string, create func(string, string, string) (*os.File, string, error)) (string, error) {
	f, reserved, err := create(dir, base, ext)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(reserved)
		return "", err
	}
	return reserved, nil
}

// ReservePath is ReserveUnique for a caller holding a whole path rather than its three
// parts. splitPath takes the extension with filepath.Ext, so a base carrying dots of
// its own does not survive the round trip — a caller that knows its three parts should
// pass them.
func ReservePath(path string) (string, error) {
	dir, base, ext := splitPath(path)
	return ReserveUnique(dir, base, ext)
}

// splitPath cuts a path into the three parts CreateUnique numbers, so the suffix lands
// before the extension (capture-<ts>-1.jpg) rather than after it.
func splitPath(path string) (dir, base, ext string) {
	dir = filepath.Dir(path)
	name := filepath.Base(path)
	ext = filepath.Ext(name)
	return dir, strings.TrimSuffix(name, ext), ext
}
