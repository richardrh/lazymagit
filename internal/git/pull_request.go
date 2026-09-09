package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PullRequest is the small, UI-facing representation of a GitHub pull request.
type PullRequest struct {
	Number int
	URL    string
	Title  string
	Body   string
	Base   string
	Head   string
	Draft  bool
}

var (
	// ErrGitHubCLIUnavailable means that gh is not installed or cannot be
	// launched. ErrGitHubUnavailable is also wrapped for callers that only
	// need to report that the integration is unavailable.
	ErrGitHubCLIUnavailable = errors.New("GitHub CLI is unavailable")
	ErrGitHubUnavailable    = errors.New("GitHub integration is unavailable")
	ErrGitHubAuthentication = errors.New("GitHub authentication is required")
	ErrGitHubPermission     = errors.New("GitHub permission denied")
	ErrGitHubRepository     = errors.New("GitHub repository is unavailable")
	ErrGitHubBranch         = errors.New("current branch is unavailable")
)

// GitHubCommandError retains bounded diagnostics from gh without ever
// including message text supplied on stdin.
type GitHubCommandError struct {
	Args            []string
	Err             error
	Stderr          string
	StderrTruncated bool
}

func (e *GitHubCommandError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("gh %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return fmt.Sprintf("gh %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Stderr)
}
func (e *GitHubCommandError) Unwrap() error { return e.Err }

// PullRequestPartialError reports a mutation which completed in part. Result
// contains the known PR identity and should be used by a caller to refresh the
// dialog before retrying.
type PullRequestPartialError struct {
	Result PullRequest
	Err    error
}

func (e *PullRequestPartialError) Error() string {
	return fmt.Sprintf("pull request %d was partially updated: %v", e.Result.Number, e.Err)
}
func (e *PullRequestPartialError) Unwrap() error { return e.Err }

type githubRepository struct {
	NameWithOwner    string `json:"nameWithOwner"`
	DefaultBranchRef *struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
}

type githubPullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Base   string `json:"baseRefName"`
	Head   string `json:"headRefName"`
	Draft  bool   `json:"isDraft"`
}

func (r *Repository) PullRequestForUI(ctx context.Context) (PullRequest, error) {
	repository, err := r.githubRepository(ctx)
	if err != nil {
		return PullRequest{}, err
	}
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return PullRequest{}, err
	}
	if branch == "" {
		return PullRequest{}, fmt.Errorf("%w: checkout a branch before composing a PR", ErrGitHubBranch)
	}
	base := repository.defaultBranch()
	if base == "" {
		return PullRequest{}, fmt.Errorf("%w: GitHub repository has no default branch", ErrGitHubRepository)
	}

	args := []string{"pr", "list", "--repo", repository.NameWithOwner, "--head", branch,
		"--state", "open", "--limit", "2", "--json", "number,url,title,body,baseRefName,headRefName,isDraft"}
	out, err := r.runGH(ctx, args, nil, nil, false)
	if err != nil {
		// A failed list is never treated as an empty list: auth, network, and
		// permissions failures must remain visible to the caller.
		return PullRequest{}, fmt.Errorf("list open pull requests: %w", err)
	}
	var listed []githubPullRequest
	if err := json.Unmarshal([]byte(out.stdout), &listed); err != nil {
		return PullRequest{}, fmt.Errorf("parse open pull requests: %w", err)
	}
	if len(listed) > 1 {
		return PullRequest{}, errors.New("multiple open PRs use this branch; select the intended PR with gh before editing")
	}
	if len(listed) != 0 {
		pr := listed[0].public()
		if pr.Head != branch {
			return PullRequest{}, fmt.Errorf("%w: GitHub returned pull request head %q for current branch %q", ErrGitHubBranch, pr.Head, branch)
		}
		return pr, nil
	}

	title, body, err := r.headMessage(ctx)
	if err != nil {
		return PullRequest{}, err
	}
	if template := localPullRequestTemplate(r.workTree); template != "" {
		body = template
	}
	return PullRequest{Title: title, Body: body, Base: base, Head: branch, Draft: true}, nil
}

