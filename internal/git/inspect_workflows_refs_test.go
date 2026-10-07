package git

import (
	"context"
	"testing"
)

func TestIsAncestorAnswersWithoutTrustingTimestamps(t *testing.T) {
	r := newTestRepo(t)
	r.write("story.txt", "one\n")
	first := r.commitAll("first")
	r.write("story.txt", "one\ntwo\n")
	second := r.commitAll("second")
	// A separate root, so this branch shares no ancestry with main at all.
	r.git("switch", "--orphan", "unrelated")
	r.write("other.txt", "other\n")
	unrelated := r.commitAll("unrelated root")

	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatalf("discover repository: %v", err)
	}
	ctx := context.Background()

	for _, test := range []struct {
		name              string
		ancestor, descend string
		want              bool
	}{
		{"a parent is an ancestor of its child", first, second, true},
		{"a commit is its own ancestor", second, second, true},
		{"a child is not an ancestor of its parent", second, first, false},
		{"a separate root shares no ancestry", first, unrelated, false},
	} {
		got, err := repo.IsAncestor(ctx, test.ancestor, test.descend)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got != test.want {
			t.Errorf("%s = %v, want %v", test.name, got, test.want)
		}
	}

	// A revision git cannot resolve is a real failure, not a negative answer:
	// reporting false there would silently drop a marked commit from a range.
	if _, err := repo.IsAncestor(ctx, "no-such-revision", second); err == nil {
		t.Error("an unresolvable revision was reported as a negative answer")
	}
}
