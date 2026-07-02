package beads

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestValidTypeAndPriority(t *testing.T) {
	for _, ty := range []string{"bug", "feature", "task", "epic", "chore"} {
		if !ValidType(ty) {
			t.Errorf("%q should be a valid type", ty)
		}
	}
	for _, ty := range []string{"", "epi", "event", "Task", "-x"} {
		if ValidType(ty) {
			t.Errorf("%q should be invalid type", ty)
		}
	}
	for _, p := range []string{"0", "1", "2", "3", "4"} {
		if !ValidPriority(p) {
			t.Errorf("%q should be valid priority", p)
		}
	}
	for _, p := range []string{"", "5", "-1", "P2", "10"} {
		if ValidPriority(p) {
			t.Errorf("%q should be invalid priority", p)
		}
	}
}

func TestValidTitle(t *testing.T) {
	for i, s := range []string{"hi", "Fix the bug (P0)", strings.Repeat("x", 500)} {
		if !ValidTitle(s) {
			t.Errorf("valid case %d should pass", i)
		}
	}
	for i, s := range []string{"", strings.Repeat("x", 501), "line\nbreak", "tab\there", "bell\x07"} {
		if ValidTitle(s) {
			t.Errorf("invalid case %d should fail", i)
		}
	}
}

// TestWriteArgvIsInjectionSafe is the regression gate for the feature's core
// safety claim: free-text values reach bd as single --flag=value tokens under
// exec.Command, so a leading '-', embedded '=', spaces, or a newline can't be
// reparsed as a bd flag. A fake bd captures argv NUL-delimited.
func TestWriteArgvIsInjectionSafe(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "argv")
	bd := filepath.Join(dir, "bd")
	script := "#!/bin/sh\nprintf '%s\\0' \"$@\" > '" + out + "'\necho demo-fake\n"
	if err := os.WriteFile(bd, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Client{bin: bd}

	// Hostile title: leading dash, embedded flag, spaces, '=', and a newline.
	hostile := "-x --type=bug=y\nsecond"
	id, err := c.Create(context.Background(), "/repo", hostile, "task", "2", "desc -d=z")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "demo-fake" {
		t.Errorf("id = %q, want demo-fake", id)
	}
	args := splitNUL(t, out)
	assertToken(t, args, "--title="+hostile)
	assertToken(t, args, "--description=desc -d=z")
	assertToken(t, args, "create")
	assertToken(t, args, "--silent")

	// Close reason with a leading dash stays bound to --reason.
	if err := c.Close(context.Background(), "/repo", "demo-1", "-rf and spaces"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	args = splitNUL(t, out)
	assertToken(t, args, "--reason=-rf and spaces")
	assertToken(t, args, "demo-1")
}

func splitNUL(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.TrimRight(string(b), "\x00"))
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}

func assertToken(t *testing.T, args []string, want string) {
	t.Helper()
	for _, a := range args {
		if a == want {
			return
		}
	}
	t.Errorf("argv missing single token %q; got %#v", want, args)
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
