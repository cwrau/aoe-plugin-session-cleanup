package main

import (
	"os"
	"path/filepath"
	"testing"
)

// addAdminDir records a linked worktree the way git does: the admin entry
// names the worktree's `.git` file, and that file names the admin entry back.
func addAdminDir(t *testing.T, adminRoot, name, branch, worktree string) {
	t.Helper()
	admin := filepath.Join(adminRoot, name)
	writeFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/"+branch+"\n")
	writeFile(t, filepath.Join(admin, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: "+admin+"\n")
}

func TestFindWorktreeByBranchRejectsOneWayLinks(t *testing.T) {
	repo := t.TempDir()
	adminRoot := filepath.Join(repo, ".git", "worktrees")

	// The admin entry points at a directory whose .git names some other admin dir.
	forged := filepath.Join(t.TempDir(), "forged")
	addAdminDir(t, adminRoot, "forged", "feature/forged", forged)
	writeFile(t, filepath.Join(forged, ".git"), "gitdir: /somewhere/else\n")
	if _, ok := findWorktreeByBranch(repo, "feature/forged"); ok {
		t.Error("accepted a worktree whose .git does not point back to the admin dir")
	}

	// The admin entry points at a main checkout, whose .git is a directory.
	main := filepath.Join(t.TempDir(), "main")
	addAdminDir(t, adminRoot, "main", "feature/main", main)
	if err := os.Remove(filepath.Join(main, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(main, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := findWorktreeByBranch(repo, "feature/main"); ok {
		t.Error("accepted a directory .git, i.e. a main checkout")
	}

	// The worktree is gone entirely.
	gone := filepath.Join(t.TempDir(), "gone")
	addAdminDir(t, adminRoot, "gone", "feature/gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if _, ok := findWorktreeByBranch(repo, "feature/gone"); ok {
		t.Error("accepted a worktree that no longer exists")
	}
}

func TestFindWorktreeByBranch(t *testing.T) {
	repo := t.TempDir()
	wtA := filepath.Join(t.TempDir(), "a")
	wtB := filepath.Join(t.TempDir(), "b")
	addAdminDir(t, filepath.Join(repo, ".git", "worktrees"), "a", "feature/a", wtA)
	addAdminDir(t, filepath.Join(repo, ".git", "worktrees"), "b", "feature/b", wtB)

	got, ok := findWorktreeByBranch(repo, "feature/b")
	if !ok || got != wtB {
		t.Errorf("got %q, %v; want %q", got, ok, wtB)
	}
	if _, ok := findWorktreeByBranch(repo, "feature/missing"); ok {
		t.Error("found a worktree for a branch nothing has checked out")
	}
}

func TestFindWorktreeByBranchBareRepoLayout(t *testing.T) {
	repo := t.TempDir()
	wt := filepath.Join(t.TempDir(), "w")
	addAdminDir(t, filepath.Join(repo, "worktrees"), "w", "fix/x", wt)

	if got, ok := findWorktreeByBranch(repo, "fix/x"); !ok || got != wt {
		t.Errorf("got %q, %v; want %q", got, ok, wt)
	}
}

func TestFindWorktreeByBranchIgnoresBadInput(t *testing.T) {
	repo := t.TempDir()
	if _, ok := findWorktreeByBranch(repo, "x"); ok {
		t.Error("found a worktree in a repo without any")
	}
	if _, ok := findWorktreeByBranch("", "x"); ok {
		t.Error("empty repo matched")
	}
	if _, ok := findWorktreeByBranch(repo, ""); ok {
		t.Error("empty branch matched")
	}
	// A relative gitdir is not something git writes; do not trust it.
	admin := filepath.Join(repo, ".git", "worktrees", "r")
	writeFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/x\n")
	writeFile(t, filepath.Join(admin, "gitdir"), "relative/.git\n")
	if _, ok := findWorktreeByBranch(repo, "x"); ok {
		t.Error("trusted a relative gitdir")
	}
	// A detached HEAD names no branch.
	writeFile(t, filepath.Join(admin, "HEAD"), "0123456789abcdef\n")
	if _, ok := findWorktreeByBranch(repo, "x"); ok {
		t.Error("matched a detached HEAD")
	}
}
