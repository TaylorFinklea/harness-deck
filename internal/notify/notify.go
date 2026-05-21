// Package notify runs the user-configured notification command when a response
// is recorded, so the user (or harness) learns that an answer landed.
package notify

import (
	"os"
	"os/exec"
	"strings"
)

// Run executes the configured notify command. The run directory is appended as
// the final argument, and HD_PROJECT / HD_RUN / HD_BLOCK are exported to the
// child so a script can react to specifics. A blank command is a no-op.
//
// Command splitting is whitespace-based — keep the configured command simple
// (a script name, not a quoted pipeline).
func Run(command, runDir, project, run, block string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	parts := strings.Fields(command)
	args := append(parts[1:], runDir)
	cmd := exec.Command(parts[0], args...)
	cmd.Env = append(os.Environ(),
		"HD_PROJECT="+project,
		"HD_RUN="+run,
		"HD_BLOCK="+block,
		"HD_RUN_DIR="+runDir,
	)
	return cmd.Run()
}
