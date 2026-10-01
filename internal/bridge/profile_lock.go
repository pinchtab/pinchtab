// profile_lock.go handles stale browser profile lock recovery for
// Chromium-based providers.
//
// When a container restarts (or the browser crashes), Chromium's
// SingletonLock, SingletonSocket, and SingletonCookie files may be left behind
// in the profile directory. On next startup the browser sees these and refuses
// to launch with
// "The profile appears to be in use by another Chromium process".
//
// This code detects that error, checks whether the owning process is actually
// still running (via PID probe and process listing), and removes the stale
// lock files if it's safe to do so. It retries browser startup once after
// clearing the locks.

package bridge

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var chromeProfileProcessLister = findChromeProfileProcesses
var chromePIDIsRunning = isChromePIDRunning
var killChromeProfileProcesses = killProcesses
var isProfileOwnedByRunningPinchtabMock = isProfileOwnedByRunningPinchtab
var isPinchTabProcessFunc = isPinchTabProcess

// lockStartupGrace is the window after a PID file is written during which the
// browser process check is skipped. AcquireProfileLock writes the PID before
// InitBrowser launches the browser, so a second process must not treat the
// missing browser as a stale lock during this startup window.
var lockStartupGrace = 2 * time.Minute

var chromeSingletonFiles = []string{
	"SingletonLock",
	"SingletonSocket",
	"SingletonCookie",
}

var chromeProfileLockPIDPattern = regexp.MustCompile(`(?:Chromium|Chrome) process \((\d+)\)`)

type chromeProfileProcess struct {
	PID     string
	Command string
}

func isProfileLockError(msg string) bool {
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "The profile appears to be in use by another Chromium process") ||
		strings.Contains(msg, "The profile appears to be in use by another Chrome process") ||
		strings.Contains(msg, "process_singleton")
}

func clearStaleProfileLocks(profileDir, errMsg string) (bool, error) {
	if strings.TrimSpace(profileDir) == "" {
		return false, nil
	}

	if pid, ok := extractChromeProfileLockPID(errMsg); ok {
		running, err := chromePIDIsRunning(pid)
		if err != nil {
			slog.Warn("failed to probe browser lock pid; falling back to process listing", "profile", profileDir, "pid", pid, "err", err)
		} else if running {
			if owned, ptPid := isProfileOwnedByRunningPinchtabMock(profileDir); owned {
				slog.Warn("browser profile lock appears active and owned by another pinchtab; leaving singleton files in place", "profile", profileDir, "pid", pid, "pinchtab_pid", ptPid)
				return false, nil
			}
			slog.Warn("browser profile lock appears active but pinchtab owner is dead; proceeding with stale cleanup", "profile", profileDir, "pid", pid)
		}
	}

	processes, err := chromeProfileProcessLister(profileDir)
	if err != nil {
		if _, ok := extractChromeProfileLockPID(errMsg); ok {
			slog.Warn("profile process listing unavailable; proceeding with stale lock cleanup based on lock pid", "profile", profileDir, "err", err)
		} else {
			return false, err
		}
	}
	if len(processes) > 0 {
		if owned, ptPid := isProfileOwnedByRunningPinchtabMock(profileDir); owned {
			pids := make([]string, 0, len(processes))
			for _, proc := range processes {
				pids = append(pids, proc.PID)
			}
			slog.Warn("browser profile lock appears active and owned by another pinchtab; leaving singleton files in place", "profile", profileDir, "pids", strings.Join(pids, ","), "pinchtab_pid", ptPid)
			return false, nil
		}

		slog.Warn("browser profile lock appears active but no pinchtab owner found; killing stale processes", "profile", profileDir)
		if err := killChromeProfileProcesses(processes); err != nil {
			slog.Error("failed to kill stale browser processes", "profile", profileDir, "err", err)
			return false, nil
		}
	}

	removed := false
	for _, name := range chromeSingletonFiles {
		path := filepath.Join(profileDir, name)
		if _, err := os.Lstat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, fmt.Errorf("inspect %s: %w", path, err)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("remove %s: %w", path, err)
		}
		removed = true
	}

	return removed, nil
}

// quarantineExitWait bounds how long quarantine waits for the dying browser
// to release the profile before renaming. Var so tests can shrink it.
var quarantineExitWait = 5 * time.Second

const quarantineSuffix = ".quarantine-"

var quarantineDirName = regexp.MustCompile(`^(.+)` + regexp.QuoteMeta(quarantineSuffix) + `(\d+)$`)

// SplitQuarantineName reads a quarantine directory name back into the profile it was
// made from and the timestamp it carries. It is the one owner of the pattern: the
// predicate below and the prune both go through it, so a reader cannot drift from what
// quarantine writes.
func SplitQuarantineName(dirName string) (profile string, stamp int64, ok bool) {
	match := quarantineDirName.FindStringSubmatch(dirName)
	if match == nil {
		return "", 0, false
	}
	stamp, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return match[1], stamp, true
}

