// purpose: Parse structured Python test, coverage, and project metadata reports.
package cli

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func parsePythonTests(data []byte) (goTestReport, error) {
	var report goTestReport
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	root := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return report, fmt.Errorf("invalid pytest JUnit report: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if root == "" {
			root = start.Name.Local
			if root != "testsuites" && root != "testsuite" {
				return report, fmt.Errorf("invalid pytest JUnit report root %q", root)
			}
		}
		if start.Name.Local != "testcase" {
			continue
		}
		var test struct {
			Name     string   `xml:"name,attr"`
			Class    string   `xml:"classname,attr"`
			Failures []string `xml:"failure"`
			Errors   []string `xml:"error"`
			Skipped  []string `xml:"skipped"`
		}
		if err := decoder.DecodeElement(&test, &start); err != nil {
			return report, fmt.Errorf("invalid pytest testcase: %w", err)
		}
		action := "pass"
		if len(test.Failures)+len(test.Errors) > 0 {
			action = "fail"
			report.Failed++
		} else if len(test.Skipped) > 0 {
			action = "skip"
			report.Skipped++
		}
		report.Tests++
		name := test.Name
		if test.Class != "" {
			name = test.Class + "::" + name
		}
		report.Events = append(report.Events, goTestEvent{Action: action, Test: name})
	}
	if root == "" {
		return report, fmt.Errorf("empty pytest JUnit report")
	}
	return report, nil
}

func parsePythonCoverage(data []byte) (float64, []coverageDetail, error) {
	type summary struct {
		Percent *float64 `json:"percent_covered"`
	}
	var report struct {
		Totals summary `json:"totals"`
		Files  map[string]struct {
			Summary summary `json:"summary"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return 0, nil, fmt.Errorf("invalid Python coverage JSON: %w", err)
	}
	valid := func(value *float64) bool {
		return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 100
	}
	if !valid(report.Totals.Percent) || len(report.Files) == 0 {
		return 0, nil, fmt.Errorf("Python coverage JSON requires valid totals and file coverage")
	}
	var details []coverageDetail
	for name, file := range report.Files {
		if !valid(file.Summary.Percent) {
			return 0, nil, fmt.Errorf("invalid coverage for %s", name)
		}
		details = append(details, coverageDetail{Label: name, Percent: *file.Summary.Percent})
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Label < details[j].Label })
	return *report.Totals.Percent, details, nil
}

func pythonProjectVersion(data []byte) (string, error) {
	var project struct {
		Project struct {
			Version string   `toml:"version"`
			Dynamic []string `toml:"dynamic"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(data, &project); err != nil {
		return "", fmt.Errorf("invalid Python project TOML: %w", err)
	}
	for _, field := range project.Project.Dynamic {
		if field == "version" {
			return "", fmt.Errorf("dynamic Python versions are not supported; set a static [project].version")
		}
	}
	version := project.Project.Version
	if !isSemver(version) {
		return "", fmt.Errorf("invalid [project].version %q: static semantic version required", version)
	}
	return version, nil
}
