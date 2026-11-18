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
	system := project.System
	if system == "" {
		system = "npm"
	}
	for index, node := range project.Nodes {
		if index >= maxAssessedNodes || ctx.Err() != nil {
			project.Missing = unique(append(project.Missing, "PROJECT_ASSESSMENT_LIMIT"))
			break
		}
		finding := Finding{Version: node.Version, Decision: Unknown, Reasons: []string{}, Missing: []string{}, Advisories: []string{}}
		advisories, osvErr := c.OSV(ctx, systems[system], node.Name, node.Version, &r.Details.Sources)
		v, err := c.Version(ctx, system, node.Name, node.Version, &r.Details.Sources)
		if err != nil || v.Key.Name != node.Name || v.Key.Version != node.Version || !strings.EqualFold(v.Key.System, system) {
			finding = EvaluateVersion(Version{Key: VersionKey{System: system, Name: node.Name, Version: node.Version}}, advisories, osvErr, p, now)
			finding.Missing = unique(append(finding.Missing, "VERSION_LOOKUP"))
		} else {
			e, release, _, releaseReasons := c.releaseEvidence(ctx, system, node.Name, node.Version, &r.Details.Sources)
			e.ProjectResolved = system != "pypi"
			e.NameScreened, e.NameConflict = screenName(system, node.Name)
			c.fillRepository(ctx, v, &e, r, cache)
			c.fillMaintenance(ctx, system, node.Name, &e, p, now, r, cache)
			finding = evaluateRelease(v, advisories, osvErr, e, p, now)
			finding.Reasons = unique(append(finding.Reasons, releaseReasons...))
			if system == "cargo" {
				c.checkCargoNode(ctx, project, node, &finding, &r.Details.Sources)
			}
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
		}
		if system == "pypi" {
			c.checkPyPINode(ctx, project, node, &finding, &r.Details.Sources)
		}
		finding.Decision = verdict(finding.Reasons, finding.Missing)
		if finding.Evidence != nil {
			finding.Evidence.Missing = finding.Missing
		}
		project.Findings = append(project.Findings, ProjectFinding{Path: node.Path, Package: system + ":" + node.Name, Finding: finding})
	}
	if system == "cargo" {
		finishCargoFeatures(project)
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
	if project.Features != nil {
		f.Missing = append(f.Missing, project.Features.Missing...)
		if len(project.Features.Missing) > 0 {
			f.Missing = append(f.Missing, "TRANSITIVE_EVIDENCE")
		}
	}
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
