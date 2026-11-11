// purpose: Validate JUnit suites with explicit testcase totals for Bun and Gradle.
package cli

import (
	"encoding/xml"
	"fmt"
)

type junitSuite struct {
	XMLName  xml.Name
	Name     string       `xml:"name,attr"`
	File     string       `xml:"file,attr"`
	Tests    *int         `xml:"tests,attr"`
	Failures *int         `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  *int         `xml:"skipped,attr"`
	Suites   []junitSuite `xml:"testsuite"`
	Cases    []junitCase  `xml:"testcase"`
}

type junitCase struct {
	Name     string   `xml:"name,attr"`
	Class    string   `xml:"classname,attr"`
	File     string   `xml:"file,attr"`
	Failures []string `xml:"failure"`
	Errors   []string `xml:"error"`
	Skipped  []string `xml:"skipped"`
}

func parseJUnitTests(data []byte, tool string) (goTestReport, error) {
	var suite junitSuite
	var report goTestReport
	if err := xml.Unmarshal(data, &suite); err != nil {
		return report, fmt.Errorf("invalid %s JUnit report: %w", tool, err)
	}
	if suite.XMLName.Local != "testsuites" && suite.XMLName.Local != "testsuite" {
		return report, fmt.Errorf("invalid %s JUnit report root", tool)
	}
	var visit func(junitSuite, string) (goTestReport, error)
	visit = func(s junitSuite, file string) (goTestReport, error) {
		var r goTestReport
		if s.File != "" {
			file = s.File
		}
		for _, child := range s.Suites {
			part, err := visit(child, file)
			if err != nil {
				return r, err
			}
			r.Tests += part.Tests
			r.Failed += part.Failed
			r.Skipped += part.Skipped
			r.Events = append(r.Events, part.Events...)
		}
		for _, test := range s.Cases {
			if test.Name == "" {
				return r, fmt.Errorf("%s JUnit testcase has no name", tool)
			}
			action := "pass"
			if len(test.Failures)+len(test.Errors) > 0 {
				action = "fail"
				r.Failed++
			} else if len(test.Skipped) > 0 {
				action = "skip"
				r.Skipped++
			}
			label := test.Name
			if test.Class != "" {
				label = test.Class + "::" + label
			}
			source := file
			if test.File != "" {
				source = test.File
			}
			if source != "" {
				label = source + "::" + label
			}
			r.Tests++
			r.Events = append(r.Events, goTestEvent{Test: label, Action: action})
		}
		if s.Tests == nil || s.Failures == nil || s.Skipped == nil || *s.Tests != r.Tests || *s.Failures+s.Errors != r.Failed || *s.Skipped != r.Skipped {
			return r, fmt.Errorf("incomplete %s JUnit report: testcase totals do not match suite %q", tool, s.Name)
		}
		return r, nil
	}
	return visit(suite, "")
}
