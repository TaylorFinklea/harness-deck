package push

import (
	"path/filepath"
	"testing"
)

// TestRemoveIfMatchesKeepsResubscription asserts that pruning a 410-Gone
// endpoint only deletes the exact (endpoint, keys) tuple — a fresh
// re-subscription that reused the same endpoint URL with new keys survives.
func TestRemoveIfMatchesKeepsResubscription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	st := NewStore(path)

	stale := Subscription{
		Endpoint: "https://push.example/abc",
		Keys:     SubscriptionKeys{P256dh: "oldp256", Auth: "oldauth"},
	}
	if err := st.Add(stale); err != nil {
		t.Fatalf("Add stale: %v", err)
	}

	// The browser re-subscribed at the same endpoint with new keys; Add
	// replaces the record (one subscription per endpoint).
	fresh := Subscription{
		Endpoint: "https://push.example/abc",
		Keys:     SubscriptionKeys{P256dh: "newp256", Auth: "newauth"},
	}
	if err := st.Add(fresh); err != nil {
		t.Fatalf("Add fresh: %v", err)
	}

	// A 410 arrived for the stale keys. RemoveIfMatches must NOT delete the
	// fresh re-subscription that now occupies the same endpoint.
	if err := st.RemoveIfMatches(stale.Endpoint, stale.Keys); err != nil {
		t.Fatalf("RemoveIfMatches stale: %v", err)
	}
	all := st.All()
	if len(all) != 1 {
		t.Fatalf("after pruning stale keys: %d subscriptions, want 1", len(all))
	}
	if all[0].Keys != fresh.Keys {
		t.Errorf("surviving subscription keys = %+v, want %+v", all[0].Keys, fresh.Keys)
	}
}

// TestRemoveIfMatchesRemovesExact asserts that when the endpoint+keys tuple
// still matches the stored record, RemoveIfMatches deletes it.
func TestRemoveIfMatchesRemovesExact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	st := NewStore(path)

	sub := Subscription{
		Endpoint: "https://push.example/xyz",
		Keys:     SubscriptionKeys{P256dh: "p", Auth: "a"},
	}
	if err := st.Add(sub); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := st.RemoveIfMatches(sub.Endpoint, sub.Keys); err != nil {
		t.Fatalf("RemoveIfMatches: %v", err)
	}
	if got := st.Count(); got != 0 {
		t.Errorf("after exact prune: %d subscriptions, want 0", got)
	}
}

// TestRemoveIfMatchesNoMatchIsNoOp asserts a non-matching endpoint or keys
// leaves the store untouched.
func TestRemoveIfMatchesNoMatchIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscriptions.json")
	st := NewStore(path)

	sub := Subscription{
		Endpoint: "https://push.example/keep",
		Keys:     SubscriptionKeys{P256dh: "p", Auth: "a"},
	}
	if err := st.Add(sub); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Wrong keys at the right endpoint.
	if err := st.RemoveIfMatches(sub.Endpoint, SubscriptionKeys{P256dh: "other", Auth: "other"}); err != nil {
		t.Fatalf("RemoveIfMatches wrong keys: %v", err)
	}
	// Right keys at the wrong endpoint.
	if err := st.RemoveIfMatches("https://push.example/gone", sub.Keys); err != nil {
		t.Fatalf("RemoveIfMatches wrong endpoint: %v", err)
	}
	if got := st.Count(); got != 1 {
		t.Errorf("after no-op prunes: %d subscriptions, want 1", got)
	}
}
