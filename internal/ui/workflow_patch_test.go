package ui

import (
	"strings"
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
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

func TestFormatPatchOptionsMapEverySupportedInfix(t *testing.T) {
	option := func(t *testing.T, upstream string) keymap.CommandID {
		t.Helper()
		for _, binding := range keymap.Registry() {
			if binding.Kind == keymap.KindInfix && binding.UpstreamCommand == upstream {
				return binding.Command
			}
		}
		t.Fatalf("missing format-patch option %q", upstream)
		return ""
	}
	options, err := formatPatchOptions(map[keymap.CommandID]OptionValue{
		option(t, "magit-format-patch:--thread"):                 {Enabled: true},
		option(t, "magit-format-patch:--to"):                     {Value: "dev@example.test, second@example.test"},
		option(t, "magit-format-patch:--cc"):                     {Value: "review@example.test"},
		option(t, "magit-format-patch:--reroll-count"):           {Value: "2"},
		option(t, "magit-format-patch:--subject-prefix"):         {Value: "RFC"},
		option(t, "transient:magit-patch-create:--cover-letter"): {Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Thread || !options.CoverLetter || options.RerollCount != 2 || options.SubjectPrefix != "RFC" || strings.Join(options.To, ",") != "dev@example.test,second@example.test" || strings.Join(options.Cc, ",") != "review@example.test" {
		t.Fatalf("format options = %#v", options)
	}
	if _, err := formatPatchOptions(map[keymap.CommandID]OptionValue{option(t, "magit:--signoff"): {Enabled: true}}); err == nil {
		t.Fatal("unsupported format-patch option was accepted")
	}
	if _, err := formatPatchOptions(map[keymap.CommandID]OptionValue{option(t, "magit-format-patch:--reroll-count"): {Value: "-1"}}); err == nil {
		t.Fatal("negative reroll count was accepted")
	}
}

func TestFormatPatchWorkflowValuesExposeTypedRecipientsAndNumbering(t *testing.T) {
	options, err := formatPatchOptionsFromValues(gitbackend.FormatPatchOptions{}, WorkflowValues{
		"directory": "out", "numbered": "true", "cover": "true", "signoff": "true", "thread": "true", "subject": "RFC", "reroll": "3", "start": "7", "to": "dev@example.test, second@example.test", "cc": "review@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Numbered || !options.CoverLetter || !options.Signoff || !options.Thread || options.SubjectPrefix != "RFC" || options.RerollCount != 3 || options.StartNumber != 7 || strings.Join(options.To, ",") != "dev@example.test,second@example.test" || strings.Join(options.Cc, ",") != "review@example.test" {
		t.Fatalf("format options = %#v", options)
	}
	if _, err := patchNonNegative("reroll count", "-1"); err == nil {
		t.Fatal("negative reroll count was accepted")
	}
	if _, err := patchAddresses("To recipients", "ok@example.test,\x00bad"); err == nil {
		t.Fatal("NUL recipient was accepted")
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
