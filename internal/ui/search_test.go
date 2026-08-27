package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestStatusSearchFiltersHighlightsAndNavigates(t *testing.T) {
	m := New(nil)
	m.loading = false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{
		{Path: "alpha.txt", Unstaged: gitbackend.ChangeModified},
		{Path: "beta.txt", Staged: gitbackend.ChangeModified},
		{Path: "alphabet.txt", Unstaged: gitbackend.ChangeUntracked},
	}}})
	m.tree.RevealGlobalDepth(4)

	_, _ = m.Update(keyMsg("/"))
	for _, r := range "alpha" {
		_, _ = m.Update(keyMsg(string(r)))
	}
	if !m.searching || m.searchQuery != "alpha" || len(m.searchMatches) != 2 || !strings.Contains(string(m.tree.Cursor()), "alpha") {
		t.Fatalf("search state query=%q matches=%v cursor=%q", m.searchQuery, m.searchMatches, m.tree.Cursor())
	}
	standard := m.renderStatusPanel(70, 16)
	if !strings.Contains(standard, "\x1b[") || !strings.Contains(ansi.Strip(standard), "alpha.txt") {
		t.Fatalf("standard search highlight missing: %q", ansi.Strip(standard))
	}
	_, _ = m.Update(keyMsg("enter"))
	first := m.tree.Cursor()
	_, _ = m.Update(keyMsg("n"))
	if m.tree.Cursor() == first || m.searching {
		t.Fatalf("next match cursor=%q first=%q searching=%v", m.tree.Cursor(), first, m.searching)
	}
	_, _ = m.Update(keyMsg("N"))
	if m.tree.Cursor() != first {
		t.Fatalf("previous match cursor=%q, want %q", m.tree.Cursor(), first)
	}
	_, _ = m.Update(keyMsg("esc"))
	if m.searchQuery != "" || len(m.searchMatches) != 0 {
		t.Fatalf("search was not cleared: query=%q matches=%v", m.searchQuery, m.searchMatches)
	}
}

func TestCompactLayoutIsDenseAndPreservesDetail(t *testing.T) {
	m := NewWithOptions(nil, Options{Compact: true})
	m.width, m.height, m.loading = 120, 24, false
	m.install(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "file.txt", Unstaged: gitbackend.ChangeModified}}}})
	m.tree.SetCursor("status/unstaged/file/file.txt")
	m.detail = "diff --git a/file.txt b/file.txt\n@@ -1 +1 @@\n-old\n+new"
	body := m.renderMainBody(20)
	plain := ansi.Strip(body)
	if strings.ContainsAny(plain, "╭╮╰╯") || !strings.Contains(plain, "file.txt") || !strings.Contains(plain, "diff --git") || !strings.Contains(plain, "│") {
		t.Fatalf("compact body is not dense split layout:\n%s", plain)
	}
	if width, height := ansi.StringWidth(strings.Split(body, "\n")[0]), strings.Count(body, "\n")+1; width != 120 || height != 20 {
		t.Fatalf("compact dimensions = %dx%d, want 120x20", width, height)
	}
}
