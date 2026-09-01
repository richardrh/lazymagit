package git

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestQueryBlameUsesLiteralPathAndReturnsLineProvenance(t *testing.T) {
	ctx := context.Background()
	r := newTestRepo(t)
	r.write("-story.txt", "one\ntwo\n")
	first := r.commitAll("first story")
	r.write("-story.txt", "one\nchanged\n")
	second := r.commitAll("second story")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := repo.QueryBlame(ctx, BlameQuery{Path: "-story.txt", OutputLimit: 64 << 10})
	if err != nil {
		t.Fatalf("QueryBlame: %v", err)
	}
	if result.Truncated || len(result.Lines) != 2 {
		t.Fatalf("blame result = %#v", result)
	}
	if result.Lines[0].Line != 1 || result.Lines[0].CommitID != first || result.Lines[0].Content != "one" || result.Lines[0].Summary != "first story" {
		t.Fatalf("first provenance = %#v", result.Lines[0])
	}
	if result.Lines[1].Line != 2 || result.Lines[1].CommitID != second || result.Lines[1].Content != "changed" || result.Lines[1].AuthorMail != "backend-test@example.invalid" {
		t.Fatalf("second provenance = %#v", result.Lines[1])
	}
	if _, err := repo.QueryBlame(ctx, BlameQuery{Path: "../outside"}); err == nil {
		t.Fatal("blame accepted a path outside the worktree")
	}
}

func TestParseBlamePorcelainRejectsMalformedAndReportsPartialRecord(t *testing.T) {
	oid := strings.Repeat("a", 40)
	partial := []byte(oid + " 1 7 1\nauthor Example\n")
	lines, truncated, err := parseBlamePorcelain(partial)
	if err != nil || len(lines) != 0 || !truncated {
		t.Fatalf("partial porcelain = %#v, %v, %v", lines, truncated, err)
	}
	if _, _, err := parseBlamePorcelain([]byte("not porcelain\n")); err == nil {
		t.Fatal("malformed blame porcelain was accepted")
	}
}

func TestParseBlamePorcelainDirectlyParsesMetadataAndBoundaryCases(t *testing.T) {
	oid := strings.Repeat("b", 40)
	out := []byte(oid + " 3 9 1\n" +
		"author Ada Lovelace\n" +
		"author-mail <ada@example.invalid>\n" +
		"author-time 123\n" +
		"summary Analytical engine\n" +
		"future-boolean-field\n" +
		"\tcontent\n")
	lines, incomplete, err := parseBlamePorcelain(out)
	if err != nil || incomplete || len(lines) != 1 {
		t.Fatalf("parsed porcelain = %#v, incomplete %v, err %v", lines, incomplete, err)
	}
	wantTime := time.Unix(123, 0).UTC()
	if got := lines[0]; got.Line != 9 || got.CommitID != oid || got.Author != "Ada Lovelace" || got.AuthorMail != "ada@example.invalid" || !got.AuthorTime.Equal(wantTime) || got.Summary != "Analytical engine" || got.Content != "content" {
		t.Fatalf("line = %#v", got)
	}

	bad := [][]byte{
		[]byte("\tcontent\n"),
		[]byte(oid + " 1 0 1\n"),
		[]byte(oid + " 1 1 1\nauthor-time nope\n"),
	}
	for _, input := range bad {
		if _, _, err := parseBlamePorcelain(input); err == nil {
			t.Errorf("parseBlamePorcelain(%q) succeeded", input)
		}
	}

	doubleHeader := []byte(oid + " 1 1 1\n" + oid + " 2 2 1\n")
	lines, incomplete, err = parseBlamePorcelain(doubleHeader)
	if err != nil || !incomplete || len(lines) != 0 {
		t.Fatalf("double header = %#v, incomplete %v, err %v", lines, incomplete, err)
	}
}
