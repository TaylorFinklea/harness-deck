package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
	"github.com/TaylorFinklea/harness-deck/internal/manifest"
)

// cmdNew scaffolds a starter report.json — id, created timestamp, status
// "draft", and a placeholder prose block — and writes it to the right
// directory. Defaults are aggressive so the common path is one flag:
// `harness-deck new --title "audit"`. The project is inferred from the
// current directory's git repo top-level (or the cwd basename).
func cmdNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	var (
		project  = fs.String("project", "", "project name (default: git repo or cwd basename)")
		title    = fs.String("title", "", "report title (required)")
		id       = fs.String("id", "", "run id (default: YYYYMMDD-HHMM)")
		harness  = fs.String("harness", "manual", "harness id: claude-code | pi-mono | opencode | codex | manual")
		agent    = fs.String("agent", "", "model id (optional)")
		kind     = fs.String("kind", "progress", "report kind: audit | review | progress | idea | decision | roadmap | …")
		template = fs.String("template", "", "scaffold from a template (also sets --kind), pre-filling blocks: "+templateNames())
		inRepo   = fs.Bool("in-repo", false, "write to <repo>/.harness/<id>/ instead of the central reports dir")
		out      = fs.String("out", "", "explicit output path (overrides --in-repo)")
		force    = fs.Bool("force", false, "overwrite if the target report.json already exists")
	)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: harness-deck new --title "TITLE" [flags]
       harness-deck new --template audit   (title + kind default from the template)

Scaffold a starter harness-deck report.json with sensible defaults.
The file is written to ~/.harness/reports/<project>/<id>/report.json
unless --in-repo (writes to <repo>/.harness/<id>/) or --out (explicit
path) is given.

With --template, the report is pre-filled with the block shapes that kind of
report usually needs (see docs/PUBLISHING.md). Without it, you get a single
placeholder prose block.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	tmpl := reportTemplate{blocks: defaultBlocks}
	if *template != "" {
		t, ok := reportTemplates[*template]
		if !ok {
			fmt.Fprintf(os.Stderr, "new: unknown template %q (want: %s)\n", *template, templateNames())
			fs.Usage()
			os.Exit(2)
		}
		tmpl = t
	}

	// A template can supply both a default title (one-flag `new --template
	// audit` works) and a default kind (the template name), each overridden by
	// the matching explicit flag.
	if *title == "" {
		if tmpl.title != "" {
			*title = tmpl.title
		} else {
			fmt.Fprintln(os.Stderr, "new: --title is required (or pass --template)")
			fs.Usage()
			os.Exit(2)
		}
	}
	if *template != "" && !flagSet(fs, "kind") {
		*kind = *template
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

	body := starterReport(*project, *id, *title, *harness, *agent, *kind, tmpl.blocks)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fatal("new: mkdir", err)
	}
	if err := jsonfile.AtomicWrite(target, append(body, '\n'), 0o644); err != nil {
		fatal("new: write", err)
	}
	fmt.Printf("wrote %s\n", target)
	if tmpl.interactive {
		// Interactive templates ship an ask/decision/approval but scaffold as
		// draft; remind the user to flip status so the question surfaces.
		fmt.Println("next: fill in the placeholders, set status to \"awaiting-review\" once the question is real, then `harness-deck validate` and refresh the dashboard.")
	} else {
		fmt.Println("next: fill in the placeholder content, then `harness-deck validate` and refresh the dashboard.")
	}
}

// starterReport produces a minimal-but-valid manifest as pretty JSON. We
// hand-build the string so the field order matches CONTRACT.md (schema /
// id / project / harness / agent? / title / kind / status / created /
// blocks) — agents tend to learn from the first example they see. blocksJSON
// is the JSON value of the "blocks" array (the default single-prose scaffold,
// or a --template block set; see templates.go). It is spliced in verbatim, so
// callers must pass a well-formed JSON array value — only the trusted
// package-level constants in templates.go do, and TestNewTemplatesValidate
// guards them.
func starterReport(project, id, title, harness, agent, kind, blocksJSON string) []byte {
	created := time.Now().UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  %q: %q,\n", "schema", manifest.Schema)
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
	fmt.Fprintf(&b, "  %q: %s\n", "blocks", blocksJSON)
	b.WriteString("}\n")
	return []byte(b.String())
}

// flagSet reports whether the named flag was explicitly passed on the command
// line (as opposed to carrying its default). Used so an explicit --kind wins
// over a template's default kind.
func flagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
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
