package manifest

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Approval answer values. They are part of the contract: harnesses read
// them back out of responses.json.
const (
	ApprovalApproved         = "approved"
	ApprovalChangesRequested = "changes-requested"
)

// CheckAnswer reports whether value (or values, for a multi-select ask) is a
// legal answer to the interactive block with the given id. The server checks
// every answer before writing responses.json, so a harness can trust that an
// approval is exactly "approved" or "changes-requested", a decision is one of
// its two tags, and a choice is one of the options it offered.
func CheckAnswer(rep *Report, blockID, value string, values []string) error {
	var target *Block
	for i := range rep.Blocks {
		if InteractiveID(rep.Blocks[i]) == blockID {
			target = &rep.Blocks[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no interactive block %q in this report", blockID)
	}
	switch body := target.Body.(type) {
	case *AskBlock:
		switch body.ResolvedMode() {
		case "choice":
			return oneOf(value, body.Options)
		case "yesno":
			return oneOf(value, []string{"yes", "no"})
		case "multi":
			if len(values) == 0 {
				return errors.New("pick at least one option")
			}
			for _, v := range values {
				if err := oneOf(v, body.Options); err != nil {
					return err
				}
			}
			return nil
		default: // text
			if strings.TrimSpace(value) == "" {
				return errors.New("answer is empty")
			}
			return nil
		}
	case *DecisionBlock:
		return oneOf(value, []string{body.A.Tag, body.B.Tag})
	case *ApprovalBlock:
		return oneOf(value, []string{ApprovalApproved, ApprovalChangesRequested})
	}
	return fmt.Errorf("block %q is not interactive", blockID)
}

func oneOf(v string, allowed []string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%q is not one of the offered answers (%s)", v, strings.Join(allowed, ", "))
}
