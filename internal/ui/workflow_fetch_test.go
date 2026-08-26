package ui

import (
	"testing"

	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

func TestFetchWorkflowRegistrationAndAvailability(t *testing.T) {
	m := New(&gitbackend.Repository{})
	ids := []keymap.CommandID{
		keymap.CommandFetchPush, keymap.CommandFetchUpstream,
		keymap.CommandFetchElsewhere, keymap.CommandFetchAll,
		commandFetchBranch, commandFetchRefspec, commandFetchModules,
		commandFetchConfigure,
	}
	for _, id := range ids {
		if m.workflowHandlers[id] == nil {
			t.Errorf("fetch handler %q is not registered", id)
		}
	}
	catalog, ok := m.transientCatalog("f")
	if !ok {
		t.Fatal("fetch transient is missing")
	}
	for _, key := range []string{"p", "u", "e", "a", "o", "r", "m", "C", "-p", "-t", "-u", "-F"} {
		entry, found := catalog.entry(key)
		if !found || !entry.Available {
			t.Errorf("fetch entry %q unavailable: found=%v entry=%+v", key, found, entry)
		}
	}
}

func TestFetchWorkflowConvertsEveryInfix(t *testing.T) {
	options := map[keymap.CommandID]OptionValue{
		optionFetchPrune:     {Enabled: true},
		optionFetchTags:      {Enabled: true},
		optionFetchUnshallow: {Enabled: true},
		optionFetchForce:     {Enabled: true},
	}
	got := fetchArgsFromCommand(WorkflowCommand{Options: options})
	if !got.Prune || got.Tags != gitbackend.FetchAllTags || !got.Unshallow || !got.Force {
		t.Fatalf("converted fetch args = %+v", got)
	}
	options[optionFetchPrune] = OptionValue{}
	if !got.Prune {
		t.Fatal("converted args aliased later option mutation")
	}
}

func TestFetchWorkflowRejectsInvalidChooserSelections(t *testing.T) {
	branches := []gitbackend.FetchRemoteBranch{{Remote: "origin", Branch: "main"}}
	for _, value := range []string{"", "-1", "1", "not-a-number"} {
		if _, err := selectedRemoteBranch(WorkflowValues{"target": value}, branches); err == nil {
			t.Errorf("invalid chooser value %q was accepted", value)
		}
	}
	if _, err := remoteField(nil); err == nil {
		t.Fatal("empty configured remote list was accepted")
	}
}
