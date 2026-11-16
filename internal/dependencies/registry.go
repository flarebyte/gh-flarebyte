// purpose: Corroborate release availability against fixed public registries.
// responsibilities: Validate exact package identities, lifecycle status and npm dependency metadata.
// architecture notes: Registry declarations alone never establish source or maintainer authority.
package dependencies

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type NPMRelease struct {
	Name                 string                     `json:"name"`
	Version              string                     `json:"version"`
	Description          string                     `json:"description"`
	Deprecated           string                     `json:"deprecated"`
	Dependencies         map[string]string          `json:"dependencies"`
	OptionalDependencies map[string]string          `json:"optionalDependencies"`
	PeerDependencies     map[string]string          `json:"peerDependencies"`
	PeerMeta             map[string]PeerRequirement `json:"peerDependenciesMeta"`
	BundledDependencies  any                        `json:"bundledDependencies"`
	BundleDependencies   any                        `json:"bundleDependencies"`
	Engines              map[string]string          `json:"engines"`
	OS                   []string                   `json:"os"`
	CPU                  []string                   `json:"cpu"`
	HasShrinkwrap        bool                       `json:"_hasShrinkwrap"`
	Shrinkwrap           bool                       `json:"hasShrinkwrap"`
	Dist                 struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

func (c *Client) npmRelease(ctx context.Context, name, version string, sources *[]Source) (NPMRelease, error) {
	var release NPMRelease
	err := c.read(ctx, "npm-registry", "https://registry.npmjs.org/"+url.PathEscape(name)+"/"+url.PathEscape(version), nil, &release, sources)
	return release, err
}

func equivalentName(system, a, b string) bool {
	if system == "cargo" {
		a = strings.ReplaceAll(strings.ToLower(a), "_", "-")
		b = strings.ReplaceAll(strings.ToLower(b), "_", "-")
	}
	if system == "pypi" {
		a = normalizePyPI.ReplaceAllString(strings.ToLower(a), "-")
		b = normalizePyPI.ReplaceAllString(strings.ToLower(b), "-")
	}
	return a == b
}

// releaseEvidence records a withdrawal only after the registry itself reports
// that an indexed version is unavailable. Other lookup failures remain unknown.
func (c *Client) releaseEvidence(ctx context.Context, system, name, version string, sources *[]Source) (TrustEvidence, *NPMRelease, *string, []string) {
	e := TrustEvidence{}
	var err error
	var description *string
	reasons := []string{}
	switch system {
	case "npm":
		r, lookupErr := c.npmRelease(ctx, name, version, sources)
		err = lookupErr
		if err == nil {
			if r.Name == "" || r.Version == "" {
				return e, nil, nil, reasons
			}
			if r.Name != name || r.Version != version {
				reasons = append(reasons, "IDENTITY_CONFLICT")
				return e, nil, nil, reasons
			}
			e.LifecycleKnown = true
			description = Description(r.Description)
			if r.Deprecated != "" {
				reasons = append(reasons, "DEPRECATED_RELEASE")
			}
			return e, &r, description, reasons
		}
	case "pypi":
		var r struct {
			Info struct {
				Name    string `json:"name"`
				Version string `json:"version"`
				Summary string `json:"summary"`
			} `json:"info"`
			URLs []struct {
				Yanked   bool   `json:"yanked"`
				Filename string `json:"filename"`
			} `json:"urls"`
		}
		err = c.read(ctx, "pypi-registry", "https://pypi.org/pypi/"+url.PathEscape(name)+"/"+url.PathEscape(version)+"/json", nil, &r, sources)
		if err == nil {
			if r.Info.Name == "" || r.Info.Version == "" {
				return e, nil, nil, reasons
			}
			if !equivalentName(system, r.Info.Name, name) || r.Info.Version != version {
				return e, nil, nil, []string{"IDENTITY_CONFLICT"}
			}
			description = Description(r.Info.Summary)
			if len(r.URLs) > 0 {
				e.LifecycleKnown = true
				e.Withdrawn = true
				for _, file := range r.URLs {
					if file.Filename == "" {
						e.LifecycleKnown = false
					}
					if !file.Yanked {
						e.Withdrawn = false
					}
				}
			}
		}
	case "cargo":
		var r struct {
			Version struct {
				Name        string `json:"crate"`
				Number      string `json:"num"`
				Yanked      *bool  `json:"yanked"`
				Description string `json:"description"`
			} `json:"version"`
		}
		err = c.read(ctx, "cargo-registry", "https://crates.io/api/v1/crates/"+url.PathEscape(name)+"/"+url.PathEscape(version), nil, &r, sources)
		if err == nil {
			if r.Version.Name == "" || r.Version.Number == "" {
				return e, nil, nil, reasons
			}
			if !equivalentName(system, r.Version.Name, name) || r.Version.Number != version {
				return e, nil, nil, []string{"IDENTITY_CONFLICT"}
			}
			description = Description(r.Version.Description)
			if r.Version.Yanked != nil {
				e.LifecycleKnown = true
				e.Withdrawn = *r.Version.Yanked
			}
		}
	}
	var lookup *LookupError
	if errors.As(err, &lookup) && lookup.Status == http.StatusNotFound {
		e.LifecycleKnown = true
		e.Withdrawn = true
	}
	return e, nil, description, reasons
}
