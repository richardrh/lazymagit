package git

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFormatPatchArgsDirectOrderingAndDefaults(t *testing.T) {
	if got, want := formatPatchArgs("out", "HEAD", FormatPatchOptions{}), []string{"format-patch", "--quiet", "--output-directory=out", "HEAD"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("formatPatchArgs defaults = %#v, want %#v", got, want)
	}

	options := FormatPatchOptions{
		Numbered: true, CoverLetter: true, Signoff: true, Thread: true, RFC: true,
		SubjectPrefix: "PATCH", RerollCount: 2, StartNumber: 3,
		From: "from@example.test", InReplyTo: "<id@example.test>", Base: "HEAD~1",
		To: []string{"one@example.test", "two@example.test"}, Cc: []string{"copy@example.test"},
	}
	want := []string{
		"format-patch", "--quiet", "--output-directory=out", "--numbered", "--cover-letter", "--signoff", "--thread", "--rfc",
		"--subject-prefix=PATCH", "--reroll-count=2", "--start-number=3", "--from=from@example.test", "--in-reply-to=<id@example.test>", "--base=HEAD~1",
		"--to=one@example.test", "--to=two@example.test", "--cc=copy@example.test", "HEAD~1..HEAD",
	}
	if got := formatPatchArgs("out", "HEAD~1..HEAD", options); !reflect.DeepEqual(got, want) {
		t.Fatalf("formatPatchArgs all options = %#v, want %#v", got, want)
	}
}

func TestReplaceFormatPatchCoverLetterDirectCases(t *testing.T) {
	t.Run("replaces placeholder and preserves permissions", func(t *testing.T) {
		dir := t.TempDir()
		cover := filepath.Join(dir, "0000-cover-letter.patch")
		original := "Subject: [PATCH 0/1] cover\n\n*** BLURB HERE ***\n-- \nfooter\n"
		if err := os.WriteFile(cover, []byte(original), 0o640); err != nil {
			t.Fatal(err)
		}
		body := "Summary\n\nDetails"
		if err := replaceFormatPatchCoverLetter([]string{cover}, body); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(cover)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "*** BLURB HERE ***") || !strings.Contains(string(data), body) {
			t.Fatalf("cover letter contents = %q", data)
		}
		info, err := os.Stat(cover)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o640 {
			t.Fatalf("cover letter mode = %o, want 640", info.Mode().Perm())
		}
	})

	t.Run("requires exactly one editable cover", func(t *testing.T) {
		dir := t.TempDir()
		plain := filepath.Join(dir, "0001.patch")
		first := filepath.Join(dir, "0000-a.patch")
		second := filepath.Join(dir, "0000-b.patch")
		if err := os.WriteFile(plain, []byte("no placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := replaceFormatPatchCoverLetter([]string{plain}, "body"); err == nil {
			t.Fatal("replaceFormatPatchCoverLetter accepted no editable cover")
		}
		for _, path := range []string{first, second} {
			if err := os.WriteFile(path, []byte("*** BLURB HERE ***"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := replaceFormatPatchCoverLetter([]string{first, second}, "body"); err == nil {
			t.Fatal("replaceFormatPatchCoverLetter accepted multiple covers")
		}
	})

	t.Run("reports unreadable path", func(t *testing.T) {
		if err := replaceFormatPatchCoverLetter([]string{filepath.Join(t.TempDir(), "missing.patch")}, "body"); err == nil {
			t.Fatal("replaceFormatPatchCoverLetter accepted a missing path")
		}
	})
}
