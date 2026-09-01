package ui

import (
	"fmt"
	"sort"
)

func markForRow(r row) (fileMark, bool) {
	switch r.kind {
	case rowUntracked, rowUnstaged, rowStaged:
		return fileMark{kind: r.kind, path: r.path}, r.path != ""
	default:
		return fileMark{}, false
	}
}

func (m *Model) toggleFileMark() bool {
	mark, ok := markForRow(m.rows[m.tree.Cursor()])
	if !ok {
		return false
	}
	if m.markedFiles == nil {
		m.markedFiles = make(map[fileMark]bool)
	}
	if m.markedFiles[mark] {
		delete(m.markedFiles, mark)
		m.setMessage("Unmarked " + sanitizeSingleLine(mark.path))
		return true
	}
	m.markedFiles[mark] = true
	m.setMessage(fmt.Sprintf("Marked %s (%d selected)", sanitizeSingleLine(mark.path), len(m.markedFiles)))
	return true
}

func (m *Model) rowMarked(r row) bool {
	mark, ok := markForRow(r)
	return ok && m.markedFiles[mark]
}

func (m *Model) fileOperationPaths(kinds ...rowKind) []string {
	allowed := make(map[rowKind]bool, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = true
	}
	paths := make(map[string]bool)
	for mark := range m.markedFiles {
		if allowed[mark.kind] {
			paths[mark.path] = true
		}
	}
	if len(paths) == 0 {
		if selected, ok := m.selectedFile(kinds...); ok {
			paths[selected.path] = true
		}
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func fileOperationName(action string, paths []string) string {
	if len(paths) == 1 {
		return action
	}
	return fmt.Sprintf("%s %d files", action, len(paths))
}

func (m *Model) pruneFileMarks() {
	available := make(map[fileMark]bool)
	for _, r := range m.rows {
		if mark, ok := markForRow(r); ok {
			available[mark] = true
		}
	}
	for mark := range m.markedFiles {
		if !available[mark] {
			delete(m.markedFiles, mark)
		}
	}
}