func IsQuarantinedProfileDir(dirName string) bool {
	_, _, ok := SplitQuarantineName(dirName)
	return ok
}

type QuarantineRemoval struct {
	Path  string
	Bytes int64
}

const KeepAllQuarantinedProfiles = 0

// PruneQuarantinedProfiles keeps the newest `keep` quarantined siblings of one profile
// and removes the rest, returning what it reclaimed. It carries the AUTOMATIC retention
// policy — a keep count of zero or less means keep everything — which is why the
// on-demand reclaim below does not go through it: there is no keep value that means
// "remove them all", so remove-all is expressed by asking removeQuarantinedProfiles for
// an unranked selection rather than by a sentinel keep count. Both end at the same
// deleter.
//
// It is not the only code that can remove a quarantine directory: DELETE /profiles/{id}
// resolves an id through the profile listing — where a quarantine appears under its own
// id — and calls ProfileManager.Delete, which removes the directory by name with neither
// guard. That route is authenticated and audit-logged and is arguably the reclaim a user
// wants, so the distinction to preserve is which deleter is scoped, not which one exists.
// TestOnlyOneFunctionRemovesADirectoryUnderTheProfilesBase is the census of that set.
//
// A user who names a profile exactly "<other profile>.quarantine-<digits>" is
// indistinguishable on disk from a real quarantine; nothing here can tell them apart,
// and what bounds the exposure is the sibling scope. See
// TestPruneOnlyReachesALookalikeThroughItsNamesakeAndStillKeepsTheNewest.
//
// justCreated is excluded by PATH, not by being newest: quarantine may proceed while a
// dying browser still holds the directory, so the one entry that can still be written to
// must survive a same-second timestamp tie.
func PruneQuarantinedProfiles(profileDir, justCreated string, keep int) ([]QuarantineRemoval, error) {
	if keep <= KeepAllQuarantinedProfiles {
		return nil, nil
	}
	profileDir = strings.TrimSpace(profileDir)
	if profileDir == "" {
		return nil, nil
	}

	siblings, err := quarantinedSiblings(profileDir)
	if err != nil {
		return nil, err
	}
	return removeQuarantinedProfiles(siblingsBeyondKeep(siblings, justCreated, keep)), nil
}

// siblingsBeyondKeep ranks one profile's quarantined siblings newest first and names the
// ones the keep count does not cover. Ranking is separated from the deletion so the
// on-demand reclaim can reach the same deleter with no ranking at all.
func siblingsBeyondKeep(siblings []quarantinedSibling, justCreated string, keep int) []string {
	sort.Slice(siblings, func(i, j int) bool { return siblings[i].stamp > siblings[j].stamp })

	// justCreated reserves a slot rather than being ranked, which is what makes the
	// outcome independent of the timestamp order.
	budget := keep
	for _, sibling := range siblings {
		if sibling.path == justCreated {
			budget--
			break
		}
	}

	var doomed []string
	for _, sibling := range siblings {
		if sibling.path == justCreated {
			continue
		}
		if budget > 0 {
			budget--
			continue
		}
		doomed = append(doomed, sibling.path)
	}
	return doomed
}

// removeQuarantinedProfiles is the one function that deletes a quarantined profile
// directory. It re-applies IsQuarantinedProfileDir to every path it is handed rather
// than trusting its caller to have filtered: the callers differ in how they select
// (ranked siblings of one profile, or every quarantine under the base), and a live
// profile must be unreachable however the selection is shaped, so the guard belongs
// where the deletion is.
func removeQuarantinedProfiles(paths []string) []QuarantineRemoval {
	var removals []QuarantineRemoval
	for _, path := range paths {
		if !IsQuarantinedProfileDir(filepath.Base(path)) {
			slog.Warn("refused to remove a profile that is not quarantined", "profile", path)
			continue
		}
		reclaimed := dirBytes(path)
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("could not remove quarantined profile", "profile", path, "err", err)
			continue
		}
		slog.Info("removed quarantined profile", "profile", path, "bytes", reclaimed)
		removals = append(removals, QuarantineRemoval{Path: path, Bytes: reclaimed})
	}
	return removals
}

// ReclaimableQuarantinedProfiles reports what a reclaim would remove and what it would
// free, removing nothing. It is the dry run the bare command depends on.
func ReclaimableQuarantinedProfiles(baseDir, only string) ([]QuarantineRemoval, error) {
	paths, err := quarantinedProfileSelection(baseDir, only)
	if err != nil {
		return nil, err
	}
	reclaimable := make([]QuarantineRemoval, 0, len(paths))
	for _, path := range paths {
		reclaimable = append(reclaimable, QuarantineRemoval{Path: path, Bytes: dirBytes(path)})
	}
	return reclaimable, nil
}

