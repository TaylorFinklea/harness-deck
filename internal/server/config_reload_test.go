package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// TestTickReloadsRegisteredProject makes the `register` --help promise true:
// after config.json gains a project, the next watcher tick reloads the
// discovery roots so the dashboard sees it without a restart.
func TestTickReloadsRegisteredProject(t *testing.T) {
	central := t.TempDir()
	proj := t.TempDir() // a project root registered after startup
	s := newTestServerFull(t, config.Config{CentralDir: central})
	ws := s.initWatchState()

	if projectDiscovered(s, proj) {
		t.Fatal("project should not be discovered before registration")
	}

	// Register it by writing config.json (HARNESS_DECK_CONFIG points here via
	// isolateState in newTestServerFull).
	cfgBody := fmt.Sprintf(`{"central_dir":%q,"projects":[%q]}`, central, proj)
	if err := os.WriteFile(config.Path(), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	s.tick(ws) // config mtime changed ⇒ roots reloaded

	if !projectDiscovered(s, proj) {
		t.Error("registered project not picked up after config change + tick")
	}
}

func projectDiscovered(s *Server, path string) bool {
	base := filepath.Base(path)
	for _, p := range s.projects.Discovered() {
		if p.Name == base {
			return true
		}
	}
	return false
}
