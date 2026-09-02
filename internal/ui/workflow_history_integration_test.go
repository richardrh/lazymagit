package ui

import (
	"context"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestCherryPickWorkflowAppliesMultipleReviewedCommits(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base", "base\n")
	r.git("add", "base")
	r.git("commit", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.git("checkout", "-b", "topic")
	r.write("one", "one\n")
	r.git("add", "one")
	r.git("commit", "-m", "one")
	one := r.git("rev-parse", "HEAD")
	r.write("two", "two\n")
	r.git("add", "two")
	r.git("commit", "-m", "two")
	two := r.git("rev-parse", "HEAD")
	r.git("checkout", "-b", "mainline", base)

	repo, err := gitbackend.Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(repo)
	var id keymap.CommandID
	for _, binding := range keymap.Registry() {
		if binding.Transient == "magit-cherry-pick" && binding.UpstreamCommand == "magit-cherry-copy" {
			id = binding.Command
			break
		}
	}
	cmd, handled := m.performWorkflow(WorkflowCommand{ID: id})
	if !handled {
		t.Fatal("cherry-pick workflow did not open")
	}
	if cmd != nil {
		_, _ = m.Update(cmd())
	}
	if m.workflow == nil || m.workflow.dialog.Fields[0].Kind != WorkflowMultiline {
		t.Fatalf("multi-commit field = %#v", m.workflow)
	}
	values := m.workflowValues()
	values["revision"] = one + "\n" + two
	review, err := m.workflow.dialog.ReviewPreflight(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.workflow.dialog.SubmitReview(context.Background(), values, review); err != nil {
		t.Fatal(err)
	}
	if got := r.git("log", "-2", "--format=%s"); got != "two\none" {
		t.Fatalf("picked subjects = %q", got)
	}
}
