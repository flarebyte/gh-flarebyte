// purpose: Order a bounded subset of Python public versions without lexical guesses.
// responsibilities: Compare epochs, zero-padded releases and stable post releases; identify canonical prereleases.
// architecture notes: Local versions and noncanonical spellings remain unsupported; original registry identities are preserved.
package dependencies

import (
	"regexp"
	"strconv"
	"strings"
)

// Canonical public grammar and ordering are specified by PyPA:
// https://packaging.python.org/en/latest/specifications/version-specifiers/
var pythonVersion = regexp.MustCompile(`^(?:([0-9]+)!)?([0-9]+(?:\.[0-9]+)*)(?:(a|b|rc)([0-9]+))?(?:\.post([0-9]+))?(?:\.dev([0-9]+))?(?:\+([a-z0-9]+(?:\.[a-z0-9]+)*))?$`)

// A 128-byte identifier can contain at most 64 single-digit release components.
const pythonReleaseParts = 64

func pythonStableVersion(version string) ([]uint64, bool) {
	if len(version) > 128 {
		return nil, false
	}
	m := pythonVersion.FindStringSubmatch(version)
	if m == nil || m[3] != "" || m[6] != "" || m[7] != "" {
		return nil, false
	}
	// Epoch, padded release tuple, post-presence flag, post number. The flag
	// distinguishes a final release from its strictly newer .post0 release.
	key := make([]uint64, 1+pythonReleaseParts+2)
	parse := func(text string) (uint64, bool) {
		text = strings.TrimLeft(text, "0")
		if text == "" {
			return 0, true
		}
		number, err := strconv.ParseUint(text, 10, 64)
		return number, err == nil
	}
	var ok bool
	if key[0], ok = parse(m[1]); !ok {
		return nil, false
	}
	for i, part := range strings.Split(m[2], ".") {
		if key[i+1], ok = parse(part); !ok {
			return nil, false
		}
	}
	if m[5] != "" {
		key[len(key)-2] = 1
		if key[len(key)-1], ok = parse(m[5]); !ok {
			return nil, false
		}
	}
	return key, true
}

func pythonPrerelease(version string) bool {
	if len(version) > 128 {
		return false
	}
	m := pythonVersion.FindStringSubmatch(version)
	return m != nil && (m[3] != "" || m[6] != "")
}
