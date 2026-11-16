// purpose: Validate a bounded subset of npm stable-version constraints for local lockfiles.
// responsibilities: Handle exact, partial, wildcard and alternative ranges plus bounded comparator conjunctions.
// architecture notes: Unsupported range forms remain unknown; they are never treated as matching.
package dependencies

import (
	"strconv"
	"strings"
)

func npmSatisfies(requirement, version string) (bool, bool) {
	// Bound work on untrusted lockfile input and validate every alternative:
	// a matching branch must never hide unsupported syntax in another branch.
	if len(requirement) > 4096 {
		return false, false
	}
	alternatives := strings.Split(requirement, "||")
	if len(alternatives) > 32 {
		return false, false
	}
	match := false
	for _, alternative := range alternatives {
		matched, known := npmConjunction(alternative, version)
		if !known {
			return false, false
		}
		match = match || matched
	}
	return match, true
}

func npmConjunction(requirement, version string) (bool, bool) {
	v, ok := stableVersion("npm", version)
	if !ok {
		return false, false
	}
	requirement = strings.TrimSpace(requirement)
	if match, known := npmPartial(requirement, v); known {
		return match, true
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

// npmPartial implements bare partial versions and trailing x/X/* components.
// Comparator and caret partials are intentionally left unsupported.
// Semantics: https://github.com/npm/node-semver#x-ranges-12x-1x-12-
func npmPartial(requirement string, version []uint64) (bool, bool) {
	parts := strings.Split(requirement, ".")
	if len(parts) > 3 {
		return false, false
	}
	wildcard, match := false, true
	for i, part := range parts {
		if part == "*" || part == "x" || part == "X" {
			wildcard = true
			continue
		}
		if wildcard || part == "" || len(part) > 1 && part[0] == '0' {
			return false, false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false, false
			}
		}
		number, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return false, false
		}
		match = match && version[i] == number
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
