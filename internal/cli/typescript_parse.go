// purpose: Validate Bun JUnit test reports and source-scoped LCOV line coverage.
package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func parseTypeScriptTests(data []byte) (goTestReport, error) {
	return parseJUnitTests(data, "Bun")
}

func parseTypeScriptCoverage(data string, root string, allowed map[string]string) (float64, []coverageDetail, error) {
	seen := map[string]bool{}
	var details []coverageDetail
	var total, hit int64
	file := ""
	lf, lh := int64(-1), int64(-1)
	lines := map[int64]int64{}
	active := false
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "SF:"):
			if active {
				return 0, nil, fmt.Errorf("incomplete TypeScript LCOV record")
			}
			active, lf, lh, lines = true, -1, -1, map[int64]int64{}
			file = strings.TrimPrefix(line, "SF:")
			if !filepath.IsAbs(file) {
				file = filepath.Join(root, file)
			}
			canonical, err := filepath.EvalSymlinks(file)
			if err == nil {
				file = canonical
			}
		case strings.HasPrefix(line, "LF:"), strings.HasPrefix(line, "LH:"):
			if !active {
				return 0, nil, fmt.Errorf("TypeScript LCOV counts without a source file")
			}
			count, err := strconv.ParseInt(line[3:], 10, 64)
			if err != nil || count < 0 {
				return 0, nil, fmt.Errorf("invalid TypeScript LCOV line count")
			}
			if strings.HasPrefix(line, "LF:") {
				if lf >= 0 {
					return 0, nil, fmt.Errorf("duplicate TypeScript LCOV LF count")
				}
				lf = count
			} else {
				if lh >= 0 {
					return 0, nil, fmt.Errorf("duplicate TypeScript LCOV LH count")
				}
				lh = count
			}
		case strings.HasPrefix(line, "DA:"):
			parts := strings.Split(line[3:], ",")
			if !active || len(parts) < 2 {
				return 0, nil, fmt.Errorf("invalid TypeScript LCOV DA entry")
			}
			number, err1 := strconv.ParseInt(parts[0], 10, 64)
			count, err2 := strconv.ParseInt(parts[1], 10, 64)
			if _, duplicate := lines[number]; duplicate || err1 != nil || err2 != nil || number <= 0 || count < 0 {
				return 0, nil, fmt.Errorf("invalid or duplicate TypeScript LCOV source line")
			}
			lines[number] = count
		case line == "end_of_record":
			if !active || lf < 0 || lh < 0 || lh > lf || int64(len(lines)) != lf {
				return 0, nil, fmt.Errorf("invalid or incomplete TypeScript LCOV line totals")
			}
			var covered int64
			for _, count := range lines {
				if count > 0 {
					covered++
				}
			}
			if covered != lh {
				return 0, nil, fmt.Errorf("TypeScript LCOV covered lines disagree with summary")
			}
			if label, ok := allowed[file]; ok {
				if seen[file] {
					return 0, nil, fmt.Errorf("duplicate TypeScript LCOV file: %s", label)
				}
				seen[file] = true
				if lf > 0 {
					total += lf
					hit += lh
					details = append(details, coverageDetail{Label: label, Percent: 100 * float64(lh) / float64(lf)})
				}
			}
			active = false
		}
	}
	if active {
		return 0, nil, fmt.Errorf("incomplete TypeScript LCOV record")
	}
	var missing []string
	for path, label := range allowed {
		if !seen[path] {
			missing = append(missing, label)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return 0, nil, fmt.Errorf("TypeScript coverage omitted production sources: %s; check c8 source mapping and tsconfig.json source selection", strings.Join(missing, ", "))
	}
	if total == 0 {
		return 0, nil, fmt.Errorf("TypeScript coverage contains no production-source line coverage")
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Label < details[j].Label })
	return 100 * float64(hit) / float64(total), details, nil
}
