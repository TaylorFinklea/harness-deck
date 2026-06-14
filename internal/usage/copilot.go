package usage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// copilotProvider reports GitHub Copilot premium-request quota usage.
//
// CAVEAT: GitHub exposes no documented per-user usage endpoint — the public
// REST billing/metrics APIs are org/enterprise-scoped. The only per-user
// source is the UNDOCUMENTED internal endpoint GET
// https://api.github.com/copilot_internal/user, which official clients
// (VS Code/JetBrains/CLI) use; GitHub states that using it from other clients
// may violate the Copilot Terms of Service. It is unversioned and can change
// without notice. This provider is opt-in (the user adds "copilot" to
// usage.providers) and reads the user's own token + their own usage on their
// own machine. If GitHub ships a documented endpoint, switch to it.
type copilotProvider struct{}

func (copilotProvider) Tool() string  { return "copilot" }
func (copilotProvider) Label() string { return "GH" }

// copilotUsageURL is the (undocumented) per-user usage endpoint, indirected
// for tests.
var copilotUsageURL = "https://api.github.com/copilot_internal/user"

type copilotResp struct {
	CopilotPlan    string `json:"copilot_plan"`
	QuotaResetDate string `json:"quota_reset_date"` // "2026-07-01" (monthly)
	QuotaSnapshots struct {
		PremiumInteractions struct {
			Entitlement      int     `json:"entitlement"`
			Remaining        float64 `json:"remaining"`
			PercentRemaining float64 `json:"percent_remaining"`
			Unlimited        bool    `json:"unlimited"`
		} `json:"premium_interactions"`
	} `json:"quota_snapshots"`
}

func (c copilotProvider) Sample(ctx context.Context) Sample {
	token, err := copilotToken()
	if err != nil {
		return Sample{Err: err.Error()}
	}
	var r copilotResp
	err = getJSON(ctx, copilotUsageURL, map[string]string{
		"Authorization":          "token " + token,
		"Editor-Version":         "harness-deck/1",
		"Copilot-Integration-Id": "vscode-chat",
		"Accept":                 "application/json",
	}, &r)
	if err != nil {
		return Sample{Err: err.Error()}
	}

	q := r.QuotaSnapshots.PremiumInteractions
	s := Sample{OK: true, Kind: KindWindow}
	switch {
	case q.Unlimited:
		s.Kind = KindBudget
		s.Text = "unlimited"
	case q.Entitlement <= 0:
		// Free plans carry no premium allotment — nothing to meter.
		s.Kind = KindBudget
		s.Text = "no premium"
	default:
		s.Percent = pct(100 - q.PercentRemaining)
		s.ResetAt = monthlyReset(r.QuotaResetDate)
		s.Detail = fmt.Sprintf("%.0f of %d premium left", q.Remaining, q.Entitlement)
	}
	if r.CopilotPlan != "" {
		s.Detail = strings.TrimSpace(s.Detail + " · " + r.CopilotPlan)
		s.Detail = strings.TrimPrefix(s.Detail, "· ")
	}
	return s
}

// copilotToken reads the Copilot OAuth token (ghu_…) from the local client
// config: apps.json (keyed by github.com:Iv1…), then hosts.json (keyed by
// github.com), both under ~/.config/github-copilot.
func copilotToken() (string, error) {
	h := home()
	if h == "" {
		return "", errors.New("no home dir")
	}
	base := filepath.Join(h, ".config", "github-copilot")

	// apps.json: {"github.com:Iv1...":{"oauth_token":"ghu_..."}}
	var apps map[string]struct {
		OAuthToken string `json:"oauth_token"`
	}
	if err := readJSONFile(filepath.Join(base, "apps.json"), &apps); err == nil {
		for k, v := range apps {
			if strings.HasPrefix(k, "github.com") && v.OAuthToken != "" {
				return v.OAuthToken, nil
			}
		}
	}
	// hosts.json: {"github.com":{"oauth_token":"ghu_..."}}
	var hosts map[string]struct {
		OAuthToken string `json:"oauth_token"`
	}
	if err := readJSONFile(filepath.Join(base, "hosts.json"), &hosts); err == nil {
		if v, ok := hosts["github.com"]; ok && v.OAuthToken != "" {
			return v.OAuthToken, nil
		}
	}
	return "", errors.New("no Copilot token in ~/.config/github-copilot (apps.json/hosts.json)")
}

// monthlyReset turns Copilot's "2006-01-02" reset date into an RFC3339
// timestamp at 00:00 UTC, or "" if it doesn't parse.
func monthlyReset(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
