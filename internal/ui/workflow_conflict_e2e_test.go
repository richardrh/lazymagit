package ui

import (
	"context"
	"strings"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
)

func TestConflictE2EInspectResolveAndContinueByMagitKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("conflict.txt", "base\n")
	r.git("add", "--", "conflict.txt")
	r.git("commit", "-m", "base")
	r.git("switch", "-c", "topic")
	r.write("conflict.txt", "theirs\n")
	r.git("add", "--", "conflict.txt")
	r.git("commit", "-m", "topic")
	r.git("switch", "main")
	r.write("conflict.txt", "ours\n")
	r.git("add", "--", "conflict.txt")
	r.git("commit", "-m", "main")
	repo, err := gitbackend.Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeWithArgs(context.Background(), gitbackend.MergeArgs{Target: "topic", Mode: gitbackend.MergePlain}); err == nil {
		t.Fatal("conflicting merge unexpectedly succeeded")
	}

	m := newE2EModel(t, r)
	selectE2EPath(t, m, "conflict.txt", rowUnstaged)
	// e is Magit's ediff-dwim key; for unresolved paths it becomes a bounded,
	// read-only terminal view of the three Git index stages.
	sendE2EKey(t, m, keyMsg("e"))
	for _, want := range []string{"--- base", "base", "--- ours", "ours", "--- theirs", "theirs"} {
		if !strings.Contains(m.detail, want) {
			t.Fatalf("conflict inspection omitted %q:\n%s", want, m.detail)
		}
	}

	// The inspection itself can select a stage and jump directly into the same
	// reviewed in-process resolver. Base remains inspect-only.
	sendE2EKey(t, m, keyMsg("1"))
	if !strings.Contains(m.message, "inspection only") {
		t.Fatalf("base selection message = %q", m.message)
	}
	sendE2EKey(t, m, keyMsg("3"))
	sendE2EKey(t, m, keyMsg("r"))
	resolution := ""
	if m.workflow != nil {
		for _, field := range m.workflow.dialog.Fields {
			if field.Name == "resolution" {
				resolution = field.Value
			}
		}
	}
	if resolution != "theirs" {
		t.Fatalf("direct resolver did not preserve theirs selection: resolution=%q workflow=%#v", resolution, m.workflow)
	}
	submitWorkflowByKeys(t, m, true)
	if got := r.git("show", ":conflict.txt"); got != "theirs" {
		t.Fatalf("resolved staged content = %q", got)
	}
	if got := r.git("status", "--porcelain"); got != "M  conflict.txt" {
		t.Fatalf("resolution status = %q, want staged path", got)
	}

	// The existing merge continuation key is also reviewed and rejects stale
	// prepared indexes before it commits the selected resolution.
	sendE2EKey(t, m, keyMsg("m"))
	sendE2EKey(t, m, keyMsg("m"))
	submitWorkflowByKeys(t, m, true)
	if got := r.git("status", "--porcelain"); got != "" {
		t.Fatalf("merge continue status = %q", got)
	}
	if got := r.git("show", "HEAD:conflict.txt"); got != "theirs" {
		t.Fatalf("merge commit content = %q", got)
	}
}
