// Agent of Empires worker: JSON-RPC 2.0 over newline-delimited JSON on stdio.
//
// The host spawns this process, sends one JSON-RPC request per line on
// stdin, and reads one response per line on stdout. The worker exits
// cleanly on EOF.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const runtimeName = "go"

// maxLineBytes bounds one ndjson line from the host; a lifecycle summary of a
// workspace session with many repos can outgrow bufio's 64 KiB default.
const maxLineBytes = 4 * 1024 * 1024

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "session-cleanup-worker: "+format+"\n", args...)
}

// handle maps a command id to a method; dispatch on the trailing segment so
// the worker is robust to the host's exact method prefix.
func handle(method string, _ json.RawMessage) (any, error) {
	parts := strings.Split(method, ".")
	command := parts[len(parts)-1]
	if command == "status" {
		return map[string]any{
			"ok":      true,
			"runtime": runtimeName,
			"message": "Session Cleanup worker is running",
		}, nil
	}
	return nil, &methodNotFoundError{method: method}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	rpc := newHostRPC(os.Stdout)
	cl := newCleaner(ctx, rpc)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	// The initial seeding calls the host, so it must not run on the reader
	// that delivers the replies.
	cl.spawn(cl.start)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		rpc.handleInboundLine([]byte(line), handle, cl.onNotification)
	}
	if err := scanner.Err(); err != nil {
		logf("reading stdin: %v", err)
	}

	cancel()
	cl.wait()
}
