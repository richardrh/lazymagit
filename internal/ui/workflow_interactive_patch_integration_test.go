package ui

import (
	"strings"
	"testing"
)

func TestInteractivePatchKeysMultiHunkAndDisjointRegions(t *testing.T) {
	t.Run("stage unstage and discard selected hunks", func(t *testing.T) {
		r := newUIE2ERepo(t)
		lines := make([]string, 120)
		for i := range lines {
			lines[i] = "line"
		}
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		r.git("add", "file.txt")
		r.git("commit", "-m", "base")
		lines[4], lines[94] = "FIRST", "SECOND"
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		m := newE2EModel(t, r)

		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())
		selectTwoHunksByKeys(t, m)
		sendE2EKey(t, m, keyMsg("s"))
		if m.workflow == nil || m.workflow.dialog.Title != "Stage selected hunks" {
			t.Fatalf("multi stage did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got := r.git("show", ":file.txt"); !strings.Contains(got, "FIRST") || !strings.Contains(got, "SECOND") {
			t.Fatalf("multi stage index:\n%s", got)
		}

		selectE2EPath(t, m, "file.txt", rowStaged)
		runE2ECmd(t, m, m.loadDetailCmd())
		selectTwoHunksByKeys(t, m)
		sendE2EKey(t, m, keyMsg("u"))
		if m.workflow == nil || m.workflow.dialog.Title != "Unstage selected hunks" {
			t.Fatalf("multi unstage did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got := r.git("diff", "--cached", "--name-only"); got != "" {
			t.Fatalf("multi unstage left index change: %q", got)
		}

		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())
		selectTwoHunksByKeys(t, m)
		sendE2EKey(t, m, keyMsg("x"))
		if m.workflow == nil || m.workflow.dialog.Title != "Discard selected unstaged hunks" {
			t.Fatalf("multi discard did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got, want := r.git("hash-object", "file.txt"), r.git("rev-parse", "HEAD:file.txt"); got != want {
			t.Fatalf("multi discard worktree blob = %q, want %q", got, want)
		}
	})

	t.Run("discard selected staged hunks", func(t *testing.T) {
		r := newUIE2ERepo(t)
		lines := make([]string, 120)
		for i := range lines {
			lines[i] = "line"
		}
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		r.git("add", "file.txt")
		r.git("commit", "-m", "base")
		lines[4], lines[94] = "FIRST", "SECOND"
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		m := newE2EModel(t, r)
		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())
		selectTwoHunksByKeys(t, m)
		sendE2EKey(t, m, keyMsg("s"))
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))

		selectE2EPath(t, m, "file.txt", rowStaged)
		runE2ECmd(t, m, m.loadDetailCmd())
		selectTwoHunksByKeys(t, m)
		sendE2EKey(t, m, keyMsg("x"))
		if m.workflow == nil || m.workflow.dialog.Title != "Discard selected staged hunks" {
			t.Fatalf("multi staged discard did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got := r.git("diff", "--cached", "--name-only"); got != "" {
			t.Fatalf("multi staged discard left index change: %q", got)
		}
		if got, want := r.git("hash-object", "file.txt"), r.git("rev-parse", "HEAD:file.txt"); got != want {
			t.Fatalf("multi staged discard worktree blob = %q, want %q", got, want)
		}
	})

	t.Run("disjoint changed-line regions", func(t *testing.T) {
		r := newUIE2ERepo(t)
		r.write("file.txt", "one\ntwo\nthree\nfour\nfive\n")
		r.git("add", "file.txt")
		r.git("commit", "-m", "base")
		r.write("file.txt", "one\nTWO\nthree\nFOUR\nfive\n")
		m := newE2EModel(t, r)
		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())

		sendE2EKey(t, m, keyMsg("]"))
		sendE2EKey(t, m, keyMsg("v"))
		sendE2EKey(t, m, keyMsg("j"))
		sendE2EKey(t, m, keyMsg("space")) // pin TWO's delete/add block
		sendE2EKey(t, m, keyMsg("j"))
		sendE2EKey(t, m, keyMsg("v"))
		sendE2EKey(t, m, keyMsg("j"))
		sendE2EKey(t, m, keyMsg("s"))
		if m.workflow == nil || m.workflow.dialog.Title != "Stage selected regions" {
			t.Fatalf("disjoint regions did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got, want := r.git("show", ":file.txt"), "one\nTWO\nthree\nFOUR\nfive"; got != want {
			t.Fatalf("disjoint regions index = %q, want %q", got, want)
		}
	})
}

func selectTwoHunksByKeys(t *testing.T, m *Model) {
	t.Helper()
	sendE2EKey(t, m, keyMsg("]"))
	sendE2EKey(t, m, keyMsg("V"))
	sendE2EKey(t, m, keyMsg("]"))
	sendE2EKey(t, m, keyMsg("V"))
	if len(m.detailSelectedHunks) != 2 {
		t.Fatalf("selected hunks = %#v", m.detailSelectedHunks)
	}
}

