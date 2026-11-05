package cleaner

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cwrau/aoe-plugin-session-cleanup/internal/session"
	"github.com/cwrau/aoe-plugin-session-cleanup/internal/testutil"
)

// fakeHost serves settings the way config.get does: a per-project value for
// the project asked about wins over the global one.
type fakeHost struct {
	mu         sync.Mutex
	global     map[string]any
	perProject map[string]map[string]any // repo path -> key -> value
	sessions   []session.Session
	reads      map[string]int // key -> number of config.get calls
}

func (f *fakeHost) GetSetting(_ context.Context, key, projectPath string) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads[key]++
	v, ok := f.perProject[projectPath][key]
	if !ok {
		v, ok = f.global[key]
	}
	if !ok {
		return json.RawMessage("null"), nil
	}
	return json.Marshal(v)
}

func (f *fakeHost) ListSessions(context.Context) ([]session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]session.Session(nil), f.sessions...), nil
}

func (f *fakeHost) readCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[key]
}

const (
	testID   = "sess1"
	testRepo = "/repos/app"
)

// fixture is a managed worktree "feat" next to a holding dir, so tests can tell
// which one a cleanup hit. The project testRepo cleans `target` per its
// settings.
type fixture struct {
	original string // where the worktree lives before it is trashed
	holding  string // where aoe parks it
	host     *fakeHost
	cl       *Cleaner
}

// newFixture configures testRepo with the given `on` ("" leaves it unset).
func newFixture(t *testing.T, on string) *fixture {
	t.Helper()
	root := t.TempDir()
	project := map[string]any{"paths": []string{"target"}}
	if on != "" {
		project["on"] = on
	}
	f := &fixture{
		original: filepath.Join(root, "wts", "feat"),
		holding:  session.HoldingPath(filepath.Join(root, "wts", "feat"), testID),
		host: &fakeHost{
			global:     map[string]any{"relocation_timeout_secs": 5},
			perProject: map[string]map[string]any{testRepo: project},
			reads:      make(map[string]int),
		},
	}
	f.cl = New(context.Background(), f.host)
	t.Cleanup(f.cl.Wait)
	return f
}

func (f *fixture) session(path string, trashed bool) session.Session {
	return session.Session{
		ID:          testID,
		ProjectPath: path,
		Trashed:     trashed,
		Worktree:    &session.WorktreeInfo{Branch: "feat", MainRepoPath: testRepo, ManagedByAOE: true},
	}
}

func (f *fixture) event(kind string, s *session.Session) {
	f.cl.handleEvent(session.LifecycleEvent{SessionID: testID, Kind: kind, Summary: s})
}

// settle waits for all spawned jobs so far.
func (f *fixture) settle() { f.cl.Wait() }

// seed makes dir a worktree with a target dir inside.
func seed(t *testing.T, dir string) {
	t.Helper()
	testutil.WriteFile(t, filepath.Join(dir, ".git"), "x")
	testutil.WriteFile(t, filepath.Join(dir, "target/x"), "x")
}

func hasTarget(dir string) bool { return testutil.Exists(filepath.Join(dir, "target")) }

func TestTrashWaitsForRelocationBeforeCleaning(t *testing.T) {
	f := newFixture(t, "trash")
	seed(t, f.original)

	// 1. trashed fires while the worktree is still at its original path.
	trashed := f.session(f.original, true)
	f.event(session.KindTrashed, &trashed)
	time.Sleep(100 * time.Millisecond)
	if !hasTarget(f.original) {
		t.Fatal("cleaned the worktree before it was moved to the trash")
	}

	// 2. aoe moves it; the relocation arrives as workspace_changed.
	seed(t, f.holding)
	moved := f.session(f.holding, true)
	f.event(session.KindWorkspaceChanged, &moved)
	f.settle()

	if hasTarget(f.holding) {
		t.Error("holding worktree was not cleaned")
	}
	if !hasTarget(f.original) {
		t.Error("original path was touched")
	}
}

func TestTrashAlreadyRelocatedCleansOnce(t *testing.T) {
	f := newFixture(t, "trash")
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.event(session.KindWorkspaceChanged, &moved) // duplicate signal for the same trash
	f.settle()

	if hasTarget(f.holding) {
		t.Error("holding worktree was not cleaned")
	}
	if got := f.host.readCount("paths"); got != 1 {
		t.Errorf("cleanup ran %d times, want 1", got)
	}
}

