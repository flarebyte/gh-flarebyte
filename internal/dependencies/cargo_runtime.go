// purpose: Compare crate minimum Rust requirements against the project's declared compiler baseline.
// responsibilities: Preserve unknown declarations and distinguish incompatibility from missing evidence.
// architecture notes: No toolchain probing or package code execution; feature and target gaps remain independent.
package dependencies

import "strings"

// Edition release floors are recorded by the Rust Edition Guide:
// https://doc.rust-lang.org/edition-guide/
var rustEditionFloors = map[string]string{"2015": "1.0", "2018": "1.31", "2021": "1.56", "2024": "1.85"}

func cargoEditionBaseline(pkg map[string]any, baseline string) bool {
	edition := "2015"
	if value, present := pkg["edition"]; present {
		var ok bool
		edition, ok = value.(string)
		if !ok {
			return false
		}
	}
	floor, ok := rustEditionFloors[edition]
	if !ok {
		return false
	}
	if baseline == "" {
		return true
	} // An edition is not a declared project MSRV.
	base, baseOK := rustVersion(baseline)
	minimum, _ := rustVersion(floor)
	return baseOK && compareNumbers(base, minimum) >= 0
}

// Cargo rust-version is a bare partial version without operators, build metadata
// or prereleases: https://doc.rust-lang.org/cargo/reference/rust-version.html
func rustVersion(version string) ([]uint64, bool) {
	if version == "" || len(version) > 128 || strings.ContainsAny(version, "+-") {
		return nil, false
	}
	components := strings.Split(version, ".")
	if len(components) > 3 {
		return nil, false
	}
	return stableVersion("cargo", version+strings.Repeat(".0", 3-len(components)))
}

func checkRustCompatibility(baseline, minimum string, f *Finding) {
	e := &RuntimeCompatibility{Runtime: "rust", Basis: "manifest-rust-version", Status: "unknown"}
	f.Runtime = e
	base, baseOK := rustVersion(baseline)
	required, requiredOK := rustVersion(minimum)
	// Only parsed numeric declarations enter explained output. Invalid upstream
	// strings may contain arbitrary text and are never repeated in diagnostics.
	if baseOK {
		e.Baseline = baseline
	}
	if requiredOK {
		e.Minimum = minimum
	}
	if !baseOK || !requiredOK {
		f.Missing = unique(append(f.Missing, "RUNTIME_COMPATIBILITY"))
		return
	}
	if compareNumbers(required, base) > 0 {
		e.Status = "incompatible"
		f.Reasons = unique(append(f.Reasons, "RUST_VERSION_INCOMPATIBLE"))
		return
	}
	e.Status = "compatible"
}
