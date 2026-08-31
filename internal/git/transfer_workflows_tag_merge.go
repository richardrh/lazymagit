package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TagInfo struct {
	Name       string
	ObjectID   string
	TargetID   string
	Subject    string
	Tagger     string
	TaggerDate time.Time
	Annotated  bool
	Signed     bool
}

func (r *Repository) ListTags(ctx context.Context) ([]TagInfo, error) {
	format := "%(refname:strip=2)%00%(objectname)%00%(*objectname)%00%(subject)%00%(taggername)%00%(taggerdate:iso-strict)%00"
	out, truncated, err := r.outputLimited(ctx, 8<<20, "for-each-ref", "--sort=-taggerdate", "--format="+format, "refs/tags")
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, &TooLargeError{Resource: "tag listing"}
	}
	var tags []TagInfo
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{0})
		if len(line) == 0 {
			continue
		}
		if len(tags) >= 100000 {
			return nil, &TooLargeError{Resource: "tag count"}
		}
		fields := bytes.Split(line, []byte{0})
		if len(fields) != 6 {
			return nil, errors.New("malformed tag listing")
		}
		tag := TagInfo{Name: string(fields[0]), ObjectID: string(fields[1]), TargetID: string(fields[2]), Subject: string(fields[3]), Tagger: string(fields[4])}
		tag.Annotated = tag.TargetID != ""
		if tag.TargetID == "" {
			tag.TargetID = tag.ObjectID
		} else {
			raw, truncated, err := r.outputLimited(ctx, 1<<20, "cat-file", "tag", tag.ObjectID)
			if err != nil {
				return nil, err
			}
			if truncated {
				return nil, &TooLargeError{Resource: "annotated tag object"}
			}
			tag.Signed = bytes.Contains(raw, []byte("-----BEGIN PGP SIGNATURE-----")) || bytes.Contains(raw, []byte("-----BEGIN SSH SIGNATURE-----"))
		}
		if len(fields[5]) != 0 {
			tag.TaggerDate, err = time.Parse(time.RFC3339, string(fields[5]))
			if err != nil {
				return nil, fmt.Errorf("parse tag date: %w", err)
			}
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

type CreateTagArgs struct {
	Name           string
	Target         string
	Annotated      bool
	Message        string
	Sign           bool
	LocalUser      string
	Force          bool
	ConfirmReplace bool
}

type TagCreatePreflight struct {
	Name                 string
	Exists               bool
	RequiresConfirmation bool
}

func (r *Repository) TagCreatePreflight(ctx context.Context, name string) (TagCreatePreflight, error) {
	if err := r.validateTag(ctx, name); err != nil {
		return TagCreatePreflight{}, err
	}
	_, err := r.output(ctx, "show-ref", "--verify", "--quiet", "refs/tags/"+name)
	p := TagCreatePreflight{Name: name, Exists: err == nil}
	if err != nil && commandExitCode(err) != 1 {
		return p, err
	}
	p.RequiresConfirmation = p.Exists
	return p, nil
}

func (r *Repository) CreateTagWithArgs(ctx context.Context, in CreateTagArgs) (TagCreatePreflight, error) {
	p, err := r.TagCreatePreflight(ctx, in.Name)
	if err != nil {
		return p, err
	}
	in, err = normalizeCreateTagArgs(p, in)
	if err != nil {
		return p, err
	}
	target := in.Target
	resolved, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", target+"^{object}")
	if err != nil {
		return p, fmt.Errorf("tag target: %w", err)
	}
	args := createTagCommandArgs(in, trimLine(resolved))
	// Supplying messages over stdin keeps them out of ProcessRecord.Args.
	if in.Annotated {
		err = r.runInput(ctx, []byte(in.Message), args...)
	} else {
		err = r.run(ctx, args...)
	}
	return p, err
}

func normalizeCreateTagArgs(p TagCreatePreflight, in CreateTagArgs) (CreateTagArgs, error) {
	if p.Exists && (!in.Force || !in.ConfirmReplace) {
		return in, errors.New("replacing a tag requires force and confirmation")
	}
	if strings.ContainsAny(in.LocalUser, "\x00\r\n") {
		return in, errors.New("tag signing identity contains a control character")
	}
	if in.LocalUser != "" {
		in.Sign = true
	}
	if in.Sign {
		in.Annotated = true
	}
	if in.Annotated && in.Message == "" {
		return in, errors.New("annotated tag message is empty")
	}
	if !in.Annotated && in.Message != "" {
		return in, errors.New("lightweight tag cannot have a message")
	}
	if in.Target == "" {
		in.Target = "HEAD"
	}
	return in, nil
}

func createTagCommandArgs(in CreateTagArgs, resolved string) []string {
	args := []string{"tag"}
	if in.Force {
		args = append(args, "--force")
	}
	if in.Sign {
		if in.LocalUser != "" {
			args = append(args, "--local-user="+in.LocalUser)
		}
		args = append(args, "--sign", "--file=-")
	} else if in.Annotated {
		args = append(args, "--annotate", "--file=-")
	}
	return append(args, "--", in.Name, resolved)
}

type DeleteTagsArgs struct {
	Names   []string
	Confirm bool
}
type TagDeletePreflight struct {
	Existing             []string
	Missing              []string
	RequiresConfirmation bool
}

func (r *Repository) DeleteTags(ctx context.Context, in DeleteTagsArgs) (TagDeletePreflight, error) {
	var p TagDeletePreflight
	seen := map[string]bool{}
	for _, name := range in.Names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if err := r.validateTag(ctx, name); err != nil {
			return p, err
		}
		if _, err := r.output(ctx, "show-ref", "--verify", "--quiet", "refs/tags/"+name); err == nil {
			p.Existing = append(p.Existing, name)
		} else if commandExitCode(err) == 1 {
			p.Missing = append(p.Missing, name)
		} else {
			return p, err
		}
	}
	p.RequiresConfirmation = len(p.Existing) != 0
	if p.RequiresConfirmation && !in.Confirm {
		return p, errors.New("tag deletion requires confirmation")
	}
	if len(p.Existing) != 0 {
		return p, r.run(ctx, append([]string{"tag", "--delete", "--"}, p.Existing...)...)
	}
	return p, nil
}

