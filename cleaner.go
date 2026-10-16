// The lifecycle brain: tracks sessions from sessions.list and
// session.lifecycle.changed, and cleans the worktree per its project's
// settings once a trashed session's worktree has been parked (or a session is
// deleted).
//
// aoe flips a session to trashed first and moves its worktree into the
// `.aoe-trash` holding directory afterwards; that move surfaces as a
// `workspace_changed` event with the new project path. The trash trigger
// therefore waits for the session's path to become its holding path instead of
// acting on the `trashed` event itself.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultRelocationTimeoutSecs = 120
	lifecycleMethod              = "session.lifecycle.changed"
)

// host is the slice of the host RPC the cleaner needs.
type host interface {
	getSetting(ctx context.Context, key, projectPath string) (json.RawMessage, error)
	listSessions(ctx context.Context) ([]session, error)
}

type cleaner struct {
	ctx  context.Context
	host host
	wg   sync.WaitGroup

	mu      sync.Mutex
	known   map[string]session // last seen summary per session
	done    map[string]bool    // sessions whose trash rules already ran
	locks   map[string]*sync.Mutex
	changed chan struct{} // closed and replaced whenever known changes
}

func newCleaner(ctx context.Context, h host) *cleaner {
	return &cleaner{
		ctx:     ctx,
		host:    h,
		known:   make(map[string]session),
		done:    make(map[string]bool),
		locks:   make(map[string]*sync.Mutex),
		changed: make(chan struct{}),
	}
}

// wait blocks until every started job has finished.
func (c *cleaner) wait() { c.wg.Wait() }

func (c *cleaner) spawn(job func()) {
	c.wg.Go(job)
}

// onNotification is the handleInboundLine notify hook. It only updates state
// and spawns jobs, never blocking the stdin reader.
func (c *cleaner) onNotification(method string, params json.RawMessage) {
	if method != lifecycleMethod {
		return
	}
	var ev lifecycleEvent
	if err := json.Unmarshal(params, &ev); err != nil || ev.SessionID == "" {
		return
	}
	c.handleEvent(ev)
}

func (c *cleaner) handleEvent(ev lifecycleEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ev.Kind == kindDeleted {
		s, ok := c.known[ev.SessionID]
		delete(c.known, ev.SessionID)
		delete(c.done, ev.SessionID)
		c.broadcastLocked()
		if ok {
			c.spawn(func() { c.deleteJob(s) })
		}
		return
	}

	if ev.Summary == nil {
		return
	}
	c.known[ev.SessionID] = *ev.Summary
	if ev.Kind == kindRestored {
		delete(c.done, ev.SessionID)
	}
	c.broadcastLocked()

	if ev.Kind == kindTrashed || (ev.Kind == kindWorkspaceChanged && ev.Summary.Trashed) {
		c.spawn(func() { c.trashJob(ev.SessionID) })
	}
}

func (c *cleaner) broadcastLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *cleaner) lockFor(id string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.locks[id]
	if !ok {
		l = &sync.Mutex{}
		c.locks[id] = l
	}
	return l
}

// start seeds the session map from sessions.list and, unless disabled, runs the
// trash rules for sessions that are already trashed.
func (c *cleaner) start() {
	sessions, err := c.host.listSessions(c.ctx)
	if err != nil {
		logf("initial sessions.list failed: %v", err)
		return
	}
	c.mu.Lock()
	for _, s := range sessions {
		if _, seen := c.known[s.ID]; !seen {
			c.known[s.ID] = s
		}
	}
	c.broadcastLocked()
	c.mu.Unlock()

	if !c.boolSetting("sweep_on_start", true) {
		return
	}
	for _, s := range sessions {
		if s.Trashed && s.managedWorktree() && isHoldingPath(s.ProjectPath, s.ID) {
			c.spawn(func() { c.trashJob(s.ID) })
		}
	}
}

func (c *cleaner) trashJob(id string) {
	lock := c.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	c.mu.Lock()
	already := c.done[id]
	c.mu.Unlock()
	if already {
		return
	}

	s, err := c.waitRelocated(id)
	if err != nil {
		logf("session %s: skipping trash cleanup: %v", id, err)
		return
	}
	c.apply(triggerTrash, s, s.ProjectPath)

	c.mu.Lock()
	c.done[id] = true
	c.mu.Unlock()
}

