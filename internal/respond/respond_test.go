package respond

import (
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