func TestRestoreThenTrashAgainCleansAgain(t *testing.T) {
	f := newFixture(t, "trash")
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()
	restored := f.session(f.original, false)
	f.event(session.KindRestored, &restored)
	f.event(session.KindTrashed, &moved)
	f.settle()

	if got := f.host.readCount("paths"); got != 2 {
		t.Errorf("cleanup ran %d times, want 2", got)
	}
}

func TestRestoredBeforeMoveAbortsCleanup(t *testing.T) {
	f := newFixture(t, "trash")
	seed(t, f.original)

	trashed := f.session(f.original, true)
	f.event(session.KindTrashed, &trashed)
	restored := f.session(f.original, false)
	f.event(session.KindRestored, &restored)
	f.settle()

	if !hasTarget(f.original) {
		t.Error("cleaned a restored session")
	}
}

func TestDeleteUsesCachedPath(t *testing.T) {
	f := newFixture(t, "delete")
	seed(t, f.original)

	live := f.session(f.original, false)
	f.event("created", &live)
	f.event(session.KindDeleted, nil) // a deleted event carries no summary
	f.settle()

	if hasTarget(f.original) {
		t.Error("delete cleanup did not run on the last known path")
	}
}

func TestDeleteFallsBackToHoldingPath(t *testing.T) {
	f := newFixture(t, "delete")
	seed(t, f.holding)

	// Our last update still has the pre-trash path; the move landed afterwards.
	stale := f.session(f.original, false)
	f.event("created", &stale)
	f.event(session.KindDeleted, nil)
	f.settle()

	if hasTarget(f.holding) {
		t.Error("delete cleanup did not find the worktree in the holding dir")
	}
}

func TestOnDefaultsToBoth(t *testing.T) {
	f := newFixture(t, "") // `on` unset everywhere
	seed(t, f.original)
	seed(t, f.holding)

	live := f.session(f.original, false)
	f.event("created", &live)
	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()
	if hasTarget(f.holding) {
		t.Error("trash did not clean with `on` unset")
	}

	// The worktree is still in the trash when the session is deleted.
	testutil.WriteFile(t, filepath.Join(f.holding, "target/x"), "x")
	f.event(session.KindDeleted, nil)
	f.settle()
	if hasTarget(f.holding) {
		t.Error("delete did not clean with `on` unset")
	}
}

func TestOnRestrictsTrigger(t *testing.T) {
	f := newFixture(t, "delete")
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()
	if !hasTarget(f.holding) {
		t.Error("a delete-only project was cleaned on trash")
	}
}

func TestProjectWithoutPathsIsNotCleaned(t *testing.T) {
	f := newFixture(t, "both")
	f.host.perProject[testRepo]["paths"] = []string{}
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()
	if !hasTarget(f.holding) {
		t.Error("cleaned a project with an empty path list")
	}
}

func TestOtherProjectsAreNotCleaned(t *testing.T) {
	f := newFixture(t, "both")
	seed(t, f.holding)

	other := f.session(f.holding, true)
	other.Worktree.MainRepoPath = "/repos/other" // no settings for this project
	f.event(session.KindTrashed, &other)
	f.settle()
	if !hasTarget(f.holding) {
		t.Error("cleaned a project that has no paths configured")
	}
}

func TestGlobalPathsApplyWhenProjectHasNone(t *testing.T) {
	f := newFixture(t, "both")
	delete(f.host.perProject, testRepo)
	f.host.global["paths"] = []string{"target"}
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()
	if hasTarget(f.holding) {
		t.Error("global fallback paths were not applied")
	}
}

func TestUnmanagedAndPlainSessionsAreNeverTouched(t *testing.T) {
	f := newFixture(t, "both")
	seed(t, f.original)

	plain := session.Session{ID: testID, ProjectPath: f.original}
	f.event("created", &plain)
	f.event(session.KindDeleted, nil)
	f.settle()

	if !hasTarget(f.original) {
		t.Error("cleaned a session that is not an aoe-managed worktree")
	}
}

func TestDisabledPluginDoesNothing(t *testing.T) {
	f := newFixture(t, "trash")
	f.host.global["enabled"] = false
	seed(t, f.holding)

	moved := f.session(f.holding, true)
	f.event(session.KindTrashed, &moved)
	f.settle()

	if !hasTarget(f.holding) {
		t.Error("cleaned while the plugin was disabled")
	}
}

