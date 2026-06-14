package push

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestBoundBody confirms Send's defense-in-depth body cap: a short body is
// untouched, an oversized body is trimmed under the cap with an ellipsis, and
// a multi-byte run is never split mid-rune.
func TestBoundBody(t *testing.T) {
	if got := boundBody("pick one"); got != "pick one" {
		t.Errorf("short body altered: %q", got)
	}

	long := strings.Repeat("a", maxPayloadBody*2)
	got := boundBody(long)
	if len(got) > maxPayloadBody {
		t.Errorf("bounded body = %d bytes, want <= %d", len(got), maxPayloadBody)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("bounded body missing ellipsis")
	}

	multibyte := strings.Repeat("é", maxPayloadBody) // 'é' is 2 bytes
	gotMB := boundBody(multibyte)
	if len(gotMB) > maxPayloadBody {
		t.Errorf("multibyte bound = %d bytes, want <= %d", len(gotMB), maxPayloadBody)
	}
	if !utf8.ValidString(gotMB) {
		t.Errorf("multibyte bound split a rune")
	}
}