type RemoteTagComparison struct {
	Remote               string
	LocalOnly            []string
	RemoteOnly           []string
	Common               []string
	RequiresConfirmation bool
}

func (r *Repository) CompareRemoteTags(ctx context.Context, remote string) (RemoteTagComparison, error) {
	if err := r.validateTransferRemote(ctx, remote); err != nil {
		return RemoteTagComparison{}, err
	}
	localOut, truncated, err := r.outputLimited(ctx, 8<<20, "for-each-ref", "--format=%(refname:strip=2)", "refs/tags")
	if err != nil {
		return RemoteTagComparison{}, err
	}
	if truncated {
		return RemoteTagComparison{}, &TooLargeError{Resource: "local tag listing"}
	}
	remoteOut, truncated, err := r.outputLimited(ctx, 8<<20, "ls-remote", "--tags", "--refs", "--", remote)
	if err != nil {
		return RemoteTagComparison{}, err
	}
	if truncated {
		return RemoteTagComparison{}, &TooLargeError{Resource: "remote tag listing"}
	}
	local, upstream := map[string]bool{}, map[string]bool{}
	for _, name := range strings.Split(trimLine(localOut), "\n") {
		if name != "" {
			local[name] = true
		}
	}
	if len(local) > 100000 {
		return RemoteTagComparison{}, &TooLargeError{Resource: "local tag count"}
	}
	for _, line := range bytes.Split(remoteOut, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) == 2 {
			upstream[strings.TrimPrefix(string(fields[1]), "refs/tags/")] = true
		}
	}
	if len(upstream) > 100000 {
		return RemoteTagComparison{}, &TooLargeError{Resource: "remote tag count"}
	}
	p := RemoteTagComparison{Remote: remote}
	for name := range local {
		if upstream[name] {
			p.Common = append(p.Common, name)
		} else {
			p.LocalOnly = append(p.LocalOnly, name)
		}
	}
	for name := range upstream {
		if !local[name] {
			p.RemoteOnly = append(p.RemoteOnly, name)
		}
	}
	sort.Strings(p.LocalOnly)
	sort.Strings(p.RemoteOnly)
	sort.Strings(p.Common)
	p.RequiresConfirmation = len(p.LocalOnly) != 0
	return p, nil
}

