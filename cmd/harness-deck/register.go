package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
)

// cmdRegister adds a project root to the harness-deck config's `projects`
// array — for projects that don't sit under any `scan_roots` and would
// otherwise stay invisible. The path is validated (directory exists),
// canonicalised to an absolute path, and dedup'd. config.json is read,
// updated, and atomically rewritten preserving any field we don't touch.
func cmdRegister(args []string) {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	var (
		remove = fs.Bool("remove", false, "remove the path from the config's `projects` array instead of adding it")
	)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: harness-deck register [--remove] <path>

Adds <path> (or removes it with --remove) from the `+"`projects`"+` array
of ~/.config/harness-deck/config.json. The dashboard will pick it up
on its next watcher tick (or restart). Paths are stored as absolute,
with ~ expanded.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	abs, err := filepath.Abs(config.Expand(fs.Arg(0)))
	if err != nil {
		fatal("register: abs path", err)
	}
	if !*remove {
		if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
			fmt.Fprintf(os.Stderr, "register: %s is not a directory\n", abs)
			os.Exit(1)
		}
	}

	path := config.Path()
	current, err := loadConfigMap(path)
	if err != nil {
		fatal("register: read config", err)
	}
	projects := stringSlice(current["projects"])

	if *remove {
		out := projects[:0]
		removed := false
		for _, p := range projects {
			if p == abs {
				removed = true
				continue
			}
			out = append(out, p)
		}
		if !removed {
			fmt.Fprintf(os.Stderr, "register: %s is not in the config's projects list\n", abs)
			os.Exit(1)
		}
		projects = out
		fmt.Printf("removed %s\n", abs)
	} else {
		for _, p := range projects {
			if p == abs {
				fmt.Printf("already registered: %s\n", abs)
				return
			}
		}
		projects = append(projects, abs)
		sort.Strings(projects)
		fmt.Printf("added %s\n", abs)
	}

	current["projects"] = projects
	if err := writeConfigMap(path, current); err != nil {
		fatal("register: write", err)
	}
	fmt.Printf("wrote %s\n", path)
}

// loadConfigMap reads the existing config as a generic map so the
// register subcommand never throws away fields it doesn't know about.
// A missing file degrades to an empty map; an existing-but-unparseable
// file is an error — never a silent restart from empty that would make
// the subsequent write clobber the user's settings.
func loadConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s exists but is not valid JSON — refusing to rewrite it: %w", path, err)
	}
	return m, nil
}

// writeConfigMap pretty-prints m and atomically renames into place.
func writeConfigMap(path string, m map[string]any) error {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return jsonfile.AtomicWrite(path, append(body, '\n'), 0o644)
}

// stringSlice coerces a JSON-decoded value to []string, accepting both
// nil and an existing []any. Anything not a string is dropped.
func stringSlice(v any) []string {
	if v == nil {
		return nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(a))
	for _, x := range a {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
