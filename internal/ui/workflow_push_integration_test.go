package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestPushWorkflowIntegrationBareRemoteTargetsAndSelectors(t *testing.T) {
	local, repo, bare := newPushWorkflowE2E(t)
	ctx := context.Background()

	// A missing pushRemote is selected, reviewed, and only then persisted.
	d, err := loadPushDialog(ctx, repo, pushCurrentRemote, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Fields) == 0 || d.Fields[0].Name != "remote" {
		t.Fatalf("missing push remote did not produce chooser: %#v", d.Fields)
	}
	review := reviewPushDialog(t, d, nil)
	if got := pushConfigGet(local.dir, "branch.main.pushRemote"); got != "" {
		t.Fatalf("review mutated pushRemote: %q", got)
	}
	if err := d.SubmitReview(ctx, valuesForDialog(d, nil), review); err != nil {
		t.Fatalf("configure and push: %v", err)
	}
	if got := local.git("config", "--get", "branch.main.pushRemote"); got != "origin" {
		t.Fatalf("persisted pushRemote = %q", got)
	}
	assertPushRef(t, bare, "refs/heads/main", local.git("rev-parse", "main"))

	local.git("config", "branch.main.remote", "origin")
	local.git("config", "branch.main.merge", "refs/heads/upstream")
	local.git("branch", "other")
	local.git("branch", "matching")
	local.git("push", "origin", "matching")
	local.git("tag", "one")
	local.git("tag", "two")
	local.git("notes", "--ref=review", "add", "-m", "reviewed", "HEAD")

	tests := []struct {
		name string
		kind pushWorkflowKind
		set  map[string]string
		want string
	}{
		{"current pushremote", pushCurrentRemote, nil, "refs/heads/main"},
		{"upstream", pushCurrentUpstream, nil, "refs/heads/upstream"},
		{"elsewhere", pushCurrentElsewhere, nil, "refs/heads/main"},
		{"arbitrary branch", pushAnotherBranch, map[string]string{"source": "other", "destination": "arbitrary"}, "refs/heads/arbitrary"},
		{"multiple refspecs", pushExplicitRefspecs, map[string]string{"refspecs": "main:explicit-one other:explicit-two"}, "refs/heads/explicit-two"},
		{"matching", pushMatchingBranches, nil, "refs/heads/matching"},
		{"one tag", pushOneTag, map[string]string{"tag": "one"}, "refs/tags/one"},
		{"all tags", pushAllTags, nil, "refs/tags/two"},
		{"one notes ref", pushOneNotesRef, map[string]string{"notes": "refs/notes/review"}, "refs/notes/review"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d, err := loadPushDialog(ctx, repo, test.kind, nil)
			if err != nil {
				t.Fatal(err)
			}
			values := valuesForDialog(d, test.set)
			review := reviewPushDialog(t, d, test.set)
			if err := d.SubmitReview(ctx, values, review); err != nil {
				t.Fatalf("submit: %v", err)
			}
			if !pushRefExists(bare, test.want) {
				t.Fatalf("remote ref %s does not exist", test.want)
			}
		})
	}
}

func TestPushWorkflowIntegrationDryRunLeaseReviewAndCancellation(t *testing.T) {
	local, repo, bare := newPushWorkflowE2E(t)
	ctx := context.Background()
	local.git("config", "branch.main.pushRemote", "origin")

	dryOptions := map[keymap.CommandID]OptionValue{"push.--dry-run": {Enabled: true}}
	d, err := loadPushDialog(ctx, repo, pushAnotherBranch, dryOptions)
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]string{"destination": "dry-only"}
	review := reviewPushDialog(t, d, set)
	if err := d.SubmitReview(ctx, valuesForDialog(d, set), review); err != nil {
		t.Fatal(err)
	}
	if pushRefExists(bare, "refs/heads/dry-only") {
		t.Fatal("dry-run workflow mutated remote")
	}

	forceOptions := map[keymap.CommandID]OptionValue{"push.--force": {Enabled: true}}
	d, err = loadPushDialog(ctx, repo, pushAnotherBranch, forceOptions)
	if err != nil {
		t.Fatal(err)
	}
	set = map[string]string{"destination": "force-reviewed"}
	applyDialogValues(&d, set)
	m := New(repo)
	m.loading = false
	m.OpenWorkflow(d)
	m.workflow.field = len(d.Fields)
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("first Enter did not start review")
	}
	_, _ = m.Update(cmd())
	if pushRefExists(bare, "refs/heads/force-reviewed") || m.workflow == nil || m.workflow.review == nil {
		t.Fatal("first Enter mutated remote or failed to retain reviewed plan")
	}
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if pushRefExists(bare, "refs/heads/force-reviewed") {
		t.Fatal("cancelling reviewed force push mutated remote")
	}

	leaseOptions := map[keymap.CommandID]OptionValue{"push.--force-with-lease": {Enabled: true}}
	d, err = loadPushDialog(ctx, repo, pushAnotherBranch, leaseOptions)
	if err != nil {
		t.Fatal(err)
	}
	set = map[string]string{"destination": "leased"}
	review = reviewPushDialog(t, d, set)
	if !strings.Contains(strings.Join(review.Plan, "\n"), "protected") {
		t.Fatalf("lease review is not visually distinct: %#v", review.Plan)
	}
	if err := d.SubmitReview(ctx, valuesForDialog(d, set), review); err != nil {
		t.Fatal(err)
	}
	if !pushRefExists(bare, "refs/heads/leased") {
		t.Fatal("lease workflow did not push")
	}
}

func reviewPushDialog(t *testing.T, d WorkflowDialog, set map[string]string) WorkflowReview {
	t.Helper()
	values := valuesForDialog(d, set)
	if err := validateWorkflow(d, values); err != nil {
		t.Fatalf("validate dialog: %v", err)
	}
	review, err := d.ReviewPreflight(context.Background(), values)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	return review
}

func valuesForDialog(d WorkflowDialog, set map[string]string) WorkflowValues {
	values := WorkflowValues{}
	for _, field := range d.Fields {
		values[field.Name] = field.Value
	}
	for name, value := range set {
		values[name] = value
	}
	return values
}

func applyDialogValues(d *WorkflowDialog, set map[string]string) {
	for i := range d.Fields {
		if value, ok := set[d.Fields[i].Name]; ok {
			d.Fields[i].Value = value
		}
	}
}

func newPushWorkflowE2E(t *testing.T) (*uiE2ERepo, *gitbackend.Repository, string) {
	t.Helper()
	local := newUIE2ERepo(t)
	local.write("base", "base\n")
	local.git("add", "base")
	local.git("commit", "-m", "base")
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v: %s", err, out)
	}
	local.git("remote", "add", "origin", bare)
	repo, err := gitbackend.Discover(local.dir)
	if err != nil {
		t.Fatal(err)
	}
	return local, repo, bare
}

func pushRefExists(dir, ref string) bool {
	cmd := exec.Command("git", "-C", dir, "show-ref", "--verify", "--quiet", ref)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	return cmd.Run() == nil
}

func pushConfigGet(dir, key string) string {
	out, _ := exec.Command("git", "-C", dir, "config", "--get", key).Output()
	return strings.TrimSpace(string(out))
}

func assertPushRef(t *testing.T, dir, ref, want string) {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != want {
		t.Fatalf("%s = %q, %v; want %q", ref, out, err, want)
	}
}
