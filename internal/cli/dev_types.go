// purpose: Define shared dev-command data structures used across parse, execution, and output layers.
// responsibilities: Model summary payloads, go test events, failure snippets, reports, and coverage detail records.
// architecture notes: Types are split from command logic to keep cross-file contracts explicit after cmd_dev decomposition.
package cli

import "time"

type devSummary struct {
	Kind     string
	Status   string
	Duration time.Duration
	Details  string
}

type goTestEvent struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
	Package string `json:"Package"`
}

type goFailure struct {
	Test    string
	At      string
	Snippet string
}

type goTestReport struct {
	Tests    int
	Failed   int
	Skipped  int
	Events   []goTestEvent
	Failures []goFailure
}

type dartTestEvent struct {
	Action string
	Test   string
}

type dartTestReport struct {
	Tests   int
	Failed  int
	Skipped int
	Events  []dartTestEvent
}

type coverageDetail struct {
	Label   string
	Percent float64
}
