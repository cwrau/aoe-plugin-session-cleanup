// Cleaning one worktree directory: delete the configured paths, every deletion
// confined to the worktree.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// isWorktreeDir reports whether dir looks like a git worktree: a linked
// worktree has a `.git` file, a main checkout a `.git` directory.
func isWorktreeDir(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// validatePattern rejects patterns that could reach outside the worktree or
// touch git metadata.
func validatePattern(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return errors.New("empty path")
	}
	if filepath.IsAbs(pattern) {
		return errors.New("absolute paths are not allowed")
	}
	clean := filepath.Clean(pattern)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("path must stay inside the worktree")
	}
	if hasGitComponent(clean) {
		return errors.New(".git is never deleted")
	}
	return nil
}

func hasGitComponent(rel string) bool {
	return slices.Contains(strings.Split(rel, string(filepath.Separator)), ".git")
}

// removePattern deletes everything in dir matching the glob pattern and
// returns how many top-level matches it removed. Matches that are git
// metadata, or whose parent resolves (through symlinks) outside dir, are
// skipped rather than deleted. The deletion itself goes through an os.Root on
// the worktree, so a symlink swapped in between the check and the delete cannot
// redirect it outside the worktree.
func removePattern(dir, pattern string) (int, error) {
	if err := validatePattern(pattern); err != nil {
		return 0, err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return 0, fmt.Errorf("resolve worktree: %w", err)
	}
	root, err := os.OpenRoot(realDir)
	if err != nil {
		return 0, fmt.Errorf("open worktree: %w", err)
	}
	defer root.Close()
	matches, err := filepath.Glob(filepath.Join(dir, filepath.Clean(pattern)))
	if err != nil {
		return 0, fmt.Errorf("bad pattern: %w", err)
	}

	var removed int
	var errs []error
	for _, match := range matches {
		rel, err := filepath.Rel(dir, match)
		if err != nil || hasGitComponent(rel) {
			continue
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(match))
		if err != nil || !within(realDir, parent) {
			continue
		}
		if err := root.RemoveAll(rel); err != nil {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}

// within reports whether path is root or below it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// cleanWorktree deletes every pattern's matches in the worktree at dir. A bad
// pattern does not stop the rest; all failures are joined into the returned
// error.
func cleanWorktree(dir string, patterns []string) error {
	if !isWorktreeDir(dir) {
		return fmt.Errorf("%s is not a git worktree; refusing to clean it", dir)
	}
	var errs []error
	for _, pattern := range patterns {
		removed, err := removePattern(dir, pattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("path %q: %w", pattern, err))
		}
		logf("%q removed %d match(es) in %s", pattern, removed, dir)
	}
	return errors.Join(errs...)
}
