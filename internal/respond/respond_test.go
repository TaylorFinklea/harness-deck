package respond

import (
	"fmt"
	"sync"
	"testing"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	f, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load of a missing file should not error: %v", err)
	}
	if len(f.Responses) != 0 {
		t.Errorf("expected no responses, got %d", len(f.Responses))
	}
}

func TestRecordPersistsAndMerges(t *testing.T) {
	dir := t.TempDir()

	if _, err := Record(dir, "proj", "run1", Response{Block: "q1", Value: "A"}); err != nil {
		t.Fatalf("Record q1: %v", err)
	}
	if _, err := Record(dir, "proj", "run1", Response{Block: "q2", Value: "yes"}); err != nil {
		t.Fatalf("Record q2: %v", err)
	}

	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Responses) != 2 {
		t.Fatalf("got %d responses, want 2 (the first must persist)", len(f.Responses))
	}
	if f.Responses["q1"].Value != "A" || f.Responses["q2"].Value != "yes" {
		t.Errorf("responses = %+v", f.Responses)
	}
	if f.Responses["q1"].At == "" {
		t.Error("Record should stamp a timestamp when none is given")
	}
}

func TestRecordOverwritesSameBlock(t *testing.T) {
	dir := t.TempDir()
	_, _ = Record(dir, "p", "r", Response{Block: "q1", Value: "first"})
	_, _ = Record(dir, "p", "r", Response{Block: "q1", Value: "second"})

	f, _ := Load(dir)
	if len(f.Responses) != 1 || f.Responses["q1"].Value != "second" {
		t.Errorf("re-answering a block should overwrite; got %+v", f.Responses)
	}
}

func TestRecordMultiValuesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	vals := []string{"fix-auth", "fix-logging"}
	_, err := Record(dir, "proj", "run1", Response{
		Block:  "findings",
		Value:  "fix-auth, fix-logging",
		Values: vals,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := f.Responses["findings"]
	if r.Value != "fix-auth, fix-logging" {
		t.Errorf("Value = %q, want %q", r.Value, "fix-auth, fix-logging")
	}
	if len(r.Values) != 2 || r.Values[0] != "fix-auth" || r.Values[1] != "fix-logging" {
		t.Errorf("Values = %v, want [fix-auth fix-logging]", r.Values)
	}
}

func TestRecordConcurrentWritersLoseNoAnswers(t *testing.T) {
	// Two clients answering different blocks of the same run (phone +
	// desktop) must both land: Record's read-modify-write has to be
	// serialized, or the later writer resurrects the earlier writer's
	// pre-answer snapshot.
	dir := t.TempDir()
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := Record(dir, "acme", "run-1", Response{
				Block: fmt.Sprintf("ask-%02d", i),
				Value: "yes",
			})
			if err != nil {
				t.Errorf("Record ask-%02d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after concurrent records: %v", err)
	}
	if len(f.Responses) != n {
		t.Errorf("answers recorded = %d, want %d (lost updates)", len(f.Responses), n)
	}
}
