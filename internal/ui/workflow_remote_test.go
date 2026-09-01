package ui

import (
	"encoding/json"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

func TestRemoteTransientSafeSuffixesAreDomainConnected(t *testing.T) {
	m := New(&gitbackend.Repository{})
	m.snapshot.remotes = []gitbackend.Remote{{Name: "origin"}}
	catalog, ok := m.transientCatalog("M")
	if !ok {
		t.Fatal("remote transient missing")
	}
	for _, key := range []string{"a", "r", "k", "C", "p", "z"} {
		entry, found := catalog.entry(key)
		if !found || !entry.Available {
			t.Errorf("M %s is not connected: found=%v entry=%+v", key, found, entry)
		}
	}
	for _, key := range []string{"P", "h"} {
		entry, found := catalog.entry(key)
		if !found || entry.Available || entry.Reason == "" {
			t.Errorf("M %s must be routed through Configure remote or remain unavailable: %+v", key, entry)
		}
	}
	if entry, found := catalog.entry("d u"); !found || !entry.Available {
		t.Errorf("M d u must route to the registered update workflow: %+v", entry)
	}
}

func TestRemoteConfigureValuesPreserveUnchangedClearAndReplace(t *testing.T) {
	values := WorkflowValues{
		"remote": "origin", "u-mode": "unchanged", "U-mode": "clear",
		"s-mode": "replace", "s": "ssh://example/repo", "S-mode": "replace",
		"S": `["refs/heads/main:refs/heads/release"]`, "O": "none", "h": "always",
	}
	args, err := remoteConfigArgs(values)
	if err != nil {
		t.Fatal(err)
	}
	if args.FetchURL != nil || args.FetchRefspecs == nil || len(args.FetchRefspecs) != 0 {
		t.Fatalf("u/U nil-empty semantics = %#v", args)
	}
	if args.PushURL == nil || *args.PushURL != "ssh://example/repo" || args.TagOpt == nil || *args.TagOpt != gitbackend.RemoteTagsNone || args.FollowRemoteHEAD == nil || *args.FollowRemoteHEAD != gitbackend.RemoteFollowRemoteHEADAlways {
		t.Fatalf("s/O/h values = %#v", args)
	}
	encoded, _ := json.Marshal(args.PushRefspecs)
	if string(encoded) != values["S"] {
		t.Fatalf("S = %s", encoded)
	}

	values["S"] = `null`
	if _, err := remoteConfigArgs(values); err == nil {
		t.Fatal("null refspec array accepted")
	}
}

func TestRemoteAddFetchInfixControlsDirectWorkflow(t *testing.T) {
	m := New(&gitbackend.Repository{})
	if cmd := remoteAddWorkflow(m, WorkflowCommand{ID: keymap.CommandAddRemote}); cmd != nil || m.mode != modeAddRemote || m.remoteFetch {
		t.Fatalf("untouched infix unexpectedly enabled fetch: mode=%v fetch=%v", m.mode, m.remoteFetch)
	}
	m.setMode(modeStatus)
	remoteAddWorkflow(m, WorkflowCommand{ID: keymap.CommandAddRemote, Options: map[keymap.CommandID]OptionValue{commandRemoteFetchAfterAdd: {Enabled: false}}})
	if m.mode != modeWorkflow || m.workflow.dialog.Fields[2].Bool {
		t.Fatalf("explicitly disabled -f was ignored: mode=%v fields=%+v", m.mode, m.workflow.dialog.Fields)
	}
}
