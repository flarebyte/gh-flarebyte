// purpose: Validate a bounded subset of npm stable-version constraints for local lockfiles.
// responsibilities: Handle exact, caret, tilde and simple comparator conjunctions conservatively.
// architecture notes: Unsupported range forms remain unknown; they are never treated as matching.
package dependencies

import "strings"

func npmSatisfies(requirement, version string) (bool, bool) {
	v, ok := stableVersion("npm", version)
	if !ok {
		return false, false
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "*" {
		return true, true
	}
	if strings.Contains(requirement, "||") {
		return false, false
	}
	if strings.HasPrefix(requirement, "^") || strings.HasPrefix(requirement, "~") {
		base, ok := stableVersion("npm", requirement[1:])
		if !ok {
			return false, false
		}
		upper := append([]uint64{}, base...)
		if requirement[0] == '~' {
			upper[1]++
			upper[2] = 0
		} else if base[0] > 0 {
			upper[0]++
			upper[1] = 0
			upper[2] = 0
		} else if base[1] > 0 {
			upper[1]++
			upper[2] = 0
		} else {
			upper[2]++
		}
		// Refuse overflow rather than wrapping a permissive range boundary.
		if compareNumbers(upper, base) <= 0 {
			return false, false
		}
		return compareNumbers(v, base) >= 0 && compareNumbers(v, upper) < 0, true
	}
	parts := strings.Fields(requirement)
	if len(parts) == 0 {
		return false, false
	}
	match := true
	for _, part := range parts {
		op := "="
		for _, prefix := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(part, prefix) {
				op = prefix
				part = strings.TrimPrefix(part, prefix)
				break
			}
		}
		base, ok := stableVersion("npm", part)
		if !ok {
			return false, false
		}
		cmp := compareNumbers(v, base)
		switch op {
		case "=":
			match = match && cmp == 0
		case ">=":
			match = match && cmp >= 0
		case "<=":
			match = match && cmp <= 0
		case ">":
			match = match && cmp > 0
		case "<":
			match = match && cmp < 0
		}
	}
	return match, true
}

func compareNumbers(a, b []uint64) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
