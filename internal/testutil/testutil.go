// Package testutil holds the filesystem fixtures shared by the plugin's tests:
// fake git worktrees and the admin entries git keeps for them.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteFile writes content to path, creating parent directories.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Exists reports whether path exists, without following a final symlink.
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// NewWorktree makes a fake linked worktree: a dir with a `.git` file.
func NewWorktree(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "wt")
	WriteFile(t, filepath.Join(dir, ".git"), "gitdir: /elsewhere\n")
	return dir
}

// AddAdminDir records a linked worktree the way git does: the admin entry
// names the worktree's `.git` file, and that file names the admin entry back.
func AddAdminDir(t *testing.T, adminRoot, name, branch, worktree string) {
	t.Helper()
	admin := filepath.Join(adminRoot, name)
	WriteFile(t, filepath.Join(admin, "HEAD"), "ref: refs/heads/"+branch+"\n")
	WriteFile(t, filepath.Join(admin, "gitdir"), filepath.Join(worktree, ".git")+"\n")
	WriteFile(t, filepath.Join(worktree, ".git"), "gitdir: "+admin+"\n")
}
