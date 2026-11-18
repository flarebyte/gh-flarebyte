// purpose: Read dated pub.dev releases without resolving or downloading Dart packages.
// responsibilities: Preserve retraction/discontinuation facts and unverified repository declarations.
package dependencies

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"time"
)

var pubPublisherID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+$`)

// Optional lifecycle flags default to false only when omitted, never when null.
type pubFlag bool

func (f *pubFlag) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "true":
		*f = true
	case "false":
		*f = false
	default:
		return fmt.Errorf("invalid pub lifecycle flag")
	}
	return nil
}

type pubRelease struct {
	Version   string    `json:"version"`
	Published time.Time `json:"published"`
	Retracted pubFlag   `json:"retracted"` // The hosted v2 API defines omission as false.
	Pubspec   struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Repository  string `json:"repository"`
	} `json:"pubspec"`
}

type pubPackage struct {
	Name           string       `json:"name"`
	IsDiscontinued pubFlag      `json:"isDiscontinued"` // Omission means false in hosted v2.
	Versions       []pubRelease `json:"versions"`
	releases       map[string]pubRelease
}

func (c *Client) pubCatalog(ctx context.Context, name string, sources *[]Source) (Package, map[string]Version, *pubPackage, error) {
	var pack Package
	canonical, version, err := Parse("pub", name)
	if err != nil || canonical != name || version != nil {
		return pack, nil, nil, fmt.Errorf("invalid pub package")
	}
	var body pubPackage
	err = c.readAccept(ctx, "pub-registry", "https://pub.dev/api/packages/"+url.PathEscape(name), nil, &body, sources, "application/vnd.pub.v2+json")
	if err != nil {
		return pack, nil, nil, err
	}
	pack.PackageKey.Name = body.Name
	versions := map[string]Version{}
	body.releases = map[string]pubRelease{}
	if body.Name != name {
		return pack, nil, nil, fmt.Errorf("conflicting pub package identity")
	}
	if len(body.Versions) == 0 || len(body.Versions) > 10000 {
		return pack, nil, nil, fmt.Errorf("invalid pub release catalog size")
	}
	for _, release := range body.Versions {
		if release.Version == "" || len(release.Version) > 128 || release.Pubspec.Name != name || release.Pubspec.Version != release.Version {
			return pack, nil, nil, fmt.Errorf("invalid pub release identity")
		}
		if _, exists := versions[release.Version]; exists {
			return pack, nil, nil, fmt.Errorf("duplicate pub release")
		}
		v := Version{Key: VersionKey{System: "pub", Name: name, Version: release.Version}, PublishedAt: release.Published}
		// Registry declarations may identify a candidate repository, never verify it.
		repo := sourceRepository(release.Pubspec.Repository)
		if repo != "" {
			v.Projects = append(v.Projects, struct {
				Key struct {
					ID string `json:"id"`
				} `json:"projectKey"`
				Provenance string `json:"relationProvenance"`
				Type       string `json:"relationType"`
			}{})
			v.Projects[0].Key.ID = repo
			v.Projects[0].Type = "SOURCE_REPO"
		}
		versions[release.Version] = v
		body.releases[release.Version] = release
		pack.Versions = append(pack.Versions, v)
	}
	return pack, versions, &body, nil
}
