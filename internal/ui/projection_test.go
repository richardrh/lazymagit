package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	sectionmodel "github.com/richard/lazymagit/internal/model"
)

func TestProjectionOrderingAndMixedFileIdentity(t *testing.T) {
	s := snapshot{
		status: gitbackend.Status{Files: []gitbackend.FileStatus{
			{Path: "z.txt", Unstaged: gitbackend.ChangeUntracked},
			{Path: "both.txt", Staged: gitbackend.ChangeModified, Unstaged: gitbackend.ChangeModified},
		}},
		recent: []gitbackend.Commit{{ID: "1234567890", Subject: "subject"}},
	}
	sections, rows := project(s)
	m := sectionmodel.New(sections)
	want := []sectionmodel.SectionID{
		"status/untracked", "status/untracked/file/z.txt",
		"status/unstaged", "status/unstaged/file/both.txt",
		"status/staged", "status/staged/file/both.txt",
		"status/recent", "status/recent/commit/1234567890",
	}
	if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible rows = %v, want %v", got, want)
	}
	if rows["status/unstaged/file/both.txt"].kind != rowUnstaged || rows["status/staged/file/both.txt"].kind != rowStaged {
		t.Fatal("mixed file did not retain separate staged and unstaged identities")
	}
}

func TestProjectionMagitCommitRowsUpstreamHeadingsAndFallback(t *testing.T) {
	s := snapshot{
		summary: gitbackend.Summary{Upstream: "origin/main", Ahead: 1, Behind: 1},
		status: gitbackend.Status{Files: []gitbackend.FileStatus{
			{Path: "modified.txt", Unstaged: gitbackend.ChangeModified},
			{Path: "new.txt", Staged: gitbackend.ChangeAdded},
		}},
		upstream: gitbackend.UpstreamRanges{
			Ahead:  []gitbackend.Commit{{ID: "123456789abcdef", Refs: "HEAD -> main, tag: v1", Subject: "subject\x1b[2J"}},
			Behind: []gitbackend.Commit{{ID: "abcdef012345678", Refs: "origin/main", Subject: "remote"}},
		},
		recent: []gitbackend.Commit{{ID: "999999999", Subject: "must not duplicate ahead"}},
	}
	sections, _ := project(s)
	m := sectionmodel.New(sections)
	want := []string{
		"Unstaged changes (1)", "modified   modified.txt", "Staged changes (1)", "new file   new.txt",
		"Unmerged into origin/main (1)", "1234567 main v1 subject␛[2J",
		"Unpulled from origin/main (1)", "abcdef0 origin/main remote",
	}
	var got []string
	for _, section := range m.VisibleSections() {
		got = append(got, section.Title())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected titles = %#v, want %#v", got, want)
	}
	if m.Section("status/recent") != nil || m.Section("status/untracked") != nil {
		t.Fatal("projection retained fallback/empty sections")
	}
}

func TestProjectionUsesRecentOnlyWithoutAheadCommits(t *testing.T) {
	sections, _ := project(snapshot{recent: []gitbackend.Commit{{ID: "123456789", Subject: "recent"}}})
	m := sectionmodel.New(sections)
	if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, []sectionmodel.SectionID{"status/recent", "status/recent/commit/123456789"}) {
		t.Fatalf("fallback rows = %v", got)
	}
	if got := m.Section("status/recent").Title(); got != "Recent commits" {
		t.Fatalf("recent heading = %q, want Magit's heading without a count", got)
	}
}

func TestProjectionMatchesMagitUntrackedHeadingAndRow(t *testing.T) {
	sections, _ := project(snapshot{status: gitbackend.Status{Files: []gitbackend.FileStatus{{Path: "new.txt", Unstaged: gitbackend.ChangeUntracked}}}})
	m := sectionmodel.New(sections)
	if got := []string{m.Section("status/untracked").Title(), m.Section("status/untracked/file/new.txt").Title()}; !reflect.DeepEqual(got, []string{"Untracked files (1)", "new.txt"}) {
		t.Fatalf("untracked display = %#v", got)
	}
}

func TestCommitTitleUsesGitShortIDAndPreservesCommaInRef(t *testing.T) {
	c := gitbackend.Commit{ID: "1234567890abcdef", ShortID: "1234567890ab", Refs: "HEAD -> topic,with-comma, tag: release,one, origin/topic", Subject: "subject"}
	if got, want := commitTitle(c), "1234567890ab topic,with-comma release,one origin/topic subject"; got != want {
		t.Fatalf("commit title = %q, want %q", got, want)
	}
	c.ShortID = ""
	if got := commitTitle(c); !strings.HasPrefix(got, "1234567 ") {
		t.Fatalf("empty ShortID did not use seven-character fallback: %q", got)
	}
}

func TestProjectionCapsUpstreamAndMarksOnlyTruncatedSides(t *testing.T) {
	commits := make([]gitbackend.Commit, 257)
	for i := range commits {
		commits[i] = gitbackend.Commit{ID: fmt.Sprintf("%040d", i), ShortID: fmt.Sprintf("%07d", i)}
	}
	s := normalizeUpstreamSnapshot(snapshot{
		summary:  gitbackend.Summary{Upstream: "origin/main", Ahead: 300, Behind: 2},
		upstream: gitbackend.UpstreamRanges{Ahead: commits, Behind: commits},
	})
	if len(s.upstream.Ahead) != 256 || !s.aheadTruncated {
		t.Fatalf("ahead retained %d commits, truncated=%v", len(s.upstream.Ahead), s.aheadTruncated)
	}
	if len(s.upstream.Behind) != 2 || s.behindTruncated {
		t.Fatalf("behind retained %d commits, truncated=%v", len(s.upstream.Behind), s.behindTruncated)
	}
	sections, _ := project(s)
	m := sectionmodel.New(sections)
	if got := m.Section("status/unpushed").Title(); got != "Unmerged into origin/main (256+)" {
		t.Fatalf("truncated heading = %q", got)
	}
	if got := m.Section("status/unpulled").Title(); got != "Unpulled from origin/main (2)" {
		t.Fatalf("exact heading = %q", got)
	}
}
