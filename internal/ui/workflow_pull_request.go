package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richardrh/lazymagit/internal/git"
	"github.com/richardrh/lazymagit/internal/keymap"
)

func init() {
	RegisterWorkflowDomain(func(m *Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{keymap.CommandPullRequest: openPullRequestWorkflow}
	})
}

func openPullRequestWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("GitHub pull request", func(ctx context.Context) (WorkflowDialog, error) {
		current, err := m.repo.PullRequestForUI(ctx)
		if err != nil {
			return WorkflowDialog{}, err
		}
		title := "Create GitHub pull request"
		if current.Number != 0 {
			title = fmt.Sprintf("Edit GitHub PR #%d", current.Number)
		}
		build := func(values WorkflowValues) gitbackend.PullRequest {
			request := current
			subject, body, _ := strings.Cut(values["pr-message"], "\n")
			request.Title, request.Body = strings.TrimSpace(subject), strings.TrimLeft(body, "\n")
			request.Base, request.Draft = strings.TrimSpace(values["base"]), values["draft"] == "true"
			return request
		}
		return WorkflowDialog{
			Title: title, Operation: "publish pull request", ActionLabel: "Publish PR",
			Fields: []WorkflowField{
				{Name: "base", Label: "Base branch", Kind: WorkflowText, Value: current.Base, Required: true},
				{Name: "draft", Label: "Draft PR", Kind: WorkflowBool, Bool: current.Draft},
				{Name: "pr-message", Label: "Title and Markdown body", Kind: WorkflowMultiline, Value: current.Title + "\n\n" + current.Body, Required: true},
			},
			Message: &MessageWorkflow{
				DraftPath: messageDraftPath(m.repo.GitDir(), "pull-request/"+current.Head),
				Context:   "GitHub · head " + current.Head + " · no automatic push",
				Result:    func() string { return "Pull request saved: " + current.URL },
			},
			Validate: func(values WorkflowValues) error {
				if build(values).Title == "" {
					return errors.New("first line must contain a PR title")
				}
				return nil
			},
			ReviewPreflight: func(_ context.Context, values WorkflowValues) (WorkflowReview, error) {
				request := build(values)
				action := "Create pull request"
				if request.Number != 0 {
					action = fmt.Sprintf("Update pull request #%d", request.Number)
				}
				return WorkflowReview{
					Confirmation: "Publish only after checking the title, base/head branches, and draft state",
					Plan:         []string{action, request.Title, request.Base + " ← " + request.Head, fmt.Sprintf("Draft: %t", request.Draft), "No automatic push.", "", "Description:", request.Body},
					Data:         request,
				}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, review WorkflowReview) error {
				request, ok := review.Data.(gitbackend.PullRequest)
				if !ok {
					return errors.New("invalid reviewed pull request")
				}
				result, err := m.repo.SubmitPullRequest(ctx, request)
				// Keep the remote identity after a partial success, so retry edits
				// the existing PR rather than publishing a duplicate.
				if result.Number != 0 {
					current = result
				}
				return err
			},
		}, nil
	})
}
