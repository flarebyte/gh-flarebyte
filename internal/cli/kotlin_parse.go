// purpose: Validate Gradle JUnit test totals and JaCoCo production line coverage.
package cli

import (
	"encoding/xml"
	"fmt"
	"sort"
)

func parseKotlinTests(data []byte) (goTestReport, error) {
	return parseJUnitTests(data, "Gradle")
}

type jacocoCounter struct {
	Type    string `xml:"type,attr"`
	Missed  *int64 `xml:"missed,attr"`
	Covered *int64 `xml:"covered,attr"`
}

func jacocoLines(counters []jacocoCounter) (int64, int64, error) {
	var found bool
	var covered, total int64
	for _, c := range counters {
		if c.Type != "LINE" {
			continue
		}
		if found || c.Missed == nil || c.Covered == nil || *c.Missed < 0 || *c.Covered < 0 {
			return 0, 0, fmt.Errorf("invalid JaCoCo LINE counter")
		}
		found = true
		covered = *c.Covered
		total = *c.Missed + covered
		if total < covered {
			return 0, 0, fmt.Errorf("JaCoCo LINE counter overflow")
		}
	}
	if !found {
		return 0, 0, fmt.Errorf("JaCoCo LINE counter missing")
	}
	return covered, total, nil
}

func parseKotlinCoverage(data []byte) (float64, []coverageDetail, error) {
	var report struct {
		XMLName  xml.Name        `xml:"report"`
		Counters []jacocoCounter `xml:"counter"`
		Packages []struct {
			Name  string `xml:"name,attr"`
			Files []struct {
				Name     string          `xml:"name,attr"`
				Counters []jacocoCounter `xml:"counter"`
			} `xml:"sourcefile"`
		} `xml:"package"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		return 0, nil, fmt.Errorf("invalid JaCoCo XML: %w", err)
	}
	covered, total, err := jacocoLines(report.Counters)
	if err != nil || total == 0 {
		return 0, nil, fmt.Errorf("JaCoCo report requires nonempty production LINE coverage: %v", err)
	}
	var entries []coverageDetail
	var sumCovered, sumTotal int64
	seen := map[string]bool{}
	for _, pkg := range report.Packages {
		for _, file := range pkg.Files {
			label := file.Name
			if pkg.Name != "" {
				label = pkg.Name + "/" + label
			}
			if file.Name == "" || seen[label] {
				return 0, nil, fmt.Errorf("invalid or duplicate JaCoCo source %q", label)
			}
			seen[label] = true
			hit, count, err := jacocoLines(file.Counters)
			if err != nil {
				return 0, nil, err
			}
			sumCovered += hit
			sumTotal += count
			if count > 0 {
				entries = append(entries, coverageDetail{Label: label, Percent: 100 * float64(hit) / float64(count)})
			}
		}
	}
	if len(entries) == 0 || sumCovered != covered || sumTotal != total {
		return 0, nil, fmt.Errorf("incomplete JaCoCo report: source LINE counters do not match totals")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Label < entries[j].Label })
	return 100 * float64(covered) / float64(total), entries, nil
}
