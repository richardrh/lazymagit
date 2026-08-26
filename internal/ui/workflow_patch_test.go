package ui

import (
	"strings"
	"testing"

	"github.com/richard/lazymagit/internal/keymap"
)

func TestPatchDomainRegistersOnlyExecutablePatchSuffixes(t *testing.T) {
	m := &Model{}
	handlers := workflowHandlersFor(m)
	want := []keymap.CommandID{amApplyMaildirID, amApplyPatchesID, amContinueID, amSkipID, amAbortID, patchApplyID, patchCreateID, patchSaveID}
	for _, id := range want {
		if handlers[id] == nil {
			t.Errorf("handler %s is not registered", id)
		}
	}
	for id := range handlers {
		if strings.Contains(string(id), "request-pull") || strings.Contains(string(id), "reject") || strings.Contains(string(id), "gpg-sign") {
			t.Errorf("unsafe/unsupported patch command was registered: %s", id)
		}
	}
}

func TestPatchPathsAreLiteralBoundedLines(t *testing.T) {
	paths, err := patchPaths(" series/0001 one.patch \r\n-series.patch\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "series/0001 one.patch" || paths[1] != "-series.patch" {
		t.Fatalf("paths = %#v", paths)
	}
	tooMany := strings.Repeat("x\n", patchFileLimit+1)
	if _, err := patchPaths(tooMany); err == nil {
		t.Fatal("oversized patch path list was accepted")
	}
	if _, err := patchPaths("bad\x00path"); err == nil {
		t.Fatal("NUL path was accepted")
	}
}
