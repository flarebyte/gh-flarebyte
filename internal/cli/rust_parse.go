// purpose: Parse stable Rust harness output, Cargo compiler artifacts, and LCOV reports.
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

func parseRustTests(output string) (goTestReport, error) {
	var report goTestReport
	eventPattern := regexp.MustCompile(`^test (.+) \.\.\. (ok|FAILED|ignored)(?:,.*)?$`)
	summaryPattern := regexp.MustCompile(`^test result: (?:ok|FAILED)\. (\d+) passed; (\d+) failed; (\d+) ignored;`)
	passed, failed, skipped, suites := 0, 0, 0, 0
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if m := eventPattern.FindStringSubmatch(line); m != nil {
			action := "pass"
			switch m[2] {
			case "FAILED":
				action = "fail"
			case "ignored":
				action = "skip"
			}
			report.Events = append(report.Events, goTestEvent{Test: m[1], Action: action})
		}
		if m := summaryPattern.FindStringSubmatch(line); m != nil {
			values := make([]int, 3)
			for i := range values {
				n, err := strconv.Atoi(m[i+1])
				if err != nil {
					return report, fmt.Errorf("invalid Rust test count: %w", err)
				}
				values[i] = n
			}
			passed += values[0]
			failed += values[1]
			skipped += values[2]
			suites++
		}
	}
	report.Tests, report.Failed, report.Skipped = passed+failed+skipped, failed, skipped
	if suites == 0 || len(report.Events) != report.Tests {
		return report, fmt.Errorf("unable to parse standard Rust test output; custom harnesses require summary style")
	}
	return report, nil
}

func rustCompilerBinary(output, binary string) (string, error) {
	var executable string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var message struct {
			Reason     string      `json:"reason"`
			Executable string      `json:"executable"`
			Target     cargoTarget `json:"target"`
		}
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return "", fmt.Errorf("invalid Cargo compiler JSON: %w", err)
		}
		if message.Reason == "compiler-artifact" && message.Target.Name == binary && contains(message.Target.Kind, "bin") && message.Executable != "" {
			if executable != "" && executable != message.Executable {
				return "", fmt.Errorf("Cargo produced ambiguous executable paths for %s", binary)
			}
			executable = message.Executable
		}
	}
	if executable == "" {
		return "", fmt.Errorf("Cargo did not report an executable for binary %s", binary)
	}
	return executable, nil
}

func parseRustCoverage(lcov, root string) (float64, []coverageDetail, error) {
	type counts struct{ found, hit int64 }
	files := map[string]counts{}
	label := ""
	var current counts
	found, hit := false, false
	flush := func() error {
		if label == "" {
			return nil
		}
		if !found || !hit || current.found < 0 || current.hit < 0 || current.hit > current.found {
			return fmt.Errorf("invalid Rust LCOV line counts for %s", label)
		}
		if current.found > 0 {
			previous := files[label]
			files[label] = counts{found: previous.found + current.found, hit: previous.hit + current.hit}
		}
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return 0, nil, err
	}
	for _, line := range strings.Split(lcov, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "SF:"):
			if err := flush(); err != nil {
				return 0, nil, err
			}
			current, found, hit, label = counts{}, false, false, ""
			path := strings.TrimPrefix(line, "SF:")
			if !filepath.IsAbs(path) {
				path = filepath.Join(rootAbs, path)
			}
			rel, err := filepath.Rel(rootAbs, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			skip := false
			for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
				if part == "target" || part == ".cargo" || part == ".cache" || part == "vendor" {
					skip = true
				}
			}
			if !skip {
				label = filepath.ToSlash(rel)
			}
		case strings.HasPrefix(line, "LF:"), strings.HasPrefix(line, "LH:"):
			if label == "" {
				continue
			}
			value, err := strconv.ParseInt(line[3:], 10, 64)
			if err != nil {
				return 0, nil, fmt.Errorf("invalid Rust LCOV count: %w", err)
			}
			if strings.HasPrefix(line, "LF:") {
				current.found, found = value, true
			} else {
				current.hit, hit = value, true
			}
		case line == "end_of_record":
			if err := flush(); err != nil {
				return 0, nil, err
			}
			label, current, found, hit = "", counts{}, false, false
		}
	}
	if err := flush(); err != nil {
		return 0, nil, err
	}
	var total counts
	var details []coverageDetail
	for label, count := range files {
		total.found += count.found
		total.hit += count.hit
		details = append(details, coverageDetail{Label: label, Percent: 100 * float64(count.hit) / float64(count.found)})
	}
	if total.found <= 0 {
		return 0, nil, fmt.Errorf("Rust LCOV report contains no project line coverage")
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Label < details[j].Label })
	return 100 * float64(total.hit) / float64(total.found), details, nil
}
