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

type coverageDetail struct {
	Label   string
	Percent float64
}