// PruneRemoteTags deletes local tags absent from remote; it never deletes a
// remote tag. The comparison is returned even when confirmation is withheld.
func (r *Repository) PruneRemoteTags(ctx context.Context, remote string, confirm bool) (RemoteTagComparison, error) {
	p, err := r.CompareRemoteTags(ctx, remote)
	if err != nil {
		return p, err
	}
	if p.RequiresConfirmation && !confirm {
		return p, errors.New("tag prune requires confirmation")
	}
	if len(p.LocalOnly) != 0 {
		err = r.run(ctx, append([]string{"tag", "--delete", "--"}, p.LocalOnly...)...)
	}
	return p, err
}

type MergeMode uint8

const (
	MergePlain MergeMode = iota
	MergeNoFF
	MergeFFOnly
)

type MergeArgs struct {
	Target          string
	Mode            MergeMode
	NoCommit        bool
	Squash          bool
	ConfirmDirty    bool
	Strategy        string
	StrategyOptions []string
	Signoff         bool
}

type MergeState struct {
	InProgress   bool
	Heads        []string
	Dirty        bool
	HasStaged    bool
	HasUnstaged  bool
	HasUntracked bool
	Conflicts    []string
}

func (r *Repository) MergeState(ctx context.Context) (MergeState, error) {
	status, err := r.Status(ctx)
	if err != nil {
		return MergeState{}, err
	}
	var state MergeState
	for _, file := range status.Files {
		if file.Staged != ChangeNone {
			state.HasStaged = true
		}
		if file.Unstaged != ChangeNone {
			state.HasUnstaged = true
		}
		if file.Unstaged == ChangeUntracked {
			state.HasUntracked = true
		}
		if file.Staged == ChangeUnmerged || file.Unstaged == ChangeUnmerged {
			state.Conflicts = append(state.Conflicts, file.Path)
		}
	}
	state.Dirty = state.HasStaged || state.HasUnstaged
	pathOut, err := r.output(ctx, "rev-parse", "--git-path", "MERGE_HEAD")
	if err != nil {
		return state, err
	}
	path := strings.TrimSpace(string(pathOut))
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.commandDir, path)
	}
	data, err := readFileLimited(path, 64<<10)
	if err == nil {
		for _, head := range strings.Fields(string(data)) {
			state.Heads = append(state.Heads, head)
		}
		state.InProgress = len(state.Heads) != 0
	} else if !errors.Is(err, os.ErrNotExist) {
		return state, fmt.Errorf("read merge state: %w", err)
	}
	return state, nil
}

type MergePreflight struct {
	State                     MergeState
	Target                    string
	TargetOID                 string
	FastForwardPossible       bool
	AlreadyUpToDate           bool
	RequiresDirtyConfirmation bool
}

