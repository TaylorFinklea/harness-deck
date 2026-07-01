package beads

import (
	"context"
	"hash/fnv"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// nowUTC is the clock, indirected so tests can pin the Updated timestamp.
var nowUTC = func() time.Time { return time.Now().UTC() }

// fetcher is the subset of Client the Monitor needs; tests inject a fake.
type fetcher interface {
	Ready(ctx context.Context, root string) ([]Issue, error)
	Blocked(ctx context.Context, root string) ([]Issue, error)
	List(ctx context.Context, root string) ([]Issue, error)
	Status(ctx context.Context, root string) (Counts, error)
}

// Monitor polls beads repos on an interval and caches a Snapshot. It re-discovers
// repos each tick (via the repos thunk) so a newly-initialized repo appears live.
// When the snapshot's data fingerprint changes, onChange fires (the server wires
// it to an SSE broadcast). Safe for concurrent use.
type Monitor struct {
	fetch    fetcher
	repos    func() []Repo
	interval time.Duration
	onChange func()

	mu   sync.RWMutex
	snap Snapshot
	fp   string
}

// NewMonitor builds a Monitor. A non-positive interval defaults to 15s.
func NewMonitor(f fetcher, repos func() []Repo, interval time.Duration, onChange func()) *Monitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &Monitor{fetch: f, repos: repos, interval: interval, onChange: onChange}
}

// Start refreshes once immediately, then on every interval until ctx is done.
// Non-blocking; safe on a nil Monitor (no-op).
func (m *Monitor) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		m.refreshOnce(ctx)
		t := time.NewTicker(m.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.refreshOnce(ctx)
			}
		}
	}()
}

// refreshOnce rebuilds the snapshot from every discovered repo, swaps the cache,
// and fires onChange when the data fingerprint changed.
func (m *Monitor) refreshOnce(ctx context.Context) {
	snap := Snapshot{Available: true, Updated: nowUTC().Format(time.RFC3339)}
	for _, r := range m.repos() {
		snap.Repos = append(snap.Repos, m.fetchRepo(ctx, r))
	}
	fp := fingerprint(snap.Repos)

	m.mu.Lock()
	changed := fp != m.fp
	m.fp = fp
	m.snap = snap
	m.mu.Unlock()

	if changed && m.onChange != nil {
		m.onChange()
	}
}

// fetchRepo pulls one repo's ready/blocked/all/status. A failure on one of the
// three essential calls (ready/blocked/list) sets Err and leaves the rest zero
// so one broken repo can't sink the others. Status is best-effort: it only
// carries summary counts, so if it fails we keep the already-fetched issues and
// derive counts from the lists rather than discarding a healthy backlog.
func (m *Monitor) fetchRepo(ctx context.Context, r Repo) RepoSnapshot {
	rs := RepoSnapshot{Name: r.Name, Root: r.Root}
	ready, err := m.fetch.Ready(ctx, r.Root)
	if err != nil {
		rs.Err = err.Error()
		return rs
	}
	blocked, err := m.fetch.Blocked(ctx, r.Root)
	if err != nil {
		rs.Err = err.Error()
		return rs
	}
	all, err := m.fetch.List(ctx, r.Root)
	if err != nil {
		rs.Err = err.Error()
		return rs
	}
	rs.Ready, rs.Blocked, rs.All = ready, blocked, all
	rs.Edges = deriveEdges(all, blocked)
	if counts, err := m.fetch.Status(ctx, r.Root); err == nil {
		rs.Counts = counts
	} else {
		rs.Counts = Counts{Ready: len(ready), Blocked: len(blocked), Open: len(all), Total: len(all)}
	}
	return rs
}

// Snapshot returns the cached snapshot. Safe on a nil Monitor (empty snapshot).
func (m *Monitor) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snap
}

// fingerprint hashes every field the Backlog view reflects — id, status,
// priority, title, blocked_by, and updated — so any change a user would see
// (e.g. a priority bump, which bd does not always stamp into updated_at) fires
// onChange, while an identical refresh does not.
func fingerprint(repos []RepoSnapshot) string {
	h := fnv.New64a()
	for _, r := range repos {
		io.WriteString(h, r.Name+"|"+r.Err+"\n")
		for _, group := range [][]Issue{r.Ready, r.Blocked, r.All} {
			for _, is := range group {
				io.WriteString(h, is.ID+"|"+is.Status+"|"+strconv.Itoa(is.Priority)+"|"+
					is.Title+"|"+strings.Join(is.BlockedBy, ",")+"|"+is.Updated+"\n")
			}
		}
	}
	return strconv.FormatUint(h.Sum64(), 16)
}
