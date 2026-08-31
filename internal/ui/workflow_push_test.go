package ui

import (
	"reflect"
	"strings"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestPushDomainRegistersEverySuffix(t *testing.T) {
	m := New(nil)
	ids := []keymap.CommandID{keymap.CommandPush, pushUpstreamID, pushElsewhereID, pushOtherID, pushRefspecsID, pushMatchingID, pushTagID, pushTagsID, pushNotesID, pushConfigureID}
	for _, id := range ids {
		if m.workflowHandlers[id] == nil {
			t.Errorf("push handler %q is not registered", id)
		}
	}
	for _, binding := range keymap.BindingsFor(keymap.SchemeMagit, keymap.ContextTransient+"P") {
		if binding.Kind == keymap.KindSuffix && strings.Contains("pu eormTtnC", strings.Join(binding.Sequence[1:], "")) && m.workflowHandlers[binding.Command] == nil {
			t.Errorf("suffix %v (%s) is disconnected", binding.Sequence, binding.Command)
		}
	}
}

func TestPushOptionsMapLosslessly(t *testing.T) {
	options := map[keymap.CommandID]OptionValue{
		"push.--force-with-lease": {Enabled: true},
		"push.--no-verify":        {Enabled: true},
		"push.--dry-run":          {Enabled: true},
		"push.--set-upstream":     {Enabled: true},
		"push.--tags":             {Enabled: true},
		"push.--follow-tags":      {Enabled: true},
		"push.push.--push-option": {Enabled: true, Value: "ci.skip\ntopic=two words"},
	}
	got, err := pushArgsFromOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	want := gitbackend.PushArgs{Force: gitbackend.PushForceWithLease, NoVerify: true, DryRun: true, SetUpstream: true, Tags: true, FollowTags: true, PushOptions: []string{"ci.skip", "topic=two words"}}
	if !reflect.DeepEqual(got.PushArgs, want) {
		t.Fatalf("PushArgs = %#v, want %#v", got.PushArgs, want)
	}
	if _, err := pushArgsFromOptions(map[keymap.CommandID]OptionValue{"push.--force-with-lease": {Enabled: true}, "push.--force": {Enabled: true}}); err == nil {
		t.Fatal("conflicting force modes were accepted")
	}
	force, err := pushArgsFromOptions(map[keymap.CommandID]OptionValue{"push.--force": {Enabled: true}})
	if err != nil || force.Force != gitbackend.PushForceUnconditionally {
		t.Fatalf("unconditional force = %#v, %v", force, err)
	}
}

func TestPushForcePlansAreVisuallyDistinct(t *testing.T) {
	lease := strings.Join(pushPlan(gitbackend.PushUIArgs{PushArgs: gitbackend.PushArgs{Target: gitbackend.PushElsewhere, Remote: "origin", Force: gitbackend.PushForceWithLease}}), "\n")
	force := strings.Join(pushPlan(gitbackend.PushUIArgs{PushArgs: gitbackend.PushArgs{Target: gitbackend.PushElsewhere, Remote: "origin", Force: gitbackend.PushForceUnconditionally}}), "\n")
	if lease == force || !strings.Contains(lease, "protected") || !strings.Contains(force, "UNCONDITIONAL FORCE") {
		t.Fatalf("force plans are not distinct: lease=%q force=%q", lease, force)
	}
}
