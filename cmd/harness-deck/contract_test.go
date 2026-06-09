package main

import (
	"strings"
	"testing"

	harnessdeck "github.com/TaylorFinklea/harness-deck"
)

func TestContractDocDefaultsToContract(t *testing.T) {
	body, err := contractDoc(nil)
	if err != nil {
		t.Fatalf("contractDoc(nil): %v", err)
	}
	if body != harnessdeck.Contract {
		t.Error("default doc should be the full contract")
	}
	if !strings.Contains(body, "harness-deck/report@1") {
		t.Error("contract body missing schema marker")
	}
}

func TestContractDocPublishingFlag(t *testing.T) {
	body, err := contractDoc([]string{"--publishing"})
	if err != nil {
		t.Fatalf("contractDoc(--publishing): %v", err)
	}
	if body != harnessdeck.Publishing {
		t.Error("--publishing should select the publishing guide")
	}
}

func TestContractDocRejectsUnknownFlag(t *testing.T) {
	if _, err := contractDoc([]string{"--bogus"}); err == nil {
		t.Error("expected an error for an unknown flag")
	}
}
