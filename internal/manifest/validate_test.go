package manifest

import "testing"

func TestValidStatus(t *testing.T) {
	for _, s := range []string{"draft", "awaiting-review", "answered", "done"} {
		if !ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "bogus", "Done"} {
		if ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = true, want false", s)
		}
	}
}
