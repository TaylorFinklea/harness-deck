// Command harness-deck renders and serves unified harness reports.
package main

import (
	"fmt"
	"os"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/render"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		cmdValidate(os.Args[2:])
	case "render":
		cmdRender(os.Args[2:])
	case "serve":
		fmt.Fprintln(os.Stderr, "serve: not yet implemented (Phase 2)")
		os.Exit(1)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `harness-deck — unified harness dashboard

usage:
  harness-deck validate <report.json>        check a manifest for problems
  harness-deck render <report.json> [-o f]   render a manifest to HTML
  harness-deck serve                         start the dashboard server
`)
}

// cmdValidate parses and validates a manifest, printing every problem found.
func cmdValidate(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: harness-deck validate <report.json>")
		os.Exit(2)
	}
	rep := mustParse(args[0])
	problems := rep.Validate()
	if len(problems) == 0 {
		fmt.Printf("ok — %s: %d block(s), no problems\n", args[0], len(rep.Blocks))
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %d problem(s)\n", args[0], len(problems))
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "  · %s\n", p)
	}
	os.Exit(1)
}

// cmdRender renders a manifest to HTML, warning about (but not blocking on)
// validation problems — the renderer degrades bad blocks gracefully. The -o
// flag may appear before or after the report path.
func cmdRender(args []string) {
	var out, src string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "render: -o needs a file path")
				os.Exit(2)
			}
			out, i = args[i+1], i+1
		default:
			if src != "" {
				fmt.Fprintln(os.Stderr, "usage: harness-deck render <report.json> [-o out.html]")
				os.Exit(2)
			}
			src = args[i]
		}
	}
	if src == "" {
		fmt.Fprintln(os.Stderr, "usage: harness-deck render <report.json> [-o out.html]")
		os.Exit(2)
	}
	rep := mustParse(src)
	if problems := rep.Validate(); len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d validation problem(s) — rendering anyway:\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "  · %s\n", p)
		}
	}
	r, err := render.New()
	if err != nil {
		fatal("renderer", err)
	}
	html, err := r.Report(rep)
	if err != nil {
		fatal("render", err)
	}
	if out == "" {
		os.Stdout.Write(html)
		return
	}
	if err := os.WriteFile(out, html, 0o644); err != nil {
		fatal("write", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d bytes)\n", out, len(html))
}

func mustParse(path string) *manifest.Report {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read", err)
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		fatal("parse "+path, err)
	}
	return rep
}

func fatal(ctx string, err error) {
	fmt.Fprintf(os.Stderr, "harness-deck: %s: %v\n", ctx, err)
	os.Exit(1)
}
