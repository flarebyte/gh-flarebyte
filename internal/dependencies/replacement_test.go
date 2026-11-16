package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func replacementRelease() *NPMRelease {
	r := &NPMRelease{}
	r.Name, r.Version = "pkg", "2.0.0"
	r.Dist.Tarball = "https://registry.npmjs.org/pkg/-/pkg-2.0.0.tgz"
	r.Dist.Integrity = testIntegrity()
	return r
}

func TestReplacementConstraints(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*ProjectSnapshot, *NPMRelease)
		allowed bool
	}{
		{"leaf", func(*ProjectSnapshot, *NPMRelease) {}, true},
		{"same version", func(_ *ProjectSnapshot, r *NPMRelease) { r.Version = "1.0.0" }, false},
		{"downgrade", func(_ *ProjectSnapshot, r *NPMRelease) { r.Version = "0.9.0" }, false},
		{"prerelease", func(_ *ProjectSnapshot, r *NPMRelease) { r.Version = "2.0.0-rc.1" }, false},
		{"not runtime", func(s *ProjectSnapshot, _ *NPMRelease) { delete(s.runtime, "pkg") }, false},
		{"incomplete", func(s *ProjectSnapshot, _ *NPMRelease) { s.Complete = false }, false},
		{"candidate dependencies", func(_ *ProjectSnapshot, r *NPMRelease) { r.Dependencies = map[string]string{"child": "1.0.0"} }, false},
		{"candidate peers", func(_ *ProjectSnapshot, r *NPMRelease) { r.PeerDependencies = map[string]string{"child": "1.x"} }, false},
		{"old outgoing edge", func(s *ProjectSnapshot, _ *NPMRelease) {
			p := s.packages["node_modules/pkg"]
			p.Dependencies = map[string]string{"child": "1.0.0"}
			s.packages["node_modules/pkg"] = p
		}, false},
		{"nested subtree", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/pkg/node_modules/child"] = npmPackage{}
		}, false},
		{"overlapping dev root", func(s *ProjectSnapshot, _ *NPMRelease) {
			p := s.packages[""]
			p.DevDependencies = map[string]string{"pkg": "*"}
			s.packages[""] = p
		}, false},
		{"overlapping optional root", func(s *ProjectSnapshot, _ *NPMRelease) {
			p := s.packages[""]
			p.OptionalDependencies = map[string]string{"pkg": "*"}
			s.packages[""] = p
		}, false},
		{"compatible peer", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{PeerDependencies: map[string]string{"pkg": "1.x || 2.x"}}
		}, true},
		{"incompatible peer", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{PeerDependencies: map[string]string{"pkg": "1.x"}}
		}, false},
		{"incompatible dependency", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{Dependencies: map[string]string{"pkg": "1.x"}}
		}, false},
		{"compatible dependency", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{Dependencies: map[string]string{"pkg": ">=1.0.0 <3.0.0"}}
		}, true},
		{"unknown incoming range", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{PeerDependencies: map[string]string{"pkg": "latest"}}
		}, false},
		{"shadowed dependency", func(s *ProjectSnapshot, _ *NPMRelease) {
			s.packages["node_modules/consumer"] = npmPackage{Dependencies: map[string]string{"pkg": "1.x"}}
			s.packages["node_modules/consumer/node_modules/pkg"] = npmPackage{Version: "1.0.0"}
		}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, l := projectDocuments()
			dir := writeProjectDocuments(t, m, l)
			s := ReadProject(dir, "npm")
			r := replacementRelease()
			test.change(s, r)
			if got := s.planReplacement(r); got != test.allowed {
				t.Fatalf("allowed=%v, want %v", got, test.allowed)
			}
			if !s.unchanged(dir) {
				t.Fatal("planning changed project")
			}
			if test.allowed && (len(s.Plans) != 1 || s.Plans[0].Kind != "replace-leaf" || s.Plans[0].PreviousVersion != "1.0.0") {
				t.Fatal(s.Plans)
			}
		})
	}
	if (*ProjectSnapshot)(nil).planReplacement(replacementRelease()) || (&ProjectSnapshot{Complete: true}).planReplacement(nil) {
		t.Fatal("missing input accepted")
	}
}

