// Package session holds the session shapes as sessions.list and
// session.lifecycle.changed deliver them, plus the path conventions aoe uses
// when it parks a trashed worktree.
package session

import (
	"encoding/json"
	"path/filepath"
)

// trashDirName is aoe's holding directory for trashed worktrees: a trashed
// managed worktree is moved to <parent>/.aoe-trash/<session-id>.
const trashDirName = ".aoe-trash"

// Lifecycle event kinds the plugin reacts to.
const (
	KindTrashed          = "trashed"
	KindRestored         = "restored"
	KindDeleted          = "deleted"
	KindWorkspaceChanged = "workspace_changed"
)

// WorktreeInfo is the `worktree` object of a worktree session.
type WorktreeInfo struct {
	Branch       string `json:"branch"`
	MainRepoPath string `json:"main_repo_path"`
	ManagedByAOE bool   `json:"managed_by_aoe"`
}

// Session is one entry of sessions.list, or the summary of a lifecycle event.
type Session struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	ProjectPath   string            `json:"project_path"`
	Trashed       bool              `json:"trashed"`
	WorkspaceRepo []json.RawMessage `json:"workspace_repos"`
	Worktree      *WorktreeInfo     `json:"worktree"`
}

// LifecycleEvent is the params of session.lifecycle.changed. Summary is nil
// for a deleted session.
type LifecycleEvent struct {
	SessionID string   `json:"session_id"`
	Kind      string   `json:"kind"`
	Summary   *Session `json:"summary"`
}

// ManagedWorktree reports whether the session is a single-repo worktree aoe
// owns -- the only kind this plugin ever touches. Plain sessions point at the
// user's real checkout and multi-repo workspaces span several repos.
func (s Session) ManagedWorktree() bool {
	return s.Worktree != nil && s.Worktree.ManagedByAOE && len(s.WorkspaceRepo) <= 1
}

// HoldingPath is where aoe parks a trashed worktree that lived at original.
func HoldingPath(original, id string) string {
	return filepath.Join(filepath.Dir(original), trashDirName, id)
}

// IsHoldingPath reports whether path is the trash holding path of session id.
func IsHoldingPath(path, id string) bool {
	clean := filepath.Clean(path)
	return filepath.Base(clean) == id && filepath.Base(filepath.Dir(clean)) == trashDirName
}
