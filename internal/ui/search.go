package ui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func (m *Model) handleStatusSearchKey(key, text string) bool {
	if !m.searching {
		if key == "/" {
			m.searching = true
			m.searchQuery = ""
			m.searchMatches = nil
			m.searchIndex = 0
			m.setMessage("Search status rows; type a query, Enter applies, Esc clears")
			return true
		}
		if m.searchQuery == "" {
			return false
		}
		switch key {
		case "n":
			m.moveStatusSearch(1)
			return true
		case "N":
			m.moveStatusSearch(-1)
			return true
		case "esc":
			m.clearStatusSearch()
			m.setMessage("Search cleared")
			return true
		}
		return false
	}
	switch key {
	case "esc":
		m.clearStatusSearch()
		m.setMessage("Search cleared")
	case "enter":
		m.searching = false
		if len(m.searchMatches) == 0 {
			m.setMessage("No status matches for " + sanitizeSingleLine(m.searchQuery))
		} else {
			m.setMessage(statusSearchMessage(m.searchQuery, m.searchIndex, len(m.searchMatches)) + "; n/N move, Esc clears")
		}
	case "backspace", "ctrl+h":
		if m.searchQuery != "" {
			_, size := utf8.DecodeLastRuneInString(m.searchQuery)
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-size]
			m.refreshStatusSearch()
		}
	default:
		if text == "" {
			return true
		}
		m.searchQuery += text
		m.refreshStatusSearch()
	}
	return true
}

func (m *Model) clearStatusSearch() {
	m.searching = false
	m.searchQuery = ""
	m.searchMatches = nil
	m.searchIndex = 0
}

func (m *Model) refreshStatusSearch() {
	query := strings.ToLower(strings.TrimSpace(m.searchQuery))
	m.searchMatches = nil
	m.searchIndex = 0
	if query == "" {
		m.setMessage("Search status rows; type a query")
		return
	}
	for _, id := range m.tree.VisibleSectionIDs() {
		section := m.tree.Section(id)
		row := m.rows[id]
		values := []string{section.Title(), row.path, row.commit.Subject, row.commit.Refs, row.stash.Subject, row.stash.Ref}
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), query) {
				m.searchMatches = append(m.searchMatches, id)
				break
			}
		}
	}
	if len(m.searchMatches) == 0 {
		m.setMessage("No status matches for " + sanitizeSingleLine(m.searchQuery))
		return
	}
	m.tree.SetCursor(m.searchMatches[0])
	m.setMessage(statusSearchMessage(m.searchQuery, 0, len(m.searchMatches)))
}

func (m *Model) moveStatusSearch(delta int) {
	if len(m.searchMatches) == 0 {
		m.setMessage("No status matches for " + sanitizeSingleLine(m.searchQuery))
		return
	}
	m.searchIndex = (m.searchIndex + delta + len(m.searchMatches)) % len(m.searchMatches)
	m.tree.SetCursor(m.searchMatches[m.searchIndex])
	m.setMessage(statusSearchMessage(m.searchQuery, m.searchIndex, len(m.searchMatches)))
}

func statusSearchMessage(query string, index, total int) string {
	return "Search " + sanitizeSingleLine(query) + " — match " + strconv.Itoa(index+1) + "/" + strconv.Itoa(total)
}

func (m *Model) statusSearchMatch(id sectionmodel.SectionID) bool {
	for _, match := range m.searchMatches {
		if match == id {
			return true
		}
	}
	return false
}
