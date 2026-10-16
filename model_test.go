package main

import (
	"encoding/json"
	"testing"
)

func TestManagedWorktree(t *testing.T) {
	managed := &worktreeInfo{ManagedByAOE: true}
	tests := []struct {
		name string
		s    session
		want bool
	}{
		{"managed", session{Worktree: managed}, true},
		{"unmanaged", session{Worktree: &worktreeInfo{}}, false},
		{"plain session", session{}, false},
		{"multi repo workspace", session{Worktree: managed, WorkspaceRepo: []json.RawMessage{{}, {}}}, false},
	}
	for _, tt := range tests {
		if got := tt.s.managedWorktree(); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHoldingPath(t *testing.T) {
	got := holdingPath("/work/app-worktrees/feat", "abc123")
	if want := "/work/app-worktrees/.aoe-trash/abc123"; got != want {
		t.Errorf("holdingPath = %q, want %q", got, want)
	}
	if !isHoldingPath(got, "abc123") {
		t.Error("holding path not recognised")
	}
	for _, p := range []string{"/work/app-worktrees/feat", "/work/.aoe-trash/other", "/work/x/abc123"} {
		if isHoldingPath(p, "abc123") {
			t.Errorf("%q wrongly recognised as holding path", p)
		}
	}
}
