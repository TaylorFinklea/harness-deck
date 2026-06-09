package main

import (
	"fmt"
	"os"

	harnessdeck "github.com/TaylorFinklea/harness-deck"
)

// contractDoc picks which embedded doc to emit. With no args it returns the
// full contract; --publishing returns the gentler walkthrough. An unknown
// flag is an error so a typo doesn't silently print the wrong doc.
func contractDoc(args []string) (string, error) {
	switch {
	case len(args) == 0:
		return harnessdeck.Contract, nil
	case len(args) == 1 && (args[0] == "--publishing" || args[0] == "-p"):
		return harnessdeck.Publishing, nil
	default:
		return "", fmt.Errorf("usage: harness-deck contract [--publishing]")
	}
}

// cmdContract prints the embedded agent-facing docs to stdout, so an agent on
// any machine can read the schema without cloning the repo (`harness-deck
// contract`). The contract is embedded at build time, so it always matches
// this binary's version.
func cmdContract(args []string) {
	doc, err := contractDoc(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Print(doc)
}
