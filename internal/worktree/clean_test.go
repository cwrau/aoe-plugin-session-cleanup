package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cwrau/aoe-plugin-session-cleanup/internal/testutil"
)

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
	dir := testutil.NewWorktree(t)
	testutil.WriteFile(t, filepath.Join(dir, "target/debug/big"), "x")
	testutil.WriteFile(t, filepath.Join(dir, "crates/a/target/x"), "x")
	testutil.WriteFile(t, filepath.Join(dir, "crates/b/target/x"), "x")
	testutil.WriteFile(t, filepath.Join(dir, "crates/a/src/lib.rs"), "keep")

	for _, p := range []string{"target", "crates/*/target", "missing"} {
		if _, err := removePattern(dir, p); err != nil {
			t.Fatalf("removePattern(%q): %v", p, err)
		}
	}
	for _, gone := range []string{"target", "crates/a/target", "crates/b/target"} {
		if testutil.Exists(filepath.Join(dir, gone)) {
			t.Errorf("%s still exists", gone)
		}
	}
	if !testutil.Exists(filepath.Join(dir, "crates/a/src/lib.rs")) || !testutil.Exists(filepath.Join(dir, ".git")) {
		t.Error("removed something it should have kept")
	}
}

func TestRemovePatternNeverTouchesGit(t *testing.T) {
	dir := testutil.NewWorktree(t)
	testutil.WriteFile(t, filepath.Join(dir, "keep"), "x")

	// A wildcard that matches `.git` must skip it but still delete the rest.
	removed, err := removePattern(dir, "*")
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || testutil.Exists(filepath.Join(dir, "keep")) {
		t.Errorf("removed %d, keep exists=%v", removed, testutil.Exists(filepath.Join(dir, "keep")))
	}
	if !testutil.Exists(filepath.Join(dir, ".git")) {
		t.Error(".git was deleted")
	}
}

func TestRemovePatternDoesNotFollowSymlinkOutOfWorktree(t *testing.T) {
	dir := testutil.NewWorktree(t)
	outside := t.TempDir()
	testutil.WriteFile(t, filepath.Join(outside, "precious"), "x")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	// The glob walks through the symlinked directory; the match's parent
	// resolves outside the worktree, so it must be skipped.
	if _, err := removePattern(dir, "link/precious"); err != nil {
		t.Fatal(err)
	}
	if !testutil.Exists(filepath.Join(outside, "precious")) {
		t.Fatal("deleted a file outside the worktree through a symlink")
	}

	// Deleting the symlink itself is fine and leaves its target alone.
	if _, err := removePattern(dir, "link"); err != nil {
		t.Fatal(err)
	}
	if testutil.Exists(filepath.Join(dir, "link")) || !testutil.Exists(filepath.Join(outside, "precious")) {
		t.Error("symlink removal touched its target")
	}
}

func TestCleanRefusesNonWorktree(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "target/x"), "x")
	if err := Clean(dir, []string{"target"}); err == nil {
		t.Fatal("want error for a directory without .git")
	}
	if !testutil.Exists(filepath.Join(dir, "target/x")) {
		t.Error("deleted inside a non-worktree")
	}
}

func TestCleanContinuesAfterBadPath(t *testing.T) {
	dir := testutil.NewWorktree(t)
	testutil.WriteFile(t, filepath.Join(dir, "target/x"), "x")
	if err := Clean(dir, []string{"../escape", "target"}); err == nil {
		t.Error("want error for the rejected path")
	}
	if testutil.Exists(filepath.Join(dir, "target")) {
		t.Error("a bad path stopped later paths from running")
	}
}
