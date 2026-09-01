package git

import (
	"errors"
	"reflect"
	"testing"
)

func TestStage2ExtractedArgumentHelpers(t *testing.T) {
	opts := CloneOptions{Bare: true, NoCheckout: true, NoTags: true, RecurseSubmodules: true, Branch: "main", Depth: 2, Jobs: 3, Origin: "upstream"}
	wantClone := []string{"clone", "--bare", "--no-checkout", "--no-tags", "--recurse-submodules", "--branch", "main", "--depth", "2", "--jobs", "3", "--origin", "upstream", "--", "src", "dst"}
	if got := cloneArgs("src", "dst", opts); !reflect.DeepEqual(got, wantClone) {
		t.Fatalf("cloneArgs = %#v", got)
	}
	if positiveInt(0) != "" || positiveInt(2) != "2" {
		t.Fatal("positiveInt")
	}
	if err := validateCloneRequest("", t.TempDir()+"/dst", CloneOptions{}); err == nil {
		t.Fatal("empty clone source accepted")
	}

	if got, err := appendFetchTags(nil, FetchAllTags); err != nil || !reflect.DeepEqual(got, []string{"--tags"}) {
		t.Fatalf("appendFetchTags = %#v, %v", got, err)
	}
	if _, err := appendFetchTags(nil, FetchTags(255)); err == nil {
		t.Fatal("invalid fetch tags accepted")
	}
	if got, err := appendFetchSubmodules(nil, SubmodulesOnDemand); err != nil || !reflect.DeepEqual(got, []string{"--recurse-submodules=on-demand"}) {
		t.Fatalf("appendFetchSubmodules = %#v, %v", got, err)
	}
	if _, err := appendFetchSubmodules(nil, SubmoduleRecursion(255)); err == nil {
		t.Fatal("invalid submodules accepted")
	}

	args := []string{"commit", "-m", "secret", "--message=other", "-mthird"}
	redacted, secrets := redactCommitArgs(args, append([]string(nil), args...))
	if len(secrets) != 3 || redacted[2] != redactionMarker {
		t.Fatalf("redactCommitArgs = %#v, %#v", redacted, secrets)
	}
	if value, replacement, consumed := commitMessageArgument(args, 1); value != "secret" || replacement != redactionMarker || consumed != 1 {
		t.Fatal("commitMessageArgument")
	}
	remoteArgs := []string{"remote", "add", "origin", "https://user:pass@example.test/repo"}
	if redacted, secrets = redactRemoteURLs(remoteArgs, append([]string(nil), remoteArgs...)); len(secrets) != 1 || redacted[3] != redactionMarker {
		t.Fatal("redactRemoteURLs")
	}

	push := PushArgs{Refspec: "main:main", Matching: true}
	if countPushSelectors(push) != 2 || !hasPushRefSelector(push) || !hasNonAllTagsSelector(push) {
		t.Fatal("push selector helpers")
	}
	if got, err := appendPushForce(nil, PushForceWithLease); err != nil || !reflect.DeepEqual(got, []string{"--force-with-lease"}) {
		t.Fatalf("appendPushForce = %#v, %v", got, err)
	}
	if _, err := appendPushForce(nil, PushForce(255)); err == nil {
		t.Fatal("invalid force accepted")
	}
	if got, err := appendPushOptions(nil, []string{"ci.skip"}); err != nil || !reflect.DeepEqual(got, []string{"--push-option=ci.skip"}) {
		t.Fatalf("appendPushOptions = %#v, %v", got, err)
	}
	if _, err := appendPushOptions(nil, []string{"bad\noption"}); err == nil {
		t.Fatal("invalid push option accepted")
	}
	if !errors.Is(pullUpstreamError(nil), ErrNoUpstream) {
		t.Fatal("pullUpstreamError")
	}
}

func TestStage2ExtractedRemoteValueHelpers(t *testing.T) {
	entries := []string{"branch.main.remote\x00origin", "branch.main.merge\x00refs/heads/main", "branch.other.merge\x00refs/heads/other"}
	if got := linkedBranchMerges(entries, []string{"main"}); !reflect.DeepEqual(got, []string{"main=refs/heads/main"}) {
		t.Fatalf("linkedBranchMerges = %#v", got)
	}
	p := RemoteChangePreflight{BranchPushRemotes: []string{"z", "a"}, BranchRemotes: []string{"z", "a"}}
	sortRemotePreflight(&p)
	if p.BranchPushRemotes[0] != "a" || p.BranchRemotes[0] != "a" {
		t.Fatal("sortRemotePreflight")
	}
	if option, err := parseRemoteTagOpt("--tags"); err != nil || *option != RemoteTagsAll {
		t.Fatal("parseRemoteTagOpt")
	}
	if _, err := parseRemoteTagOpt("bad"); err == nil {
		t.Fatal("invalid tag option accepted")
	}
	if mode, err := parseRemoteFollowRemoteHEAD("warn"); err != nil || *mode != RemoteFollowRemoteHEADWarn {
		t.Fatal("parseRemoteFollowRemoteHEAD")
	}
	if _, err := parseRemoteFollowRemoteHEAD("bad"); err == nil {
		t.Fatal("invalid follow mode accepted")
	}
	badURL := "bad\nurl"
	if err := validateRemoteURLs(RemoteConfigArgs{FetchURL: &badURL}); err == nil {
		t.Fatal("invalid remote URL accepted")
	}
	badTag := RemoteTagOpt(255)
	if err := validateRemoteOptions(RemoteConfigArgs{TagOpt: &badTag}); err == nil {
		t.Fatal("invalid remote option accepted")
	}
	value := "value"
	if stringPointerValue(nil) != "<unset>" || stringPointerValue(&value) != value || printablePointer[int](nil) != "<unset>" {
		t.Fatal("pointer formatting")
	}
	before := RemoteConfiguration{FetchURL: &value}
	in := RemoteConfigArgs{Remote: "origin", FetchURL: &value}
	if got := remoteConfigurationChangeValues(before, in); len(got) != 1 || got[0].key != "remote.origin.url" {
		t.Fatalf("remoteConfigurationChangeValues = %#v", got)
	}
}
