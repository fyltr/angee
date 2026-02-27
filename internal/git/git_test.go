package git

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	r := New(dir)
	if err := r.Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := r.ConfigureUser("test", "test@test.com"); err != nil {
		t.Fatalf("ConfigureUser() error: %v", err)
	}
	return r
}

func commitFile(t *testing.T, r *Repo, name, content, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(r.Path, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(name); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	sha, err := r.Commit(msg)
	if err != nil {
		t.Fatalf("Commit() error: %v", err)
	}
	return sha
}

func TestInitAndCommit(t *testing.T) {
	r := newTestRepo(t)
	sha := commitFile(t, r, "test.txt", "hello", "initial commit")

	if sha == "" {
		t.Error("expected non-empty SHA")
	}
	if len(sha) < 7 {
		t.Errorf("SHA = %q, expected at least 7 chars", sha)
	}
}

func TestHasChanges(t *testing.T) {
	r := newTestRepo(t)
	commitFile(t, r, "test.txt", "hello", "initial commit")

	// Clean state
	has, err := r.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges() error: %v", err)
	}
	if has {
		t.Error("expected no changes after commit")
	}

	// Make a change
	if err := os.WriteFile(filepath.Join(r.Path, "test.txt"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	has, err = r.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges() error: %v", err)
	}
	if !has {
		t.Error("expected changes after modifying file")
	}
}

func TestLog(t *testing.T) {
	r := newTestRepo(t)
	commitFile(t, r, "a.txt", "aaa", "first commit")
	commitFile(t, r, "b.txt", "bbb", "second commit")
	commitFile(t, r, "c.txt", "ccc", "third commit")

	commits, err := r.Log(10)
	if err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("Log() returned %d commits, want 3", len(commits))
	}

	// Most recent first
	if commits[0].Message != "third commit" {
		t.Errorf("commits[0].Message = %q, want %q", commits[0].Message, "third commit")
	}
	if commits[2].Message != "first commit" {
		t.Errorf("commits[2].Message = %q, want %q", commits[2].Message, "first commit")
	}
}

func TestCurrentBranch(t *testing.T) {
	r := newTestRepo(t)
	commitFile(t, r, "test.txt", "hello", "init")

	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch() error: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}
}

func TestCheckout(t *testing.T) {
	r := newTestRepo(t)
	commitFile(t, r, "test.txt", "hello", "init")

	// Create and switch to new branch
	if err := r.Checkout("feature", true); err != nil {
		t.Fatalf("Checkout() error: %v", err)
	}

	branch, err := r.CurrentBranch()
	if err != nil {
		t.Fatal(err)
	}
	if branch != "feature" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "feature")
	}

	// Switch back
	if err := r.Checkout("main", false); err != nil {
		t.Fatalf("Checkout(main) error: %v", err)
	}
	branch, _ = r.CurrentBranch()
	if branch != "main" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "main")
	}
}

func TestRevert(t *testing.T) {
	r := newTestRepo(t)
	commitFile(t, r, "test.txt", "original", "first")
	sha2 := commitFile(t, r, "test.txt", "changed", "second")

	// Revert the second commit
	if err := r.Revert(sha2); err != nil {
		t.Fatalf("Revert() error: %v", err)
	}

	// File should be back to original
	data, err := os.ReadFile(filepath.Join(r.Path, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Errorf("file content = %q, want %q after revert", string(data), "original")
	}
}

func TestIsRepo(t *testing.T) {
	r := newTestRepo(t)
	if !IsRepo(r.Path) {
		t.Error("expected IsRepo=true for initialized repo")
	}

	empty := t.TempDir()
	if IsRepo(empty) {
		t.Error("expected IsRepo=false for empty dir")
	}
}
