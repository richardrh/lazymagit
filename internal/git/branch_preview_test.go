package git

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewBranchSwitchSeparatesDivergenceUpstreamAndDirtyConflicts(t *testing.T) {
	r := newTestRepo(t)
	r.write("shared.txt", "base\n")
	r.commitAll("base")
	base := r.git("rev-parse", "HEAD")
	r.git("branch", "target", base)
	r.git("checkout", "target")
	r.write("shared.txt", "target\n")
	r.commitAll("target change")
	r.write("target-extra.txt", "extra\n")
	r.commitAll("target extra")
	target := r.git("rev-parse", "HEAD")
	r.git("config", "branch.target.remote", "origin")
	r.git("config", "branch.target.merge", "refs/heads/missing")
	r.git("checkout", "main")
	r.write("shared.txt", "main\n")
	r.commitAll("main change")
	r.write("shared.txt", "local overlap\n")
	r.write("unrelated.txt", "local only\n")
	r.git("config", "branch.target.description", "target review")
	r.git("add", "unrelated.txt")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewBranchSwitch(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if preview.CurrentAhead != 1 || preview.TargetAhead != 2 {
		t.Fatalf("divergence = current +%d target +%d", preview.CurrentAhead, preview.TargetAhead)
	}
	if !preview.UpstreamUnavailable || preview.HasUpstream {
		t.Fatalf("missing upstream = has=%v unavailable=%v", preview.HasUpstream, preview.UpstreamUnavailable)
	}
	if len(preview.PotentialBlockingPaths) != 1 || preview.PotentialBlockingPaths[0] != "shared.txt" {
		t.Fatalf("dirty conflict paths = %#v", preview.PotentialBlockingPaths)
	}
	hasJoin := false
	for _, line := range preview.GraphLines {
		if strings.TrimSpace(line) == "|/" {
			hasJoin = true
			break
		}
	}
	if !hasJoin {
		t.Fatalf("graph lost diverged join connector: %#v", preview.GraphLines)
	}
	if preview.TargetID != target || preview.BaseID != base || preview.ChangeSummary.Files != 2 || preview.ChangeSummary.Insertions != 2 || !preview.Description.Set || preview.Description.Value != "target review" {
		t.Fatalf("target/base summary = %#v", preview)
	}
}

func TestPreviewBranchSwitchReportsNoCommonAncestor(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("checkout", "--orphan", "unrelated")
	r.write("unrelated.txt", "unrelated\n")
	r.git("add", ".")
	r.git("commit", "-m", "unrelated root")
	r.git("checkout", "main")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewBranchSwitch(context.Background(), "unrelated")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.NoCommonAncestor || preview.BaseID != "" {
		t.Fatalf("no-base preview = noBase=%v base=%q", preview.NoCommonAncestor, preview.BaseID)
	}
	if preview.Latest.Subject != "unrelated root" {
		t.Fatalf("latest target subject = %q", preview.Latest.Subject)
	}
}

func TestPathTreeCollisionRecognizesFileAndDirectoryConflicts(t *testing.T) {
	targetFiles := map[string]bool{
		"docs/readme.md": true,
		"src":            true,
	}
	tests := []struct {
		path string
		want bool
	}{
		{path: "docs/readme.md", want: true},
		{path: "docs", want: true},
		{path: "src", want: true},
		{path: "src/main.go", want: false},
		{path: "test", want: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if got := pathTreeCollision(test.path, targetFiles); got != test.want {
				t.Fatalf("pathTreeCollision(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func TestPreviewBranchSwitchAcceptsRevisionTargets(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repo.PreviewBranchSwitch(context.Background(), "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Target != "HEAD" || preview.TargetID == "" || preview.CurrentID == "" {
		t.Fatalf("revision target preview = %#v", preview)
	}
	if _, err := repo.PreviewBranchSwitch(context.Background(), ""); err == nil {
		t.Fatal("empty branch target unexpectedly succeeded")
	}
}