func (r *Repository) SubmitPullRequest(ctx context.Context, request PullRequest) (PullRequest, error) {
	if request.Number < 0 {
		return PullRequest{}, errors.New("pull request number cannot be negative")
	}
	repository, err := r.githubRepository(ctx)
	if err != nil {
		return PullRequest{}, err
	}
	branch, err := r.currentBranch(ctx)
	if err != nil {
		return PullRequest{}, err
	}
	if branch == "" {
		return PullRequest{}, fmt.Errorf("%w: checkout a branch before publishing a PR", ErrGitHubBranch)
	}
	if request.Head != "" && request.Head != branch {
		return PullRequest{}, fmt.Errorf("%w: requested %q, current branch is %q", ErrGitHubBranch, request.Head, branch)
	}
	request.Head = branch
	if request.Base == "" {
		request.Base = repository.defaultBranch()
	}
	if request.Base == "" {
		return PullRequest{}, fmt.Errorf("%w: GitHub repository has no default branch", ErrGitHubRepository)
	}
	if strings.TrimSpace(request.Title) == "" {
		return PullRequest{}, errors.New("pull request title is required")
	}

	if request.Number == 0 {
		return r.createPullRequest(ctx, repository.NameWithOwner, request)
	}
	return r.editPullRequest(ctx, repository.NameWithOwner, request)
}

func (r *Repository) createPullRequest(ctx context.Context, repository string, request PullRequest) (PullRequest, error) {
	if err := r.verifyCurrentBranch(ctx, request.Head); err != nil {
		return PullRequest{}, err
	}
	args := []string{"pr", "create", "--repo", repository, "--head", request.Head, "--base", request.Base,
		"--title", request.Title, "--body-file", "-"}
	if request.Draft {
		args = append(args, "--draft")
	}
	out, err := r.runGH(ctx, args, []byte(request.Body), []string{request.Title, request.Body}, true)
	if err != nil {
		return PullRequest{}, fmt.Errorf("create pull request: %w", err)
	}
	url := pullRequestURL(out.stdout)
	if url == "" {
		return PullRequest{}, errors.New("create pull request: gh did not return a pull request URL")
	}
	result, err := r.viewPullRequest(ctx, repository, url)
	if err != nil {
		request.URL = url
		number := strings.TrimRight(url, "/")
		_, _ = fmt.Sscanf(number[strings.LastIndex(number, "/")+1:], "%d", &request.Number)
		return request, &PullRequestPartialError{Result: request, Err: fmt.Errorf("read created pull request: %w", err)}
	}
	if result.Head != request.Head {
		return PullRequest{}, fmt.Errorf("%w: created pull request head %q differs from current branch %q", ErrGitHubBranch, result.Head, request.Head)
	}
	return result, nil
}

func (r *Repository) editPullRequest(ctx context.Context, repository string, request PullRequest) (PullRequest, error) {
	// Query the explicit PR before mutating it. Besides preserving the actual
	// URL, this prevents a stale dialog from editing a PR whose head changed.
	current, err := r.viewPullRequest(ctx, repository, strconv.Itoa(request.Number))
	if err != nil {
		return PullRequest{}, fmt.Errorf("inspect pull request %d: %w", request.Number, err)
	}
	if current.Head != request.Head {
		return PullRequest{}, fmt.Errorf("%w: pull request %d belongs to %q, current branch is %q", ErrGitHubBranch, request.Number, current.Head, request.Head)
	}
	if request.URL == "" {
		request.URL = current.URL
	}

	args := []string{"pr", "edit", strconv.Itoa(request.Number), "--repo", repository, "--title", request.Title,
		"--body-file", "-", "--base", request.Base}
	if err := r.verifyCurrentBranch(ctx, request.Head); err != nil {
		return PullRequest{}, err
	}
	if _, err := r.runGH(ctx, args, []byte(request.Body), []string{request.Title, request.Body}, true); err != nil {
		return PullRequest{}, fmt.Errorf("edit pull request %d: %w", request.Number, err)
	}

	result, err := r.viewPullRequest(ctx, repository, strconv.Itoa(request.Number))
	if err != nil {
		request.URL, request.Draft = current.URL, current.Draft
		return request, &PullRequestPartialError{Result: request, Err: fmt.Errorf("read edited pull request: %w", err)}
	}
	if result.Draft != request.Draft {
		readyArgs := []string{"pr", "ready", strconv.Itoa(request.Number), "--repo", repository}
		if request.Draft {
			readyArgs = append(readyArgs, "--undo")
		}
		if _, readyErr := r.runGH(ctx, readyArgs, nil, nil, true); readyErr != nil {
			result.URL = firstNonEmpty(result.URL, request.URL)
			return result, &PullRequestPartialError{Result: result, Err: fmt.Errorf("change draft state: %w", readyErr)}
		}
		updated, err := r.viewPullRequest(ctx, repository, strconv.Itoa(request.Number))
		if err != nil {
			result.Draft = request.Draft
			return result, &PullRequestPartialError{Result: result, Err: fmt.Errorf("read pull request after draft change: %w", err)}
		}
		result = updated
		if result.Head != request.Head {
			return PullRequest{}, fmt.Errorf("%w: pull request %d head changed to %q", ErrGitHubBranch, request.Number, result.Head)
		}
	}
	result.URL = firstNonEmpty(result.URL, request.URL)
	return result, nil
}