func TestInteractivePatchKeysStageFocusedHunkAndLineRange(t *testing.T) {
	t.Run("focused hunk", func(t *testing.T) {
		r := newUIE2ERepo(t)
		lines := make([]string, 120)
		for i := range lines {
			lines[i] = "line"
		}
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		r.git("add", "file.txt")
		r.git("commit", "-m", "base")
		lines[4], lines[94] = "first change", "second change"
		r.write("file.txt", strings.Join(lines, "\n")+"\n")
		m := newE2EModel(t, r)
		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())

		sendE2EKey(t, m, keyMsg("]"))
		if m.detailHunk < 0 || m.message != "Next hunk" {
			t.Fatalf("hunk focus = %d message=%q", m.detailHunk, m.message)
		}
		sendE2EKey(t, m, keyMsg("s"))
		if m.workflow == nil || m.workflow.dialog.Title != "Stage focused hunk" {
			t.Fatalf("focused stage did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		if m.workflow == nil || m.workflow.review == nil {
			t.Fatal("focused stage did not produce review")
		}
		sendE2EKey(t, m, keyMsg("enter"))
		cached := r.git("diff", "--cached", "--", "file.txt")
		unstaged := r.git("diff", "--", "file.txt")
		if !strings.Contains(cached, "first change") || strings.Contains(cached, "second change") {
			t.Fatalf("cached patch targeted wrong hunk:\n%s", cached)
		}
		if strings.Contains(unstaged, "first change") || !strings.Contains(unstaged, "second change") {
			t.Fatalf("unstaged patch targeted wrong hunk:\n%s", unstaged)
		}
	})

	t.Run("changed line range", func(t *testing.T) {
		r := newUIE2ERepo(t)
		r.write("file.txt", "one\ntwo\nthree\nfour\n")
		r.git("add", "file.txt")
		r.git("commit", "-m", "base")
		r.write("file.txt", "one\nTWO\nthree\nFOUR\n")
		m := newE2EModel(t, r)
		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())

		sendE2EKey(t, m, keyMsg("]"))
		sendE2EKey(t, m, keyMsg("v"))
		sendE2EKey(t, m, keyMsg("j"))
		if m.detailRangeStart < 0 || m.detailRangeEnd <= m.detailRangeStart {
			t.Fatalf("line range = %d..%d", m.detailRangeStart, m.detailRangeEnd)
		}
		sendE2EKey(t, m, keyMsg("s"))
		if m.workflow == nil || m.workflow.dialog.Title != "Stage selected lines" {
			t.Fatalf("line stage did not open review: %+v", m.workflow)
		}
		sendE2EKey(t, m, keyMsg("enter"))
		sendE2EKey(t, m, keyMsg("enter"))
		if got, want := r.git("show", ":file.txt"), "one\nTWO\nthree\nfour"; got != want {
			t.Fatalf("line-range stage = %q, want %q", got, want)
		}
	})

	t.Run("search query does not steal range extension in Magit mode", func(t *testing.T) {
		r := newUIE2ERepo(t)
		r.write("file.txt", "one\ntwo\nthree\n")
		r.git("add", "--", "file.txt")
		r.git("commit", "-m", "base")
		r.write("file.txt", "one\nTWO\nthree\n")
		m := newE2EModel(t, r)
		selectE2EPath(t, m, "file.txt", rowUnstaged)
		runE2ECmd(t, m, m.loadDetailCmd())

		// Switch to Magit mode, then enter and complete a status search.
		sendE2EKey(t, m, keyMsg("f2"))
		sendE2EKey(t, m, keyMsg("/"))
		for _, char := range "file" {
			sendE2EKey(t, m, keyMsg(string(char)))
		}
		sendE2EKey(t, m, keyMsg("enter"))
		if m.searching || len(m.searchMatches) == 0 {
			t.Fatalf("search should finish with matches, got searching=%v matches=%d", m.searching, len(m.searchMatches))
		}

		target := m.tree.Cursor()
		sendE2EKey(t, m, keyMsg("]"))
		sendE2EKey(t, m, keyMsg("v"))
		if m.detailRangeStart < 0 || m.detailRangeEnd < 0 {
			t.Fatalf("line range did not start")
		}
		startLine := m.detailLine

		sendE2EKey(t, m, keyMsg("n"))
		if m.tree.Cursor() != target {
			t.Fatalf("search cursor moved during range extension: %q", m.tree.Cursor())
		}
		if m.detailLine == startLine {
			t.Fatalf("range n did not extend selected lines")
		}
	})
}
