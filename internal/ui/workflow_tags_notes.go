package ui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	gitbackend "github.com/richard/lazymagit/internal/git"
	"github.com/richard/lazymagit/internal/keymap"
)

const (
	tagCreateID   keymap.CommandID = "tag.tag-create"
	tagReleaseID  keymap.CommandID = "tag.tag-release"
	tagDeleteID   keymap.CommandID = "tag.tag-delete"
	tagPruneID    keymap.CommandID = "tag.tag-prune"
	tagForceID    keymap.CommandID = "tag.--force"
	tagAnnotateID keymap.CommandID = "tag.--annotate"
	tagSignID     keymap.CommandID = "tag.--sign"

	notesEditID          keymap.CommandID = "notes.notes-edit"
	notesRemoveID        keymap.CommandID = "notes.notes-remove"
	notesMergeID         keymap.CommandID = "notes.notes-merge"
	notesPruneID         keymap.CommandID = "notes.notes-prune"
	notesMergeContinueID keymap.CommandID = "notes.notes-merge-commit"
	notesMergeAbortID    keymap.CommandID = "notes.notes-merge-abort"
	notesRefID           keymap.CommandID = "notes.--ref"
	notesDryRunID        keymap.CommandID = "notes.--dry-run"
)

func init() {
	RegisterWorkflowDomain(func(*Model) map[keymap.CommandID]WorkflowHandler {
		return map[keymap.CommandID]WorkflowHandler{
			tagCreateID: tagCreateWorkflow(false), tagReleaseID: tagCreateWorkflow(true),
			tagDeleteID: tagDeleteWorkflow, tagPruneID: tagPruneWorkflow,
			notesEditID: notesEditWorkflow, notesRemoveID: notesRemoveWorkflow,
			notesMergeID: notesMergeWorkflow, notesPruneID: notesPruneWorkflow,
			notesMergeContinueID: notesMergeContinueWorkflow, notesMergeAbortID: notesMergeAbortWorkflow,
		}
	})
}

func tagCreateWorkflow(release bool) WorkflowHandler {
	return func(m *Model, command WorkflowCommand) tea.Cmd {
		kind := "lightweight"
		if release || command.Options[tagAnnotateID].Enabled {
			kind = "annotated"
		}
		if command.Options[tagSignID].Enabled {
			kind = "signed"
		}
		force := command.Options[tagForceID].Enabled
		return m.OpenWorkflow(WorkflowDialog{
			Title: "Create tag", Operation: "create tag",
			Fields: []WorkflowField{
				{Name: "name", Label: "Tag name", Kind: WorkflowText, Required: true},
				{Name: "target", Label: "Target", Kind: WorkflowText, Value: "HEAD", Required: true},
				{Name: "kind", Label: "Tag kind", Kind: WorkflowEnum, Value: kind, Choices: []WorkflowChoice{{Value: "lightweight", Label: "lightweight"}, {Value: "annotated", Label: "annotated"}, {Value: "signed", Label: "signed"}}},
				{Name: "message", Label: "Message (internal editor)", Kind: WorkflowText},
				{Name: "sign-consent", Label: "Allow interactive signing", Kind: WorkflowBool},
			},
			Validate: func(v WorkflowValues) error {
				if strings.ContainsAny(v["name"], "\x00\r\n") || strings.ContainsAny(v["target"], "\x00\r\n") {
					return errors.New("tag name and target must be single-line values")
				}
				if v["kind"] != "lightweight" && v["message"] == "" {
					return errors.New("annotated tag message is required")
				}
				if v["kind"] == "signed" && v["sign-consent"] != "true" {
					return errors.New("signed tags require explicit interactive consent")
				}
				if v["kind"] == "lightweight" && v["message"] != "" {
					return errors.New("lightweight tags cannot have a message")
				}
				return nil
			},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				args := tagArgs(v, force)
				review, err := m.repo.ReviewTagCreate(ctx, args)
				if err != nil {
					return WorkflowReview{}, err
				}
				plan := []string{"tag: " + args.Name, "target: " + args.Target, "kind: " + v["kind"]}
				if review.PreviousID != "" {
					plan = append(plan, "replace object: "+review.PreviousID)
				}
				return WorkflowReview{Plan: plan, Confirmation: "Create exactly this tag?", Data: review}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
				review, ok := wr.Data.(gitbackend.ReviewedTagCreate)
				if !ok {
					return errors.New("invalid tag creation review")
				}
				return m.repo.CreateTagReviewed(ctx, review)
			},
		})
	}
}