func (r *Repository) viewPullRequest(ctx context.Context, repository, selector string) (PullRequest, error) {
	args := []string{"pr", "view", selector, "--repo", repository, "--json", "number,url,title,body,baseRefName,headRefName,isDraft"}
	out, err := r.runGH(ctx, args, nil, nil, false)
	if err != nil {
		return PullRequest{}, err
	}
	var value githubPullRequest
	if err := json.Unmarshal([]byte(out.stdout), &value); err != nil {
		return PullRequest{}, fmt.Errorf("parse pull request: %w", err)
	}
	if value.Number == 0 {
		return PullRequest{}, errors.New("GitHub returned a pull request without a number")
	}
	return value.public(), nil
}

func (r *Repository) githubRepository(ctx context.Context) (githubRepository, error) {
	args := []string{"repo", "view", "--json", "nameWithOwner,defaultBranchRef"}
	out, err := r.runGH(ctx, args, nil, nil, false)
	if err != nil {
		return githubRepository{}, fmt.Errorf("%w: resolve GitHub repository: %w", ErrGitHubRepository, classifyGitHubError(err))
	}
	var repository githubRepository
	if err := json.Unmarshal([]byte(out.stdout), &repository); err != nil {
		return githubRepository{}, fmt.Errorf("%w: parse repository details: %w", ErrGitHubRepository, err)
	}
	if repository.NameWithOwner == "" {
		return githubRepository{}, fmt.Errorf("%w: gh did not report a GitHub repository", ErrGitHubRepository)
	}
	return repository, nil
}

func (r *Repository) verifyCurrentBranch(ctx context.Context, expected string) error {
	current, err := r.currentBranch(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return fmt.Errorf("%w: requested %q, current branch is %q", ErrGitHubBranch, expected, current)
	}
	return nil
}

func (r *Repository) headMessage(ctx context.Context) (string, string, error) {
	out, err := r.output(ctx, "log", "-1", "--format=%s%x00%b", "HEAD")
	if err != nil {
		if commandExitCode(err) == 128 {
			return "", "", nil // unborn repository: leave both fields empty
		}
		return "", "", fmt.Errorf("read HEAD message: %w", err)
	}
	text := string(out)
	subject, body, ok := strings.Cut(text, "\x00")
	if !ok {
		return strings.TrimRight(text, "\r\n"), "", nil
	}
	return strings.TrimRight(subject, "\r\n"), strings.TrimRight(body, "\r\n"), nil
}

func (g githubRepository) defaultBranch() string {
	if g.DefaultBranchRef == nil {
		return ""
	}
	return strings.TrimSpace(g.DefaultBranchRef.Name)
}

func (g githubPullRequest) public() PullRequest {
	return PullRequest{Number: g.Number, URL: g.URL, Title: g.Title, Body: g.Body, Base: g.Base, Head: g.Head, Draft: g.Draft}
}

