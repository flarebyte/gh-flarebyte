// purpose: Corroborate availability of unorderable Python catalog entries.
// responsibilities: Exclude only registry-confirmed ineligible releases while keeping ambiguity blocking.
// architecture notes: Original version strings and exact-version checks remain unchanged; no ordering is guessed.
package dependencies

import "context"

const maxPythonCatalogProbes = 4

func (c *Client) pythonCatalogUnavailable(ctx context.Context, name, version string, sources *[]Source) bool {
	parsed, exact, err := Parse("pypi", name+"@"+version)
	if err != nil || parsed != name || exact == nil || *exact != version {
		return false
	}
	start := len(*sources)
	e, _, _, reasons := c.releaseEvidence(ctx, "pypi", name, version, sources)
	for i := start; i < len(*sources); i++ {
		(*sources)[i].Package = "pypi:" + name
		(*sources)[i].Version = version
	}
	return e.LifecycleKnown && e.Withdrawn && len(reasons) == 0
}
