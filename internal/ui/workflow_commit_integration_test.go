package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
)

func TestCommitWorkflowNineVariantsIntegration(t *testing.T) {
	for _, spec := range commitWorkflowSpecs {
		spec := spec
		t.Run(string(spec.variant), func(t *testing.T) {
			r := newUIE2ERepo(t)
			r.write("base", "base\n")
			r.git("add", "--", "base")
			r.git("commit", "-m", "base subject")
			base := r.git("rev-parse", "HEAD")
			r.write("staged", string(spec.variant)+"\n")
			r.git("add", "--", "staged")

			repo, err := gitbackend.Discover(r.dir)
			if err != nil {
				t.Fatal(err)
			}
			m := New(repo)
			id, _ := commitCommandID(spec.upstream)
			load, handled := m.performWorkflow(WorkflowCommand{ID: id})
			if !handled || load == nil {
				t.Fatalf("handler did not asynchronously load: handled=%v cmd=%v", handled, load != nil)
			}
			_, _ = m.Update(load())
			if m.workflow == nil {
				t.Fatalf("dialog load failed: %s", m.message)
			}
			values := m.workflowValues()
			if spec.target {
				values[commitTargetField] = base
			}
			if spec.message {
				values[commitMessageField] = "message for " + string(spec.variant)
			}
			if err := m.workflow.dialog.Submit(context.Background(), values); err != nil {
				t.Fatal(err)
			}
			if head := r.git("rev-parse", "HEAD"); head == base {
				t.Fatalf("%s did not create or rewrite a commit", spec.variant)
			}
			if spec.variant == gitbackend.CommitUIReword || spec.variant == gitbackend.CommitUIRevise {
				if got := r.git("diff", "--cached", "--name-only"); got != "staged" {
					t.Fatalf("%s consumed staged index: %q", spec.variant, got)
				}
			}
			if strings.Contains(fmt.Sprint(m.processBatches), "message for "+string(spec.variant)) {
				t.Fatal("commit message leaked into UI process history")
			}
		})
	}
}
