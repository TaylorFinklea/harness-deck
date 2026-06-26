package usage

import "testing"

// fixture mimics the box-drawing table printed by `opencode stats --days 7`.
const statsFixture = `
┌──────────────────────────────────────────────────────┐
│ opencode stats (last 7 days)                         │
├──────────────────────────────────────────────────────┤
│Total Cost                                        $10.02 │
│Input                                             14.5M │
│Output                                           545.5K │
└──────────────────────────────────────────────────────┘
`

const statsFixtureZero = `
┌──────────────────────────────────────────────────────┐
│ opencode stats (last 7 days)                         │
├──────────────────────────────────────────────────────┤
│Total Cost                                         $0.00 │
│Input                                                  0 │
│Output                                                 0 │
└──────────────────────────────────────────────────────┘
`

func TestParseOpenCodeStats(t *testing.T) {
	t.Run("normal values", func(t *testing.T) {
		cost, input, output, ok := parseOpenCodeStats(statsFixture)
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if cost != "$10.02" {
			t.Errorf("cost: got %q, want %q", cost, "$10.02")
		}
		if input != "14.5M" {
			t.Errorf("input: got %q, want %q", input, "14.5M")
		}
		if output != "545.5K" {
			t.Errorf("output: got %q, want %q", output, "545.5K")
		}
	})

	t.Run("zero values", func(t *testing.T) {
		cost, _, _, ok := parseOpenCodeStats(statsFixtureZero)
		if !ok {
			t.Fatal("expected ok=true for $0.00, got false")
		}
		if cost != "$0.00" {
			t.Errorf("cost: got %q, want %q", cost, "$0.00")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_, _, _, ok := parseOpenCodeStats("")
		if ok {
			t.Fatal("expected ok=false for empty input, got true")
		}
	})

	t.Run("garbage input", func(t *testing.T) {
		_, _, _, ok := parseOpenCodeStats("this is not a stats table\nno useful data here")
		if ok {
			t.Fatal("expected ok=false for garbage input, got true")
		}
	})
}
