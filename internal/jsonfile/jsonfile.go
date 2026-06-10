// Package jsonfile centralizes the repo's JSON-on-disk write discipline:
// atomic writes through unique temp files, and read-modify-write patches
// that preserve unknown fields, keep number literals byte-exact, and never
// clobber an existing-but-unparseable file.
package jsonfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path via a unique temp file in the same
// directory followed by a rename, creating parent directories as needed.
// A crash mid-write can never truncate the target, and concurrent writers
// can never collide on a shared temp name.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Best-effort cleanup; a no-op once the rename has moved the file.
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Patch reads the JSON document at path (which must exist), applies mutate,
// and writes the result back atomically. Unknown fields are preserved and
// numbers round-trip as their original literals. An existing file that
// fails to parse is an error — Patch never rewrites a file it could not
// read back.
func Patch(path string, mutate func(doc map[string]any) error) error {
	return patch(path, false, mutate)
}

// Upsert is Patch for files that may not exist yet: a missing file starts
// from an empty document. An existing-but-unparseable file is still an
// error, never a silent restart from empty.
func Upsert(path string, mutate func(doc map[string]any) error) error {
	return patch(path, true, mutate)
}

func patch(path string, allowMissing bool, mutate func(doc map[string]any) error) error {
	// The lock file lives beside the target, so the parent must exist
	// before the lock can be taken (Upsert may be creating both).
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Advisory lock shared by every writer of this file — including ones
	// in other processes (dashboard vs MCP server) — so two concurrent
	// read-modify-writes can't erase each other's changes.
	unlock, err := lockFile(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

	doc := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		// UseNumber keeps every untouched number as its original literal
		// (json.Number marshals back byte-for-byte), so a patch can never
		// round large integers through float64.
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if derr := dec.Decode(&doc); derr != nil {
			return fmt.Errorf("%s exists but is not valid JSON — refusing to rewrite it: %w", path, derr)
		}
		if dec.More() {
			return fmt.Errorf("%s has trailing data after the JSON document — refusing to rewrite it", path)
		}
	case errors.Is(err, fs.ErrNotExist) && allowMissing:
		// Start fresh.
	default:
		return err
	}
	if err := mutate(doc); err != nil {
		return err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(out, '\n'), 0o644)
}
