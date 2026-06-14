package usage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// openCodeProvider reports OpenCode subscription-plan usage (rolling 5h +
// weekly %), the CodexBar-style numbers. There is no OpenCode usage/balance
// API (open feature request); the only source is the opencode.ai web app's
// internal `_server` RPC, authenticated with the site's "auth" session cookie
// (pasted into config; it is NOT in a tidy local file).
//
// FRAGILE BY DESIGN: the _server function IDs below are build fingerprints of
// the opencode.ai frontend and CHANGE ON THEIR DEPLOYS. When they shift the
// calls 404 and this provider degrades to OK:false (the footer simply drops
// OpenCode) — update the consts here, or override the workspace id via config
// to skip the first lookup. The session cookie also expires and must be
// re-pasted.
type openCodeProvider struct {
	cookie      string
	workspaceID string
}

func (openCodeProvider) Tool() string  { return "opencode" }
func (openCodeProvider) Label() string { return "OC" }

// opencode.ai _server build-fingerprint IDs (see the fragility note above).
const (
	openCodeWorkspacesHash   = "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f"
	openCodeSubscriptionHash = "7abeebee372f304e050aaaf92be863f4a86490e382f8c79db68fd94040d691b4"
)

var (
	reOpenCodeWrk  = regexp.MustCompile(`wrk_[A-Za-z0-9]+`)
	reUsagePercent = regexp.MustCompile(`"usagePercent"\s*:\s*([0-9.]+)`)
	reResetInSec   = regexp.MustCompile(`"resetInSec"\s*:\s*([0-9]+)`)
)

func (o openCodeProvider) Sample(ctx context.Context) Sample {
	if o.cookie == "" {
		return Sample{Err: "no opencode cookie (paste the opencode.ai 'auth' cookie into usage.opencode_cookie)"}
	}
	wrk := o.workspaceID
	if wrk == "" {
		body, err := o.serverGet(ctx, openCodeWorkspacesHash, "")
		if err != nil {
			return Sample{Err: "workspace lookup: " + err.Error()}
		}
		if m := reOpenCodeWrk.FindString(body); m != "" {
			wrk = m
		} else {
			return Sample{Err: "no workspace id in opencode response (hash may be stale)"}
		}
	}

	body, err := o.serverGet(ctx, openCodeSubscriptionHash, `["`+wrk+`"]`)
	if err != nil {
		return Sample{Err: "usage lookup: " + err.Error()}
	}
	rp, rsec, ok := extractOpenCodeUsage(body, "rollingUsage")
	if !ok {
		return Sample{Err: "no rollingUsage in opencode response (hash may be stale)"}
	}
	s := Sample{OK: true, Kind: KindWindow, Percent: pct(rp)}
	if rsec > 0 {
		s.ResetAt = nowUTC().Add(time.Duration(rsec) * time.Second).Format(time.RFC3339)
	}
	if wp, wsec, ok := extractOpenCodeUsage(body, "weeklyUsage"); ok {
		d := fmt.Sprintf("weekly %.0f%%", wp)
		if wsec > 0 {
			d += " · resets " + nowUTC().Add(time.Duration(wsec)*time.Second).Local().Format("Mon 15:04")
		}
		s.Detail = d
	}
	return s
}

// serverGet calls an opencode.ai _server function by its build-fingerprint id,
// with the session cookie and the browser headers the endpoint requires.
func (o openCodeProvider) serverGet(ctx context.Context, id, args string) (string, error) {
	u := "https://opencode.ai/_server?id=" + id
	if args != "" {
		u += "&args=" + url.QueryEscape(args)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", "auth="+o.cookie)
	req.Header.Set("X-Server-Id", id)
	req.Header.Set("Origin", "https://opencode.ai")
	req.Header.Set("Referer", "https://opencode.ai/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &httpError{status: resp.StatusCode}
	}
	return string(body), nil
}

// extractOpenCodeUsage finds a named usage block (rollingUsage / weeklyUsage)
// in the _server response and pulls its usagePercent + resetInSec. The body is
// text/javascript, not guaranteed clean JSON, so this is a tolerant scan — but
// it is bounded to the target block's own braces so a sibling block's numbers
// can never leak in (an empty/partial block degrades to ok:false instead).
func extractOpenCodeUsage(body, key string) (percent float64, resetSec int64, ok bool) {
	block, found := jsonBlockAfter(body, `"`+key+`"`)
	if !found {
		return 0, 0, false
	}
	pm := reUsagePercent.FindStringSubmatch(block)
	if pm == nil {
		return 0, 0, false
	}
	percent, _ = strconv.ParseFloat(pm[1], 64)
	if rm := reResetInSec.FindStringSubmatch(block); rm != nil {
		resetSec, _ = strconv.ParseInt(rm[1], 10, 64)
	}
	return percent, resetSec, true
}

// jsonBlockAfter returns the brace-balanced {…} object that follows the first
// occurrence of marker, so a scan stays within one block. (Good enough for the
// numeric usage blocks; it does not account for braces inside string values,
// which these blocks don't contain.)
func jsonBlockAfter(body, marker string) (string, bool) {
	i := strings.Index(body, marker)
	if i < 0 {
		return "", false
	}
	open := strings.IndexByte(body[i:], '{')
	if open < 0 {
		return "", false
	}
	open += i
	depth := 0
	for j := open; j < len(body); j++ {
		switch body[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[open : j+1], true
			}
		}
	}
	return "", false
}
