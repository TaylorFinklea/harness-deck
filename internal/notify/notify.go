// Package notify runs the user-configured notification command when a response
// is recorded, so the user (or harness) learns that an answer landed.
package notify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runTimeout bounds how long a notify command may run before it is killed, so
// a hanging command can't block the respond handler indefinitely. It is a var
// (not a const) so tests can lower it without sleeping for the real budget.
var runTimeout = 10 * time.Second

// Run executes the configured notify command. The run directory is appended as
// the final argument, and HD_PROJECT / HD_RUN / HD_BLOCK / HD_RUN_DIR /
// HD_RESPONSE_VALUE / HD_RESPONSE_JSON are exported to the child so a script
// can react to specifics. A blank command is a no-op.
//
// The command is bounded by runTimeout via a context; a command that outlives
// it is killed and Run returns the resulting error.
//
// Command splitting is whitespace-based — keep the configured command simple
// (a script name, not a quoted pipeline).
func Run(command, runDir, project, run, block, value, note string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	parts := strings.Fields(command)
	args := append(parts[1:], runDir)
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, parts[0], args...)
	responseJSON, _ := json.Marshal(map[string]string{
		"block": block,
		"value": value,
		"note":  note,
	})
	cmd.Env = append(os.Environ(),
		"HD_PROJECT="+project,
		"HD_RUN="+run,
		"HD_BLOCK="+block,
		"HD_RUN_DIR="+runDir,
		"HD_RESPONSE_VALUE="+value,
		"HD_RESPONSE_JSON="+string(responseJSON),
	)
	return cmd.Run()
}
