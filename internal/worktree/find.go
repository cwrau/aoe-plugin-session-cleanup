package worktree

// Locating a session's worktree from git's own bookkeeping, for when the path
// the plugin last saw is stale. Every linked worktree has an admin directory
// under `<repo>/.git/worktrees/<name>` (`<repo>/worktrees/<name>` in a bare
// repo) whose `HEAD` names the checked-out branch and whose `gitdir` file
// points at the worktree's `.git` file. Reading those files needs no git
// process.

import (
	"os"
	"path/filepath"
	"strings"
)

const branchRefPrefix = "ref: refs/heads/"

// FindByBranch returns the directory of the linked worktree of repo that has
// branch checked out. A branch can be checked out in only one worktree, so the
// match is unique.
func FindByBranch(repo, branch string) (string, bool) {
	if repo == "" || branch == "" {
		return "", false
	}
	for _, root := range []string{
		filepath.Join(repo, ".git", "worktrees"),
		filepath.Join(repo, "worktrees"),
	} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			admin := filepath.Join(root, entry.Name())
			if dir, ok := worktreeOfAdminDir(admin, branch); ok {
				return dir, true
			}
		}
	}
	return "", false
}

// worktreeOfAdminDir returns the worktree directory recorded in one admin
// directory when its HEAD is branch.
func worktreeOfAdminDir(admin, branch string) (string, bool) {
	head, err := os.ReadFile(filepath.Join(admin, "HEAD"))
	if err != nil || strings.TrimSpace(string(head)) != branchRefPrefix+branch {
		return "", false
	}
	gitdir, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	if err != nil {
		return "", false
	}
	dotGit := strings.TrimSpace(string(gitdir))
	if dotGit == "" || !filepath.IsAbs(dotGit) || !pointsBack(dotGit, admin) {
		return "", false
	}
	return filepath.Dir(dotGit), true
}

// pointsBack verifies the link from the other side: dotGit must be a regular
// file (a linked worktree's, never a main checkout's `.git` directory) whose
// `gitdir:` line names admin. Without this, a forged or stale admin entry could
// steer a deletion at an arbitrary directory.
func pointsBack(dotGit, admin string) bool {
	info, err := os.Lstat(dotGit)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	content, err := os.ReadFile(dotGit)
	if err != nil {
		return false
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(content)), "gitdir:")
	if !ok {
		return false
	}
	return sameDir(strings.TrimSpace(target), admin)
}

// sameDir reports whether a and b are the same directory, resolving symlinks.
func sameDir(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		return false
	}
	return ra == rb
}
