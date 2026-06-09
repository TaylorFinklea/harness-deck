package harnessdeck

import "testing"

func TestContractEmbedded(t *testing.T) {
	if Contract == "" {
		t.Fatal("Contract is empty — CONTRACT.md failed to embed")
	}
	if want := "harness-deck/report@1"; !contains(Contract, want) {
		t.Errorf("Contract missing schema marker %q — did CONTRACT.md move or change?", want)
	}
}

func TestPublishingEmbedded(t *testing.T) {
	if Publishing == "" {
		t.Fatal("Publishing is empty — docs/PUBLISHING.md failed to embed")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
