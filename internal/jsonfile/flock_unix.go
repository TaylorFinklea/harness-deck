//go:build unix

package jsonfile

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory flock on path (created if absent)
// and returns the release func. flock contends across processes AND across
// goroutines within one process (each call opens its own descriptor), which
// is exactly the serialization Patch needs: the dashboard and the MCP
// server are separate processes rewriting the same report.json. The lock
// file is left in place after release — removing it would race a waiter
// that already opened it.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
