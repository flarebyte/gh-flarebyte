// purpose: Compare corroborated wheel identities and generic tags with a declared Python lower bound.
// responsibilities: Preserve unsupported native environments; never infer a Python target from the CLI host.
package dependencies

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type pythonWheel struct {
	Name, Version         string
	Python, ABI, Platform []string
}

var wheelName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.]*$`)
var wheelBuild = regexp.MustCompile(`^[0-9][A-Za-z0-9_]*$`)
var wheelTag = regexp.MustCompile(`^[a-z0-9_]+$`)
var genericPythonTag = regexp.MustCompile(`^py([23])([0-9]{1,3})?$`)

func parsePythonWheel(filename string) (pythonWheel, bool) {
	var result pythonWheel
	if len(filename) > 255 || !strings.HasSuffix(filename, ".whl") {
		return result, false
	}
	parts := strings.Split(strings.TrimSuffix(filename, ".whl"), "-")
	if len(parts) != 5 && len(parts) != 6 || !wheelName.MatchString(parts[0]) {
		return result, false
	}
	if len(parts) == 6 && !wheelBuild.MatchString(parts[2]) {
		return result, false
	}
	if _, ok := stableVersion("pypi", parts[1]); !ok {
		return result, false
	}
	result.Name, result.Version = parts[0], parts[1]
	groups := []*[]string{&result.Python, &result.ABI, &result.Platform}
	count := 1
	for i, group := range groups {
		tags := strings.Split(parts[len(parts)-3+i], ".")
		if len(tags) > 8 {
			return pythonWheel{}, false
		}
		count *= len(tags)
		if count > 64 {
			return pythonWheel{}, false
		}
		for _, tag := range tags {
			if len(tag) > 64 || !wheelTag.MatchString(tag) {
				return pythonWheel{}, false
			}
		}
		slices.Sort(tags)
		*group = slices.Compact(tags)
	}
	return result, true
}

func checkPythonWheel(filename, name, version, manifest string, f *Finding) {
	e := &WheelCompatibility{Basis: "wheel-filename-manifest-lower-bound", Status: "unknown"}
	f.Wheel = e
	unknown := func() { f.Missing = unique(append(f.Missing, "PYPI_WHEEL_COMPATIBILITY")) }
	wheel, ok := parsePythonWheel(filename)
	if !ok {
		unknown()
		return
	}
	e.PythonTags, e.ABITags, e.PlatformTags = wheel.Python, wheel.ABI, wheel.Platform
	locked, lockedOK := stableVersion("pypi", version)
	claimed, _ := stableVersion("pypi", wheel.Version)
	if !lockedOK {
		unknown()
		return
	}
	if !equivalentName("pypi", wheel.Name, name) || compareNumbers(locked, claimed) != 0 {
		e.Status = "conflict"
		f.Reasons = unique(append(f.Reasons, "IDENTITY_CONFLICT"))
		return
	}
	manifest = strings.TrimSpace(manifest)
	if !strings.HasPrefix(manifest, ">=") {
		unknown()
		return
	}
	baseline := strings.TrimSpace(strings.TrimPrefix(manifest, ">="))
	base, ok := pythonRuntimeVersion(baseline)
	if !ok {
		unknown()
		return
	}
	e.Baseline = baseline
	// Native ABI/platform compatibility needs interpreter and target evidence.
	if !slices.Contains(wheel.ABI, "none") || !slices.Contains(wheel.Platform, "any") {
		unknown()
		return
	}
	matched, unsupported := false, false
	for _, tag := range wheel.Python {
		m := genericPythonTag.FindStringSubmatch(tag)
		if m == nil || len(m[2]) > 1 && m[2][0] == '0' {
			unsupported = true
			continue
		}
		major, _ := strconv.ParseUint(m[1], 10, 64)
		minor := uint64(0)
		if m[2] != "" {
			minor, _ = strconv.ParseUint(m[2], 10, 64)
		}
		if major == base[0] && (m[2] == "" || minor <= base[1]) {
			matched = true
		}
	}
	if matched {
		e.Status = "compatible"
		return
	}
	// A supported generic alternative can prove compatibility, but unknown
	// alternatives must not be ignored when concluding incompatibility.
	if unsupported || len(wheel.ABI) != 1 || len(wheel.Platform) != 1 {
		unknown()
		return
	}
	e.Status = "incompatible"
	f.Reasons = unique(append(f.Reasons, "PYPI_WHEEL_TAG_INCOMPATIBLE"))
}
