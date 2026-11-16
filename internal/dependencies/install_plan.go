// purpose: Materialize only a fully approved npm plan against unchanged local inputs.
// responsibilities: Pin the selected runtime dependency and preserve the exact assessed artifact graph.
// architecture notes: This prepares bytes in memory; it neither executes tools nor edits project files.
package dependencies

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrInstallUnsupported = errors.New("installation capability unsupported")

type InstallArtifact struct {
	Node     ProjectNode
	Manifest npmPackage
}
type InstallPlan struct {
	Manifest  []byte
	Lockfile  []byte
	Snapshot  *ProjectSnapshot
	Artifacts []InstallArtifact
}

func PrepareInstall(dir string, r Report) (InstallPlan, error) {
	var plan InstallPlan
	if r.Decision != Allow || r.Scope != "project-resolution" || r.Recommendation == nil || len(r.Evidence.Missing) > 0 || r.Evidence.Identity != "verified" || r.Evidence.Maintainers != "multiple" || r.Details == nil || r.Details.Project == nil || !r.Details.Project.Complete {
		return plan, fmt.Errorf("%w: installation requires a fully approved project plan", ErrInstallUnsupported)
	}
	system, name, _ := strings.Cut(r.Package, ":")
	if system != "npm" || !npmName.MatchString(name) {
		return plan, fmt.Errorf("%w: installation ecosystem", ErrInstallUnsupported)
	}
	snapshot := ReadProject(dir, "npm")
	if !snapshot.Complete || snapshot.ManifestDigest != r.Details.Project.ManifestDigest || snapshot.LockDigest != r.Details.Project.LockDigest {
		return plan, fmt.Errorf("project changed after assessment")
	}
	if len(snapshot.Nodes) > maxAssessedNodes || len(r.Details.Project.Findings) != len(snapshot.Nodes) {
		return plan, fmt.Errorf("%w: incomplete artifact graph", ErrInstallUnsupported)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return plan, err
	}
	defer root.Close()
	m, err := projectFile(root, "package.json", 1<<20)
	if err != nil {
		return plan, err
	}
	l, err := projectFile(root, "package-lock.json", maxBody)
	if err != nil {
		return plan, err
	}
	var manifest map[string]json.RawMessage
	var lock map[string]json.RawMessage
	if uniqueJSON(m, &manifest) != nil || uniqueJSON(l, &lock) != nil {
		return plan, fmt.Errorf("invalid project JSON")
	}
	var format int
	if json.Unmarshal(lock["lockfileVersion"], &format) != nil || format != 3 {
		return plan, fmt.Errorf("%w: installation requires lockfile v3", ErrInstallUnsupported)
	}
	version := r.Recommendation.Version
	replaced := ""
	added := ""
	if !snapshot.binds(name, version) {
		var changePlan *ProjectPlan
		for i := range r.Details.Project.Plans {
			p := &r.Details.Project.Plans[i]
			if p.Name == name && p.Version == version && (p.Kind == "add-leaf" || p.Kind == "replace-leaf") {
				if changePlan != nil {
					return plan, fmt.Errorf("ambiguous installation plan")
				}
				changePlan = p
			}
		}
		if changePlan == nil || !npmArtifact(changePlan.Resolved, changePlan.Integrity) {
			return plan, fmt.Errorf("candidate has no supported installation plan")
		}
		release := &NPMRelease{}
		release.Name, release.Version = name, version
		release.Dist.Tarball, release.Dist.Integrity = changePlan.Resolved, changePlan.Integrity
		valid := false
		if changePlan.Kind == "replace-leaf" {
			valid = snapshot.planReplacement(release)
			if valid && snapshot.Plans[len(snapshot.Plans)-1].PreviousVersion == changePlan.PreviousVersion {
				replaced = "node_modules/" + name
			} else {
				valid = false
			}
		} else {
			valid = snapshot.planLeaf(release)
		}
		if !valid {
			return plan, fmt.Errorf("candidate plan no longer valid")
		}
		node := ProjectNode{Path: "node_modules/" + name, Name: name, Version: version, Resolved: changePlan.Resolved, Integrity: changePlan.Integrity}
		if replaced == "" {
			added = node.Path
			snapshot.Nodes = append(snapshot.Nodes, node)
		} else {
			for i := range snapshot.Nodes {
				if snapshot.Nodes[i].Path == replaced {
					snapshot.Nodes[i] = node
				}
			}
		}
		snapshot.packages[node.Path] = npmPackage{Version: version, Resolved: node.Resolved, Integrity: node.Integrity}
	}
	// Only the replaced node's historical verdict may be omitted. Every retained
	// artifact must have exactly one matching, fully approved assessment.
	for _, node := range snapshot.Nodes {
		if node.Path == replaced || node.Path == added {
			continue
		}
		matches := 0
		for _, assessed := range r.Details.Project.Findings {
			if assessed.Path != node.Path {
				continue
			}
			matches++
			if assessed.Package != "npm:"+node.Name || assessed.Finding.Version != node.Version || assessed.Finding.Decision != Allow || len(assessed.Finding.Missing) > 0 {
				return plan, fmt.Errorf("%w: unresolved transitive evidence", ErrInstallUnsupported)
			}
		}
		if matches != 1 {
			return plan, fmt.Errorf("%w: incomplete artifact assessment", ErrInstallUnsupported)
		}
	}
	var dependencies map[string]string
	if raw := manifest["dependencies"]; len(raw) > 0 {
		if json.Unmarshal(raw, &dependencies) != nil {
			return plan, fmt.Errorf("invalid runtime dependencies")
		}
	}
	if dependencies == nil {
		dependencies = map[string]string{}
	}
	dependencies[name] = version
	encoded, _ := json.Marshal(dependencies)
	manifest["dependencies"] = encoded
	var packages map[string]json.RawMessage
	if json.Unmarshal(lock["packages"], &packages) != nil {
		return plan, fmt.Errorf("invalid locked packages")
	}
	var lockedRoot map[string]json.RawMessage
	if json.Unmarshal(packages[""], &lockedRoot) != nil {
		return plan, fmt.Errorf("invalid lock root")
	}
	lockedRoot["dependencies"] = encoded
	packages[""], _ = json.Marshal(lockedRoot)
	for _, node := range snapshot.Nodes {
		if raw := bytes.TrimSpace(snapshot.packages[node.Path].Bin); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) && !bytes.Equal(raw, []byte("{}")) {
			return plan, fmt.Errorf("%w: executable bin packages", ErrInstallUnsupported)
		}
		if _, exists := packages[node.Path]; !exists || node.Path == replaced {
			packages[node.Path], _ = json.Marshal(map[string]string{"version": node.Version, "resolved": node.Resolved, "integrity": node.Integrity})
		}
		plan.Artifacts = append(plan.Artifacts, InstallArtifact{Node: node, Manifest: snapshot.packages[node.Path]})
	}
	lock["packages"], _ = json.Marshal(packages)
	plan.Manifest, _ = json.MarshalIndent(manifest, "", "  ")
	plan.Manifest = append(plan.Manifest, '\n')
	plan.Lockfile, _ = json.MarshalIndent(lock, "", "  ")
	plan.Lockfile = append(plan.Lockfile, '\n')
	plan.Snapshot = snapshot
	return plan, nil
}

func (p InstallPlan) Unchanged(dir string) bool {
	return p.Snapshot != nil && p.Snapshot.unchanged(dir)
}

func (p InstallPlan) VerifyInstalled(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, a := range p.Artifacts {
		data, err := projectFile(root, a.Node.Path+"/package.json", 1<<20)
		if err != nil {
			return fmt.Errorf("installed package manifest unavailable")
		}
		var manifest npmPackage
		if uniqueJSON(data, &manifest) != nil || manifest.Name != a.Node.Name || manifest.Version != a.Node.Version {
			return fmt.Errorf("installed package identity mismatch")
		}
		manifest.DevDependencies = nil
		expected := a.Manifest
		expected.DevDependencies = nil
		if !sameDeps(manifest, expected) {
			return fmt.Errorf("installed package metadata mismatch")
		}
	}
	return nil
}
