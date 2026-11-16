// purpose: Flag close spellings of a small, explicit reference set of established packages.
// responsibilities: Provide deterministic lookalike screening with visible, limited coverage.
// architecture notes: This is an additional adoption warning, not a claim of exhaustive malware detection.
package dependencies

import "strings"

const nameReferenceRevision = "curated-v1"

var nameReferences = map[string][]string{
	"npm":   {"lodash", "react", "express", "axios", "typescript", "commander", "chalk", "zod"},
	"cargo": {"serde", "tokio", "clap", "anyhow", "reqwest"},
	"pypi":  {"requests", "numpy", "pandas", "django", "flask", "pydantic"},
	"go":    {"github.com/spf13/cobra", "github.com/stretchr/testify", "golang.org/x/net"},
	"maven": {"org.slf4j:slf4j-api", "com.google.guava:guava"},
	"pub":   {"archive", "http", "collection"},
}

func screenName(system, name string) (bool, bool) {
	references, ok := nameReferences[system]
	if !ok {
		return false, false
	}
	// The reviewed @types namespace deliberately mirrors library names.
	if system == "npm" && strings.HasPrefix(name, "@types/") {
		return true, false
	}
	for _, reference := range references {
		if name == reference {
			return true, false
		}
	}
	for _, reference := range references {
		if nearName(name, reference) {
			return true, true
		}
		if system == "npm" && strings.HasPrefix(name, "@") && strings.HasSuffix(name, "/"+reference) {
			return true, true
		}
	}
	return true, false
}

func nearName(a, b string) bool {
	if a == b {
		return false
	}
	if len(a) > len(b)+1 || len(b) > len(a)+1 {
		return false
	}
	if len(a) == len(b) {
		diff := []int{}
		for i := range a {
			if a[i] != b[i] {
				diff = append(diff, i)
			}
		}
		return len(diff) == 1 || (len(diff) == 2 && diff[1] == diff[0]+1 && a[diff[0]] == b[diff[1]] && a[diff[1]] == b[diff[0]])
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	for i := 0; i < len(b); i++ {
		if a == b[:i]+b[i+1:] {
			return true
		}
	}
	return false
}
