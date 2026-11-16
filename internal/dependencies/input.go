// purpose: Validate package coordinates and order a conservative subset of stable releases.
// responsibilities: Preserve exact versions, normalize registry identities and reject ambiguous input.
// architecture notes: Unsupported version syntax remains unknown instead of using lexical ordering.
package dependencies

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var systems = map[string]string{"npm": "npm", "cargo": "crates.io", "pypi": "PyPI", "go": "Go", "maven": "Maven", "pub": "Pub", "swift": "SwiftURL"}
var npmName = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var simpleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var mavenName = regexp.MustCompile(`^[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+$`)
var goName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]*$`)
var normalizePyPI = regexp.MustCompile(`[-_.]+`)
var buildMetadata = regexp.MustCompile(`^[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*$`)
var semverPrerelease = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z.-]+(?:\+[0-9A-Za-z.-]+)?$`)
var pythonPrerelease = regexp.MustCompile(`(?i)^[0-9]+(?:\.[0-9]+)*(?:a|b|rc|\.dev)[0-9]+$`)

func Parse(system, arg string) (string, *string, error) {
	if _, ok := systems[system]; !ok {
		return "", nil, fmt.Errorf("unsupported ecosystem %q", system)
	}
	if len(arg) == 0 || len(arg) > 512 {
		return "", nil, fmt.Errorf("invalid package coordinate")
	}
	name := arg
	var version *string
	if at := strings.LastIndex(arg, "@"); at > 0 {
		name = arg[:at]
		v := arg[at+1:]
		if len(v) == 0 || len(v) > 128 || strings.ContainsAny(v, "\r\n\t /\\^*<>=|@") {
			return "", nil, fmt.Errorf("expected an exact version")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*$`).MatchString(v) {
			return "", nil, fmt.Errorf("invalid exact version")
		}
		version = &v
	}
	ok := false
	switch system {
	case "npm":
		ok = npmName.MatchString(name)
	case "cargo", "pypi", "pub":
		ok = simpleName.MatchString(name)
	case "maven":
		ok = mavenName.MatchString(name)
	case "go":
		ok = goName.MatchString(name) && strings.Contains(name, ".") && !strings.Contains(name, "..") && !strings.Contains(name, "//") && !strings.HasSuffix(name, "/")
	case "swift":
		ok = strings.HasPrefix(name, "https://github.com/") && Repository(strings.TrimPrefix(name, "https://")) != ""
	}
	if !ok {
		return "", nil, fmt.Errorf("invalid %s package name", system)
	}
	if system == "pypi" {
		name = normalizePyPI.ReplaceAllString(strings.ToLower(name), "-")
	}
	if system == "cargo" {
		name = strings.ToLower(name)
	}
	return name, version, nil
}

// Repository permits only canonical GitHub owner/repository coordinates. Package
// metadata never controls a network origin, query string, fragment or local path.
func Repository(repo string) string {
	parts := strings.Split(repo, "/")
	if len(parts) != 3 || parts[0] != "github.com" || !simpleName.MatchString(parts[1]) || !simpleName.MatchString(parts[2]) || parts[1] == "." || parts[1] == ".." || parts[2] == "." || parts[2] == ".." {
		return ""
	}
	return repo
}

func stableVersion(system, version string) ([]uint64, bool) {
	if len(version) > 128 {
		return nil, false
	}
	if system == "maven" || system == "swift" {
		return nil, false
	}
	if system == "go" {
		if !strings.HasPrefix(version, "v") {
			return nil, false
		}
		version = strings.TrimPrefix(version, "v")
		version = strings.TrimSuffix(version, "+incompatible")
	}
	if system == "npm" || system == "cargo" {
		if at := strings.IndexByte(version, '+'); at >= 0 {
			if !buildMetadata.MatchString(version[at+1:]) {
				return nil, false
			}
			version = version[:at]
		}
	}
	parts := strings.Split(version, ".")
	if len(parts) > 4 || ((system == "npm" || system == "cargo" || system == "go") && len(parts) != 3) {
		return nil, false
	}
	values := make([]uint64, 4)
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return nil, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return nil, false
			}
		}
		v, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nil, false
		}
		values[i] = v
	}
	return values, true
}

func knownPrerelease(system, version string) bool {
	if system == "npm" || system == "cargo" || system == "go" {
		return semverPrerelease.MatchString(version)
	}
	return system == "pypi" && pythonPrerelease.MatchString(version)
}

func newer(system, a, b string) bool {
	aa, oka := stableVersion(system, a)
	bb, okb := stableVersion(system, b)
	if !oka || !okb {
		return a > b
	} // Only a deterministic fallback, never eligibility.
	for i := range aa {
		if aa[i] != bb[i] {
			return aa[i] > bb[i]
		}
	}
	return a > b
}
