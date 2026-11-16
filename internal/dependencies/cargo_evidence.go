// purpose: Corroborate Cargo lockfile artifacts and dependency edges with fixed-origin registry evidence.
// responsibilities: Reject checksum/metadata disagreements and retain feature or schema uncertainty.
package dependencies

import (
	"context"
	"net/url"
)

func (c *Client) checkCargoNode(ctx context.Context, project *ProjectSnapshot, node ProjectNode, f *Finding, sources *[]Source) {
	if project.cargo != nil && project.cargo.featuresUnverified {
		f.Missing = unique(append(f.Missing, "CARGO_FEATURE_RESOLUTION"))
	}
	base := "https://crates.io/api/v1/crates/" + url.PathEscape(node.Name) + "/" + url.PathEscape(node.Version)
	var release struct {
		Version struct {
			Name        string `json:"crate"`
			Version     string `json:"num"`
			Checksum    string `json:"checksum"`
			RustVersion string `json:"rust_version"`
			Yanked      *bool  `json:"yanked"`
		} `json:"version"`
	}
	if err := c.read(ctx, "cargo-registry", base, nil, &release, sources); err != nil {
		f.Missing = unique(append(f.Missing, "CARGO_ARTIFACT_LOOKUP"))
	} else if release.Version.Name == "" || release.Version.Version == "" {
		f.Missing = unique(append(f.Missing, "CARGO_ARTIFACT_SCHEMA"))
	} else if release.Version.Name != node.Name || release.Version.Version != node.Version {
		f.Reasons = unique(append(f.Reasons, "IDENTITY_CONFLICT"))
	} else {
		if release.Version.Yanked == nil {
			f.Missing = unique(append(f.Missing, "RELEASE_LIFECYCLE"))
		} else if *release.Version.Yanked {
			f.Reasons = unique(append(f.Reasons, "WITHDRAWN_RELEASE"))
		}
		if !cargoChecksum.MatchString(release.Version.Checksum) {
			f.Missing = unique(append(f.Missing, "CARGO_ARTIFACT_SCHEMA"))
		} else if node.Integrity != "sha256-"+release.Version.Checksum {
			f.Reasons = unique(append(f.Reasons, "ARTIFACT_MISMATCH"))
		}
		if release.Version.RustVersion != "" {
			checkRustCompatibility(project.RuntimeBaseline, release.Version.RustVersion, f)
		}
	}
	var result struct {
		Dependencies *[]struct {
			Name        string `json:"crate_id"`
			Requirement string `json:"req"`
			Kind        string `json:"kind"`
			Optional    *bool  `json:"optional"`
		} `json:"dependencies"`
	}
	if err := c.read(ctx, "cargo-registry", base+"/dependencies", nil, &result, sources); err != nil {
		f.Missing = unique(append(f.Missing, "CARGO_DEPENDENCY_LOOKUP"))
		return
	}
	if result.Dependencies == nil || len(*result.Dependencies) > maxProjectNodes || project.cargo == nil {
		f.Missing = unique(append(f.Missing, "CARGO_DEPENDENCY_SCHEMA"))
		return
	}
	g := project.cargo
	explained := map[string]bool{}
	uncertain := false
	for _, d := range *result.Dependencies {
		if !cargoName.MatchString(d.Name) || d.Optional == nil || d.Requirement == "" || (d.Kind != "normal" && d.Kind != "build" && d.Kind != "dev") {
			f.Missing = append(f.Missing, "CARGO_DEPENDENCY_SCHEMA")
			uncertain = true
			continue
		}
		if d.Kind == "dev" {
			continue
		} // Registry crates' own test dependencies are not activated by this consumer.
		matched := false
		for _, edge := range g.edges[node.Path] {
			p := g.packages[edge]
			if p.Name != d.Name {
				continue
			}
			match, known := cargoSatisfies(d.Requirement, p.Version)
			if !known {
				f.Missing = append(f.Missing, "PROJECT_RANGE_SEMANTICS")
				continue
			}
			if match {
				matched = true
				explained[edge] = true
			}
		}
		if _, known := cargoSatisfies(d.Requirement, "0.0.0"); !known {
			f.Missing = append(f.Missing, "PROJECT_RANGE_SEMANTICS")
			uncertain = true
			continue
		}
		if !matched {
			if *d.Optional {
				f.Missing = append(f.Missing, "CARGO_FEATURE_RESOLUTION")
			} else {
				f.Reasons = append(f.Reasons, "DEPENDENCY_METADATA_MISMATCH")
			}
		}
	}
	for _, edge := range g.edges[node.Path] {
		if !explained[edge] && !uncertain {
			f.Reasons = append(f.Reasons, "DEPENDENCY_METADATA_MISMATCH")
		}
	}
	f.Missing = unique(f.Missing)
	f.Reasons = unique(f.Reasons)
}
