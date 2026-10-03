package bridge

import (
	"bytes"
	"os"
	"slices"
	"strconv"
)

// Test seams: overridden to describe a process tree without ps, /proc or
// signalling real processes.
var (
	parentPIDsFunc                 = parentPIDs
	allChromePIDsByProfileDirFunc  = findChromePIDsByProfileDir
	forceKillPIDsFunc              = forceKillPIDs
	findChromePIDsByProfileDirFunc = findReclaimableChromePIDsByProfileDir
)

const (
	maxAncestorHops = 64
	noSelf          = -1
)

func findReclaimableChromePIDsByProfileDir(profileDir string) []int {
	return reclaimableChromePIDs(allChromePIDsByProfileDirFunc(profileDir))
}

func reclaimableChromePIDs(pids []int) []int {
	return chromePIDsWithoutLivePeer(pids, os.Getpid())
}

// Descendants of this process are deliberately not orphans: a server sweeping
// after one instance stops must not reap a sibling instance's Chrome.
func orphanedChromePIDs(pids []int) []int {
	return chromePIDsWithoutLivePeer(pids, noSelf)
}

// An empty process table means the probe failed, and nothing can be proven
// ownerless, so nothing is returned.
func chromePIDsWithoutLivePeer(pids []int, self int) []int {
	if len(pids) == 0 {
		return nil
	}
	parents := parentPIDsFunc()
	if len(parents) == 0 {
		return nil
	}
	chrome := make(map[int]bool, len(pids))
	for _, pid := range pids {
		chrome[pid] = true
	}
	var ownerless []int
	for _, pid := range pids {
		if pid == os.Getpid() {
			continue
		}
		lineage := processLineage(pid, parents)
		if slices.Contains(lineage, self) || !hasLiveLauncher(lineage, chrome, parents) {
			ownerless = append(ownerless, pid)
		}
	}
	return ownerless
}

func processLineage(pid int, parents map[int]int) []int {
	lineage := []int{pid}
	for cur := pid; len(lineage) < maxAncestorHops; {
		ppid, ok := parents[cur]
		if !ok {
			break
		}
		lineage = append(lineage, ppid)
		cur = ppid
	}
	return lineage
}

// The launcher is the parent of the topmost Chrome process in the lineage.
// Unix reparents an orphan to init (pid 1); Windows leaves it pointing at a
// pid that is no longer in the process table.
func hasLiveLauncher(lineage []int, chrome map[int]bool, parents map[int]int) bool {
	top := 0
	for i, pid := range lineage {
		if chrome[pid] {
			top = i
		}
	}
	if top+1 >= len(lineage) {
		return false
	}
	launcher := lineage[top+1]
	_, alive := parents[launcher]
	return launcher > 1 && alive
}

func profileHasLiveOwner(profileDir string) bool {
	if owned, _ := isProfileOwnedByRunningPinchtabMock(profileDir); owned {
		return true
	}
	all := allChromePIDsByProfileDirFunc(profileDir)
	return len(orphanedChromePIDs(all)) < len(all)
}

func killOrphanedChromeByProfileDir(profileDir string) int {
	orphans := orphanedChromePIDs(allChromePIDsByProfileDirFunc(profileDir))
	if len(orphans) == 0 {
		return 0
	}
	return forceKillPIDsFunc(orphans)
}

func parsePIDPairs(out []byte) map[int]int {
	parents := make(map[int]int)
	for line := range bytes.SplitSeq(out, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(string(fields[0]))
		if err != nil || pid <= 0 {
			continue
		}
		ppid, err := strconv.Atoi(string(fields[1]))
		if err != nil {
			continue
		}
		parents[pid] = ppid
	}
	return parents
}
