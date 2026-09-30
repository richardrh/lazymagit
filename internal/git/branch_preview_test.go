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
	if preview.TargetID != target || preview.BaseID != base || preview.ChangeSummary.Files != 2 || preview.ChangeSummary.Insertions != 2 {
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
