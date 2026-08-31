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

func TestFormatPatchTransientOnlyExposesSeriesChangingOptions(t *testing.T) {
	available := map[string]bool{}
	for _, upstream := range []string{
		"magit-format-patch:--in-reply-to", "magit-format-patch:--thread", "magit-format-patch:--from", "magit-format-patch:--to", "magit-format-patch:--cc", "magit-format-patch:--base", "magit-format-patch:--reroll-count", "magit-format-patch:--subject-prefix", "transient:magit-patch-create:--rfc", "transient:magit-patch-create:--cover-letter", "magit-format-patch:--output-directory",
	} {
		available[upstream] = len(keymap.OptionConsumerCommands("W c", upstream)) > 0
	}
	for upstream, enabled := range available {
		if !enabled {
			t.Errorf("series-changing option %s was not exposed", upstream)
		}
	}
	for _, upstream := range []string{"magit-format-patch:--interdiff", "magit-format-patch:--range-diff", "magit-format-patch:--notes", "magit-format-patch:--cover-from-description", "magit-diff:--diff-algorithm"} {
		if got := keymap.OptionConsumerCommands("W c", upstream); len(got) != 0 {
			t.Errorf("unsupported option %s was exposed to a format-patch workflow: %v", upstream, got)
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
		option(t, "magit-format-patch:--in-reply-to"):            {Value: "<series@example.test>"},
		option(t, "magit-format-patch:--thread"):                 {Value: "deep"},
		option(t, "magit-format-patch:--from"):                   {Value: "Author <author@example.test>"},
		option(t, "magit-format-patch:--to"):                     {Value: "dev@example.test, second@example.test"},
		option(t, "magit-format-patch:--cc"):                     {Value: "review@example.test"},
		option(t, "magit-format-patch:--base"):                   {Value: "HEAD~2"},
		option(t, "magit-format-patch:--reroll-count"):           {Value: "2"},
		option(t, "magit-format-patch:--subject-prefix"):         {Value: "RFC"},
		option(t, "transient:magit-patch-create:--rfc"):          {Enabled: true},
		option(t, "transient:magit-patch-create:--cover-letter"): {Enabled: true},
		option(t, "magit-format-patch:--output-directory"):       {Value: "out"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Thread || options.ThreadStyle != "deep" || !options.CoverLetter || !options.RFC || options.OutputDirectory != "out" || options.From != "Author <author@example.test>" || options.InReplyTo != "<series@example.test>" || options.Base != "HEAD~2" || options.RerollCount != 2 || options.SubjectPrefix != "RFC" || strings.Join(options.To, ",") != "dev@example.test,second@example.test" || strings.Join(options.Cc, ",") != "review@example.test" {
		t.Fatalf("format options = %#v", options)
	}
	if _, err := formatPatchOptions(map[keymap.CommandID]OptionValue{option(t, "magit:--signoff"): {Enabled: true}}); err == nil {
		t.Fatal("unsupported format-patch option was accepted")
	}
	if _, err := formatPatchOptions(map[keymap.CommandID]OptionValue{option(t, "magit-format-patch:--reroll-count"): {Value: "-1"}}); err == nil {
		t.Fatal("negative reroll count was accepted")
	}
}

func TestAMOptionsAndExtractedHelper(t *testing.T) {
	option := func(upstream string) keymap.CommandID {
		for _, binding := range keymap.Registry() {
			if binding.Kind == keymap.KindInfix && binding.UpstreamCommand == upstream {
				return binding.Command
			}
		}
		t.Fatalf("missing AM option %q", upstream)
		return ""
	}
	got, err := amOptions(map[keymap.CommandID]OptionValue{
		option("transient:magit-am:--3way"):     {Enabled: true},
		option("transient:magit-am:--scissors"): {Enabled: true},
		option("magit:--signoff"):               {Enabled: true},
		"unrelated":                             {Enabled: true},
	})
	if err != nil || !got.ThreeWay || !got.Scissors || !got.Signoff {
		t.Fatalf("am options = %+v, %v", got, err)
	}
	var direct gitbackend.AMOptions
	if err := applyAMOption(&direct, "unsupported"); err == nil {
		t.Fatal("unsupported AM option accepted")
	}
}

func TestFormatPatchSettersRejectInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		set   formatPatchOptionSetter
		value string
	}{
		{"message id", setFormatPatchInReplyTo, "bad"},
		{"thread", setFormatPatchThread, "sideways"},
		{"from", setFormatPatchFrom, "bad"},
		{"to", setFormatPatchTo, "bad"},
		{"cc", setFormatPatchCc, "bad"},
		{"base", setFormatPatchBase, "bad\x00base"},
		{"reroll", setFormatPatchRerollCount, "-1"},
		{"subject", setFormatPatchSubjectPrefix, "bad\nsubject"},
		{"directory", setFormatPatchOutputDirectory, "bad\x00path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.set(&gitbackend.FormatPatchOptions{}, OptionValue{Value: tt.value}); err == nil {
				t.Fatal("invalid value accepted")
			}
		})
	}
}

func TestFormatPatchWorkflowValuesExposeTypedRecipientsAndNumbering(t *testing.T) {
	options, err := formatPatchOptionsFromValues(gitbackend.FormatPatchOptions{}, WorkflowValues{
		"directory": "out", "numbered": "true", "cover": "true", "cover-message": "series overview\n\nChanges since v2", "signoff": "true", "thread": "true", "subject": "RFC", "reroll": "3", "start": "7", "from": "Author <author@example.test>", "in-reply-to": "<series@example.test>", "base": "HEAD~2", "to": "dev@example.test, second@example.test", "cc": "review@example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Numbered || !options.CoverLetter || !options.Signoff || !options.Thread || options.SubjectPrefix != "RFC" || options.CoverLetterBody != "series overview\n\nChanges since v2" || options.From != "Author <author@example.test>" || options.InReplyTo != "<series@example.test>" || options.Base != "HEAD~2" || options.RerollCount != 3 || options.StartNumber != 7 || strings.Join(options.To, ",") != "dev@example.test,second@example.test" || strings.Join(options.Cc, ",") != "review@example.test" {
		t.Fatalf("format options = %#v", options)
	}
	if _, err := patchNonNegative("reroll count", "-1"); err == nil {
		t.Fatal("negative reroll count was accepted")
	}
	if _, err := patchAddresses("To recipients", "\"Display, Name\" <named@example.test>, ok@example.test"); err != nil {
		t.Fatalf("quoted recipient list rejected: %v", err)
	}
	for _, value := range []string{"ok@example.test,\x00bad", "ok@example.test\r\nBcc: injected@example.test"} {
		if _, err := patchAddresses("To recipients", value); err == nil {
			t.Fatalf("unsafe recipient was accepted: %q", value)
		}
	}
	if err := patchMessageID("bad@example.test"); err == nil {
		t.Fatal("bare In-Reply-To was accepted")
	}
	if err := patchCoverMessage(strings.Repeat("x", patchInputLimit+1)); err == nil {
		t.Fatal("oversized cover message was accepted")
	}
}

func TestWorkflowMultilineEditorUsesEnterForNewlines(t *testing.T) {
	field := WorkflowField{Kind: WorkflowMultiline, Value: "first"}
	if !editWorkflowMultiline(&field, "enter", "") || field.Value != "first\n" {
		t.Fatalf("multiline enter = %q", field.Value)
	}
	if !editWorkflowMultiline(&field, "x", "second") || field.Value != "first\nsecond" {
		t.Fatalf("multiline text = %q", field.Value)
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
