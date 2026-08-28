package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPatchE2EFormatAndApplyGeneratedSeriesByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("base.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.write("one.txt", "one\n")
	r.git("add", ".")
	r.git("commit", "-m", "one")
	r.write("two.txt", "two\n")
	r.git("add", ".")
	r.git("commit", "-m", "two")

	m := newE2EModel(t, r)
	out := t.TempDir()
	openPatchByKeys(t, m, "W", "c")
	setPatchFieldByKeys(t, m, "range", base+"..HEAD")
	setPatchFieldByKeys(t, m, "directory", out)
	submitPatchWorkflowByKeys(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("format-patch review missing: %q", m.message)
	}
	sendE2EKey(t, m, keyMsg("enter"))
	patches := patchFilesIn(t, out)
	if len(patches) != 2 {
		t.Fatalf("format-patch created %d files: %#v", len(patches), patches)
	}

	r.git("reset", "--hard", base)
	m = newE2EModel(t, r)
	openPatchByKeys(t, m, "W", "a")
	setPatchFieldByKeys(t, m, "path", patches[0])
	submitPatchWorkflowByKeys(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("apply review missing: %q", m.message)
	}
	sendE2EKey(t, m, keyMsg("enter"))
	if m.isError || r.git("rev-parse", "HEAD") != base || r.git("status", "--porcelain", "--", "one.txt") != "?? one.txt" {
		t.Fatalf("plain patch apply changed the wrong state: %q", m.message)
	}

	r.git("reset", "--hard", base)
	if err := os.Remove(filepath.Join(r.dir, "one.txt")); err != nil {
		t.Fatal(err)
	}
	m = newE2EModel(t, r)
	openPatchByKeys(t, m, "w", "w")
	setPatchFieldByKeys(t, m, "paths", strings.Join(patches, "\n"))
	submitPatchWorkflowByKeys(t, m)
	confirmPatchReviewByKeys(t, m)
	if m.isError {
		t.Fatalf("am series failed: %s", m.message)
	}
	if got := r.git("rev-list", "--reverse", "--format=%s", base+"..HEAD"); !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Fatalf("applied subjects:\n%s", got)
	}
	if r.git("show", "HEAD:two.txt") != "two" || r.git("show", "HEAD~1:one.txt") != "one" {
		t.Fatal("applied series did not reproduce the exact files")
	}
	if got := r.git("rev-list", "--count", base+"..HEAD"); got != "2" {
		t.Fatalf("applied commit count = %q", got)
	}
}

func TestPatchE2EAMConflictContinueAndAbortByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("conflict.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	base := r.git("rev-parse", "HEAD")
	r.write("conflict.txt", "from patch\n")
	r.git("commit", "-am", "mail change")
	patchDir := t.TempDir()
	r.git("format-patch", "-o", patchDir, base+"..HEAD")
	patch := patchFilesIn(t, patchDir)[0]
	r.git("reset", "--hard", base)
	r.write("conflict.txt", "local\n")
	r.git("commit", "-am", "local change")
	before := r.git("rev-parse", "HEAD")

	m := newE2EModel(t, r)
	openPatchByKeys(t, m, "w", "w")
	setPatchFieldByKeys(t, m, "paths", patch)
	submitPatchWorkflowByKeys(t, m)
	confirmPatchReviewByKeys(t, m)
	if r.git("rev-parse", "HEAD") != before || !amApplying(t, r) {
		t.Fatalf("conflicting am did not stop at the exact prior commit: %s", m.message)
	}
	r.write("conflict.txt", "resolved\n")
	r.git("add", "--", "conflict.txt")
	openPatchByKeys(t, m, "w", "w")
	if amApplying(t, r) || r.git("log", "-1", "--format=%s") != "mail change" || r.git("show", "HEAD:conflict.txt") != "resolved" {
		t.Fatalf("am continue result: %s", m.message)
	}

	// Recreate the same conflict and verify abort restores both HEAD and files.
	r.git("reset", "--hard", before)
	m = newE2EModel(t, r)
	openPatchByKeys(t, m, "w", "w")
	setPatchFieldByKeys(t, m, "paths", patch)
	submitPatchWorkflowByKeys(t, m)
	confirmPatchReviewByKeys(t, m)
	if !amApplying(t, r) {
		t.Fatal("second conflicting am did not enter progress state")
	}
	openPatchByKeys(t, m, "w", "a")
	if amApplying(t, r) || r.git("rev-parse", "HEAD") != before || r.git("show", "HEAD:conflict.txt") != "local" {
		t.Fatalf("am abort did not restore exact state: %s", m.message)
	}
}

