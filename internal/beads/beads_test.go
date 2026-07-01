package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinFallback(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveBin("definitely-not-on-path-xyz", []string{bin})
	if !ok || got != bin {
		t.Fatalf("want %s, got %q ok=%v", bin, got, ok)
	}
	if _, ok := resolveBin("definitely-not-on-path-xyz", []string{filepath.Join(dir, "nope")}); ok {
		t.Error("missing fallback should not resolve")
	}
}

func TestValidID(t *testing.T) {
	for _, s := range []string{"harness-deck-5ph.1", "abc_1", "tga-0iqq"} {
		if !ValidID(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "-rf", "--force", "a b", "a/b", "a;rm", "a$(x)"} {
		if ValidID(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestFlagLike(t *testing.T) {
	for _, s := range []string{"-rf", "--force", "-1"} {
		if !flagLike(s) {
			t.Errorf("%q should be flag-like", s)
		}
	}
	for _, s := range []string{"harness-deck-5ph.1", "abc"} {
		if flagLike(s) {
			t.Errorf("%q should NOT be flag-like", s)
		}
	}
}

func TestDiscoverKeysOnBeads(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, subs ...string) {
		for _, s := range subs {
			if err := os.MkdirAll(filepath.Join(root, name, s), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if len(subs) == 0 {
			os.MkdirAll(filepath.Join(root, name), 0o755)
		}
	}
	mk("tga", ".beads")                      // .beads, no .docs/ai — the landmine case
	mk("harness-deck", ".beads", ".docs/ai") // both
	mk("chezmoi-config", ".docs/ai")         // .docs/ai only → excluded
	mk("plain")                              // neither → excluded
	mk(".hidden", ".beads")                  // hidden child → skipped

	repos := Discover([]string{root}, nil)
	names := map[string]bool{}
	for _, r := range repos {
		names[r.Name] = true
	}
	if !names["tga"] {
		t.Error("tga (.beads, no .docs/ai) must be discovered")
	}
	if !names["harness-deck"] {
		t.Error("harness-deck (.beads) must be discovered")
	}
	if names["chezmoi-config"] {
		t.Error(".docs/ai-only must NOT be discovered")
	}
	if names["plain"] {
		t.Error("neither must NOT be discovered")
	}
	if names[".hidden"] {
		t.Error("hidden child must be skipped")
	}
}

func TestDiscoverExplicitAndDedup(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "explicitrepo")
	os.MkdirAll(filepath.Join(repo, ".beads"), 0o755)
	// explicit + scan-root discovery of the same repo → deduped to one.
	repos := Discover([]string{root}, []string{repo})
	n := 0
	for _, r := range repos {
		if r.Name == "explicitrepo" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want explicitrepo once (deduped), got %d", n)
	}
}