// ReclaimQuarantinedProfiles removes the quarantined directories a reclaim selects,
// on demand and with no retention policy: this is the half PruneQuarantinedProfiles
// cannot express, and it stays reachable when quarantineKeep is set to keep everything.
func ReclaimQuarantinedProfiles(baseDir, only string) ([]QuarantineRemoval, error) {
	paths, err := quarantinedProfileSelection(baseDir, only)
	if err != nil {
		return nil, err
	}
	return removeQuarantinedProfiles(paths), nil
}

// quarantinedProfileSelection names the quarantined directories directly under baseDir,
// optionally narrowed to one of them.
//
// `only` is matched against the names of directories already enumerated and already
// found quarantined; it is never joined onto baseDir. That is what makes a traversal
// impossible rather than merely refused — caller text cannot become a path component
// here, so no amount of "../" reaches outside the profile root. The explicit refusal
// below exists so a caller that sends a path gets told that, instead of a bare not-found.
func quarantinedProfileSelection(baseDir, only string) ([]string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("no profiles directory configured")
	}
	only = strings.TrimSpace(only)
	if only != "" && only != filepath.Base(only) {
		return nil, fmt.Errorf("%q is a path, not a quarantined profile name", only)
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profiles dir: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() || !IsQuarantinedProfileDir(entry.Name()) {
			continue
		}
		if only != "" && entry.Name() != only {
			continue
		}
		paths = append(paths, filepath.Join(baseDir, entry.Name()))
	}
	if only != "" && len(paths) == 0 {
		return nil, fmt.Errorf("no quarantined profile named %q", only)
	}
	return paths, nil
}

type quarantinedSibling struct {
	path  string
	stamp int64
}

// quarantinedSiblings finds the quarantined directories belonging to one profile. Both
// halves come from SplitQuarantineName — whether a name is a quarantine at all, and whose
// it is — so neither rule has a second implementation that could drift.
func quarantinedSiblings(profileDir string) ([]quarantinedSibling, error) {
	parent := filepath.Dir(profileDir)
	profileName := filepath.Base(profileDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read profiles dir: %w", err)
	}

	var siblings []quarantinedSibling
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile, stamp, ok := SplitQuarantineName(entry.Name())
		if !ok {
			continue
		}
		if profile != profileName {
			continue
		}
		siblings = append(siblings, quarantinedSibling{path: filepath.Join(parent, entry.Name()), stamp: stamp})
	}
	return siblings, nil
}

func dirBytes(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// quarantineCorruptedProfile renames profileDir to "<profileDir>.quarantine-<ts>"
// and recreates an empty directory at the original path. Used to recover
// from silent CDP attach failures where CloakBrowser refuses to ingest
// existing profile state.
//
// keep bounds how many quarantined siblings of this profile survive, newest first,
// pruned here rather than on a startup sweep: a sweep would delete directories the
// operator never asked about, while this only reclaims as the same profile keeps
// failing. Directories belonging to profiles that never quarantine again are
// therefore never reclaimed by this path.
func quarantineCorruptedProfile(profileDir string, keep int) (string, error) {
	profileDir = strings.TrimSpace(profileDir)
	if profileDir == "" {
		return "", fmt.Errorf("empty profile dir")
	}
	if _, err := os.Stat(profileDir); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat profile dir: %w", err)
	}
	// The caller cancels chromedp contexts without waiting for the process to
	// exit; renaming under a still-running browser fails on Windows and lets
	// the old process keep writing into the quarantined dir on POSIX. On
	// timeout, proceed anyway — refusing entirely would block recovery.
	if !waitForChromeExit(profileDir, quarantineExitWait) {
		slog.Warn("quarantining profile while a browser process may still hold it", "profile", profileDir)
	}
	quarantinePath := fmt.Sprintf("%s%s%d", profileDir, quarantineSuffix, time.Now().Unix())
	if err := os.Rename(profileDir, quarantinePath); err != nil {
		return "", fmt.Errorf("rename profile dir: %w", err)
	}
	// The rename carries profile.json along, where it now names a profile this
	// directory no longer is. Drop it so nothing on disk makes that claim.
	if err := os.Remove(filepath.Join(quarantinePath, "profile.json")); err != nil && !os.IsNotExist(err) {
		slog.Warn("stale profile metadata left in quarantined profile", "profile", quarantinePath, "err", err)
	}
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return quarantinePath, fmt.Errorf("recreate profile dir: %w", err)
	}
	if err := EnsureChromiumSkeleton(profileDir); err != nil {
		return quarantinePath, fmt.Errorf("seed recreated profile dir: %w", err)
	}
	// After the rename, so a failed quarantine never costs an older artefact.
	removals, err := PruneQuarantinedProfiles(profileDir, quarantinePath, keep)
	if err != nil {
		slog.Warn("could not prune older quarantined profiles", "profile", profileDir, "err", err)
	}
	for _, removal := range removals {
		slog.Info("pruned older quarantined profile", "profile", removal.Path, "bytesReclaimed", removal.Bytes, "keep", keep)
	}
	return quarantinePath, nil
}

