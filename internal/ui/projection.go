package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
	sectionmodel "github.com/richardrh/lazymagit/internal/model"
)

type rowKind uint8

const (
	rowHeading rowKind = iota
	rowUntracked
	rowUnstaged
	rowStaged
	rowCommit
	rowStash
)

type row struct {
	id      sectionmodel.SectionID
	kind    rowKind
	path    string
	commit  gitbackend.Commit
	stash   gitbackend.Stash
	depth   int
	section string
}

// snapshot is the complete status-buffer input. It is fetched off the Bubble
// Tea update loop using a bounded sequence of Git queries.
type snapshot struct {
	summary                         gitbackend.Summary
	status                          gitbackend.Status
	stashes                         []gitbackend.Stash
	recent                          []gitbackend.Commit
	upstream                        gitbackend.UpstreamRanges
	remotes                         []gitbackend.Remote
	pushRemote                      string
	operations                      gitbackend.OperationState
	sparse                          gitbackend.SparseCheckoutState
	aheadTruncated, behindTruncated bool
}

var sectionIDs = []string{"untracked", "unstaged", "staged", "stashes", "unpushed", "unpulled", "recent"}

func project(s snapshot) ([]*sectionmodel.Section, map[sectionmodel.SectionID]row) {
	s = normalizeUpstreamSnapshot(s)
	children := make(map[string][]*sectionmodel.Section, len(sectionIDs))
	rows := make(map[sectionmodel.SectionID]row)

	files := append([]gitbackend.FileStatus(nil), s.status.Files...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		if f.Unstaged == gitbackend.ChangeUntracked {
			addFile(children, rows, "untracked", rowUntracked, f.Path, f.Unstaged)
		} else if f.Unstaged != gitbackend.ChangeNone {
			addFile(children, rows, "unstaged", rowUnstaged, f.Path, f.Unstaged)
		}
		if f.Staged != gitbackend.ChangeNone {
			addFile(children, rows, "staged", rowStaged, f.Path, f.Staged)
		}
	}
	addCommits := func(section string, commits []gitbackend.Commit) {
		for _, c := range commits {
			id := sectionmodel.SectionID("status/" + section + "/commit/" + c.ID)
			children[section] = append(children[section], sectionmodel.NewSection(id, commitTitle(c)))
			rows[id] = row{id: id, kind: rowCommit, commit: c, depth: 2, section: section}
		}
	}
	addCommits("unpushed", s.upstream.Ahead)
	addCommits("unpulled", s.upstream.Behind)
	seenStashes := make(map[string]bool, len(s.stashes))
	for _, stash := range s.stashes {
		if seenStashes[stash.ID] {
			continue
		}
		seenStashes[stash.ID] = true
		id := sectionmodel.SectionID("status/stashes/stash/" + stash.ID)
		children["stashes"] = append(children["stashes"], sectionmodel.NewSection(id, stashTitle(stash)))
		rows[id] = row{id: id, kind: rowStash, stash: stash, depth: 2, section: "stashes"}
	}
	if len(s.upstream.Ahead) == 0 {
		addCommits("recent", s.recent)
	}

	titles := map[string]string{
		"untracked": "Untracked files", "unstaged": "Unstaged changes", "staged": "Staged changes", "stashes": "Stashes",
		"unpushed": "Unmerged into " + sanitizeSingleLine(s.summary.Upstream),
		"unpulled": "Unpulled from " + sanitizeSingleLine(s.summary.Upstream),
		"recent":   "Recent commits",
	}
	roots := make([]*sectionmodel.Section, 0, len(sectionIDs))
	for _, section := range sectionIDs {
		if len(children[section]) == 0 {
			continue
		}
		id := sectionmodel.SectionID("status/" + section)
		title := strings.TrimSpace(titles[section])
		if section != "recent" {
			count := fmt.Sprintf("%d", len(children[section]))
			if (section == "unpushed" && s.aheadTruncated) || (section == "unpulled" && s.behindTruncated) {
				count += "+"
			}
			title = fmt.Sprintf("%s (%s)", title, count)
		}
		roots = append(roots, sectionmodel.NewSection(id, title, children[section]...))
		rows[id] = row{id: id, kind: rowHeading, depth: 1, section: section}
	}
	return roots, rows
}

func addFile(children map[string][]*sectionmodel.Section, rows map[sectionmodel.SectionID]row, section string, kind rowKind, path string, change gitbackend.Change) {
	id := sectionmodel.SectionID("status/" + section + "/file/" + path)
	title := sanitizeSingleLine(filepath.ToSlash(path))
	if kind != rowUntracked {
		title = fmt.Sprintf("%-11s%s", changeWord(change), title)
	}
	children[section] = append(children[section], sectionmodel.NewSection(id, title))
	rows[id] = row{id: id, kind: kind, path: path, depth: 2, section: section}
}

func changeWord(c gitbackend.Change) string {
	switch c {
	case gitbackend.ChangeAdded, gitbackend.ChangeUntracked:
		return "new file"
	case gitbackend.ChangeDeleted:
		return "deleted"
	case gitbackend.ChangeRenamed:
		return "renamed"
	case gitbackend.ChangeCopied:
		return "copied"
	case gitbackend.ChangeTypeChanged:
		return "type changed"
	case gitbackend.ChangeUnmerged:
		return "unmerged"
	default:
		return "modified"
	}
}

func stashTitle(stash gitbackend.Stash) string {
	id := sanitizeSingleLine(stash.ShortID)
	if id == "" {
		idRunes := []rune(sanitizeSingleLine(stash.ID))
		if len(idRunes) > 7 {
			idRunes = idRunes[:7]
		}
		id = string(idRunes)
	}
	if subject := sanitizeSingleLine(strings.TrimSpace(stash.Subject)); subject != "" {
		return strings.TrimSpace(id + " " + subject)
	}
	return id
}

func commitTitle(c gitbackend.Commit) string {
	id := sanitizeSingleLine(c.ShortID)
	if id == "" {
		idRunes := []rune(sanitizeSingleLine(c.ID))
		if len(idRunes) > 7 {
			idRunes = idRunes[:7]
		}
		id = string(idRunes)
	}
	parts := []string{id}
	for _, ref := range strings.Split(sanitizeSingleLine(c.Refs), ", ") {
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "tag: ")
		if strings.HasPrefix(ref, "HEAD -> ") {
			ref = strings.TrimPrefix(ref, "HEAD -> ")
		}
		if ref == "HEAD" {
			ref = "@"
		}
		if ref != "" {
			parts = append(parts, ref)
		}
	}
	if subject := sanitizeSingleLine(strings.TrimSpace(c.Subject)); subject != "" {
		parts = append(parts, subject)
	}
	return strings.Join(parts, " ")
}

func normalizeUpstreamSnapshot(s snapshot) snapshot {
	normalize := func(commits []gitbackend.Commit, exact int, hasExact bool, truncated bool) ([]gitbackend.Commit, bool) {
		if hasExact && exact <= len(commits) {
			commits = commits[:exact]
			if exact <= 256 {
				truncated = false
			}
		}
		if len(commits) > 256 {
			commits = commits[:256]
			truncated = true
		}
		return commits, truncated
	}
	hasExact := s.summary.Upstream != ""
	s.upstream.Ahead, s.aheadTruncated = normalize(s.upstream.Ahead, s.summary.Ahead, hasExact, s.aheadTruncated)
	s.upstream.Behind, s.behindTruncated = normalize(s.upstream.Behind, s.summary.Behind, hasExact, s.behindTruncated)
	return s
}
