// purpose: Discover dated public GitHub releases for named Swift packages.
// responsibilities: Bound pagination and reject ambiguous tags without cloning or running Swift.
package dependencies

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const maxSwiftReleasePages = 5

type swiftRelease struct {
	Tag         string    `json:"tag_name"`
	Draft       *bool     `json:"draft"`
	Prerelease  *bool     `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// GitHub releases are the publication clock, not tag creation or commit dates.
// Tags without a GitHub release have unknown age and are not discovered here.
func (c *Client) swiftCatalog(ctx context.Context, name string, sources *[]Source) (Package, map[string]Version, error) {
	var pack Package
	canonical, version, err := Parse("swift", name)
	if err != nil || canonical != name || version != nil {
		return pack, nil, fmt.Errorf("invalid Swift repository")
	}
	pack.PackageKey.Name = name
	versions := map[string]Version{}
	repo := strings.TrimPrefix(name, "https://github.com/")
	for page := 1; page <= maxSwiftReleasePages; page++ {
		var releases []swiftRelease
		endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100&page=%d", repo, page)
		if err := c.read(ctx, "github-releases", endpoint, nil, &releases, sources); err != nil {
			return pack, nil, err
		}
		if len(releases) > 100 {
			return pack, nil, fmt.Errorf("oversized GitHub release page")
		}
		for _, release := range releases {
			if release.Draft == nil || release.Prerelease == nil || release.Tag == "" {
				return pack, nil, fmt.Errorf("incomplete GitHub release metadata")
			}
			if *release.Draft || *release.Prerelease {
				continue
			}
			tag := strings.TrimPrefix(release.Tag, "v")
			if knownPrerelease("swift", tag) {
				continue
			}
			if _, exists := versions[tag]; exists {
				return pack, nil, fmt.Errorf("ambiguous Swift release tags")
			}
			v := Version{Key: VersionKey{System: "swift", Name: name, Version: tag}, PublishedAt: release.PublishedAt}
			versions[tag] = v
			pack.Versions = append(pack.Versions, v)
		}
		if len(releases) < 100 {
			return pack, versions, nil
		}
	}
	return pack, nil, fmt.Errorf("Swift release catalog exceeds bounded coverage")
}
