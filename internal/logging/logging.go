// Package logging writes the worker's diagnostics. The host captures stderr in
// the plugin's worker log; stdout is reserved for JSON-RPC.
package logging

import (
	"fmt"
	"os"
)

// Logf writes one prefixed line to stderr.
func Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "session-cleanup-worker: "+format+"\n", args...)
}
