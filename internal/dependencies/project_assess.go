// purpose: Assess exact packages in a validated local lock graph with dependency paths.
// responsibilities: Preserve transitive violations and block on incomplete evidence or artifact disagreement.
// architecture notes: No package manager, build backend or package code executes during inspection.
package dependencies

import (
	"context"
	"strings"
	"time"
)

const maxAssessedNodes = 32

func (c *Client) assessProject(ctx context.Context, project *ProjectSnapshot, p Policy, now time.Time, r *Report, cache map[string]TrustEvidence) {
	for index, node := range project.Nodes {
		if index >= maxAssessedNodes || ctx.Err() != nil {
			project.Missing = unique(append(project.Missing, "PROJECT_ASSESSMENT_LIMIT"))
			break
		}
		finding := Finding{Version: node.Version, Decision: Unknown, Reasons: []string{}, Missing: []string{}, Advisories: []string{}}
		v, err := c.Version(ctx, "npm", node.Name, node.Version, &r.Details.Sources)
		if err != nil || v.Key.Name != node.Name || v.Key.Version != node.Version || !strings.EqualFold(v.Key.System, "npm") {
			finding.Missing = []string{"VERSION_LOOKUP"}
		} else {
			advisories, osvErr := c.OSV(ctx, "npm", node.Name, node.Version, &r.Details.Sources)
			e, release, _, releaseReasons := c.releaseEvidence(ctx, "npm", node.Name, node.Version, &r.Details.Sources)
			e.ProjectResolved = true
			e.NameScreened, e.NameConflict = screenName("npm", node.Name)
			c.fillRepository(ctx, v, &e, r, cache)
			c.fillMaintainers(ctx, &e, p, now, r, cache)
			finding = evaluateRelease(v, advisories, osvErr, e, p, now)
			finding.Reasons = unique(append(finding.Reasons, releaseReasons...))
			if release != nil {
				if release.Dist.Tarball != node.Resolved || release.Dist.Integrity != node.Integrity {
					finding.Reasons = unique(append(finding.Reasons, "ARTIFACT_MISMATCH"))
				}
				locked := project.packages[node.Path]
				declared := npmPackage{Dependencies: release.Dependencies, OptionalDependencies: release.OptionalDependencies, PeerDependencies: release.PeerDependencies, PeerMeta: release.PeerMeta}
				locked.DevDependencies = nil
				if !sameDeps(declared, locked) {
					finding.Reasons = unique(append(finding.Reasons, "DEPENDENCY_METADATA_MISMATCH"))
				}
				if hasBundled(release.BundleDependencies) || hasBundled(release.BundledDependencies) {
					finding.Missing = unique(append(finding.Missing, "BUNDLED_DEPENDENCIES"))
				}
				if release.HasShrinkwrap || release.Shrinkwrap {
					finding.Missing = unique(append(finding.Missing, "SHRINKWRAP_RESOLUTION"))
				}
				if len(release.Engines) > 0 || len(release.OS) > 0 || len(release.CPU) > 0 {
					finding.Missing = unique(append(finding.Missing, "RUNTIME_COMPATIBILITY"))
				}
			}
			finding.Decision = verdict(finding.Reasons, finding.Missing)
		}
		project.Findings = append(project.Findings, ProjectFinding{Path: node.Path, Package: "npm:" + node.Name, Finding: finding})
	}
}

func hasBundled(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

func mergeProjectFindings(f *Finding, project *ProjectSnapshot, name string) {
	if len(project.Missing) > 0 || len(project.Findings) != len(project.Nodes) {
		f.Missing = append(f.Missing, "PROJECT_ASSESSMENT_INCOMPLETE")
	}
	for _, node := range project.Findings {
		if f.Resolution == "replace-leaf" && name != "" && node.Path == "node_modules/"+name {
			continue // The independently assessed candidate replaces this exact node.
		}
		f.Reasons = append(f.Reasons, node.Finding.Reasons...)
		f.Advisories = append(f.Advisories, node.Finding.Advisories...)
		if len(node.Finding.Missing) > 0 {
			f.Missing = append(f.Missing, "TRANSITIVE_EVIDENCE")
		}
	}
	f.Reasons = unique(f.Reasons)
	f.Missing = unique(f.Missing)
	f.Advisories = unique(f.Advisories)
	if f.Evidence != nil {
		f.Evidence.Advisories = f.Advisories
		f.Evidence.Missing = f.Missing
	}
}
