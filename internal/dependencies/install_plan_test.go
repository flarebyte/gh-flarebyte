package dependencies

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func approvedInstallFixture(t *testing.T) (string, Report) {
	t.Helper()
	_, a := fixtureArtifact(t)
	m, l := projectDocuments()
	delete(m, "dependencies")
	packages := l["packages"].(map[string]any)
	delete(packages[""].(map[string]any), "dependencies")
	delete(packages, "node_modules/pkg")
	dir := writeProjectDocuments(t, m, l)
	r := NewReport("npm", a.Node.Name, nil, DefaultPolicy(14), checked)
	r.Decision = Allow
	r.Scope = "project-resolution"
	r.Evidence.Identity = "verified"
	r.Evidence.Maintainers = "multiple"
	r.Recommendation = &Candidate{Version: a.Node.Version}
	r.Details.Project = ReadProject(dir, "npm")
	r.Details.Project.Plans = []ProjectPlan{{Name: a.Node.Name, Version: a.Node.Version, Kind: "add-leaf", Resolved: a.Node.Resolved, Integrity: a.Node.Integrity}}
	return dir, r
}

func TestPrepareApprovedInstall(t *testing.T) {
	dir, r := approvedInstallFixture(t)
	plan, err := PrepareInstall(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unchanged(dir) || len(plan.Artifacts) != 1 {
		t.Fatal(plan)
	}
	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(plan.Manifest, &manifest); err != nil || manifest.Dependencies["fixture-lib"] != "1.0.0" {
		t.Fatal(manifest, err)
	}
	stage := t.TempDir()
	for name, b := range map[string][]byte{"package.json": plan.Manifest, "package-lock.json": plan.Lockfile} {
		if err := os.WriteFile(filepath.Join(stage, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := ReadProject(stage, "npm")
	if !snapshot.Complete || !snapshot.binds("fixture-lib", "1.0.0") {
		t.Fatalf("invalid prepared graph %+v", snapshot)
	}
	if err := plan.VerifyInstalled(stage); err == nil {
		t.Fatal("missing install accepted")
	}
	module := filepath.Join(stage, "node_modules", "fixture-lib")
	if err := os.MkdirAll(module, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "package.json"), []byte(`{"name":"fixture-lib","version":"1.0.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := plan.VerifyInstalled(stage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "package.json"), []byte(`{"name":"fixture-lib","version":"2.0.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := plan.VerifyInstalled(stage); err == nil {
		t.Fatal("wrong installed version")
	}
	if !plan.Unchanged(dir) {
		t.Fatal("prepare mutated source")
	}
}

func TestInstallPlanRequiresApproval(t *testing.T) {
	for _, change := range []func(*Report){
		func(r *Report) { r.Decision = Unknown }, func(r *Report) { r.Scope = "package" }, func(r *Report) { r.Recommendation = nil }, func(r *Report) { r.Evidence.Missing = []string{"gap"} },
		func(r *Report) { r.Evidence.Identity = "unverified" }, func(r *Report) { r.Evidence.Maintainers = "unknown" }, func(r *Report) { r.Details = nil }, func(r *Report) { r.Details.Project.Complete = false },
		func(r *Report) { r.Package = "cargo:fixture-lib" }, func(r *Report) { r.Details.Project.ManifestDigest = "changed" }, func(r *Report) { r.Details.Project.Plans = nil },
		func(r *Report) { r.Details.Project.Plans = append(r.Details.Project.Plans, r.Details.Project.Plans[0]) },
	} {
		dir, r := approvedInstallFixture(t)
		change(&r)
		if _, err := PrepareInstall(dir, r); err == nil {
			t.Fatal("unsafe plan accepted")
		}
	}
}
