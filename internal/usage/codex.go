package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// codexProvider reads OpenAI Codex CLI rate-limit usage from the local session
// logs — no auth, no network. Codex writes one JSONL file per session under
// $CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl (CODEX_HOME defaults to
// ~/.codex); the rate-limit windows ride on `token_count` events. We scan the
// newest files for the most recent event whose primary window is populated.
//
// The schema is internal to Codex and undocumented (it has already drifted —
// resets_at is now an absolute unix epoch), so parsing is lenient.
type codexProvider struct{}

func (codexProvider) Tool() string  { return "codex" }
func (codexProvider) Label() string { return "CX" }

type codexWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"` // unix epoch seconds
}

type codexLine struct {
	Payload struct {
		Type       string `json:"type"`
		RateLimits *struct {
			PlanType  string       `json:"plan_type"`
			Primary   *codexWindow `json:"primary"`
			Secondary *codexWindow `json:"secondary"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

func (c codexProvider) Sample(ctx context.Context) Sample {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		if h := home(); h != "" {
			root = filepath.Join(h, ".codex")
		}
	}
	if root == "" {
		return Sample{Err: "no CODEX_HOME / home dir"}
	}

	files := codexSessionFiles(root)
	if len(files) == 0 {
		return Sample{Err: "no codex session logs found"}
	}
	newestFirst(files)

	// Scan newest files for the most recent token_count carrying a populated
	// primary window. Cap the scan so a deep history can't make a tick crawl.
	const maxFiles = 40
	for i, path := range files {
		if i >= maxFiles {
			break
		}
		if ctx.Err() != nil {
			return Sample{Err: "cancelled"}
		}
		for _, line := range jsonlLinesReverse(path) {
			var l codexLine
			if err := json.Unmarshal(line, &l); err != nil {
				continue
			}
			if l.Payload.Type != "token_count" || l.Payload.RateLimits == nil || l.Payload.RateLimits.Primary == nil {
				continue
			}
			return codexSample(*l.Payload.RateLimits.Primary, l.Payload.RateLimits.Secondary, l.Payload.RateLimits.PlanType)
		}
	}
	return Sample{Err: "no token_count event with rate limits"}
}

func codexSample(primary codexWindow, secondary *codexWindow, plan string) Sample {
	s := Sample{OK: true, Kind: KindWindow}
	s.Percent, s.ResetAt = windowState(primary)

	var detail []string
	if plan != "" {
		detail = append(detail, "plan "+plan)
	}
	if secondary != nil {
		wp, wr := windowState(*secondary)
		d := fmt.Sprintf("weekly %.0f%%", *wp)
		if wr != "" {
			if t, err := time.Parse(time.RFC3339, wr); err == nil {
				d += " · resets " + t.Local().Format("Mon 15:04")
			}
		}
		detail = append(detail, d)
	}
	s.Detail = joinDetail(detail)
	return s
}

// windowState turns a rate-limit window into a clamped percent + reset, or 0%
// with no reset once the window has already elapsed (the stale on-disk percent
// no longer applies after a reset).
func windowState(w codexWindow) (*float64, string) {
	if w.ResetsAt > 0 {
		reset := time.Unix(w.ResetsAt, 0).UTC()
		if nowUTC().After(reset) {
			return pct(0), ""
		}
		return pct(w.UsedPercent), reset.Format(time.RFC3339)
	}
	return pct(w.UsedPercent), ""
}

// codexSessionFiles collects rollout-*.jsonl paths under sessions/ and
// archived_sessions/.
func codexSessionFiles(root string) []string {
	var files []string
	for _, sub := range []string{"sessions", "archived_sessions"} {
		dir := filepath.Join(root, sub)
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && filepath.Ext(path) == ".jsonl" {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

func joinDetail(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " · "
		}
		out += p
	}
	return out
}
