package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// cmdNew scaffolds a starter report.json — id, created timestamp, status
// "draft", and a placeholder prose block — and writes it to the right
// directory. Defaults are aggressive so the common path is one flag:
// `harness-deck new --title "audit"`. The project is inferred from the
// current directory's git repo top-level (or the cwd basename).
func cmdNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	var (
		project = fs.String("project", "", "project name (default: git repo or cwd basename)")
		title   = fs.String("title", "", "report title (required)")
		id      = fs.String("id", "", "run id (default: YYYYMMDD-HHMM)")
		harness = fs.String("harness", "manual", "harness id: claude-code | pi-mono | opencode | codex | manual")
		agent   = fs.String("agent", "", "model id (optional)")
		kind    = fs.String("kind", "progress", "report kind: audit | progress | idea | ask | roadmap | debug")
		inRepo  = fs.Bool("in-repo", false, "write to <repo>/.harness/<id>/ instead of the central reports dir")
		out     = fs.String("out", "", "explicit output path (overrides --in-repo)")
		force   = fs.Bool("force", false, "overwrite if the target report.json already exists")
	)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: harness-deck new --title "TITLE" [flags]

Scaffold a starter harness-deck report.json with sensible defaults.
The file is written to ~/.harness/reports/<project>/<id>/report.json
unless --in-repo (writes to <repo>/.harness/<id>/) or --out (explicit
path) is given.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *title == "" {
		fmt.Fprintln(os.Stderr, "new: --title is required")
		fs.Usage()
		os.Exit(2)
	}

	cwd, _ := os.Getwd()
	if *project == "" {
		*project = inferProject(cwd)
	}
	if *id == "" {
		*id = time.Now().UTC().Format("20060102-150405")
	}

	// Pick the run directory.
	var runDir string
	switch {
	case *out != "":
		runDir = filepath.Dir(*out)
	case *inRepo:
		root, err := repoRoot(cwd)
		if err != nil {
			fatal("new", fmt.Errorf("--in-repo: %w", err))
		}
		runDir = filepath.Join(root, ".harness", *id)
	default:
		// Resolve the configured central dir (handles ~ expansion).
		cfg, err := config.Load()
		if err != nil {
			fatal("new", fmt.Errorf("load config: %w", err))
		}
		runDir = filepath.Join(config.Expand(cfg.CentralDir), *project, *id)
	}

	target := *out
	if target == "" {
		target = filepath.Join(runDir, "report.json")
	}
	if _, err := os.Stat(target); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "new: %s already exists. Pass --force to overwrite.\n", target)
		os.Exit(1)
	}

	body := starterReport(*project, *id, *title, *harness, *agent, *kind)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fatal("new: mkdir", err)
	}
	if err := atomicWrite(target, append(body, '\n'), 0o644); err != nil {
		fatal("new: write", err)
	}
	fmt.Printf("wrote %s\n", target)
	fmt.Println("next: edit the prose block(s), then `harness-deck validate` and refresh the dashboard.")
}

// starterReport produces a minimal-but-valid manifest as pretty JSON. We
// hand-build the string so the field order matches CONTRACT.md (schema /
// id / project / harness / agent? / title / kind / status / created /
// blocks) — agents tend to learn from the first example they see.
func starterReport(project, id, title, harness, agent, kind string) []byte {
	created := time.Now().UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  %q: %q,\n", "schema", "harness-deck/report@1")
	fmt.Fprintf(&b, "  %q: %q,\n", "id", id)
	fmt.Fprintf(&b, "  %q: %q,\n", "project", project)
	fmt.Fprintf(&b, "  %q: %q,\n", "harness", harness)
	if agent != "" {
		fmt.Fprintf(&b, "  %q: %q,\n", "agent", agent)
	}
	fmt.Fprintf(&b, "  %q: %q,\n", "title", title)
	fmt.Fprintf(&b, "  %q: %q,\n", "kind", kind)
	fmt.Fprintf(&b, "  %q: %q,\n", "status", "draft")
	fmt.Fprintf(&b, "  %q: %q,\n", "created", created)
	b.WriteString("  \"blocks\": [\n")
	b.WriteString("    {\n")
	b.WriteString("      \"type\": \"prose\",\n")
	b.WriteString("      \"title\": \"summary\",\n")
	b.WriteString("      \"markdown\": \"Replace this prose block with the actual report content.\\n\\n- Add bullets, **bold**, `code`.\\n- Switch `status` to `awaiting-review` once you add interactive blocks (ask/decision/approval).\"\n")
	b.WriteString("    }\n")
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	return []byte(b.String())
}

// inferProject returns the basename of the nearest git repo root,
// falling back to the basename of dir.
func inferProject(dir string) string {
	if root, err := repoRoot(dir); err == nil {
		return filepath.Base(root)
	}
	return filepath.Base(dir)
}

// repoRoot walks dir upward looking for a .git marker.
func repoRoot(dir string) (string, error) {
	cur := dir
	for cur != "" && cur != "/" {
		if fi, err := os.Stat(filepath.Join(cur, ".git")); err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", fmt.Errorf("no git repo found above %s", dir)
}

// atomicWrite is the temp+rename pattern reused across the repo so a
// crash mid-write cannot truncate a target file.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
