package main

import (
	"os"
	"path/filepath"
	"testing"
)

// newWorktree makes a fake linked worktree: a dir with a `.git` file.
func newWorktree(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wt")
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: /elsewhere\n")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestValidatePattern(t *testing.T) {
	bad := []string{"", "  ", "/etc", ".", "..", "../x", "a/../../x", ".git", ".git/config", "sub/.git", "./"}
	for _, p := range bad {
		if validatePattern(p) == nil {
			t.Errorf("validatePattern(%q) accepted", p)
		}
	}
	good := []string{"target", "crates/*/target", "node_modules", "a/../b", "./target"}
	for _, p := range good {
		if err := validatePattern(p); err != nil {
			t.Errorf("validatePattern(%q) = %v", p, err)
		}
	}
}

func TestRemovePatternDeletesMatches(t *testing.T) {
	dir := newWorktree(t)
	writeFile(t, filepath.Join(dir, "target/debug/big"), "x")
	writeFile(t, filepath.Join(dir, "crates/a/target/x"), "x")
	writeFile(t, filepath.Join(dir, "crates/b/target/x"), "x")
	writeFile(t, filepath.Join(dir, "crates/a/src/lib.rs"), "keep")

	for _, p := range []string{"target", "crates/*/target", "missing"} {
		if _, err := removePattern(dir, p); err != nil {
			t.Fatalf("removePattern(%q): %v", p, err)
		}
	}
	for _, gone := range []string{"target", "crates/a/target", "crates/b/target"} {
		if exists(filepath.Join(dir, gone)) {
			t.Errorf("%s still exists", gone)
		}
	}
	if !exists(filepath.Join(dir, "crates/a/src/lib.rs")) || !exists(filepath.Join(dir, ".git")) {
		t.Error("removed something it should have kept")
	}
}

func TestRemovePatternNeverTouchesGit(t *testing.T) {
	dir := newWorktree(t)
	writeFile(t, filepath.Join(dir, "keep"), "x")

	// A wildcard that matches `.git` must skip it but still delete the rest.
	removed, err := removePattern(dir, "*")
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || exists(filepath.Join(dir, "keep")) {
		t.Errorf("removed %d, keep exists=%v", removed, exists(filepath.Join(dir, "keep")))
	}
	if !exists(filepath.Join(dir, ".git")) {
		t.Error(".git was deleted")
	}
}

func TestRemovePatternDoesNotFollowSymlinkOutOfWorktree(t *testing.T) {
	dir := newWorktree(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "precious"), "x")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	// The glob walks through the symlinked directory; the match's parent
	// resolves outside the worktree, so it must be skipped.
	if _, err := removePattern(dir, "link/precious"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(outside, "precious")) {
		t.Fatal("deleted a file outside the worktree through a symlink")
	}

	// Deleting the symlink itself is fine and leaves its target alone.
	if _, err := removePattern(dir, "link"); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "link")) || !exists(filepath.Join(outside, "precious")) {
		t.Error("symlink removal touched its target")
	}
}

func TestCleanWorktreeRefusesNonWorktree(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "target/x"), "x")
	if err := cleanWorktree(dir, []string{"target"}); err == nil {
		t.Fatal("want error for a directory without .git")
	}
	if !exists(filepath.Join(dir, "target/x")) {
		t.Error("deleted inside a non-worktree")
	}
}

func TestCleanWorktreeContinuesAfterBadPath(t *testing.T) {
	dir := newWorktree(t)
	writeFile(t, filepath.Join(dir, "target/x"), "x")
	if err := cleanWorktree(dir, []string{"../escape", "target"}); err == nil {
		t.Error("want error for the rejected path")
	}
	if exists(filepath.Join(dir, "target")) {
		t.Error("a bad path stopped later paths from running")
	}
}