func TestPatchE2ESaveOverwriteStaleAndCancelByKeys(t *testing.T) {
	r := newUIE2ERepo(t)
	r.write("tracked.txt", "base\n")
	r.git("add", ".")
	r.git("commit", "-m", "base")
	r.write("tracked.txt", "working\n")
	out := filepath.Join(r.dir, "saved.patch")
	if err := os.WriteFile(out, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newE2EModel(t, r)

	openSaveOverwriteReviewByKeys(t, m, out)
	if err := os.WriteFile(out, []byte("changed after review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sendE2EKey(t, m, keyMsg("enter"))
	if !m.isError || !strings.Contains(m.message, "stale") || string(mustRead(t, out)) != "changed after review\n" {
		t.Fatalf("stale overwrite result: error=%v message=%q", m.isError, m.message)
	}

	openSaveOverwriteReviewByKeys(t, m, out)
	sendE2EKey(t, m, keyMsg("esc"))
	if string(mustRead(t, out)) != "changed after review\n" || m.mode != modeStatus {
		t.Fatal("cancelled overwrite changed the destination")
	}

	openSaveOverwriteReviewByKeys(t, m, out)
	sendE2EKey(t, m, keyMsg("enter"))
	got := string(mustRead(t, out))
	if m.isError || !strings.Contains(got, "-base") || !strings.Contains(got, "+working") {
		t.Fatalf("reviewed overwrite result: %q\n%s", m.message, got)
	}
}

func openPatchByKeys(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		sendE2EKey(t, m, keyMsg(key))
	}
	if m.mode != modeWorkflow && !amControlKey(keys) {
		t.Fatalf("keys %q did not open patch workflow: mode=%d message=%q", keys, m.mode, m.message)
	}
}

func amControlKey(keys []string) bool {
	return len(keys) == 2 && keys[0] == "w" && (keys[1] == "w" || keys[1] == "s" || keys[1] == "a")
}

func setPatchFieldByKeys(t *testing.T, m *Model, name, value string) {
	t.Helper()
	if m.workflow == nil {
		t.Fatal("patch workflow is not open")
	}
	index := -1
	for i := range m.workflow.dialog.Fields {
		if m.workflow.dialog.Fields[i].Name == name {
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("field %q is missing", name)
	}
	for m.workflow.field > index {
		sendE2EKey(t, m, keyMsg("up"))
	}
	for m.workflow.field < index {
		sendE2EKey(t, m, keyMsg("tab"))
	}
	for m.workflow.dialog.Fields[index].Value != "" {
		sendE2EKey(t, m, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace}))
	}
	// A terminal paste arrives as one key message and preserves embedded newlines.
	sendE2EKey(t, m, keyMsg(value))
}

func confirmPatchReviewByKeys(t *testing.T, m *Model) {
	t.Helper()
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("patch review missing: %q", m.message)
	}
	sendE2EKey(t, m, keyMsg("enter"))
}

func submitPatchWorkflowByKeys(t *testing.T, m *Model) {
	t.Helper()
	for m.workflow != nil && m.workflow.field < len(m.workflow.dialog.Fields) {
		sendE2EKey(t, m, keyMsg("tab"))
	}
	sendE2EKey(t, m, keyMsg("enter"))
}

func openSaveOverwriteReviewByKeys(t *testing.T, m *Model, output string) {
	t.Helper()
	openPatchByKeys(t, m, "W", "s")
	setPatchFieldByKeys(t, m, "path", output)
	for m.workflow.dialog.Fields[m.workflow.field].Name != "overwrite" {
		sendE2EKey(t, m, keyMsg("tab"))
	}
	sendE2EKey(t, m, keyMsg("space"))
	submitPatchWorkflowByKeys(t, m)
	if m.workflow == nil || m.workflow.review == nil {
		t.Fatalf("overwrite review missing: %q", m.message)
	}
}

func patchFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".patch") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func amApplying(t *testing.T, r *uiE2ERepo) bool {
	t.Helper()
	gitDir := r.git("rev-parse", "--git-dir")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(r.dir, gitDir)
	}
	_, err := os.Stat(filepath.Join(gitDir, "rebase-apply", "applying"))
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return false
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
