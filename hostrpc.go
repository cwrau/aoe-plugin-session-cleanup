// Outbound JSON-RPC calls from worker to host, layered on the same ndjson
// stdio channel the host uses to send this worker requests. handleInboundLine
// is the single entry point for every stdin line: it demuxes replies to our
// own outbound calls (config.get, sessions.list) from genuine host->worker
// requests and notifications (handed to the caller-supplied funcs).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type hostRPC struct {
	mu      sync.Mutex
	out     io.Writer
	nextID  int
	pending map[int]chan rpcReply
}

type rpcReply struct {
	result json.RawMessage
	err    *rpcErrorBody
}

// inboundLine covers every shape a stdin line can take: a genuine
// host->worker request (ID and Method set), a notification (Method set, ID
// absent), or a reply to one of our own outbound calls (ID set, Method
// empty, Result or Error set).
type inboundLine struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *rpcErrorBody   `json:"error"`
}

// newHostRPC's id counter starts in a high range so it never collides with
// the ids the host assigns to its own requests to this worker.
func newHostRPC(out io.Writer) *hostRPC {
	return &hostRPC{out: out, nextID: 1_000_000, pending: make(map[int]chan rpcReply)}
}

func (h *hostRPC) writeMessage(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	data = append(data, '\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, _ = h.out.Write(data)
}

func (h *hostRPC) allocID() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	return h.nextID
}

// call sends a request and blocks until handleInboundLine delivers the
// matching reply or ctx is done, whichever comes first.
func (h *hostRPC) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := h.allocID()
	ch := make(chan rpcReply, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	h.writeMessage(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})

	select {
	case reply := <-ch:
		if reply.err != nil {
			return nil, fmt.Errorf("%s: %s (code %d)", method, reply.err.Message, reply.err.Code)
		}
		return reply.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// handleInboundLine processes one stdin line. dispatch answers a genuine
// host->worker request; notify receives host notifications and must not block
// or call the host synchronously, since this runs on the stdin reader that
// also delivers the replies to those calls.
func (h *hostRPC) handleInboundLine(
	line []byte,
	dispatch func(method string, params json.RawMessage) (any, error),
	notify func(method string, params json.RawMessage),
) {
	var in inboundLine
	if err := json.Unmarshal(line, &in); err != nil {
		return
	}

	if in.Method != "" {
		if len(in.ID) == 0 || string(in.ID) == "null" {
			notify(in.Method, in.Params)
			return
		}
		result, err := dispatch(in.Method, in.Params)
		if err != nil {
			h.writeMessage(errorResponse(in.ID, err))
		} else {
			h.writeMessage(resultResponse(in.ID, result))
		}
		return
	}

	if len(in.ID) == 0 {
		return
	}
	var id int
	if err := json.Unmarshal(in.ID, &id); err != nil {
		return // reply to a call we didn't make (non-numeric id)
	}
	h.mu.Lock()
	ch, ok := h.pending[id]
	h.mu.Unlock()
	if !ok {
		return // stray reply: not one of ours, or already timed out
	}
	ch <- rpcReply{result: in.Result, err: in.Error}
}

// getSetting reads one of this plugin's own settings via config.get. A
// non-empty projectPath resolves a per-project setting for that registered
// project (project, then global, then default); a host without per-project
// settings ignores it and returns the global value. An unset setting comes back
// as a null value.
func (h *hostRPC) getSetting(ctx context.Context, key, projectPath string) (json.RawMessage, error) {
	params := map[string]any{"key": key}
	if projectPath != "" {
		params["project_path"] = projectPath
	}
	result, err := h.call(ctx, "config.get", params)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, fmt.Errorf("decode config.get %s: %w", key, err)
	}
	return parsed.Value, nil
}

// listSessions returns every stored session of the host's profile.
func (h *hostRPC) listSessions(ctx context.Context) ([]session, error) {
	result, err := h.call(ctx, "sessions.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Sessions []session `json:"sessions"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, fmt.Errorf("decode sessions.list: %w", err)
	}
	return parsed.Sessions, nil
}
