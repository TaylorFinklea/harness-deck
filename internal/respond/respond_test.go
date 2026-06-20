package respond

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestRecordHistoryTwoAnswers(t *testing.T) {
	dir := t.TempDir()

	// First answer: A.
	_, err := Record(dir, "p", "r", Response{Block: "q1", Value: "A", At: "2026-01-01T00:00:00Z"})
	if err != nil {
		t.Fatalf("Record A: %v", err)
	}

	// Second answer: B — should push A into Prior.
	f, err := Record(dir, "p", "r", Response{Block: "q1", Value: "B", At: "2026-01-02T00:00:00Z"})
	if err != nil {
		t.Fatalf("Record B: %v", err)
	}

	got := f.Responses["q1"]
	if got.Value != "B" {
		t.Errorf("Value = %q, want B", got.Value)
	}
	if len(got.Prior) != 1 {
		t.Fatalf("len(Prior) = %d, want 1", len(got.Prior))
	}
	if got.Prior[0].Value != "A" || got.Prior[0].At != "2026-01-01T00:00:00Z" {
		t.Errorf("Prior[0] = %+v, want {Value:A At:2026-01-01T00:00:00Z}", got.Prior[0])
	}
	if f.Version != responsesVersion {
		t.Errorf("File.Version = %d, want %d", f.Version, responsesVersion)
	}
}

func TestRecordHistoryThreeAnswers(t *testing.T) {
	dir := t.TempDir()

	_, _ = Record(dir, "p", "r", Response{Block: "q1", Value: "A", At: "2026-01-01T00:00:00Z"})
	_, _ = Record(dir, "p", "r", Response{Block: "q1", Value: "B", At: "2026-01-02T00:00:00Z"})
	f, err := Record(dir, "p", "r", Response{Block: "q1", Value: "C", At: "2026-01-03T00:00:00Z"})
	if err != nil {
		t.Fatalf("Record C: %v", err)
	}

	got := f.Responses["q1"]
	if got.Value != "C" {
		t.Errorf("Value = %q, want C", got.Value)
	}
	if len(got.Prior) != 2 {
		t.Fatalf("len(Prior) = %d, want 2 (newest-first: B, A)", len(got.Prior))
	}
	if got.Prior[0].Value != "B" {
		t.Errorf("Prior[0].Value = %q, want B (newest-first)", got.Prior[0].Value)
	}
	if got.Prior[1].Value != "A" {
		t.Errorf("Prior[1].Value = %q, want A", got.Prior[1].Value)
	}
}

func TestRecordHistoryCap(t *testing.T) {
	dir := t.TempDir()

	// Record maxPrior+2 answers — should never grow beyond maxPrior entries in Prior.
	for i := 0; i < maxPrior+2; i++ {
		_, err := Record(dir, "p", "r", Response{Block: "q1", Value: fmt.Sprintf("v%d", i)})
		if err != nil {
			t.Fatalf("Record v%d: %v", i, err)
		}
	}

	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := f.Responses["q1"]
	if len(got.Prior) != maxPrior {
		t.Errorf("len(Prior) = %d, want %d (cap)", len(got.Prior), maxPrior)
	}
}

func TestLoadLegacyNoVersionNoHistory(t *testing.T) {
	// Simulate a legacy responses.json with no version field and no prior arrays.
	dir := t.TempDir()
	legacy := `{"run":"r","project":"p","updated":"2026-01-01T00:00:00Z","responses":{"q1":{"block":"q1","value":"old","at":"2026-01-01T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}

	f, err := Load(dir)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	if f.Responses["q1"].Value != "old" {
		t.Errorf("Value = %q, want old", f.Responses["q1"].Value)
	}
	if len(f.Responses["q1"].Prior) != 0 {
		t.Errorf("Prior should be empty on legacy file, got %v", f.Responses["q1"].Prior)
	}

	// Now record a new answer — it should build history from the legacy entry.
	f2, err := Record(dir, "p", "r", Response{Block: "q1", Value: "new"})
	if err != nil {
		t.Fatalf("Record over legacy: %v", err)
	}
	if f2.Responses["q1"].Value != "new" {
		t.Errorf("Value = %q, want new", f2.Responses["q1"].Value)
	}
	if len(f2.Responses["q1"].Prior) != 1 || f2.Responses["q1"].Prior[0].Value != "old" {
		t.Errorf("Prior after legacy upgrade = %v, want [{old}]", f2.Responses["q1"].Prior)
	}
	if f2.Version != responsesVersion {
		t.Errorf("File.Version = %d after legacy upgrade, want %d", f2.Version, responsesVersion)
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
