package herdr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinProbesDir(t *testing.T) {
	dir := t.TempDir()
	// Use a name that is guaranteed not to exist in $PATH so the fallback probe
	// path is exercised (herdr itself may be installed on this machine).
	const fakeName = "herdr-test-probe-xyz"
	bin := filepath.Join(dir, fakeName)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveBin(fakeName, []string{filepath.Join(dir, "nope"), bin})
	if !ok || got != bin {
		t.Fatalf("resolveBin = (%q,%v), want (%q,true)", got, ok, bin)
	}
}

func TestResolveBinMissing(t *testing.T) {
	if got, ok := resolveBin("herdr-nonexistent-xyz", []string{"/no/such/path"}); ok {
		t.Fatalf("resolveBin found %q, want not found", got)
	}
}

// TestSendRejectsFlagLike verifies the argv flag-smuggling guard: a "-"-leading
// target or text is refused before any exec, so herdr's non-POSIX parser can't
// be tricked into treating the value as a flag. The bin path is bogus on
// purpose — the guard must return before Send ever tries to exec it.
func TestSendRejectsFlagLike(t *testing.T) {
	c := &Client{bin: "/nonexistent/herdr"}
	if err := c.Send(context.Background(), "-n", "yes"); err == nil {
		t.Error("Send with flag-like target should error before exec")
	}
	if err := c.Send(context.Background(), "w1:p1", "-rf"); err == nil {
		t.Error("Send with flag-like text should error before exec")
	}
}
