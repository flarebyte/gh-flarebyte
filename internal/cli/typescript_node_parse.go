// purpose: Read Node's JUnit reports without requiring Bun-specific suite totals.
package cli

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
)

func parseNodeTests(data []byte, root string) (goTestReport, error) {
	var suite junitSuite
	var report goTestReport
	if err := xml.Unmarshal(data, &suite); err != nil {
		return report, fmt.Errorf("invalid Node JUnit report: %w", err)
	}
	if suite.XMLName.Local != "testsuites" && suite.XMLName.Local != "testsuite" {
		return report, fmt.Errorf("invalid Node JUnit report root")
	}
	var visit func(junitSuite) error
	visit = func(s junitSuite) error {
		for _, child := range s.Suites {
			if err := visit(child); err != nil {
				return err
			}
		}
		for _, tc := range s.Cases {
			if tc.Name == "" {
				return fmt.Errorf("Node JUnit testcase has no name")
			}
			action := "pass"
			if len(tc.Failures)+len(tc.Errors) > 0 {
				action = "fail"
				report.Failed++
			} else if len(tc.Skipped) > 0 {
				action = "skip"
				report.Skipped++
			}
			label := tc.Name
			file := tc.File
			if file == "" {
				file = s.File
			}
			if filepath.IsAbs(file) {
				if rel, err := filepath.Rel(root, file); err == nil {
					file = filepath.ToSlash(rel)
				}
			}
			if file != "" {
				label = file + "::" + label
			}
			report.Tests++
			report.Events = append(report.Events, goTestEvent{Test: label, Action: action})
		}
		return nil
	}
	return report, visit(suite)
}