// waitRelocated returns the session once it is trashed and its worktree sits in
// the holding directory. It gives up when the session is restored or the
// relocation timeout passes (aoe leaves some worktrees in place on purpose).
func (c *cleaner) waitRelocated(id string) (session, error) {
	timer := time.NewTimer(time.Duration(c.intSetting("relocation_timeout_secs", defaultRelocationTimeoutSecs)) * time.Second)
	defer timer.Stop()
	refreshed := false

	for {
		c.mu.Lock()
		s, ok := c.known[id]
		changed := c.changed
		c.mu.Unlock()

		switch {
		case !ok:
			return session{}, errors.New("session is gone")
		case !s.Trashed:
			return session{}, errors.New("session was restored")
		case !s.managedWorktree():
			return session{}, errors.New("not an aoe-managed single worktree")
		case isHoldingPath(s.ProjectPath, id):
			return s, nil
		}

		select {
		case <-changed:
		case <-c.ctx.Done():
			return session{}, c.ctx.Err()
		case <-timer.C:
			if refreshed {
				return session{}, fmt.Errorf("worktree %s was not moved into the trash holding directory in time", s.ProjectPath)
			}
			// Events are best-effort; look once at the stored state before giving up.
			refreshed = true
			c.refresh(id)
			timer.Reset(time.Second)
		}
	}
}

// refresh re-reads one session from sessions.list into the map.
func (c *cleaner) refresh(id string) {
	sessions, err := c.host.listSessions(c.ctx)
	if err != nil {
		logf("refresh sessions.list failed: %v", err)
		return
	}
	for _, s := range sessions {
		if s.ID == id {
			c.mu.Lock()
			c.known[id] = s
			c.broadcastLocked()
			c.mu.Unlock()
			return
		}
	}
}

// deleteJob runs the delete rules for a session that is gone. Its worktree may
// still be where it was last seen, in the holding directory if the trash move
// landed after our last update, or anywhere git says its branch is checked out
// if a restore did (aoe reports changes with a delay, so a session restored and
// deleted again quickly leaves our last-seen path stale).
func (c *cleaner) deleteJob(s session) {
	lock := c.lockFor(s.ID)
	lock.Lock()
	defer lock.Unlock()

	if dir, ok := c.findDeletedWorktree(s); ok {
		c.apply(triggerDelete, s, dir)
		return
	}
	logf("session %s: deleted, no worktree left at %s", s.ID, s.ProjectPath)
}

// findDeletedWorktree locates what is left of a deleted session's worktree.
func (c *cleaner) findDeletedWorktree(s session) (string, bool) {
	for _, dir := range []string{s.ProjectPath, holdingPath(s.ProjectPath, s.ID)} {
		if isWorktreeDir(dir) {
			return dir, true
		}
	}
	if !s.managedWorktree() {
		return "", false
	}
	dir, ok := findWorktreeByBranch(s.Worktree.MainRepoPath, s.Worktree.Branch)
	if !ok || !isWorktreeDir(dir) || c.inUse(dir) {
		return "", false
	}
	return dir, true
}

// inUse reports whether a known session works in dir, so a branch lookup never
// cleans a worktree some other live session now owns.
func (c *cleaner) inUse(dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.known {
		if filepath.Clean(s.ProjectPath) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// apply cleans dir according to the session's project, if the project asks for
// cleanup on this trigger.
func (c *cleaner) apply(trigger string, s session, dir string) {
	if !s.managedWorktree() {
		return
	}
	if !c.boolSetting("enabled", true) {
		return
	}
	rule, err := c.projectRule(s.Worktree.MainRepoPath)
	if err != nil {
		logf("session %s: %v", s.ID, err)
		return
	}
	if !rule.applies(trigger) {
		return
	}
	logf("session %s: cleaning %s (%s)", s.ID, dir, trigger)
	if err := cleanWorktree(dir, rule.Paths); err != nil {
		logf("session %s: %v", s.ID, err)
	}
}

// projectRule reads the per-project `paths` and `on` settings of the project
// whose main repository is at repoPath.
func (c *cleaner) projectRule(repoPath string) (projectRule, error) {
	paths, err := c.host.getSetting(c.ctx, "paths", repoPath)
	if err != nil {
		return projectRule{}, fmt.Errorf("reading paths: %w", err)
	}
	on, err := c.host.getSetting(c.ctx, "on", repoPath)
	if err != nil {
		return projectRule{}, fmt.Errorf("reading on: %w", err)
	}
	return parseProjectRule(paths, on)
}

func (c *cleaner) boolSetting(key string, fallback bool) bool {
	raw, err := c.host.getSetting(c.ctx, key, "")
	if err != nil {
		logf("reading setting %s failed: %v", key, err)
		return false // fail closed: never delete when settings are unreadable
	}
	var v *bool
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return fallback
	}
	return *v
}

func (c *cleaner) intSetting(key string, fallback int) int {
	raw, err := c.host.getSetting(c.ctx, key, "")
	if err != nil {
		return fallback
	}
	var v *int
	if json.Unmarshal(raw, &v) != nil || v == nil || *v <= 0 {
		return fallback
	}
	return *v
}
