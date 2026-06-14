package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// claudeProvider reports Claude Code's true subscription rate-limit window
// (5-hour + weekly utilization %), which is NOT in any local file — it comes
// from an authenticated GET https://api.anthropic.com/api/oauth/usage. The
// bearer token is Claude Code's OAuth access token; on macOS it lives only in
// the login Keychain (item "Claude Code-credentials"), read here exactly as
// Claude Code itself does, via the `security` CLI. The first read prompts once
// ("Always Allow"); after that it is silent and the token auto-refreshes in
// place. Credential lookup order avoids the Keychain when possible:
// $CLAUDE_CODE_OAUTH_TOKEN → ~/.claude/.credentials.json (Linux / opt-in) →
// Keychain.
type claudeProvider struct{}

func (claudeProvider) Tool() string  { return "claude-code" }
func (claudeProvider) Label() string { return "CC" }

// claudeUsageURL is the OAuth usage endpoint, indirected for tests.
var claudeUsageURL = "https://api.anthropic.com/api/oauth/usage"

type claudeWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type claudeUsageResp struct {
	FiveHour *claudeWindow `json:"five_hour"`
	SevenDay *claudeWindow `json:"seven_day"`
}

func (c claudeProvider) Sample(ctx context.Context) Sample {
	token, err := claudeToken(ctx)
	if err != nil {
		return Sample{Err: err.Error()}
	}
	var r claudeUsageResp
	err = getJSON(ctx, claudeUsageURL, map[string]string{
		"Authorization":  "Bearer " + token,
		"anthropic-beta": "oauth-2025-04-20",
		"Accept":         "application/json",
		"User-Agent":     "claude-code/2.1.0",
	}, &r)
	if err != nil {
		return Sample{Err: err.Error()}
	}
	if r.FiveHour == nil {
		return Sample{Err: "usage response had no five_hour window"}
	}
	s := Sample{OK: true, Kind: KindWindow}
	s.Percent = pct(r.FiveHour.Utilization)
	s.ResetAt = normalizeReset(r.FiveHour.ResetsAt)
	if r.SevenDay != nil {
		d := fmt.Sprintf("weekly %.0f%%", r.SevenDay.Utilization)
		if t := normalizeReset(r.SevenDay.ResetsAt); t != "" {
			if pt, e := time.Parse(time.RFC3339, t); e == nil {
				d += " · resets " + pt.Local().Format("Mon 15:04")
			}
		}
		s.Detail = d
	}
	return s
}

// claudeToken resolves the OAuth access token, preferring credential sources
// that don't touch the Keychain.
func claudeToken(ctx context.Context) (string, error) {
	if t := strings.TrimSpace(envToken()); t != "" {
		return t, nil
	}
	if h := home(); h != "" {
		var blob claudeCreds
		if err := readJSONFile(filepath.Join(h, ".claude", ".credentials.json"), &blob); err == nil {
			if t := blob.ClaudeAiOauth.AccessToken; t != "" {
				return t, nil
			}
		}
	}
	return claudeKeychainToken(ctx)
}

func envToken() string {
	// Claude Code's own override env var.
	return os.Getenv("CLAUDE_CODE_OAUTH_TOKEN")
}

type claudeCreds struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
	} `json:"claudeAiOauth"`
}

// claudeKeychainToken shells out to the macOS `security` CLI to read the
// "Claude Code-credentials" generic-password item, the same item Claude Code
// uses. A short timeout keeps a locked/contended Keychain from hanging the
// poller (CodexBar uses ~1.5s).
func claudeKeychainToken(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "security",
		"find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return "", fmt.Errorf("read Claude Keychain item: %w", err)
	}
	var blob claudeCreds
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &blob); err != nil {
		return "", errors.New("Claude Keychain blob not JSON as expected")
	}
	if blob.ClaudeAiOauth.AccessToken == "" {
		return "", errors.New("Claude Keychain blob had no accessToken")
	}
	return blob.ClaudeAiOauth.AccessToken, nil
}

// normalizeReset accepts the API's ISO8601 reset string and returns RFC3339,
// or the input unchanged if it doesn't parse (the frontend tolerates either).
func normalizeReset(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}
