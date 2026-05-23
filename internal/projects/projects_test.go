package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeState seeds a projects.json before constructing a Manager — handy for
// driving Discovered's order/disabled handling.
func writeState(t *testing.T, path string, state map[string]any) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkProject creates a directory named name under root. With withDocs it also
// creates the .docs/ai marker directory that makes it a discoverable project.
func mkProject(t *testing.T, root, name string, withDocs bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	target := dir
	if withDocs {
		target = filepath.Join(dir, ".docs", "ai")
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestStatePathFollowsConfigOverride checks that projects.json is placed
// beside the config file, so the HARNESS_DECK_CONFIG override relocates both.
func TestStatePathFollowsConfigOverride(t *testing.T) {
	t.Setenv("HARNESS_DECK_CONFIG", "/tmp/hd-test/config.json")
	if got, want := StatePath(), "/tmp/hd-test/projects.json"; got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}

// TestDiscoveredFindsOnlyDirsWithDocsAI checks depth-1 discovery: a direct
// child of a scan root counts only if it holds a .docs/ai directory. Files
// and dotfile directories are ignored, and new projects default to enabled.
func TestDiscoveredFindsOnlyDirsWithDocsAI(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	mkProject(t, root, "gamma", false)  // no .docs/ai
	mkProject(t, root, ".hidden", true) // dotfile dir, skipped
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	var names []string
	for _, p := range m.Discovered() {
		names = append(names, p.Name)
		if !p.Enabled {
			t.Errorf("%s: Enabled=false, want true by default", p.Name)
		}
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered names = %v, want %v", names, want)
	}
}

// TestToggleDisablesAndPersists checks that Toggle hides a project and that
// the hidden state survives into a freshly constructed Manager.
func TestToggleDisablesAndPersists(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	state := filepath.Join(t.TempDir(), "projects.json")

	if err := NewManager([]string{root}, nil, state).Toggle("beta"); err != nil {
		t.Fatalf("Toggle: %v", err)
	}

	for _, p := range NewManager([]string{root}, nil, state).Discovered() {
		want := p.Name != "beta"
		if p.Enabled != want {
			t.Errorf("%s: Enabled=%t, want %t", p.Name, p.Enabled, want)
		}
	}
}

// TestToggleTwiceReEnables checks that Toggle is a flip, not a one-way hide.
func TestToggleTwiceReEnables(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	m := NewManager([]string{root}, nil, state)

	if err := m.Toggle("alpha"); err != nil {
		t.Fatalf("Toggle 1: %v", err)
	}
	if err := m.Toggle("alpha"); err != nil {
		t.Fatalf("Toggle 2: %v", err)
	}
	for _, p := range m.Discovered() {
		if p.Name == "alpha" && !p.Enabled {
			t.Error("alpha hidden after two toggles, want visible")
		}
	}
}

// TestToggleUnknownProjectErrors checks that toggling a name that was never
// discovered is rejected rather than silently recorded.
func TestToggleUnknownProjectErrors(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	if err := m.Toggle("nonexistent"); err == nil {
		t.Error("Toggle(nonexistent) = nil, want error")
	}
}

// TestEnabledExcludesDisabled checks that Enabled returns only visible projects.
func TestEnabledExcludesDisabled(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	if err := m.Toggle("alpha"); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	en := m.Enabled()
	if len(en) != 1 || en[0].Name != "beta" {
		t.Fatalf("Enabled() = %v, want [beta]", en)
	}
}

// TestCorruptStateFileDegradesToAllVisible checks that an unreadable
// projects.json does not crash discovery — it falls back to everything
// visible, matching harness-deck's graceful-degradation rule.
func TestCorruptStateFileDegradesToAllVisible(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	if err := os.WriteFile(state, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, p := range NewManager([]string{root}, nil, state).Discovered() {
		if !p.Enabled {
			t.Errorf("%s hidden; a corrupt state file should degrade to visible", p.Name)
		}
	}
}

// TestExplicitProjectsIncluded checks that explicit project roots appear
// even when they sit outside any scan root and lack a .docs/ai directory.
func TestExplicitProjectsIncluded(t *testing.T) {
	scanRoot := t.TempDir()
	mkProject(t, scanRoot, "alpha", true)
	explicit := mkProject(t, t.TempDir(), "manual", false)

	m := NewManager([]string{scanRoot}, []string{explicit}, filepath.Join(t.TempDir(), "projects.json"))

	var names []string
	for _, p := range m.Discovered() {
		names = append(names, p.Name)
	}
	if want := []string{"alpha", "manual"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered names = %v, want %v", names, want)
	}
}

// TestFingerprintChangesOnToggle checks that the watcher fingerprint reacts
// to visibility changes.
func TestFingerprintChangesOnToggle(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	before := m.Fingerprint()
	if err := m.Toggle("alpha"); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if m.Fingerprint() == before {
		t.Error("Fingerprint unchanged after Toggle")
	}
}

// TestDiscoveredAppliesSavedOrder checks that a saved order in projects.json
// drives Discovered's output order instead of alphabetical.
func TestDiscoveredAppliesSavedOrder(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	mkProject(t, root, "gamma", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	writeState(t, state, map[string]any{"order": []string{"gamma", "alpha", "beta"}})

	m := NewManager([]string{root}, nil, state)
	var names []string
	for _, p := range m.Discovered() {
		names = append(names, p.Name)
	}
	if want := []string{"gamma", "alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered = %v, want %v", names, want)
	}
}

// TestDiscoveredAppendsNewProjectsAlpha checks that projects not in the saved
// order land at the end, alpha-sorted — newcomers don't shuffle existing ones.
func TestDiscoveredAppendsNewProjectsAlpha(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	mkProject(t, root, "gamma", true)
	mkProject(t, root, "delta", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	writeState(t, state, map[string]any{"order": []string{"gamma", "alpha"}})

	m := NewManager([]string{root}, nil, state)
	var names []string
	for _, p := range m.Discovered() {
		names = append(names, p.Name)
	}
	if want := []string{"gamma", "alpha", "beta", "delta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered = %v, want %v", names, want)
	}
}

// TestDiscoveredDropsMissingFromOrder checks that an order entry pointing at a
// project that no longer exists on disk is silently skipped.
func TestDiscoveredDropsMissingFromOrder(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	writeState(t, state, map[string]any{"order": []string{"ghost", "alpha"}})

	m := NewManager([]string{root}, nil, state)
	var names []string
	for _, p := range m.Discovered() {
		names = append(names, p.Name)
	}
	if want := []string{"alpha"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered = %v, want %v", names, want)
	}
}

// TestReorderPersists checks that Reorder writes a new order that a fresh
// Manager picks up from projects.json.
func TestReorderPersists(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	mkProject(t, root, "gamma", true)
	state := filepath.Join(t.TempDir(), "projects.json")

	if err := NewManager([]string{root}, nil, state).Reorder([]string{"gamma", "alpha", "beta"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	var names []string
	for _, p := range NewManager([]string{root}, nil, state).Discovered() {
		names = append(names, p.Name)
	}
	if want := []string{"gamma", "alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Discovered after reorder = %v, want %v", names, want)
	}
}

// TestReorderUnknownNameErrors checks that a name not in the discovered set
// is rejected — the user can't smuggle ghost projects into the order.
func TestReorderUnknownNameErrors(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	if err := m.Reorder([]string{"alpha", "ghost"}); err == nil {
		t.Error("Reorder(...ghost) = nil, want error")
	}
}

// TestReorderPreservesDisabled checks that reordering doesn't touch the
// hidden set — order and visibility are independent axes.
func TestReorderPreservesDisabled(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	state := filepath.Join(t.TempDir(), "projects.json")
	m := NewManager([]string{root}, nil, state)

	if err := m.Toggle("beta"); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if err := m.Reorder([]string{"beta", "alpha"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	for _, p := range m.Discovered() {
		want := p.Name != "beta"
		if p.Enabled != want {
			t.Errorf("%s: Enabled=%t, want %t", p.Name, p.Enabled, want)
		}
	}
}

// TestFingerprintChangesOnReorder checks that the watcher fingerprint reacts
// to order changes so the dashboard can refresh.
func TestFingerprintChangesOnReorder(t *testing.T) {
	root := t.TempDir()
	mkProject(t, root, "alpha", true)
	mkProject(t, root, "beta", true)
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	before := m.Fingerprint()
	if err := m.Reorder([]string{"beta", "alpha"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if m.Fingerprint() == before {
		t.Error("Fingerprint unchanged after Reorder")
	}
}

// TestFingerprintChangesOnDocEdit checks that the fingerprint reacts to edits
// of a project's .docs/ai docs, so the watcher can trigger a refresh.
func TestFingerprintChangesOnDocEdit(t *testing.T) {
	root := t.TempDir()
	proj := mkProject(t, root, "alpha", true)
	doc := filepath.Join(proj, ".docs", "ai", "current-state.md")
	if err := os.WriteFile(doc, []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager([]string{root}, nil, filepath.Join(t.TempDir(), "projects.json"))

	before := m.Fingerprint()
	if err := os.WriteFile(doc, []byte("a considerably longer body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m.Fingerprint() == before {
		t.Error("Fingerprint unchanged after editing current-state.md")
	}
}