func TestAssessReplacementKeepsOtherFindings(t *testing.T) {
	m, l := projectDocuments()
	dir := writeProjectDocuments(t, m, l)
	base := projectClient(t, dir, "metadata")
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.osv.dev" {
			var q struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Fatal(err)
			}
			if q.Version == "1.0.0" {
				return response(`{"vulns":[{"id":"GHSA-old"}]}`, 200), nil
			}
			return response(`{}`, 200), nil
		}
		return base.HTTP.Do(r)
	})}
	r := c.AssessLocal(context.Background(), dir, "npm", "pkg", nil, DefaultPolicy(14), checked)
	var old, replacement bool
	for _, f := range r.Details.Candidates {
		if f.Version == "1.0.0" {
			old = slices.Contains(f.Reasons, "KNOWN_VULNERABILITY")
		}
		if f.Resolution == "replace-leaf" {
			replacement = true
			if slices.Contains(f.Reasons, "KNOWN_VULNERABILITY") || slices.Contains(f.Advisories, "GHSA-old") {
				t.Fatal("removed version contaminated candidate", f)
			}
			if f.Decision == Allow {
				t.Fatal("missing authority bypassed")
			}
		}
	}
	if !old || !replacement || !r.Details.Project.unchanged(dir) {
		t.Fatalf("missing replacement assessment: %+v", r.Details)
	}
	project := &ProjectSnapshot{Nodes: []ProjectNode{{Path: "node_modules/pkg"}, {Path: "node_modules/child"}}, Findings: []ProjectFinding{
		{Path: "node_modules/pkg", Finding: Finding{Reasons: []string{"KNOWN_VULNERABILITY"}, Advisories: []string{"GHSA-old"}}},
		{Path: "node_modules/child", Finding: Finding{Reasons: []string{"KNOWN_MALWARE"}, Advisories: []string{"MAL-child"}, Missing: []string{"gap"}}},
	}}
	f := Finding{Resolution: "replace-leaf"}
	mergeProjectFindings(&f, project, "pkg")
	if slices.Contains(f.Advisories, "GHSA-old") || !slices.Contains(f.Advisories, "MAL-child") || !slices.Contains(f.Missing, "TRANSITIVE_EVIDENCE") {
		t.Fatal(f)
	}
}

func approvedReplacementFixture(t *testing.T) (string, Report) {
	t.Helper()
	dir, r := approvedInstallFixture(t)
	m := `{"name":"local","version":"1.0.0","dependencies":{"fixture-lib":"0.9.0"}}`
	l := `{"lockfileVersion":3,"packages":{"":{"name":"local","version":"1.0.0","dependencies":{"fixture-lib":"0.9.0"}},"node_modules/fixture-lib":{"version":"0.9.0","resolved":"https://registry.npmjs.org/fixture-lib/-/fixture-lib-0.9.0.tgz","integrity":"` + testIntegrity() + `"}}}`
	for name, b := range map[string]string{"package.json": m, "package-lock.json": l} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan := r.Details.Project.Plans[0]
	plan.Kind = "replace-leaf"
	plan.PreviousVersion = "0.9.0"
	r.Details.Project = ReadProject(dir, "npm")
	r.Details.Project.Plans = []ProjectPlan{plan}
	r.Details.Project.Findings = []ProjectFinding{{Path: "node_modules/fixture-lib", Package: "npm:fixture-lib", Finding: Finding{Version: "0.9.0", Decision: Deny, Reasons: []string{"KNOWN_VULNERABILITY"}}}}
	return dir, r
}