func TestStartSweepsAlreadyTrashedSessions(t *testing.T) {
	f := newFixture(t, "trash")
	seed(t, f.holding)
	f.host.sessions = []session.Session{f.session(f.holding, true)}

	f.cl.start()
	f.settle()
	if hasTarget(f.holding) {
		t.Error("startup sweep did not clean the trashed worktree")
	}
}

func TestStartSweepCanBeDisabled(t *testing.T) {
	f := newFixture(t, "trash")
	f.host.global["sweep_on_start"] = false
	seed(t, f.holding)
	f.host.sessions = []session.Session{f.session(f.holding, true)}

	f.cl.start()
	f.settle()
	if !hasTarget(f.holding) {
		t.Error("swept despite sweep_on_start=false")
	}
}

func TestStartStillTracksSessionsForLaterDelete(t *testing.T) {
	f := newFixture(t, "delete")
	f.host.global["sweep_on_start"] = false
	seed(t, f.original)
	f.host.sessions = []session.Session{f.session(f.original, false)}

	f.cl.start()
	f.event(session.KindDeleted, nil)
	f.settle()
	if hasTarget(f.original) {
		t.Error("session known only from the initial list was not cleaned on delete")
	}
}

func TestOnNotificationIgnoresOtherMethodsAndGarbage(t *testing.T) {
	f := newFixture(t, "both")
	f.cl.OnNotification("session.status.changed", json.RawMessage(`{"session_id":"x"}`))
	f.cl.OnNotification(lifecycleMethod, json.RawMessage(`not json`))
	f.cl.OnNotification(lifecycleMethod, json.RawMessage(`{"kind":"deleted"}`))
	f.settle()
}

func TestOnNotificationDecodesHostShape(t *testing.T) {
	f := newFixture(t, "delete")
	seed(t, f.original)

	created := `{"session_id":"sess1","kind":"created","at":"2026-10-16T12:00:00Z","summary":{"id":"sess1","title":"t","project_path":"` +
		f.original + `","trashed":false,"workspace_repos":[],"worktree":{"branch":"feat","main_repo_path":"` + testRepo + `","managed_by_aoe":true}}}`
	f.cl.OnNotification(lifecycleMethod, json.RawMessage(created))
	f.cl.OnNotification(lifecycleMethod, json.RawMessage(`{"session_id":"sess1","kind":"deleted","at":"2026-10-16T12:01:00Z"}`))
	f.settle()
	if hasTarget(f.original) {
		t.Error("real notification payloads did not drive a cleanup")
	}
}

// staleDeleteFixture is a session our last event saw in the trash, while the
// worktree was in fact restored to its original path (aoe reports changes with
// a delay) before the session was deleted. Git's admin dir knows the truth.
func staleDeleteFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t, "delete")
	repo := t.TempDir()
	f.host.perProject[repo] = f.host.perProject[testRepo]
	seed(t, f.original)
	testutil.AddAdminDir(t, filepath.Join(repo, ".git", "worktrees"), "feat", "feat", f.original) // after seed: writes the real .git link

	stale := f.session(f.holding, true) // the trash path we last heard about
	stale.Worktree.MainRepoPath = repo
	f.event(session.KindTrashed, &stale)
	f.settle()
	return f, repo
}

func TestDeleteFindsRestoredWorktreeViaGit(t *testing.T) {
	f, _ := staleDeleteFixture(t)

	f.event(session.KindDeleted, nil)
	f.settle()
	if hasTarget(f.original) {
		t.Error("delete cleanup did not find the restored worktree through git")
	}
}

func TestDeleteViaGitSkipsWorktreeAnotherSessionOwns(t *testing.T) {
	f, repo := staleDeleteFixture(t)

	// A different live session now works in that checkout.
	other := f.session(f.original, false)
	other.ID = "other"
	other.Worktree.MainRepoPath = repo
	f.cl.handleEvent(session.LifecycleEvent{SessionID: "other", Kind: "created", Summary: &other})

	f.event(session.KindDeleted, nil)
	f.settle()
	if !hasTarget(f.original) {
		t.Error("cleaned a worktree owned by another session")
	}
}
