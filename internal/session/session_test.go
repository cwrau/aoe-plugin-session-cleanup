package session

import (
	"encoding/json"
	"testing"
)

func TestManagedWorktree(t *testing.T) {
	managed := &WorktreeInfo{ManagedByAOE: true}
	tests := []struct {
		name string
		s    Session
		want bool
	}{
		{"managed", Session{Worktree: managed}, true},
		{"unmanaged", Session{Worktree: &WorktreeInfo{}}, false},
		{"plain session", Session{}, false},
		{"multi repo workspace", Session{Worktree: managed, WorkspaceRepo: []json.RawMessage{{}, {}}}, false},
	}
	for _, tt := range tests {
		if got := tt.s.ManagedWorktree(); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHoldingPath(t *testing.T) {
	got := HoldingPath("/work/app-worktrees/feat", "abc123")
	if want := "/work/app-worktrees/.aoe-trash/abc123"; got != want {
		t.Errorf("HoldingPath = %q, want %q", got, want)
	}
	if !IsHoldingPath(got, "abc123") {
		t.Error("holding path not recognised")
	}
	for _, p := range []string{"/work/app-worktrees/feat", "/work/.aoe-trash/other", "/work/x/abc123"} {
		if IsHoldingPath(p, "abc123") {
			t.Errorf("%q wrongly recognised as holding path", p)
		}
	}
}
