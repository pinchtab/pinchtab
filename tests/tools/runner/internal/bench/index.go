package bench

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

var (
	groupFileRegex    = regexp.MustCompile(`^group-(\d+)\.md$`)
	listItemRegex     = regexp.MustCompile(`^-\s+` + "`?" + `group-(\d+)\.md` + "`?")
	htmlCommentStart  = regexp.MustCompile(`<!--`)
	htmlCommentEnd    = regexp.MustCompile(`-->`)
	commentedListItem = regexp.MustCompile(`<!--\s*-\s+` + "`?" + `group-(\d+)\.md` + "`?")
)

type IndexFile struct {
	Dir      string
	Active   []int
	Disabled []int
}

func ParseIndexFile(path string) (IndexFile, error) {
	dir := filepath.Dir(path)
	content, err := os.ReadFile(path)
	if err != nil {
		return IndexFile{Dir: dir}, err
	}
	return parseIndexContent(dir, string(content))
}

func parseIndexContent(dir, content string) (IndexFile, error) {
	idx := IndexFile{Dir: dir}
	disabledSet := make(map[int]struct{})
	activeSet := make(map[int]struct{})

	scanner := bufio.NewScanner(strings.NewReader(content))
	inComment := false
	inGroupSection := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(strings.TrimSpace(line), "## Group Files") {
			inGroupSection = true
			continue
		}

		if inGroupSection && strings.HasPrefix(strings.TrimSpace(line), "## ") {
			break
		}

		if !inGroupSection {
			continue
		}

		if htmlCommentStart.MatchString(line) && !htmlCommentEnd.MatchString(line) {
			inComment = true
			if m := listItemRegex.FindStringSubmatch(stripCommentStart(line)); m != nil {
				n, _ := strconv.Atoi(m[1])
				disabledSet[n] = struct{}{}
			}
			continue
		}

		if inComment {
			if m := listItemRegex.FindStringSubmatch(line); m != nil {
				n, _ := strconv.Atoi(m[1])
				disabledSet[n] = struct{}{}
			}
			if htmlCommentEnd.MatchString(line) {
				inComment = false
			}
			continue
		}

		if m := commentedListItem.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			disabledSet[n] = struct{}{}
			continue
		}

		if m := listItemRegex.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			activeSet[n] = struct{}{}
		}
	}

	for n := range activeSet {
		idx.Active = append(idx.Active, n)
	}
	sort.Ints(idx.Active)

	for n := range disabledSet {
		idx.Disabled = append(idx.Disabled, n)
	}
	sort.Ints(idx.Disabled)

	return idx, nil
}

func stripCommentStart(line string) string {
	_, after, ok := strings.Cut(line, "<!--")
	if ok {
		return after
	}
	return line
}

func ScanGroupFiles(dir string) ([]int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var groups []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if m := groupFileRegex.FindStringSubmatch(e.Name()); m != nil {
			n, _ := strconv.Atoi(m[1])
			groups = append(groups, n)
		}
	}
	sort.Ints(groups)
	return groups, nil
}

func (idx IndexFile) IsDisabled(group int) bool {
	return slices.Contains(idx.Disabled, group)
}
