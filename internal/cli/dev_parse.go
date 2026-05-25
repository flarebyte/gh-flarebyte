// purpose: Parse raw tool outputs into typed test/coverage summaries used by dev commands.
// responsibilities: Extract total coverage; parse per-entry coverage details; decode go test JSON events into structured reports.
// architecture notes: Parsers are intentionally tolerant of unrelated lines so command summaries remain stable across minor tool output changes.
package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func parseTotalCoverage(out string) (float64, error) {
	re := regexp.MustCompile(`(?m)^total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	m := re.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, fmt.Errorf("unable to parse total coverage from go tool cover output")
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("unable to parse total coverage value: %w", err)
	}
	return v, nil
}

func parseCoverageDetails(out string) []coverageDetail {
	lines := strings.Split(out, "\n")
	details := make([]coverageDetail, 0, len(lines))
	re := regexp.MustCompile(`^(.*)\s+([0-9]+(?:\.[0-9]+)?)%$`)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total:") {
			continue
		}
		m := re.FindStringSubmatch(line)
		if len(m) < 3 {
			continue
		}
		pct, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		details = append(details, coverageDetail{Label: strings.TrimSpace(m[1]), Percent: pct})
	}
	return details
}

func parseGoTestJSONSummary(out string) (tests int, failed int, skipped int) {
	report := parseGoTestReport(out)
	return report.Tests, report.Failed, report.Skipped
}

func parseGoTestReport(out string) goTestReport {
	report := goTestReport{}
	lastOutput := map[string]string{}
	lastLocation := map[string]string{}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		report.Events = append(report.Events, ev)
		switch ev.Action {
		case "pass", "fail", "skip":
			if ev.Test == "" {
				continue
			}
			report.Tests++
		}
		if ev.Action == "fail" {
			if ev.Test != "" {
				report.Failed++
				report.Failures = append(report.Failures, goFailure{Test: ev.Test, At: lastLocation[ev.Test], Snippet: lastOutput[ev.Test]})
			}
		}
		if ev.Action == "skip" {
			report.Skipped++
		}
		if ev.Test != "" && ev.Output != "" {
			output := strings.TrimSpace(ev.Output)
			if output != "" {
				lastOutput[ev.Test] = output
				if loc := extractFailureLocation(output); loc != "" {
					lastLocation[ev.Test] = loc
				}
			}
		}
	}
	return report
}

func parseDartTestReport(out string) dartTestReport {
	type dartJSONTestRef struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Skip   bool   `json:"skip"`
		Hidden bool   `json:"hidden"`
	}
	type dartJSONEvent struct {
		Type   string           `json:"type"`
		Test   *dartJSONTestRef `json:"test"`
		Result string           `json:"result"`
	}

	report := dartTestReport{}
	testsByID := map[int]dartJSONTestRef{}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev dartJSONEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Test != nil {
			testsByID[ev.Test.ID] = *ev.Test
		}
		if ev.Type != "testDone" || ev.Test == nil {
			continue
		}
		test := testsByID[ev.Test.ID]
		if test.Hidden {
			continue
		}
		name := strings.TrimSpace(test.Name)
		if name == "" {
			continue
		}
		report.Tests++
		if test.Skip {
			report.Skipped++
			report.Events = append(report.Events, dartTestEvent{Action: "skip", Test: name})
			continue
		}
		switch ev.Result {
		case "success":
			report.Events = append(report.Events, dartTestEvent{Action: "pass", Test: name})
		default:
			report.Failed++
			report.Events = append(report.Events, dartTestEvent{Action: "fail", Test: name})
		}
	}

	return report
}

func parseDartLCOVCoverage(lcov string) (float64, []coverageDetail, error) {
	type fileCoverage struct {
		label string
		lf    float64
		lh    float64
	}
	var current fileCoverage
	resetCurrent := func() {
		current = fileCoverage{}
	}
	finalizeCurrent := func(details []coverageDetail) []coverageDetail {
		if current.label == "" || current.lf <= 0 {
			return details
		}
		details = append(details, coverageDetail{
			Label:   current.label,
			Percent: (current.lh / current.lf) * 100,
		})
		return details
	}

	totalLF := 0.0
	totalLH := 0.0
	details := make([]coverageDetail, 0)
	resetCurrent()

	for _, line := range strings.Split(lcov, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "SF:"):
			current.label = strings.TrimPrefix(line, "SF:")
		case strings.HasPrefix(line, "LF:"):
			lf, err := strconv.ParseFloat(strings.TrimPrefix(line, "LF:"), 64)
			if err != nil {
				return 0, nil, fmt.Errorf("unable to parse dart lcov LF value")
			}
			current.lf = lf
			totalLF += lf
		case strings.HasPrefix(line, "LH:"):
			lh, err := strconv.ParseFloat(strings.TrimPrefix(line, "LH:"), 64)
			if err != nil {
				return 0, nil, fmt.Errorf("unable to parse dart lcov LH value")
			}
			current.lh = lh
			totalLH += lh
		case line == "end_of_record":
			details = finalizeCurrent(details)
			resetCurrent()
		}
	}
	// Handle files even if end_of_record is missing.
	details = finalizeCurrent(details)

	if totalLF <= 0 {
		return 0, nil, fmt.Errorf("unable to parse total coverage from dart lcov output")
	}
	return (totalLH / totalLF) * 100, details, nil
}
