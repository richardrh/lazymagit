package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReviewBranchConfigUpdateResolvesAndValidatesConfiguration(t *testing.T) {
	r := newTestRepo(t)
	r.write("base.txt", "base\n")
	r.commitAll("base")
	r.git("remote", "add", "origin", "https://example.invalid/repository")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	keep := ConfigUpdate{Action: ConfigKeep}
	request := BranchConfigUpdate{
		Branch:            "main",
		Description:       keep,
		Upstream:          ConfigUpdate{Action: ConfigSet, Value: "main"},
		Rebase:            keep,
		PushRemote:        ConfigUpdate{Action: ConfigSet, Value: "origin"},
		PullRebase:        keep,
		RemotePushDefault: ConfigUpdate{Action: ConfigSet, Value: "origin"},
		AutoSetupMerge:    keep,
		AutoSetupRebase:   keep,
	}

	reviewed, err := repo.ReviewBranchConfigUpdate(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if reviewed.Upstream.Value != "refs/heads/main" {
		t.Fatalf("resolved upstream = %q", reviewed.Upstream.Value)
	}
	if reviewed.Before.OID == "" || len(reviewed.Plan) != 3 {
		t.Fatalf("reviewed update = %#v", reviewed)
	}
	if !reviewed.Token.validFor(branchConfigIdentity(reviewed)) {
		t.Fatal("review token does not match the reviewed update")
	}

	tests := []struct {
		name   string
		mutate func(*BranchConfigUpdate)
	}{
		{"snapshot failure", func(update *BranchConfigUpdate) { update.Branch = "missing" }},
		{"invalid update", func(update *BranchConfigUpdate) { update.Description.Action = "invalid" }},
		{"invalid push remote", func(update *BranchConfigUpdate) { update.PushRemote.Value = "missing" }},
		{"invalid default push remote", func(update *BranchConfigUpdate) {
			update.PushRemote = keep
			update.RemotePushDefault.Value = "missing"
		}},
		{"invalid upstream", func(update *BranchConfigUpdate) {
			update.PushRemote = keep
			update.RemotePushDefault = keep
			update.Upstream.Value = "missing"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := request
			test.mutate(&invalid)
			if _, err := repo.ReviewBranchConfigUpdate(ctx, invalid); err == nil {
				t.Fatal("invalid review succeeded")
			}
		})
	}
}

func TestValidateBranchConfigUpdate(t *testing.T) {
	valid := BranchConfigUpdate{
		Description: ConfigUpdate{Action: ConfigKeep}, Upstream: ConfigUpdate{Action: ConfigKeep},
		Rebase: ConfigUpdate{Action: ConfigSet, Value: "merges"}, PushRemote: ConfigUpdate{Action: ConfigUnset},
		PullRebase: ConfigUpdate{Action: ConfigSet, Value: "interactive"}, RemotePushDefault: ConfigUpdate{Action: ConfigKeep},
		AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: ConfigUpdate{Action: ConfigSet, Value: "remote"},
	}
	if err := validateBranchConfigUpdate(valid); err != nil {
		t.Fatalf("valid update: %v", err)
	}
	invalid := valid
	invalid.Description.Action = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("invalid action error = %v", err)
	}
	invalid = valid
	invalid.AutoSetupMerge.Value = "invalid"
	if err := validateBranchConfigUpdate(invalid); err == nil || !strings.Contains(err.Error(), "autoSetupMerge") {
		t.Fatalf("invalid mode error = %v", err)
	}
}

func TestApplyConfigUpdate(t *testing.T) {
	var calls []string
	set := func(value string) error { calls = append(calls, "set "+value); return nil }
	unset := func() error { calls = append(calls, "unset"); return nil }
	for _, update := range []ConfigUpdate{{Action: ConfigKeep}, {Action: ConfigSet, Value: "value"}, {Action: ConfigUnset}} {
		if err := applyConfigUpdate(configMutation{update: update, set: set, unset: unset}); err != nil {
			t.Fatalf("applyConfigUpdate(%q): %v", update.Action, err)
		}
	}
	if !reflect.DeepEqual(calls, []string{"set value", "unset"}) {
		t.Fatalf("config mutation calls = %#v", calls)
	}
	if err := applyConfigUpdate(configMutation{update: ConfigUpdate{Action: "invalid"}, set: set, unset: unset}); err == nil {
		t.Fatal("invalid config update succeeded")
	}
}

func TestBranchConfigExecutionHelpers(t *testing.T) {
	r := newTestRepo(t)
	r.write("base", "base\n")
	r.commitAll("base")
	repo, err := Discover(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	keep := ConfigUpdate{Action: ConfigKeep}
	reviewed := BranchConfigUpdate{Branch: "main", Description: keep, Upstream: keep, Rebase: keep, PushRemote: keep, PullRebase: keep, RemotePushDefault: keep, AutoSetupMerge: ConfigUpdate{Action: ConfigSet, Value: "simple"}, AutoSetupRebase: keep}
	if err := repo.applyBranchConfigUpdates(context.Background(), reviewed); err != nil {
		t.Fatal(err)
	}
	if got := r.git("config", "--get", "branch.autoSetupMerge"); got != "simple" {
		t.Fatalf("autoSetupMerge = %q", got)
	}
	path := filepath.Join(t.TempDir(), "config")
	cause := errors.New("mutation failed")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := rollbackBranchConfig(path, []byte("before"), true, cause); !errors.Is(got, cause) {
		t.Fatalf("rollbackBranchConfig = %v", got)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "before" {
		t.Fatalf("restored config = %q, %v", data, err)
	}
}
