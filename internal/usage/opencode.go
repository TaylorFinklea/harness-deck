package usage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// openCodeProvider reports OpenCode spend by shelling out to `opencode stats
// --days N`. It reads the CLI's local data — no cookie, no network, no
// web-fingerprint hashes. The binary must be installed and logged in.
//
// Feature-flagged off by default (Options.OpenCodeEnabled). `opencode stats`
// only counts local TUI sessions, so it reads $0 when the real spend runs
// through the opencode-go/Zen cloud plan (orchestra/pi) — that usage is
// account-scoped on opencode.ai and invisible to the local CLI. The code is
// kept behind the flag to revisit once a cloud-usage source exists (a headless
// Zen-API endpoint or the web session). See decisions.md.
type openCodeProvider struct {
	days int
}

func (openCodeProvider) Tool() string  { return "opencode" }
func (openCodeProvider) Label() string { return "OC" }

func (o openCodeProvider) Sample(ctx context.Context) Sample {
	days := o.days
	if days <= 0 {
		days = 7
	}
	bin, found := opencodeBin()
	if !found {
		return Sample{Err: "opencode CLI not found"}
	}
	// cmd.Stdin is left nil: Go gives the child /dev/null — required so a TUI
	// terminal-capability query on stdin doesn't block the poll goroutine forever.
	cmd := exec.CommandContext(ctx, bin, "stats", "--days", strconv.Itoa(days))
	out, err := cmd.Output()
	if err != nil {
		return Sample{Err: "opencode stats failed: " + err.Error()}
	}

	cost, input, output, ok := parseOpenCodeStats(string(out))
	if !ok {
		return Sample{Err: "could not parse opencode stats output"}
	}
	detail := fmt.Sprintf("%dd", days)
	if input != "" || output != "" {
		detail = fmt.Sprintf("%dd · in %s / out %s", days, input, output)
	}
	return Sample{OK: true, Kind: KindBudget, Text: cost, Detail: detail}
}

// opencodeBin resolves the opencode CLI: $PATH first, then common install
// locations. The dashboard usually runs under launchd (macOS) or systemd
// (Linux), which give a minimal PATH that omits Homebrew (/opt/homebrew/bin)
// and ~/.local/bin — so a bare exec/LookPath fails even when opencode is
// installed and works in an interactive shell.
func opencodeBin() (string, bool) {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p, true
	}
	cands := []string{"/opt/homebrew/bin/opencode", "/usr/local/bin/opencode"}
	if h := home(); h != "" {
		cands = append(cands,
			filepath.Join(h, ".opencode", "bin", "opencode"),
			filepath.Join(h, ".local", "bin", "opencode"),
		)
	}
	for _, c := range cands {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, true
		}
	}
	return "", false
}

// reCost matches a dollar amount like $10.02 or $0.00.
var reCost = regexp.MustCompile(`\$[0-9][0-9.,]*`)

// parseOpenCodeStats extracts cost, input tokens, and output tokens from the
// box-drawing table printed by `opencode stats --days N`. It is package-internal
// so it can be tested independently of the opencode binary.
//
// Expected line shapes (box chars and spacing vary):
//
//	│Total Cost                                        $10.02 │
//	│Input                                             14.5M │
//	│Output                                           545.5K │
func parseOpenCodeStats(out string) (cost, input, output string, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "Total Cost"):
			if m := reCost.FindString(line); m != "" {
				cost = m
			}
		case strings.Contains(line, "Input"):
			if tok := lastToken(line); tok != "" {
				input = tok
			}
		case strings.Contains(line, "Output"):
			if tok := lastToken(line); tok != "" {
				output = tok
			}
		}
	}
	ok = cost != ""
	return
}

// lastToken returns the last whitespace-separated token before the trailing
// box-drawing character (│ or similar) on a line. The token is the value field
// in the stats table (e.g. "14.5M", "545.5K", "$10.02").
func lastToken(line string) string {
	// Strip trailing box char and whitespace.
	line = strings.TrimRight(line, " \t│|")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
