// Command session-cleanup-worker is the Agent of Empires worker of the Session
// Cleanup plugin: JSON-RPC 2.0 over newline-delimited JSON on stdio.
//
// The host spawns this process, sends one JSON-RPC request per line on
// stdin, and reads one response per line on stdout. The worker exits
// cleanly on EOF.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/cwrau/aoe-plugin-session-cleanup/internal/cleaner"
	"github.com/cwrau/aoe-plugin-session-cleanup/internal/logging"
	"github.com/cwrau/aoe-plugin-session-cleanup/internal/rpc"
)

const runtimeName = "go"

// maxLineBytes bounds one ndjson line from the host; a lifecycle summary of a
// workspace session with many repos can outgrow bufio's 64 KiB default.
const maxLineBytes = 4 * 1024 * 1024

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
	return nil, &rpc.MethodNotFoundError{Method: method}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	host := rpc.NewHost(os.Stdout)
	cl := cleaner.New(ctx, host)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	// The initial seeding calls the host, so it must not run on the reader
	// that delivers the replies.
	cl.Start()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		host.HandleInboundLine([]byte(line), handle, cl.OnNotification)
	}
	if err := scanner.Err(); err != nil {
		logging.Logf("reading stdin: %v", err)
	}

	cancel()
	cl.Wait()
}
