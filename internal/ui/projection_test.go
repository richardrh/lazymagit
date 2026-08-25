package ui

import (
	"reflect"
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
		"status/unpushed", "status/unpulled", "status/recent", "status/recent/commit/1234567890",
	}
	if got := m.VisibleSectionIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible rows = %v, want %v", got, want)
	}
	if rows["status/unstaged/file/both.txt"].kind != rowUnstaged || rows["status/staged/file/both.txt"].kind != rowStaged {
		t.Fatal("mixed file did not retain separate staged and unstaged identities")
	}
}
