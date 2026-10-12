// purpose: Parse XCTest results, versioned Swift Testing events, and LLVM coverage exports.
package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func parseSwiftTests(output, stream string) (goTestReport, error) {
	var report goTestReport
	cases := regexp.MustCompile(`^Test Case '(.+)' (passed|failed|skipped) \(`)
	summary := regexp.MustCompile(`^Executed (\d+) tests?,(?: with (\d+) tests? skipped and| with)? (\d+) failures?`)
	expected, failed, skipped := -1, 0, 0
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if m := cases.FindStringSubmatch(line); m != nil {
			action := map[string]string{"passed": "pass", "failed": "fail", "skipped": "skip"}[m[2]]
			report.Events = append(report.Events, goTestEvent{Test: m[1], Action: action})
		}
		if m := summary.FindStringSubmatch(line); m != nil {
			expected, _ = strconv.Atoi(m[1])
			skipped, _ = strconv.Atoi(m[2])
			failed, _ = strconv.Atoi(m[3])
		}
	}
	xcFailures, xcSkipped := 0, 0
	for _, event := range report.Events {
		if event.Action == "fail" {
			xcFailures++
		}
		if event.Action == "skip" {
			xcSkipped++
		}
	}
	if expected < 0 || expected != len(report.Events) || (failed < xcFailures || (failed > 0 && xcFailures == 0)) || skipped != xcSkipped {
		return report, fmt.Errorf("incomplete XCTest report: expected a serial test summary matching completed test cases")
	}
	type testState struct {
		name, kind, action     string
		started, ended, failed bool
	}
	states := map[string]*testState{}
	started, ended := false, false
	for _, line := range strings.Split(stream, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record struct {
			Version *int   `json:"version"`
			Kind    string `json:"kind"`
			Payload struct {
				Kind   string `json:"kind"`
				ID     string `json:"id"`
				Name   string `json:"name"`
				TestID string `json:"testID"`
				Issue  struct {
					IsKnown json.RawMessage `json:"isKnown"`
				} `json:"issue"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return report, fmt.Errorf("invalid Swift Testing event JSON: %w", err)
		}
		if record.Version == nil || *record.Version != 0 {
			return report, fmt.Errorf("unsupported Swift Testing event version: expected 0")
		}
		p := record.Payload
		if record.Kind == "test" {
			if p.ID == "" || p.Name == "" {
				return report, fmt.Errorf("invalid Swift Testing test record")
			}
			if _, duplicate := states[p.ID]; duplicate {
				return report, fmt.Errorf("duplicate Swift Testing test record")
			}
			states[p.ID] = &testState{name: p.Name, kind: p.Kind}
		}
		if record.Kind != "event" {
			continue
		}
		if p.Kind == "runStarted" {
			started = true
		}
		if p.Kind == "runEnded" {
			ended = true
		}
		if p.TestID == "" {
			continue
		}
		state, ok := states[p.TestID]
		if !ok {
			return report, fmt.Errorf("Swift Testing event references an unknown test")
		}
		switch p.Kind {
		case "testStarted":
			state.started = true
		case "issueRecorded":
			known := string(p.Issue.IsKnown)
			if known == "" || known == "false" || known == "null" {
				state.failed = true
			}
		case "testEnded":
			state.ended = true
			state.action = "pass"
		case "testSkipped":
			state.ended = true
			state.action = "skip"
		}
	}
	if !started || !ended {
		return report, fmt.Errorf("incomplete Swift Testing event stream: missing run boundary")
	}
	// Use IDs for stable ordering and unambiguous names across suites and parameterized functions.
	ids := make([]string, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state := states[id]
		if state.kind != "function" {
			continue
		}
		if !state.ended || (state.action != "skip" && !state.started) {
			return report, fmt.Errorf("incomplete Swift Testing report for %s", state.name)
		}
		action := state.action
		if state.failed {
			action = "fail"
		}
		report.Events = append(report.Events, goTestEvent{Test: id, Action: action})
	}
	report.Tests = len(report.Events)
	for _, event := range report.Events {
		if event.Action == "fail" {
			report.Failed++
		}
		if event.Action == "skip" {
			report.Skipped++
		}
	}
	return report, nil
}

func parseSwiftCoverage(data []byte, allowed map[string]string) (float64, []coverageDetail, error) {
	var export struct {
		Type string `json:"type"`
		Data []struct {
			Files []struct {
				Filename string `json:"filename"`
				Summary  struct {
					Lines struct{ Count, Covered int64 } `json:"lines"`
				} `json:"summary"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		return 0, nil, fmt.Errorf("invalid Swift coverage JSON: %w", err)
	}
	if export.Type != "llvm.coverage.json.export" {
		return 0, nil, fmt.Errorf("unsupported Swift coverage JSON format")
	}
	type counts struct{ count, covered int64 }
	files := map[string]counts{}
	for _, d := range export.Data {
		for _, file := range d.Files {
			path, err := filepath.EvalSymlinks(file.Filename)
			if err != nil {
				continue
			} // Deleted generated files and external dependencies are not root sources.
			label, ok := allowed[path]
			if !ok {
				continue
			}
			lines := file.Summary.Lines
			if lines.Count < 0 || lines.Covered < 0 || lines.Covered > lines.Count {
				return 0, nil, fmt.Errorf("invalid Swift coverage line counts for %s", label)
			}
			if previous, exists := files[label]; exists && previous != (counts{lines.Count, lines.Covered}) {
				return 0, nil, fmt.Errorf("conflicting Swift coverage entries for %s", label)
			}
			files[label] = counts{lines.Count, lines.Covered}
		}
	}
	var count, covered int64
	var details []coverageDetail
	for label, lines := range files {
		if lines.count == 0 {
			continue
		}
		count += lines.count
		covered += lines.covered
		details = append(details, coverageDetail{Label: label, Percent: 100 * float64(lines.covered) / float64(lines.count)})
	}
	if count == 0 {
		return 0, nil, fmt.Errorf("Swift coverage report contains no production-source line coverage")
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Label < details[j].Label })
	return 100 * float64(covered) / float64(count), details, nil
}
