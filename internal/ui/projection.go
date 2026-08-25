package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	gitbackend "github.com/richard/lazymagit/internal/git"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

type rowKind uint8

const (
	rowHeading rowKind = iota
	rowUntracked
	rowUnstaged
	rowStaged
	rowCommit
)

type row struct {
	id      sectionmodel.SectionID
	kind    rowKind
	path    string
	commit  gitbackend.Commit
	depth   int
	section string
}

// snapshot is the complete status-buffer input. It is fetched off the Bubble
// Tea update loop using a bounded sequence of Git queries.
type snapshot struct {
	summary  gitbackend.Summary
	status   gitbackend.Status
	recent   []gitbackend.Commit
	upstream gitbackend.UpstreamRanges
}

var sectionOrder = []struct {
	id, title string
}{
	{"untracked", "Untracked"},
	{"unstaged", "Unstaged"},
	{"staged", "Staged"},
	{"unpushed", "Unpushed"},
	{"unpulled", "Unpulled"},
	{"recent", "Recent commits"},
}

func project(s snapshot) ([]*sectionmodel.Section, map[sectionmodel.SectionID]row) {
	children := make(map[string][]*sectionmodel.Section, len(sectionOrder))
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
	addCommits("recent", s.recent)

	roots := make([]*sectionmodel.Section, 0, len(sectionOrder))
	for _, spec := range sectionOrder {
		id := sectionmodel.SectionID("status/" + spec.id)
		title := fmt.Sprintf("%s  (%d)", spec.title, len(children[spec.id]))
		roots = append(roots, sectionmodel.NewSection(id, title, children[spec.id]...))
		rows[id] = row{id: id, kind: rowHeading, depth: 1, section: spec.id}
	}
	return roots, rows
}

func addFile(children map[string][]*sectionmodel.Section, rows map[sectionmodel.SectionID]row, section string, kind rowKind, path string, change gitbackend.Change) {
	id := sectionmodel.SectionID("status/" + section + "/file/" + path)
	title := changeMark(change) + " " + sanitizeSingleLine(filepath.ToSlash(path))
	children[section] = append(children[section], sectionmodel.NewSection(id, title))
	rows[id] = row{id: id, kind: kind, path: path, depth: 2, section: section}
}

func changeMark(c gitbackend.Change) string {
	switch c {
	case gitbackend.ChangeAdded, gitbackend.ChangeUntracked:
		return "A"
	case gitbackend.ChangeDeleted:
		return "D"
	case gitbackend.ChangeRenamed:
		return "R"
	case gitbackend.ChangeCopied:
		return "C"
	case gitbackend.ChangeTypeChanged:
		return "T"
	case gitbackend.ChangeUnmerged:
		return "U"
	default:
		return "M"
	}
}

func commitTitle(c gitbackend.Commit) string {
	idRunes := []rune(sanitizeSingleLine(c.ID))
	if len(idRunes) > 8 {
		idRunes = idRunes[:8]
	}
	return fmt.Sprintf("%s  %s", string(idRunes), sanitizeSingleLine(strings.TrimSpace(c.Subject)))
}
