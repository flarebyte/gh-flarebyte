// purpose: Interpret a conservative subset of file-specific Python core metadata.
// responsibilities: Validate identities and bounded headers; preserve unsupported dependency contexts as unknown.
package dependencies

import (
	"bufio"
	"bytes"
	"net/textproto"
	"regexp"
	"strings"
	"unicode/utf8"
)

type pythonCoreMetadata struct {
	Name         string
	Version      string
	Dependencies []string
	Python       *string
}

func parsePyPIMetadata(b []byte) (pythonCoreMetadata, bool) {
	var result pythonCoreMetadata
	if len(b) > maxPyPIMetadata || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return result, false
	}
	// Only headers affect assessment. The long description is never interpreted.
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	header, _, found := strings.Cut(text, "\n\n")
	if !found || len(header) > 64<<10 || strings.Contains(header, "\r") {
		return result, false
	}
	lines := strings.Split(header, "\n")
	if len(lines) > 1024 {
		return result, false
	}
	for _, line := range lines {
		if len(line) > 8192 {
			return result, false
		}
		for _, r := range line {
			if r < 32 && r != '\t' || r == 127 {
				return result, false
			}
		}
	}
	h, err := textproto.NewReader(bufio.NewReader(strings.NewReader(header + "\n\n"))).ReadMIMEHeader()
	if err != nil || len(h.Values("Name")) != 1 || len(h.Values("Version")) != 1 || len(h.Values("Metadata-Version")) != 1 || len(h.Values("Requires-Python")) > 1 {
		return result, false
	}
	switch h.Get("Metadata-Version") {
	case "2.1", "2.2", "2.3", "2.4", "2.5", "2.6":
	default:
		return result, false
	}
	result.Name = h.Get("Name")
	result.Version = h.Get("Version")
	if _, exact, err := Parse("pypi", result.Name+"@"+result.Version); err != nil || exact == nil {
		return result, false
	}
	if _, ok := stableVersion("pypi", result.Version); !ok {
		return result, false
	}
	result.Dependencies = h.Values("Requires-Dist")
	if len(result.Dependencies) > maxProjectNodes {
		return result, false
	}
	if values := h.Values("Requires-Python"); len(values) == 1 {
		value := values[0]
		result.Python = &value
	}
	return result, true
}

var pythonDistributionName = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)(.*)$`)

func pythonDistributionRequirement(raw string) (string, string, bool) {
	if len(raw) > 512 || strings.ContainsAny(raw, ";[]@\\\r\n") {
		return "", "", false
	}
	match := pythonDistributionName.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return "", "", false
	}
	name, _, err := Parse("pypi", match[1])
	if err != nil {
		return "", "", false
	}
	constraint := strings.TrimSpace(match[2])
	if strings.HasPrefix(constraint, "(") && strings.HasSuffix(constraint, ")") {
		constraint = strings.TrimSpace(constraint[1 : len(constraint)-1])
		if constraint == "" {
			return "", "", false
		}
	}
	// Validate all clauses before checking presence in the inventory: unsupported
	// syntax does not prove that a dependency would apply to this environment.
	if _, ok := pythonDistributionMatches(constraint, "1.0.0"); !ok {
		return "", "", false
	}
	return name, constraint, true
}

func pythonDistributionMatches(requirement, version string) (bool, bool) {
	// Exclusive PEP 440 comparisons have special post-release rules. Keep
	// post releases outside this numeric subset rather than guessing semantics.
	if strings.ContainsAny(version, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ+-") {
		return false, false
	}
	base, ok := stableVersion("pypi", version)
	if !ok {
		return false, false
	}
	if requirement == "" {
		return true, true
	}
	return pythonNumericConjunction(requirement, base, func(raw string) ([]uint64, bool) {
		if strings.ContainsAny(raw, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ+-") {
			return nil, false
		}
		return stableVersion("pypi", raw)
	})
}