func localPullRequestTemplate(workTree string) string {
	if workTree == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(workTree, ".github", "pull_request_template.md"),
		filepath.Join(workTree, ".github", "PULL_REQUEST_TEMPLATE.md"),
		filepath.Join(workTree, "docs", "pull_request_template.md"),
		filepath.Join(workTree, "pull_request_template.md"),
		filepath.Join(workTree, "PULL_REQUEST_TEMPLATE.md"),
	}
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			continue
		}
		contents, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimRight(string(contents), "\r\n")
		}
	}
	return ""
}

type ghResult struct {
	stdout          string
	stderr          string
	stdoutTruncated bool
	stderrTruncated bool
}

func (r *Repository) runGH(ctx context.Context, args []string, input []byte, sensitive []string, mutating bool) (ghResult, error) {
	if len(args) == 0 {
		return ghResult{}, errors.New("gh arguments are empty")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return ghResult{}, fmt.Errorf("%w: %w: %v", ErrGitHubUnavailable, ErrGitHubCLIUnavailable, err)
	}
	started := time.Now()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = r.commandDir
	cmd.Env = githubCommandEnv()
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	stdout, stderr := &limitedCapture{remaining: 1 << 20}, new(headTailCapture)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	runErr := cmd.Run()
	duration := time.Since(started)
	recordedArgs := redactGitHubArgs(args)
	recordedStdout, stdoutRedacted := redactCaptured(stdout.buf.String(), sensitive)
	recordedStderr, stderrRedacted := redactCaptured(stderr.String(), sensitive)
	result := ghResult{stdout: stdout.buf.String(), stderr: stderr.String(), stdoutTruncated: stdout.truncated, stderrTruncated: stderr.truncated}
	if mutating {
		if recorder, ok := ctx.Value(processRecorderKey{}).(func(ProcessRecord)); ok {
			recorder(ProcessRecord{Program: "gh", Dir: r.commandDir, Args: recordedArgs, Started: started, Duration: duration,
				ExitCode: processExitCode(ctx, runErr), Stdout: recordedStdout, Stderr: recordedStderr,
				StdoutTruncated: stdout.truncated || stdoutRedacted, StderrTruncated: stderr.truncated || stderrRedacted})
		}
	}
	if runErr != nil {
		commandErr := &GitHubCommandError{Args: recordedArgs, Err: runErr, Stderr: strings.TrimSpace(recordedStderr), StderrTruncated: stderr.truncated || stderrRedacted}
		return result, classifyGitHubError(commandErr)
	}
	if stdout.truncated {
		return result, errors.New("GitHub response exceeds 1 MiB")
	}
	return result, nil
}

func githubCommandEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GH_PROMPT_DISABLED", "GH_PAGER", "GH_FORCE_TTY", "GIT_TERMINAL_PROMPT", "LC_ALL":
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

func redactGitHubArgs(args []string) []string {
	result := append([]string(nil), args...)
	for i := range result {
		if result[i] == "--title" || result[i] == "--body" {
			if i+1 < len(result) {
				result[i+1] = redactionMarker
			}
		}
	}
	return result
}

func classifyGitHubError(err error) error {
	if err == nil {
		return nil
	}
	var commandErr *GitHubCommandError
	if !errors.As(err, &commandErr) {
		return err
	}
	message := strings.ToLower(commandErr.Stderr)
	switch {
	case strings.Contains(message, "not logged in"), strings.Contains(message, "authentication required"), strings.Contains(message, "gh auth login"), strings.Contains(message, "http 401"):
		return fmt.Errorf("%w: %w: %w", ErrGitHubUnavailable, ErrGitHubAuthentication, err)
	case strings.Contains(message, "http 403"), strings.Contains(message, "forbidden"), strings.Contains(message, "permission denied"), strings.Contains(message, "resource not accessible"), strings.Contains(message, "insufficient permission"):
		return fmt.Errorf("%w: %w: %w", ErrGitHubUnavailable, ErrGitHubPermission, err)
	default:
		return err
	}
}

var pullRequestURLPattern = regexp.MustCompile(`https?://[^[:space:]<>]+/pull/[0-9]+(?:[^[:space:]<>]*)?`)

func pullRequestURL(output string) string {
	match := pullRequestURLPattern.FindString(output)
	return strings.TrimRight(match, ".,;)")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
