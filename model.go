// Session shapes as sessions.list and session.lifecycle.changed deliver them,
// plus the path conventions aoe uses when it parks a trashed worktree.
package main

import (
	"encoding/json"
	"path/filepath"
)

// trashDirName is aoe's holding directory for trashed worktrees: a trashed
// managed worktree is moved to <parent>/.aoe-trash/<session-id>.
const trashDirName = ".aoe-trash"

const (
	kindTrashed          = "trashed"
	kindRestored         = "restored"
	kindDeleted          = "deleted"
	kindWorkspaceChanged = "workspace_changed"
)

type worktreeInfo struct {
	Branch       string `json:"branch"`
	MainRepoPath string `json:"main_repo_path"`
	ManagedByAOE bool   `json:"managed_by_aoe"`
}

type session struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	ProjectPath   string            `json:"project_path"`
	Trashed       bool              `json:"trashed"`
	WorkspaceRepo []json.RawMessage `json:"workspace_repos"`
	Worktree      *worktreeInfo     `json:"worktree"`
}

// lifecycleEvent is the params of session.lifecycle.changed. Summary is nil
// for a deleted session.
type lifecycleEvent struct {
	SessionID string   `json:"session_id"`
	Kind      string   `json:"kind"`
	Summary   *session `json:"summary"`
}

// managedWorktree reports whether the session is a single-repo worktree aoe
// owns -- the only kind this plugin ever touches. Plain sessions point at the
// user's real checkout and multi-repo workspaces span several repos.
func (s session) managedWorktree() bool {
	return s.Worktree != nil && s.Worktree.ManagedByAOE && len(s.WorkspaceRepo) <= 1
}

// holdingPath is where aoe parks a trashed worktree that lived at original.
func holdingPath(original, id string) string {
	return filepath.Join(filepath.Dir(original), trashDirName, id)
}

// isHoldingPath reports whether path is the trash holding path of session id.
func isHoldingPath(path, id string) bool {
	clean := filepath.Clean(path)
	return filepath.Base(clean) == id && filepath.Base(filepath.Dir(clean)) == trashDirName
}