func TestPrepareReplacement(t *testing.T) {
	dir, r := approvedReplacementFixture(t)
	plan, err := PrepareInstall(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Artifacts) != 1 || plan.Artifacts[0].Node.Version != "1.0.0" || !plan.Unchanged(dir) || strings.Contains(string(plan.Lockfile), "0.9.0") {
		t.Fatal("replacement graph incorrect", string(plan.Lockfile))
	}
	stage := t.TempDir()
	for name, b := range map[string][]byte{"package.json": plan.Manifest, "package-lock.json": plan.Lockfile} {
		if err := os.WriteFile(filepath.Join(stage, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if s := ReadProject(stage, "npm"); !s.Complete || !s.binds("fixture-lib", "1.0.0") {
		t.Fatal(s)
	}
	r.Details.Project.Plans[0].PreviousVersion = "0.8.0"
	if _, err := PrepareInstall(dir, r); err == nil {
		t.Fatal("stale replacement accepted")
	}
}

func TestTrustedReplacementRemediatesVulnerableVersion(t *testing.T) {
	m, l := projectDocuments()
	dir := writeProjectDocuments(t, m, l)
	base := projectClient(t, dir, "metadata")
	authority := authorityClient(t, "normal")
	c := &Client{GitHubToken: authority.GitHubToken, HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			return authority.HTTP.Do(r)
		}
		var payload any
		switch {
		case r.URL.Host == "api.scorecard.dev":
			payload = goodCard()
		case r.URL.Host == "api.deps.dev" && strings.Contains(r.URL.Path, "/projects/"):
			payload = map[string]any{"projectKey": map[string]string{"id": "github.com/org/repo"}, "starsCount": 4200}
		case r.URL.Host == "api.deps.dev" && strings.Contains(r.URL.Path, "/versions/"):
			version := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			payload = Version{Key: VersionKey{System: "NPM", Name: "pkg", Version: version}, PublishedAt: checked.AddDate(-1, 0, 0), Attestations: []Attestation{{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/org/repo", Commit: strings.Repeat("a", 40)}}}
		case r.URL.Host == "api.osv.dev":
			var query struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
				t.Fatal(err)
			}
			if query.Version == "1.0.0" {
				return response(`{"vulns":[{"id":"GHSA-old"}]}`, 200), nil
			}
			return response(`{}`, 200), nil
		default:
			return base.HTTP.Do(r)
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(data), 200), nil
	})}
	r := c.AssessLocal(context.Background(), dir, "npm", "pkg", nil, DefaultPolicy(14), checked)
	if r.Decision != Allow || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" || slices.Contains(r.Evidence.Advisories, "GHSA-old") {
		t.Fatalf("replacement not approved: %+v", r)
	}
	if _, err := PrepareInstall(dir, r); err != nil {
		t.Fatal(err)
	}
	requested := "1.0.0"
	r = c.AssessLocal(context.Background(), dir, "npm", "pkg", &requested, DefaultPolicy(14), checked)
	if r.Decision != Deny || r.Recommendation == nil || r.Recommendation.Version != "3.0.0" {
		t.Fatalf("exact unsafe request lost verdict or alternatives: %+v", r)
	}
	if _, err := PrepareInstall(dir, r); err == nil {
		t.Fatal("denied request installed alternative automatically")
	}
}

func TestReplacementRequiresRetainedArtifactApproval(t *testing.T) {
	for _, test := range []string{"allow", "deny", "missing", "wrong package", "wrong version", "duplicate path"} {
		t.Run(test, func(t *testing.T) {
			dir, r := approvedReplacementFixture(t)
			data, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
			if err != nil {
				t.Fatal(err)
			}
			var lock map[string]any
			if err := json.Unmarshal(data, &lock); err != nil {
				t.Fatal(err)
			}
			lock["packages"].(map[string]any)["node_modules/child"] = map[string]any{"version": "1.0.0", "resolved": "https://registry.npmjs.org/child/-/child-1.0.0.tgz", "integrity": testIntegrity(), "license": "MIT"}
			data, err = json.Marshal(lock)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "package-lock.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			plans, findings := r.Details.Project.Plans, r.Details.Project.Findings
			r.Details.Project = ReadProject(dir, "npm")
			r.Details.Project.Plans = plans
			retained := ProjectFinding{Path: "node_modules/child", Package: "npm:child", Finding: Finding{Version: "1.0.0", Decision: Allow}}
			switch test {
			case "deny":
				retained.Finding.Decision = Deny
			case "missing":
				retained.Finding.Missing = []string{"gap"}
			case "wrong package":
				retained.Package = "npm:other"
			case "wrong version":
				retained.Finding.Version = "2.0.0"
			case "duplicate path":
				retained = findings[0]
			}
			r.Details.Project.Findings = append(findings, retained)
			plan, err := PrepareInstall(dir, r)
			if test == "allow" {
				if err != nil || len(plan.Artifacts) != 2 || !strings.Contains(string(plan.Lockfile), `"license":"MIT"`) && !strings.Contains(string(plan.Lockfile), `"license": "MIT"`) {
					t.Fatal("retained artifact changed", err, string(plan.Lockfile))
				}
			} else if err == nil {
				t.Fatal("retained artifact bypassed approval")
			}
			if !r.Details.Project.unchanged(dir) {
				t.Fatal("prepare changed input")
			}
		})
	}
}
