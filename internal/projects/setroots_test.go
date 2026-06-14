package projects

import (
	"path/filepath"
	"testing"
)

// TestSetRootsReloadsExplicitProject confirms SetRoots swaps the discovery
// roots so a project registered after construction shows up on the next
// Discovered() — the runtime config-reload path.
func TestSetRootsReloadsExplicitProject(t *testing.T) {
	proj := t.TempDir()
	m := NewManager(nil, nil, filepath.Join(t.TempDir(), "projects.json"))
	if got := m.Discovered(); len(got) != 0 {
		t.Fatalf("expected no projects initially, got %d", len(got))
	}

	m.SetRoots(nil, []string{proj})

	got := m.Discovered()
	if len(got) != 1 || got[0].Name != filepath.Base(proj) {
		t.Fatalf("after SetRoots: %+v, want the registered project %q", got, filepath.Base(proj))
	}
	if !got[0].Enabled {
		t.Errorf("newly registered project should default to enabled")
	}
}
