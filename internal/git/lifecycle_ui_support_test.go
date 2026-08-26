package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneRepositoryForUICleanupOwnershipContract(t *testing.T) {
	missingSource := filepath.Join(t.TempDir(), "missing source")
	createdDestination := filepath.Join(t.TempDir(), "new clone")
	if err := CloneRepositoryForUI(context.Background(), missingSource, createdDestination, CloneOptions{}); err == nil {
		t.Fatal("clone from missing source succeeded")
	}
	if _, err := os.Lstat(createdDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new partial destination was not removed: %v", err)
	}

	ownedDestination := filepath.Join(t.TempDir(), "owned empty directory")
	if err := os.Mkdir(ownedDestination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CloneRepositoryForUI(context.Background(), missingSource, ownedDestination, CloneOptions{}); err == nil {
		t.Fatal("clone from missing source succeeded")
	}
	if info, err := os.Stat(ownedDestination); err != nil || !info.IsDir() {
		t.Fatalf("caller-owned empty directory was removed: %v", err)
	}

	cancelledDestination := filepath.Join(t.TempDir(), "cancelled clone")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CloneRepositoryForUI(ctx, missingSource, cancelledDestination, CloneOptions{}); err == nil {
		t.Fatal("cancelled clone succeeded")
	}
	if _, err := os.Lstat(cancelledDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled partial destination was not removed: %v", err)
	}
}

func TestInitRepositoryForUIPreservesContentsAndRefusesOverwrite(t *testing.T) {
	destination := t.TempDir()
	kept := filepath.Join(destination, "keep.txt")
	if err := os.WriteFile(kept, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitRepositoryForUI(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(kept); err != nil || string(content) != "keep\n" {
		t.Fatalf("ordinary destination content changed: %q, %v", content, err)
	}
	if err := InitRepositoryForUI(context.Background(), destination); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("repository reinitialization error = %v", err)
	}
}
