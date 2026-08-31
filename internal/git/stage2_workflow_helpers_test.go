package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStage2FormatPatchValidationHelpers(t *testing.T) {
	if err := validateFormatPatchScalars(FormatPatchOptions{ThreadStyle: "deep"}); err != nil {
		t.Fatal(err)
	}
	if err := validateFormatPatchScalars(FormatPatchOptions{RerollCount: -1}); err == nil {
		t.Fatal("negative count accepted")
	}
	if err := validateFormatPatchReplyAndBase(FormatPatchOptions{InReplyTo: "bad"}); err == nil {
		t.Fatal("bad message ID accepted")
	}
	if err := validateFormatPatchCoverBody("ok\n"); err != nil {
		t.Fatal(err)
	}
	if err := validateFormatPatchCoverBody("bad\x00"); err == nil {
		t.Fatal("control accepted")
	}
}

func TestStage2CreateTagHelpers(t *testing.T) {
	in, err := normalizeCreateTagArgs(TagCreatePreflight{}, CreateTagArgs{Name: "v1", LocalUser: "key", Message: "release"})
	if err != nil || !in.Sign || !in.Annotated || in.Target != "HEAD" {
		t.Fatalf("normalize = %#v, %v", in, err)
	}
	if _, err := normalizeCreateTagArgs(TagCreatePreflight{Exists: true}, CreateTagArgs{}); err == nil {
		t.Fatal("replacement accepted")
	}
	got := createTagCommandArgs(in, "abc")
	want := []string{"tag", "--local-user=key", "--sign", "--file=-", "--", "v1", "abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestStage2RepositoryManagementHelpers(t *testing.T) {
	if got := worktreeMoveArgs("a", "b", Confirmed, true); strings.Join(got, " ") != "worktree move --force --force -- a b" {
		t.Fatalf("move args = %q", got)
	}
	if err := validateSubmoduleUpdateOptions(SubmoduleUpdateOptions{Merge: true, Rebase: true}); err == nil {
		t.Fatal("conflicting modes accepted")
	}
	args := submoduleUpdateArgs(SubmoduleUpdateOptions{Init: true, Recursive: true, Depth: 2, Jobs: 3, Force: Confirmed})
	if strings.Join(args, " ") != "submodule update --init --recursive --force --depth 2 --jobs 3" {
		t.Fatalf("update args = %q", args)
	}
	modules := []Submodule{{Name: "module-name", Path: "vendor/module"}, {Name: "unsafe", Path: "../bad"}}
	if got := configuredSubmoduleName(modules, filepath.FromSlash("vendor/module")); got != "module-name" {
		t.Fatalf("module = %q", got)
	}
	if boolCount(true, false, true) != 2 {
		t.Fatal("boolCount")
	}
}

func TestStage2DiffAndHistoryHelpers(t *testing.T) {
	if got, err := diffNoRevisions(DiffQuery{}, []string{"--cached"}, "bad"); err != nil || !reflect.DeepEqual(got, []string{"--cached"}) {
		t.Fatalf("diff = %q, %v", got, err)
	}
	if _, err := diffNoRevisions(DiffQuery{Base: "HEAD"}, nil, "bad"); err == nil {
		t.Fatal("revision accepted")
	}
	if err := validateHistoryApply("revert", []string{"a"}, PickOptions{FastForward: true}); err == nil {
		t.Fatal("revert ff accepted")
	}
	args := historyApplyArgs("revert", []string{"abc"}, PickOptions{Signoff: true})
	if strings.Join(args, " ") != "revert --signoff --no-edit -- abc" {
		t.Fatalf("history args = %q", args)
	}
}

func TestStage2UnbornDefaultLogHelper(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	repo := &Repository{commandDir: dir}
	_, logErr := repo.output(context.Background(), "log")
	if !repo.isUnbornDefaultLog(context.Background(), LogQuery{}, logErr) {
		t.Fatal("unborn default log not recognized")
	}
	if repo.isUnbornDefaultLog(context.Background(), LogQuery{Revision: "HEAD"}, logErr) {
		t.Fatal("explicit revision treated as default log")
	}
}

func TestStage2ReviewStagedFormatPatchHelpers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "0001-topic.patch")
	if err := os.WriteFile(path, []byte("patch"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := reviewStagedFormatPatchFile(path, nil)
	if err != nil || file.Name != filepath.Base(path) || file.Size != 5 {
		t.Fatalf("file = %#v, %v", file, err)
	}
	if _, err := reviewStagedFormatPatchFiles([]string{path}, []string{filepath.Base(path)}); err == nil {
		t.Fatal("existing output accepted")
	}
}
