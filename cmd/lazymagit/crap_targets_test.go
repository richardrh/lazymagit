package main

import (
	"context"
	"errors"
	"testing"

	gitbackend "github.com/richardrh/lazymagit/internal/git"
)

func TestRunWithDirectFailurePropagation(t *testing.T) {
	t.Run("argument error stops before discovery", func(t *testing.T) {
		discovered := false
		r, _, _, _ := testRuntime("", false)
		r.discover = func(string) (repository, error) {
			discovered = true
			return nil, nil
		}
		if err := runWith(context.Background(), []string{"--bad"}, r); err == nil || discovered {
			t.Fatalf("runWith argument error = %v, discovered=%t", err, discovered)
		}
	})

	t.Run("initialization error is retained", func(t *testing.T) {
		want := errors.New("disk full")
		r, _, _, uiCalls := testRuntime("", false)
		r.discover = func(string) (repository, error) { return nil, gitbackend.ErrNotRepository }
		r.init = func(context.Context, string) (repository, error) { return nil, want }
		err := runWith(context.Background(), []string{"--init", "repo"}, r)
		if !errors.Is(err, want) || *uiCalls != 0 {
			t.Fatalf("runWith init error = %v, UI calls=%d", err, *uiCalls)
		}
	})

	t.Run("UI error is retained with selected options", func(t *testing.T) {
		want := errors.New("terminal unavailable")
		var theme, layout string
		r, _, _, _ := testRuntime("", false)
		r.discover = func(string) (repository, error) {
			return fakeRepository{workTree: "/repo", gitDir: "/repo/.git"}, nil
		}
		r.startUI = func(_ repository, gotTheme, gotLayout string) error {
			theme, layout = gotTheme, gotLayout
			return want
		}
		err := runWith(context.Background(), []string{"--theme=night", "--layout=compact", "/repo"}, r)
		if !errors.Is(err, want) || theme != "night" || layout != "compact" {
			t.Fatalf("runWith UI error = %v, theme/layout = %q/%q", err, theme, layout)
		}
	})
}
