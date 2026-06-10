package jsonfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAtomicWriteCreatesParentsAndContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "out.json")
	if err := AtomicWrite(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Errorf("content = %q", data)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", fi.Mode().Perm())
	}
}

func TestAtomicWriteNeverUsesTheFixedTmpName(t *testing.T) {
	// Two processes writing the same file must not share a temp path.
	// A sentinel parked at the old fixed name (path + ".tmp") must come
	// through a write completely untouched.
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	sentinel := path + ".tmp"
	if err := os.WriteFile(sentinel, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel gone: %v", err)
	}
	if string(got) != "sentinel" {
		t.Errorf("sentinel overwritten: %q", got)
	}
}

func TestAtomicWriteLeavesNoTempResidue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := AtomicWrite(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "out.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dir contents = %v, want only out.json", names)
	}
}

func TestPatchAppliesMutationAndPreservesUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	seed := `{"status":"draft","future_field":{"nested":true},"list":[1,2]}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Patch(path, func(doc map[string]any) error {
		doc["status"] = "done"
		return nil
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	data, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if got["status"] != "done" {
		t.Errorf("status = %v", got["status"])
	}
	if _, ok := got["future_field"]; !ok {
		t.Errorf("unknown field dropped: %s", data)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("missing trailing newline")
	}
}

func TestPatchPreservesLargeIntegerLiterals(t *testing.T) {
	// encoding/json decodes numbers into float64 by default, corrupting
	// integers past 2^53. Patch must round-trip untouched numbers
	// byte-for-byte.
	path := filepath.Join(t.TempDir(), "doc.json")
	seed := `{"tokens":9007199254740993,"status":"draft"}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Patch(path, func(doc map[string]any) error {
		doc["status"] = "done"
		return nil
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "9007199254740993") {
		t.Errorf("large integer mangled: %s", data)
	}
}

func TestPatchMissingFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	err := Patch(path, func(doc map[string]any) error { return nil })
	if err == nil {
		t.Fatal("Patch on a missing file should error")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("Patch created the missing file")
	}
}

func TestPatchRefusesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	corrupt := `{"status":"draft",` // truncated mid-write
	if err := os.WriteFile(path, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Patch(path, func(doc map[string]any) error {
		doc["status"] = "done"
		return nil
	})
	if err == nil {
		t.Fatal("Patch on a corrupt file should error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != corrupt {
		t.Errorf("corrupt file was rewritten: %q", data)
	}
}

func TestPatchMutateErrorAborts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	seed := `{"a":1}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := func(doc map[string]any) error {
		doc["a"] = 2
		return os.ErrInvalid
	}
	if err := Patch(path, boom); err == nil {
		t.Fatal("mutate error should propagate")
	}
	data, _ := os.ReadFile(path)
	if string(data) != seed {
		t.Errorf("file rewritten despite mutate error: %q", data)
	}
}

func TestUpsertCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	err := Upsert(path, func(doc map[string]any) error {
		doc["notifications"] = []any{}
		return nil
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if _, ok := got["notifications"]; !ok {
		t.Errorf("mutation missing: %s", data)
	}
}

func TestUpsertRefusesCorruptFile(t *testing.T) {
	// The clobber bug this package exists to kill: a hand-edit typo in
	// config.json must make writers refuse, not silently rewrite the file
	// down to the one key they own.
	path := filepath.Join(t.TempDir(), "config.json")
	corrupt := `{"central_dir":"/x","bind":"0.0.0.0",}` // trailing comma
	if err := os.WriteFile(path, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Upsert(path, func(doc map[string]any) error {
		doc["notifications"] = []any{}
		return nil
	})
	if err == nil {
		t.Fatal("Upsert on a corrupt file should error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != corrupt {
		t.Errorf("corrupt config was clobbered: %q", data)
	}
}

func TestPatchRejectsTrailingGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte("{}\ngarbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Patch(path, func(doc map[string]any) error { return nil })
	if err == nil {
		t.Fatal("trailing garbage should be an error, not silently dropped")
	}
}

func TestPatchConcurrentMutationsAreSerialized(t *testing.T) {
	// Patch is used by two different PROCESSES on the same report.json
	// (dashboard close/archive vs MCP update_live). The advisory lock has
	// to serialize the read-modify-write — concurrent goroutines contend
	// on the same flock path, so this exercises the identical syscall.
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte(`{"n":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := Patch(path, func(doc map[string]any) error {
				cur, _ := doc["n"].(json.Number)
				v, _ := cur.Int64()
				doc["n"] = v + 1
				return nil
			})
			if err != nil {
				t.Errorf("Patch: %v", err)
			}
		}()
	}
	wg.Wait()

	data, _ := os.ReadFile(path)
	var got struct{ N int64 }
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if got.N != n {
		t.Errorf("n = %d, want %d (lost updates — read-modify-write not serialized)", got.N, n)
	}
}