// ProfileOwnedByRunningPinchtab reports whether a live pinchtab process holds
// profileDir via its pinchtab.pid lock. It is the per-directory truth the
// profiles destructive-route guard falls back to on surfaces that have no
// orchestrator instance mapping.
func ProfileOwnedByRunningPinchtab(profileDir string) (bool, int) {
	return isProfileOwnedByRunningPinchtab(profileDir)
}

func isProfileOwnedByRunningPinchtab(profileDir string) (bool, int) {
	pidFile := filepath.Join(profileDir, "pinchtab.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("failed to read pinchtab pid file", "path", pidFile, "err", err)
		}
		return false, 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		slog.Debug("failed to parse pinchtab pid file", "path", pidFile, "err", err)
		return false, 0
	}

	if pid == os.Getpid() {
		return false, pid // It's us
	}

	running, err := chromePIDIsRunning(pid)
	if err == nil && running {
		// Even if the PID is running, check if it's actually a pinchtab process
		// to handle PID reuse.
		if isPinchTabProcessFunc(pid) {
			// Skip the browser check while the PID file is fresh: AcquireProfileLock
			// writes the file before InitBrowser launches the browser, so a second
			// process must not steal the lock during that startup window.
			if info, statErr := os.Stat(pidFile); statErr == nil && time.Since(info.ModTime()) < lockStartupGrace {
				slog.Debug("profile lock belongs to a recently started pinchtab; treating as active",
					"profile", profileDir, "pid", pid)
				return true, pid
			}
			processes, procErr := chromeProfileProcessLister(profileDir)
			if procErr == nil && len(processes) == 0 {
				slog.Debug("pinchtab pid file points to a running pinchtab but no browser is using the profile; treating lock as stale",
					"profile", profileDir, "pid", pid)
				return false, 0
			}
			if procErr != nil {
				slog.Debug("unable to verify browser ownership for running pinchtab pid; keeping profile locked",
					"profile", profileDir, "pid", pid, "err", procErr)
			}
			slog.Debug("profile is owned by another active pinchtab", "profile", profileDir, "pid", pid)
			return true, pid
		}
		slog.Debug("PID in lockfile is running but not a pinchtab process (PID reuse)", "profile", profileDir, "pid", pid)
	} else {
		slog.Debug("PID in lockfile is not running", "profile", profileDir, "pid", pid, "err", err)
	}

	return false, 0
}

func AcquireProfileLock(profileDir string) error {
	if profileDir == "" {
		return nil
	}
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return fmt.Errorf("mkdir profile dir: %w", err)
	}

	if owned, pid := isProfileOwnedByRunningPinchtab(profileDir); owned {
		return fmt.Errorf("profile %s is already in use by pinchtab process %d", profileDir, pid)
	}

	pidFile := filepath.Join(profileDir, "pinchtab.pid")
	slog.Debug("acquiring profile lock", "profile", profileDir, "pid", os.Getpid())
	return os.WriteFile(pidFile, fmt.Appendf(nil, "%d", os.Getpid()), 0644)
}

func extractChromeProfileLockPID(msg string) (int, bool) {
	if msg == "" {
		return 0, false
	}
	match := chromeProfileLockPIDPattern.FindStringSubmatch(msg)
	if len(match) != 2 {
		return 0, false
	}
	pid := 0
	for _, ch := range match[1] {
		pid = pid*10 + int(ch-'0')
	}
	if pid <= 0 {
		return 0, false
	}
	return pid, true
}

func findChromeProfileProcesses(profileDir string) ([]chromeProfileProcess, error) {
	if strings.TrimSpace(profileDir) == "" {
		return nil, nil
	}

	cmd := exec.Command("ps", "-axo", "pid=,args=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list chrome processes: %w", err)
	}

	return parseChromeProfileProcesses(out, profileDir), nil
}

func parseChromeProfileProcesses(out []byte, profileDir string) []chromeProfileProcess {
	if len(out) == 0 || strings.TrimSpace(profileDir) == "" {
		return nil
	}

	needleEquals := "--user-data-dir=" + profileDir
	needleSpace := "--user-data-dir " + profileDir
	lines := bytes.Split(out, []byte{'\n'})
	processes := make([]chromeProfileProcess, 0)

	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || (!strings.Contains(line, needleEquals) && !strings.Contains(line, needleSpace)) {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		processes = append(processes, chromeProfileProcess{
			PID:     fields[0],
			Command: strings.TrimSpace(strings.TrimPrefix(line, fields[0])),
		})
	}

	return processes
}