func tagArgs(v WorkflowValues, force bool) gitbackend.CreateTagArgs {
	return gitbackend.CreateTagArgs{Name: strings.TrimSpace(v["name"]), Target: strings.TrimSpace(v["target"]), Annotated: v["kind"] != "lightweight", Sign: v["kind"] == "signed", Message: v["message"], Force: force}
}

func tagChoices(ctx context.Context, m *Model) ([]WorkflowChoice, error) {
	tags, err := m.repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, errors.New("no tags")
	}
	choices := make([]WorkflowChoice, 0, len(tags))
	for _, tag := range tags {
		choices = append(choices, WorkflowChoice{Value: tag.Name, Label: tag.Name + "  " + tag.ObjectID})
	}
	return choices, nil
}

func tagDeleteWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("tag deletion", func(ctx context.Context) (WorkflowDialog, error) {
		choices, err := tagChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Delete tag", Operation: "delete tag", Fields: []WorkflowField{{Name: "tag", Label: "Tag", Kind: WorkflowSelect, Value: choices[0].Value, Choices: choices}},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				p, err := m.repo.ReviewTagDelete(ctx, []string{v["tag"]})
				if err != nil {
					return WorkflowReview{}, err
				}
				return WorkflowReview{Plan: []string{"delete " + p.Names[0] + " at " + p.ObjectIDs[p.Names[0]]}, Confirmation: "Delete this exact tag?", Data: p}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
				p, ok := wr.Data.(gitbackend.ReviewedTagDelete)
				if !ok {
					return errors.New("invalid tag deletion review")
				}
				return m.repo.DeleteTagsReviewed(ctx, p)
			}}, nil
	})
}

func tagPruneWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.LoadWorkflow("tag prune", func(ctx context.Context) (WorkflowDialog, error) {
		remotes, err := remoteChoices(ctx, m)
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Prune local tags absent from remote", Operation: "prune tags", Fields: []WorkflowField{remoteSelectField(remotes)},
			ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
				p, err := m.repo.ReviewRemoteTagPrune(ctx, v["remote"])
				if err != nil {
					return WorkflowReview{}, err
				}
				lines := []string{"remote: " + p.Comparison.Remote}
				for _, n := range p.Comparison.LocalOnly {
					lines = append(lines, "delete "+n+" at "+p.ObjectIDs[n])
				}
				if len(p.Comparison.LocalOnly) == 0 {
					lines = append(lines, "No local-only tags")
				}
				return WorkflowReview{Plan: lines, Confirmation: "Prune this reviewed set?", Data: p}, nil
			},
			SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
				p, ok := wr.Data.(gitbackend.ReviewedRemoteTagPrune)
				if !ok {
					return errors.New("invalid tag prune review")
				}
				return m.repo.PruneRemoteTagsReviewed(ctx, p)
			}}, nil
	})
}

func notesEditWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	ref := command.Options[notesRefID].Value
	return m.LoadWorkflow("note editor", func(ctx context.Context) (WorkflowDialog, error) {
		message, _, err := m.repo.NotesMessage(ctx, ref, "HEAD")
		if err != nil {
			return WorkflowDialog{}, err
		}
		return WorkflowDialog{Title: "Add or edit note", Operation: "write note", Fields: []WorkflowField{{Name: "ref", Label: "Notes ref (blank = commits)", Kind: WorkflowText, Value: ref}, {Name: "object", Label: "Object", Kind: WorkflowText, Value: "HEAD", Required: true}, {Name: "message", Label: "Message (internal editor, max 64 KiB)", Kind: WorkflowText, Value: message, Required: true}},
			Validate: func(v WorkflowValues) error {
				if len(v["message"]) > gitbackend.MaxNotesUIMessageBytes {
					return errors.New("note message exceeds 64 KiB")
				}
				return nil
			},
			Submit: func(ctx context.Context, v WorkflowValues) error {
				ref, object := strings.TrimSpace(v["ref"]), strings.TrimSpace(v["object"])
				_, exists, err := m.repo.NotesMessage(ctx, ref, object)
				if err != nil {
					return err
				}
				return m.repo.NotesWriteMessage(ctx, ref, object, v["message"], exists)
			}}, nil
	})
}

func notesRemoveWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Remove note", Operation: "remove note", Fields: []WorkflowField{{Name: "ref", Label: "Notes ref (blank = commits)", Kind: WorkflowText, Value: command.Options[notesRefID].Value}, {Name: "object", Label: "Object", Kind: WorkflowText, Value: "HEAD", Required: true}}, ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
		p, err := m.repo.ReviewNotesRemoval(ctx, strings.TrimSpace(v["ref"]), []string{strings.TrimSpace(v["object"])})
		if err != nil {
			return WorkflowReview{}, err
		}
		return WorkflowReview{Plan: []string{"notes ref: " + p.Ref, "remove from object: " + p.ObjectIDs[0], "notes ref object: " + p.NotesOID}, Confirmation: "Remove this exact note?", Data: p}, nil
	}, SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
		p, ok := wr.Data.(gitbackend.ReviewedNotesRemoval)
		if !ok {
			return errors.New("invalid notes removal review")
		}
		return m.repo.RemoveNotesReviewed(ctx, p)
	}})
}

func notesPruneWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	dryRun := command.Options[notesDryRunID].Enabled
	return m.OpenWorkflow(WorkflowDialog{Title: "Prune unreachable notes", Operation: "prune notes", Fields: []WorkflowField{{Name: "ref", Label: "Notes ref (blank = commits)", Kind: WorkflowText, Value: command.Options[notesRefID].Value}}, ReviewPreflight: func(ctx context.Context, v WorkflowValues) (WorkflowReview, error) {
		p, err := m.repo.ReviewNotesPrune(ctx, strings.TrimSpace(v["ref"]))
		if err != nil {
			return WorkflowReview{}, err
		}
		lines := []string{"notes ref object: " + p.NotesOID}
		for _, oid := range p.Objects {
			lines = append(lines, "prune note object: "+oid)
		}
		if len(p.Objects) == 0 {
			lines = append(lines, "No unreachable notes")
		}
		if dryRun {
			lines = append(lines, "Dry run: no refs will be changed")
		}
		return WorkflowReview{Plan: lines, Confirmation: "Prune this reviewed set?", Data: p}, nil
	}, SubmitReview: func(ctx context.Context, _ WorkflowValues, wr WorkflowReview) error {
		p, ok := wr.Data.(gitbackend.ReviewedNotesPrune)
		if !ok {
			return errors.New("invalid notes prune review")
		}
		if dryRun {
			return nil
		}
		return m.repo.PruneNotesReviewed(ctx, p)
	}})
}

func notesMergeWorkflow(m *Model, command WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Merge notes", Operation: "merge notes", Fields: []WorkflowField{{Name: "ref", Label: "Destination notes ref (blank = commits)", Kind: WorkflowText, Value: command.Options[notesRefID].Value}, {Name: "source", Label: "Source notes ref", Kind: WorkflowText, Required: true}}, Submit: func(ctx context.Context, v WorkflowValues) error {
		return m.repo.NotesMergeStart(ctx, strings.TrimSpace(v["ref"]), strings.TrimSpace(v["source"]))
	}})
}

func notesMergeContinueWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.StartWorkflowOperation("continue notes merge", func(ctx context.Context) error { return m.repo.NotesMergeContinue(ctx, "") })
}
func notesMergeAbortWorkflow(m *Model, _ WorkflowCommand) tea.Cmd {
	return m.OpenWorkflow(WorkflowDialog{Title: "Abort notes merge", Operation: "abort notes merge", Confirmation: "Discard the in-progress notes merge?", Plan: []string{"abort current notes merge"}, Submit: func(ctx context.Context, _ WorkflowValues) error { return m.repo.NotesMergeAbort(ctx, "") }})
}
