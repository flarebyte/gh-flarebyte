// purpose: Check a bounded subset of stable Cargo requirements using Cargo's caret defaults.
// responsibilities: Distinguish Rust bare-version ranges from npm exact versions and reject unknown grammar.
package dependencies

import (
	"regexp"
	"strings"
)

var cargoWildcard = regexp.MustCompile(`^(?:[0-9]+\.){0,2}\*$`)

func cargoSatisfies(requirement, version string) (bool, bool) {
	v, ok := stableVersion("cargo", version)
	if !ok || len(requirement) > 4096 {
		return false, false
	}
	parts := strings.Split(requirement, ",")
	if len(parts) > 16 {
		return false, false
	}
	match := true
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			return false, false
		}
		if strings.Contains(part, "*") {
			if !cargoWildcard.MatchString(part) {
				return false, false
			}
			m, known := npmPartial(part, v)
			if !known {
				return false, false
			}
			match = match && m
			continue
		}
		op := "^"
		for _, prefix := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
			if strings.HasPrefix(part, prefix) {
				op = prefix
				part = strings.TrimSpace(strings.TrimPrefix(part, prefix))
				break
			}
		}
		digits := strings.Split(part, ".")
		if len(digits) > 3 || len(digits) == 0 {
			return false, false
		}
		if strings.ContainsAny(part, "+-") {
			return false, false
		}
		padded := part + strings.Repeat(".0", 3-len(digits))
		base, known := stableVersion("cargo", padded)
		if !known {
			return false, false
		}
		if op != "^" && op != "~" {
			if len(digits) != 3 {
				return false, false
			}
			m, known := npmSatisfies(op+part, version)
			if !known {
				return false, false
			}
			match = match && m
			continue
		}
		index := 2
		if op == "~" {
			index = 1
			if len(digits) == 1 {
				index = 0
			}
		} else if base[0] > 0 || len(digits) == 1 {
			index = 0
		} else if base[1] > 0 || len(digits) == 2 {
			index = 1
		}
		upper := append([]uint64{}, base...)
		upper[index]++
		for i := index + 1; i < len(upper); i++ {
			upper[i] = 0
		}
		if compareNumbers(upper, base) <= 0 {
			return false, false
		}
		match = match && compareNumbers(v, base) >= 0 && compareNumbers(v, upper) < 0
	}
	return match, true
}
