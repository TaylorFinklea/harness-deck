package usage

import (
	"context"
	"fmt"
)

// openRouterProvider reads OpenRouter key usage via the documented
// GET /api/v1/key endpoint (a plain bearer GET). It reports a credit/spend
// budget — OpenRouter has no time-window rate-limit reset (limit_reset is a
// cadence word, not a timestamp), so Kind is budget.
type openRouterProvider struct {
	key string
}

func (openRouterProvider) Tool() string  { return "openrouter" }
func (openRouterProvider) Label() string { return "OR" }

// openRouterURL is the key-status endpoint, indirected for tests.
var openRouterURL = "https://openrouter.ai/api/v1/key"

type openRouterKeyResp struct {
	Data struct {
		Usage          float64  `json:"usage"`
		UsageDaily     float64  `json:"usage_daily"`
		UsageWeekly    float64  `json:"usage_weekly"`
		UsageMonthly   float64  `json:"usage_monthly"`
		Limit          *float64 `json:"limit"`
		LimitRemaining *float64 `json:"limit_remaining"`
		IsFreeTier     bool     `json:"is_free_tier"`
	} `json:"data"`
}

func (o openRouterProvider) Sample(ctx context.Context) Sample {
	if o.key == "" {
		return Sample{Err: "no OpenRouter key (set usage.openrouter_key or $OPENROUTER_API_KEY)"}
	}
	var r openRouterKeyResp
	err := getJSON(ctx, openRouterURL,
		map[string]string{"Authorization": "Bearer " + o.key}, &r)
	if err != nil {
		return Sample{Err: err.Error()}
	}
	d := r.Data

	s := Sample{OK: true, Kind: KindBudget}
	detail := fmt.Sprintf("today %s · wk %s · mo %s",
		money(d.UsageDaily), money(d.UsageWeekly), money(d.UsageMonthly))
	if d.Limit != nil && *d.Limit > 0 {
		used := d.Usage / *d.Limit * 100
		s.Percent = pct(used)
		remaining := *d.Limit - d.Usage
		if d.LimitRemaining != nil {
			remaining = *d.LimitRemaining
		}
		s.Text = money(remaining) + " left"
		detail += fmt.Sprintf(" · %.0f%% of %s", used, money(*d.Limit))
	} else {
		s.Text = money(d.UsageMonthly) + "/mo"
		detail += " · uncapped"
	}
	if d.IsFreeTier {
		detail += " · free tier"
	}
	s.Detail = detail
	return s
}

// money formats a dollar amount compactly: whole dollars drop the cents.
func money(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("$%d", int64(v))
	}
	return fmt.Sprintf("$%.2f", v)
}
