package push

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
)

// Store persists the set of push Subscriptions we deliver notifications to.
// It is safe for concurrent use. One Subscription per push endpoint URL —
// the same browser/device resubscribing replaces its prior record.
type Store struct {
	path string
	mu   sync.Mutex
}

// NewStore opens the JSON-backed subscription store at path. A missing
// file is fine: it materializes on first Save.
func NewStore(path string) *Store { return &Store{path: path} }

// All returns every stored subscription (snapshot copy).
func (s *Store) All() []Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Add inserts sub, replacing any existing entry with the same endpoint.
func (s *Store) Add(sub Subscription) error {
	if sub.Endpoint == "" {
		return errors.New("subscription missing endpoint")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.load()
	replaced := false
	for i := range subs {
		if subs[i].Endpoint == sub.Endpoint {
			subs[i] = sub
			replaced = true
			break
		}
	}
	if !replaced {
		subs = append(subs, sub)
	}
	return s.save(subs)
}

// Remove deletes the subscription with the given endpoint. A missing
// endpoint is a no-op so unsubscribe is idempotent.
func (s *Store) Remove(endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	subs := s.load()
	out := subs[:0]
	for _, x := range subs {
		if x.Endpoint != endpoint {
			out = append(out, x)
		}
	}
	return s.save(out)
}

// Count returns the number of stored subscriptions.
func (s *Store) Count() int {
	return len(s.All())
}

// load reads subscriptions.json. A missing or corrupt file yields zero
// subscriptions — push features simply stay dormant.
func (s *Store) load() []Subscription {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return nil
	}
	var subs []Subscription
	if json.Unmarshal(data, &subs) != nil {
		return nil
	}
	return subs
}

// save writes subs atomically (unique temp file + rename).
func (s *Store) save(subs []Subscription) error {
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return err
	}
	return jsonfile.AtomicWrite(s.path, append(data, '\n'), 0o600)
}
