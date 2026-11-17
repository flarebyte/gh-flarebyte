// purpose: Bind crate archive verification to an unchanged, fully approved Cargo assessment.
// responsibilities: Seal exact locked identities and freeze the corroborated registry manifest facts.
// architecture notes: Preparation is read-only and never authorizes native installation on its own.
package dependencies

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

type CargoArtifact struct {
	Node     ProjectNode
	expected *cargoArchiveExpectation
}
type cargoArchiveExpectation struct {
	node        ProjectNode
	metadata    *cargoFeaturePackage
	maintenance *cargoMaintenanceProof
	policy      Policy
	checkedAt   time.Time
}

func PrepareCargoArtifacts(dir string, r Report) ([]CargoArtifact, error) {
	unsupported := func() ([]CargoArtifact, error) {
		return nil, fmt.Errorf("%w: crate verification requires an approved unchanged Cargo graph", ErrInstallUnsupported)
	}
	if r.Decision != Allow || r.Scope != "project-resolution" || r.Recommendation == nil || r.Evidence.Identity != "verified" || r.Evidence.Maintainers != "multiple" || len(r.Evidence.Missing) > 0 || r.Details == nil || r.Details.Project == nil {
		return unsupported()
	}
	system, name, _ := strings.Cut(r.Package, ":")
	approved := r.Details.Project
	if system != "cargo" || !approved.Complete || approved.cargo == nil || approved.Features == nil || approved.Features.Status != "resolved" || len(approved.Features.Missing) > 0 {
		return unsupported()
	}
	current := ReadProject(dir, "cargo")
	if !current.Complete || current.ManifestDigest != approved.ManifestDigest || current.LockDigest != approved.LockDigest {
		return nil, fmt.Errorf("Cargo project changed after assessment")
	}
	if !current.binds(name, r.Recommendation.Version) || len(current.Nodes) > maxAssessedNodes || len(approved.Findings) != len(current.Nodes) {
		return unsupported()
	}
	artifacts := []CargoArtifact{}
	for _, node := range current.Nodes {
		matches := 0
		for _, f := range approved.Findings {
			if f.Path == node.Path {
				matches++
				if f.Package != "cargo:"+node.Name || f.Finding.Version != node.Version || f.Finding.Decision != Allow || len(f.Finding.Missing) > 0 || f.Finding.Evidence == nil || len(f.Finding.Evidence.Missing) > 0 || f.Finding.Evidence.Identity != "verified" || f.Finding.Evidence.Maintainers != "multiple" {
					return unsupported()
				}
			}
		}
		metadata := approved.cargo.featurePackages[node.Path]
		var maintenance *cargoMaintenanceProof
		for _, f := range approved.Findings {
			if f.Path == node.Path {
				if !validCargoMaintenanceFinding(f.Finding, node.Name, r) {
					return unsupported()
				}
				maintenance = f.Finding.cargoMaintenance
				if node.Name == name && node.Version == r.Recommendation.Version && !reflect.DeepEqual(r.Evidence.Maintenance, f.Finding.Evidence.Maintenance) {
					return unsupported()
				}
			}
		}
		locked := approved.cargo.packages[node.Path]
		if matches != 1 || metadata == nil || !metadata.complete || metadata.features == nil || locked.Source != cargoRegistry || locked.Name != node.Name || locked.Version != node.Version || node.Integrity != "sha256-"+locked.Checksum || !cargoArtifactIdentity(node) {
			return unsupported()
		}
		frozen := &cargoFeaturePackage{complete: true, rustVersion: metadata.rustVersion, edition: metadata.edition, features: map[string][]string{}}
		for key, values := range metadata.features {
			frozen.features[key] = append([]string{}, values...)
		}
		for _, d := range metadata.dependencies {
			d.features = append([]string{}, d.features...)
			frozen.dependencies = append(frozen.dependencies, d)
		}
		if _, ok, _ := normaliseCargoFeatures(frozen); !ok {
			return unsupported()
		}
		policy := r.Details.Policy
		policy.ScorecardChecks = slices.Clone(policy.ScorecardChecks)
		artifacts = append(artifacts, CargoArtifact{Node: node, expected: &cargoArchiveExpectation{node: node, metadata: frozen, maintenance: maintenance.clone(), policy: policy, checkedAt: r.CheckedAt}})
	}
	return artifacts, nil
}

func cargoArtifactIdentity(node ProjectNode) bool {
	if !cargoName.MatchString(node.Name) || !strings.HasPrefix(node.Integrity, "sha256-") || !cargoChecksum.MatchString(strings.TrimPrefix(node.Integrity, "sha256-")) {
		return false
	}
	if _, ok := stableVersion("cargo", node.Version); !ok {
		return false
	}
	return node.Resolved == "https://static.crates.io/crates/"+node.Name+"/"+node.Name+"-"+node.Version+".crate"
}

func (a CargoArtifact) valid() bool {
	if a.expected == nil {
		return false
	}
	if p := a.expected.maintenance; p != nil {
		if _, ok := p.evidence(a.expected.policy, a.expected.checkedAt); !ok || p.name != a.Node.Name {
			return false
		}
	}
	return a.expected.metadata != nil && a.expected.metadata.complete && a.expected.metadata.features != nil && a.Node == a.expected.node && cargoArtifactIdentity(a.Node)
}
