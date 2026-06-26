package usage

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// openCodeProvider reports OpenCode spend by shelling out to `opencode stats
// --days N`. It reads the CLI's local data — no cookie, no network, no
// web-fingerprint hashes. The binary must be installed and logged in.
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
	// cmd.Stdin is left nil: Go gives the child /dev/null — required so a TUI
	// terminal-capability query on stdin doesn't block the poll goroutine forever.
	cmd := exec.CommandContext(ctx, "opencode", "stats", "--days", strconv.Itoa(days))
	out, err := cmd.Output()
	if err != nil {
		// exec.Command wraps a LookPath failure in *exec.Error{Err: exec.ErrNotFound}.
		var e *exec.Error
		if (errors.As(err, &e) && errors.Is(e.Err, exec.ErrNotFound)) ||
			errors.Is(err, exec.ErrNotFound) {
			return Sample{Err: "opencode CLI not found"}
		}
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
