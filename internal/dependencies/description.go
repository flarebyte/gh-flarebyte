// purpose: Fetch short package descriptions from fixed registry origins.
// responsibilities: Prefer package metadata without fetching untrusted homepage URLs.
// architecture notes: Missing descriptions are informational and never block security assessment.
package dependencies

import (
	"context"
	"net/url"
)

func (c *Client) registryDescription(ctx context.Context, system, name string, version *string, sources *[]Source) *string {
	var endpoint string
	var response struct {
		Description string `json:"description"`
		Info        struct {
			Summary string `json:"summary"`
		} `json:"info"`
		Crate struct {
			Description string `json:"description"`
		} `json:"crate"`
	}
	switch system {
	case "npm":
		// The abbreviated registry document is bounded by the same response cap.
		endpoint = "https://registry.npmjs.org/" + url.PathEscape(name)
		if version != nil {
			endpoint += "/" + url.PathEscape(*version)
		} else {
			endpoint += "/latest"
		}
	case "pypi":
		endpoint = "https://pypi.org/pypi/" + url.PathEscape(name)
		if version != nil {
			endpoint += "/" + url.PathEscape(*version)
		}
		endpoint += "/json"
	case "cargo":
		endpoint = "https://crates.io/api/v1/crates/" + url.PathEscape(name)
	default:
		return nil
	}
	if err := c.read(ctx, system+"-registry", endpoint, nil, &response, sources); err != nil {
		return nil
	}
	switch system {
	case "pypi":
		return Description(response.Info.Summary)
	case "cargo":
		return Description(response.Crate.Description)
	default:
		return Description(response.Description)
	}
}
