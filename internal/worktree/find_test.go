package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cwrau/aoe-plugin-session-cleanup/internal/testutil"
)

func TestFindByBranchRejectsOneWayLinks(t *testing.T) {
	repo := t.TempDir()
	adminRoot := filepath.Join(repo, ".git", "worktrees")

	// The admin entry points at a directory whose .git names some other admin dir.
	forged := filepath.Join(t.TempDir(), "forged")
	testutil.AddAdminDir(t, adminRoot, "forged", "feature/forged", forged)
	testutil.WriteFile(t, filepath.Join(forged, ".git"), "gitdir: /somewhere/else\n")
	if _, ok := FindByBranch(repo, "feature/forged"); ok {
		t.Error("accepted a worktree whose .git does not point back to the admin dir")
	}

	// The admin entry points at a main checkout, whose .git is a directory.
	main := filepath.Join(t.TempDir(), "main")
	testutil.AddAdminDir(t, adminRoot, "main", "feature/main", main)
	if err := os.Remove(filepath.Join(main, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(main, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindByBranch(repo, "feature/main"); ok {
		t.Error("accepted a directory .git, i.e. a main checkout")
	}

	// The worktree is gone entirely.
	gone := filepath.Join(t.TempDir(), "gone")
	testutil.AddAdminDir(t, adminRoot, "gone", "feature/gone", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindByBranch(repo, "feature/gone"); ok {
		t.Error("accepted a worktree that no longer exists")
	}
}

func TestFindByBranch(t *testing.T) {
	repo := t.TempDir()
	wtA := filepath.Join(t.TempDir(), "a")
	wtB := filepath.Join(t.TempDir(), "b")
	testutil.AddAdminDir(t, filepath.Join(repo, ".git", "worktrees"), "a", "feature/a", wtA)
	testutil.AddAdminDir(t, filepath.Join(repo, ".git", "worktrees"), "b", "feature/b", wtB)

	got, ok := FindByBranch(repo, "feature/b")
	if !ok || got != wtB {
		t.Errorf("got %q, %v; want %q", got, ok, wtB)
	}
	if _, ok := FindByBranch(repo, "feature/missing"); ok {
		t.Error("found a worktree for a branch nothing has checked out")
	}
}

func TestFindByBranchBareRepoLayout(t *testing.T) {
	repo := t.TempDir()
	wt := filepath.Join(t.TempDir(), "w")
	testutil.AddAdminDir(t, filepath.Join(repo, "worktrees"), "w", "fix/x", wt)

	if got, ok := FindByBranch(repo, "fix/x"); !ok || got != wt {
		t.Errorf("got %q, %v; want %q", got, ok, wt)
	}
}

func TestFindByBranchIgnoresBadInput(t *testing.T) {
	repo := t.TempDir()
	if _, ok := FindByBranch(repo, "x"); ok {
		t.Error("found a worktree in a repo without any")
	}
	if _, ok := FindByBranch("", "x"); ok {
		t.Error("empty repo matched")
	}
	if _, ok := FindByBranch(repo, ""); ok {
		t.Error("empty branch matched")
	}
	// A relative gitdir is not something git writes; do not trust it.
	admin := filepath.Join(repo, ".git", "worktrees", "r")
	testutil.WriteFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/x\n")
	testutil.WriteFile(t, filepath.Join(admin, "gitdir"), "relative/.git\n")
	if _, ok := FindByBranch(repo, "x"); ok {
		t.Error("trusted a relative gitdir")
	}
	// A detached HEAD names no branch.
	testutil.WriteFile(t, filepath.Join(admin, "HEAD"), "0123456789abcdef\n")
	if _, ok := FindByBranch(repo, "x"); ok {
		t.Error("matched a detached HEAD")
	}
}