func (r *Repository) MergePreflight(ctx context.Context, target string) (MergePreflight, error) {
	if err := validateToken("merge target", target); err != nil {
		return MergePreflight{}, err
	}
	state, err := r.MergeState(ctx)
	if err != nil {
		return MergePreflight{}, err
	}
	p := MergePreflight{State: state, Target: target, RequiresDirtyConfirmation: state.Dirty}
	resolved, err := r.output(ctx, "rev-parse", "--verify", "--end-of-options", target+"^{commit}")
	if err != nil {
		return p, fmt.Errorf("merge target: %w", err)
	}
	p.TargetOID = trimLine(resolved)
	if _, err = r.output(ctx, "merge-base", "--is-ancestor", "HEAD", p.TargetOID); err == nil {
		p.FastForwardPossible = true
	} else if commandExitCode(err) != 1 {
		return p, err
	}
	if _, err = r.output(ctx, "merge-base", "--is-ancestor", p.TargetOID, "HEAD"); err == nil {
		p.AlreadyUpToDate = true
	} else if commandExitCode(err) != 1 {
		return p, err
	}
	return p, nil
}

func (r *Repository) MergeWithArgs(ctx context.Context, in MergeArgs) (MergePreflight, error) {
	if err := validateMergeArgs(in); err != nil {
		return MergePreflight{}, err
	}
	p, err := r.MergePreflight(ctx, in.Target)
	if err != nil {
		return p, err
	}
	if err := validateMergePreflight(p, in.ConfirmDirty); err != nil {
		return p, err
	}
	args, err := mergeArgs(in, p.TargetOID)
	if err != nil {
		return p, err
	}
	return p, r.run(ctx, args...)
}

func validateMergeArgs(in MergeArgs) error {
	if err := validateHistoryStrategy(in.Strategy); err != nil {
		return err
	}
	for _, option := range in.StrategyOptions {
		if err := validateMergeStrategyOption(option); err != nil {
			return err
		}
	}
	return nil
}

func validateMergePreflight(p MergePreflight, confirmDirty bool) error {
	if p.State.InProgress {
		return errors.New("a merge is already in progress")
	}
	if len(p.State.Conflicts) != 0 {
		return errors.New("cannot merge with unresolved conflicts")
	}
	if p.RequiresDirtyConfirmation && !confirmDirty {
		return errors.New("merge with a dirty worktree requires confirmation")
	}
	return nil
}

func mergeArgs(in MergeArgs, targetOID string) ([]string, error) {
	args := []string{"merge"}
	switch in.Mode {
	case MergePlain:
		args = append(args, "--no-edit")
	case MergeNoFF:
		args = append(args, "--no-ff", "--no-edit")
	case MergeFFOnly:
		args = append(args, "--ff-only")
	default:
		return nil, errors.New("invalid merge mode")
	}
	if in.NoCommit {
		args = append(args, "--no-commit")
	}
	if in.Squash {
		args = append(args, "--squash")
	}
	if in.Strategy != "" {
		args = append(args, "--strategy="+in.Strategy)
	}
	for _, option := range in.StrategyOptions {
		args = append(args, "--strategy-option="+option)
	}
	if in.Signoff {
		args = append(args, "--signoff")
	}
	return append(args, "--", targetOID), nil
}

func validateMergeStrategyOption(option string) error {
	if strings.TrimSpace(option) != option || option == "" || strings.HasPrefix(option, "-") || strings.ContainsAny(option, "\x00\r\n") {
		return fmt.Errorf("invalid merge strategy option %q", option)
	}
	return nil
}

func (r *Repository) ContinueMerge(ctx context.Context) error {
	state, err := r.MergeState(ctx)
	if err != nil {
		return err
	}
	if !state.InProgress {
		return errors.New("no merge is in progress")
	}
	if len(state.Conflicts) != 0 {
		return errors.New("cannot continue merge with unresolved conflicts")
	}
	// `merge --continue` delegates to commit and may honor an inherited
	// GIT_EDITOR over core.editor. Commit the prepared MERGE_MSG explicitly;
	// --no-edit is the non-interactive equivalent and retains normal hooks.
	return r.run(ctx, "commit", "--no-edit")
}

func (r *Repository) AbortMerge(ctx context.Context) error {
	state, err := r.MergeState(ctx)
	if err != nil {
		return err
	}
	if !state.InProgress {
		return errors.New("no merge is in progress")
	}
	return r.run(ctx, "merge", "--abort")
}
