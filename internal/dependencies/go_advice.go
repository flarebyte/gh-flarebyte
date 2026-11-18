// purpose: Corroborate GitHub repository associations from indexed Go module coordinates.
// responsibilities: Label path-based evidence; never crawl vanity hosts or claim artifact provenance.
package dependencies

import (
	"regexp"
	"strings"
)

// Detection only: pseudo-version ordering is deliberately left unsupported.
var goPseudoVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+-(?:[0-9A-Za-z.-]+\.)?[0-9]{14}-[0-9a-f]{12}(?:\+incompatible)?$`)

func goAdviceRepository(module, repo, identity string) (string, string, string) {
	if identity == "conflict" {
		return repo, identity, ""
	}
	parts := strings.Split(module, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		derived := Repository(strings.TrimSuffix(strings.Join(parts[:3], "/"), ".git"))
		if derived != "" {
			if repo != "" && !strings.EqualFold(repo, derived) {
				return "", "conflict", "github-module-path"
			}
			return derived, "verified", "github-module-path"
		}
	}
	if identity == "verified" {
		return repo, identity, "verified-provenance"
	}
	return repo, identity, ""
}
