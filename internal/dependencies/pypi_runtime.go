// purpose: Compare a wheel's declared Python requirement with a project's declared lower bound.
// responsibilities: Keep unsupported specifiers unknown and expose definite baseline incompatibilities.
// architecture notes: This does not establish an installed interpreter, wheel tag compatibility or resolution.
package dependencies

import "strings"

func pythonRuntimeVersion(raw string) ([]uint64, bool) {
	if len(raw) == 0 || len(raw) > 64 {
		return nil, false
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, false
	}
	for _, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return nil, false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return nil, false
			}
		}
	}
	return stableVersion("cargo", raw+strings.Repeat(".0", 3-len(parts)))
}

// Parse the complete supported conjunction before returning a mismatch: a valid
// clause must not turn malformed or unsupported trailing syntax into a denial.
func pythonRequirementMatches(requirement string, baseline []uint64) (bool, bool) {
	if len(requirement) == 0 || len(requirement) > 512 {
		return false, false
	}
	clauses := strings.Split(requirement, ",")
	if len(clauses) > 16 {
		return false, false
	}
	matches := true
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		op := ""
		for _, candidate := range []string{">=", "<=", "==", "!=", ">", "<"} {
			if strings.HasPrefix(clause, candidate) {
				op = candidate
				break
			}
		}
		if op == "" {
			return false, false
		}
		version, ok := pythonRuntimeVersion(strings.TrimSpace(strings.TrimPrefix(clause, op)))
		if !ok {
			return false, false
		}
		cmp := compareNumbers(baseline, version)
		match := op == ">=" && cmp >= 0 || op == "<=" && cmp <= 0 || op == ">" && cmp > 0 || op == "<" && cmp < 0 || op == "==" && cmp == 0 || op == "!=" && cmp != 0
		matches = matches && match
	}
	return matches, true
}

func checkPythonCompatibility(manifest, lock string, wheel *string, f *Finding) {
	e := &RuntimeCompatibility{Runtime: "python", Basis: "manifest-python-lower-bound", Status: "unknown"}
	f.Runtime = e
	unknown := func() { f.Missing = unique(append(f.Missing, "RUNTIME_COMPATIBILITY")) }
	// Only a single inclusive numeric lower bound defines the baseline here.
	manifest = strings.TrimSpace(manifest)
	if !strings.HasPrefix(manifest, ">=") {
		unknown()
		return
	}
	base := strings.TrimSpace(strings.TrimPrefix(manifest, ">="))
	version, ok := pythonRuntimeVersion(base)
	if !ok {
		unknown()
		return
	}
	e.Baseline = base
	if lock != "" {
		match, supported := pythonRequirementMatches(lock, version)
		if !supported {
			unknown()
			return
		}
		if !match {
			e.Status = "incompatible"
			f.Reasons = unique(append(f.Reasons, "PYTHON_LOCK_BASELINE_INCOMPATIBLE"))
			return
		}
	}
	if wheel == nil {
		unknown()
		return
	}
	// An explicit empty requirement is unconstrained. Missing/null is unknown.
	if *wheel != "" {
		match, supported := pythonRequirementMatches(*wheel, version)
		if !supported {
			unknown()
			return
		}
		if !match {
			e.Status = "incompatible"
			f.Reasons = unique(append(f.Reasons, "PYTHON_VERSION_INCOMPATIBLE"))
			return
		}
	}
	e.Status = "compatible"
}
