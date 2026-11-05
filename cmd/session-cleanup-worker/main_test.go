package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cwrau/aoe-plugin-session-cleanup/internal/rpc"
	"github.com/cwrau/aoe-plugin-session-cleanup/internal/testutil"
)

var workerBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "session-cleanup-worker-test")
	if err != nil {
		panic(err)
	}

	workerBinary = filepath.Join(dir, "session-cleanup-worker")
	build := exec.Command("go", "build", "-o", workerBinary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build worker: " + err.Error() + "\n" + string(out))
	}

	// os.Exit below skips deferred calls, so clean up explicitly before it runs.
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		panic(err)
	}
	os.Exit(code)
}

// hostHarness plays the aoe host against the real worker binary: it answers
// the worker's config.get and sessions.list calls and can push notifications.
type hostHarness struct {
	t     *testing.T
	mu    sync.Mutex // serializes writes to stdin
	stdin io.WriteCloser
	cmd   *exec.Cmd
	done  chan error
	// replies maps a worker->host method (config.get as "config.get:<key>") to its result.
	replies map[string]any
	// responses collects the worker's replies to the harness's own requests.
	responses chan map[string]any
	// projectReads records the project_path of each config.get lookup that sent one.
	projectReads sync.Map
}

func startWorker(t *testing.T, replies map[string]any) *hostHarness {
	t.Helper()
	cmd := exec.Command(workerBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	h := &hostHarness{
		t: t, stdin: stdin, cmd: cmd, replies: replies,
		done:      make(chan error, 1),
		responses: make(chan map[string]any, 16),
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
				t.Errorf("bad json line %q: %v", sc.Text(), err)
				continue
			}
			h.handleWorkerMessage(msg)
		}
		if err := sc.Err(); err != nil {
			t.Errorf("reading worker stdout: %v", err)
		}
		h.done <- cmd.Wait()
	}()
	return h
}

// send writes one message to the worker. A write after close is dropped: the
// worker may still be answering its last host call when stdin is closed.
func (h *hostHarness) send(msg map[string]any) {
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, _ = fmt.Fprintf(h.stdin, "%s\n", data)
}

// handleWorkerMessage answers a worker->host call from h.replies. config.get is
// looked up as "config.get:<key>" and falls back to a null (unset) value.
func (h *hostHarness) handleWorkerMessage(msg map[string]any) {
	method, isCall := msg["method"].(string)
	if !isCall {
		h.responses <- msg
		return
	}
	lookup := method
	if method == "config.get" {
		params, _ := msg["params"].(map[string]any)
		lookup = fmt.Sprintf("config.get:%v", params["key"])
		if project, ok := params["project_path"].(string); ok {
			h.projectReads.Store(lookup, project)
		}
	}
	result, known := h.replies[lookup]
	switch {
	case known:
	case method == "config.get":
		result = map[string]any{"value": nil}
	default:
		result = map[string]any{}
	}
	h.send(map[string]any{"jsonrpc": "2.0", "id": msg["id"], "result": result})
}

func (h *hostHarness) request(id int, method string) map[string]any {
	h.t.Helper()
	h.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": map[string]any{}})
	select {
	case resp := <-h.responses:
		return resp
	case <-time.After(10 * time.Second):
		h.t.Fatal("timed out waiting for response")
		return nil
	}
}

// close ends stdin and requires a clean exit.
func (h *hostHarness) close() {
	h.t.Helper()
	_ = h.stdin.Close()
	select {
	case err := <-h.done:
		if err != nil {
			h.t.Fatalf("worker exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = h.cmd.Process.Kill()
		h.t.Fatal("worker did not exit on EOF")
	}
}

func TestStatusAnswersAndExitsOnEOF(t *testing.T) {
	h := startWorker(t, map[string]any{"sessions.list": map[string]any{"sessions": []any{}}})
	resp := h.request(1, "session-cleanup.status")
	result, _ := resp["result"].(map[string]any)
	if result["ok"] != true {
		t.Errorf("status result = %v", resp)
	}
	h.close()
}

func TestUnknownMethodReturns32601(t *testing.T) {
	h := startWorker(t, map[string]any{"sessions.list": map[string]any{"sessions": []any{}}})
	resp := h.request(2, "session-cleanup.nope")
	errBody, _ := resp["error"].(map[string]any)
	if errBody["code"] != float64(rpc.CodeMethodNotFound) {
		t.Errorf("error = %v", resp)
	}
	h.close()
}

// TestDeleteEventCleansWorktreeEndToEnd drives the real binary the way the host
// does: sessions.list seeds it, a lifecycle notification for the deletion
// arrives, and the matching rule removes the build directory.
func TestDeleteEventCleansWorktreeEndToEnd(t *testing.T) {
	wt := testutil.NewWorktree(t)
	testutil.WriteFile(t, filepath.Join(wt, "target/x"), "x")

	sess := map[string]any{
		"id": "sid", "title": "t", "project_path": wt, "trashed": false,
		"workspace_repos": []any{},
		"worktree":        map[string]any{"branch": "b", "main_repo_path": "/repos/app", "managed_by_aoe": true},
	}
	h := startWorker(t, map[string]any{
		"sessions.list":    map[string]any{"sessions": []any{sess}},
		"config.get:paths": map[string]any{"revision": 1, "scope": "project", "value": []string{"target"}},
		"config.get:on":    map[string]any{"revision": 1, "scope": "project", "value": "delete"},
	})
	defer h.close()

	// Announce the session first: events are handled in order on the reader, so
	// the delete below cannot beat the worker's own initial sessions.list.
	h.send(map[string]any{
		"jsonrpc": "2.0", "method": "session.lifecycle.changed",
		"params": map[string]any{"session_id": "sid", "kind": "created", "at": "2026-10-16T12:00:00Z", "summary": sess},
	})
	h.send(map[string]any{
		"jsonrpc": "2.0", "method": "session.lifecycle.changed",
		"params": map[string]any{"session_id": "sid", "kind": "deleted", "at": "2026-10-16T12:00:01Z"},
	})

	// The cleanup runs asynchronously, and closing stdin cancels in-flight work,
	// so wait for its effect first.
	deadline := time.After(10 * time.Second)
	for testutil.Exists(filepath.Join(wt, "target")) {
		select {
		case <-deadline:
			t.Fatal("worker did not clean the deleted session's worktree")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !testutil.Exists(filepath.Join(wt, ".git")) {
		t.Error("worker deleted .git")
	}
	for _, key := range []string{"config.get:paths", "config.get:on"} {
		if got, _ := h.projectReads.Load(key); got != "/repos/app" {
			t.Errorf("%s was read for project %v, want the session's main repo /repos/app", key, got)
		}
	}
}
