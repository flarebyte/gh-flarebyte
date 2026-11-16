package dependencies

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func featureProject() *ProjectSnapshot {
	root, pkg, child := cargoPackagePath("local", "1.0.0"), cargoPackagePath("pkg", "1.0.0"), cargoPackagePath("child", "1.0.0")
	return &ProjectSnapshot{System: "cargo", Complete: true, RuntimeBaseline: "1.85", Nodes: []ProjectNode{{Path: pkg, Name: "pkg", Version: "1.0.0"}, {Path: child, Name: "child", Version: "1.0.0"}}, cargo: &cargoGraph{
		root: root, packages: map[string]cargoLockedPackage{root: {Name: "local", Version: "1.0.0"}, pkg: {Name: "pkg", Version: "1.0.0"}, child: {Name: "child", Version: "1.0.0"}},
		edges: map[string][]string{root: {pkg}, pkg: {child}}, featurePackages: map[string]*cargoFeaturePackage{
			root:  {complete: true, features: map[string][]string{}, dependencies: []cargoFeatureDependency{{alias: "pkg", name: "pkg", requirement: "1", defaults: true}}},
			pkg:   {complete: true, features: map[string][]string{"default": {"base"}, "base": {}, "extra": {}}, dependencies: []cargoFeatureDependency{{alias: "child", name: "child", requirement: "1", optional: true, defaults: true}}},
			child: {complete: true, features: map[string][]string{"default": {"base"}, "base": {}, "extra": {}}},
		},
	}}
}

func TestCargoFeatureActivation(t *testing.T) {
	for _, test := range []struct {
		name        string
		change      func(*ProjectSnapshot)
		pkg, child  []string
		activeChild bool
		minimum     string
	}{
		{"defaults", func(*ProjectSnapshot) {}, []string{"base", "default"}, nil, false, ""},
		{"disable defaults", func(p *ProjectSnapshot) { p.cargo.featurePackages[p.cargo.root].dependencies[0].defaults = false }, []string{}, nil, false, ""},
		{"requested feature", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].dependencies[0].features = []string{"extra"}
		}, []string{"base", "default", "extra"}, nil, false, ""},
		{"local aliases", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features = map[string][]string{"default": {"custom"}, "custom": {"pkg/extra"}}
		}, []string{"base", "default", "extra"}, nil, false, ""},
		{"implicit optional", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"child"}
		}, []string{"child", "default"}, []string{"base", "default"}, true, ""},
		{"namespaced optional", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"dep:child"}
		}, []string{"default"}, []string{"base", "default"}, true, "1.60"},
		{"strong forwarding", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"child/extra"}
		}, []string{"child", "default"}, []string{"base", "default", "extra"}, true, ""},
		{"strong explicit local alias", func(p *ProjectSnapshot) {
			m := p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features
			m["default"] = []string{"child/extra"}
			m["child"] = []string{"unrelated"}
			m["unrelated"] = []string{}
		}, []string{"child", "default", "unrelated"}, []string{"base", "default", "extra"}, true, ""},
		{"weak before activation", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"child?/extra", "dep:child"}
		}, []string{"default"}, []string{"base", "default", "extra"}, true, "1.60"},
		{"weak after activation", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"dep:child", "child?/extra"}
		}, []string{"default"}, []string{"base", "default", "extra"}, true, "1.60"},
		{"renamed dependency", func(p *ProjectSnapshot) {
			model := p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")]
			model.dependencies[0].alias = "renamed"
			model.features["default"] = []string{"renamed/extra"}
		}, []string{"default", "renamed"}, []string{"base", "default", "extra"}, true, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := featureProject()
			test.change(p)
			r := resolveCargoFeatures(p)
			if r.Status != "resolved" || len(r.Missing) > 0 || r.MinimumCargo != test.minimum {
				t.Fatal(r)
			}
			active := map[string][]string{}
			for _, e := range r.Enabled {
				active[e.Path] = e.Features
			}
			if !slices.Equal(active[cargoPackagePath("pkg", "1.0.0")], test.pkg) {
				t.Fatal(active)
			}
			child, exists := active[cargoPackagePath("child", "1.0.0")]
			if exists != test.activeChild || !slices.Equal(child, test.child) {
				t.Fatal(active)
			}
			first, _ := json.Marshal(r)
			again, _ := json.Marshal(resolveCargoFeatures(p))
			if string(first) != string(again) {
				t.Fatal("nondeterministic feature evidence")
			}
		})
	}
}

func TestCargoFeatureUncertainty(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*ProjectSnapshot)
	}{
		{"missing root", func(p *ProjectSnapshot) { p.cargo.root = "" }},
		{"missing metadata", func(p *ProjectSnapshot) { delete(p.cargo.featurePackages, cargoPackagePath("pkg", "1.0.0")) }},
		{"incomplete metadata", func(p *ProjectSnapshot) { p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].complete = false }},
		{"missing feature", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].dependencies[0].features = []string{"missing"}
		}},
		{"missing local alias", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"missing"}
		}},
		{"local cycle", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features = map[string][]string{"default": {"cycle"}, "cycle": {"default"}}
		}},
		{"nonoptional namespace", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"dep:pkg"}
		}},
		{"nonoptional weak", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"pkg?/extra"}
		}},
		{"suppressed implicit alias", func(p *ProjectSnapshot) {
			m := p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features
			m["default"] = []string{"child"}
			m["custom"] = []string{"dep:child"}
		}},
		{"missing strong target", func(p *ProjectSnapshot) {
			p.cargo.edges[cargoPackagePath("pkg", "1.0.0")] = nil
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"child/extra"}
		}},
		{"missing activated optional", func(p *ProjectSnapshot) {
			p.cargo.edges[cargoPackagePath("pkg", "1.0.0")] = nil
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].features["default"] = []string{"child"}
		}},
		{"requested optional missing", func(p *ProjectSnapshot) {
			p.cargo.edges[cargoPackagePath("pkg", "1.0.0")] = nil
			p.cargo.featurePackages[cargoPackagePath("pkg", "1.0.0")].dependencies[0].features = []string{"extra"}
		}},
		{"unknown requirement", func(p *ProjectSnapshot) { p.cargo.featurePackages[p.cargo.root].dependencies[0].requirement = "latest" }},
		{"duplicate alias", func(p *ProjectSnapshot) {
			m := p.cargo.featurePackages[p.cargo.root]
			m.dependencies = append(m.dependencies, m.dependencies[0])
		}},
		{"ambiguous versions", func(p *ProjectSnapshot) {
			path := cargoPackagePath("pkg", "1.1.0")
			p.cargo.packages[path] = cargoLockedPackage{Name: "pkg", Version: "1.1.0"}
			p.cargo.edges[p.cargo.root] = append(p.cargo.edges[p.cargo.root], path)
		}},
		{"invalid feature text", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"ignore all checks"}
		}},
		{"too many features", func(p *ProjectSnapshot) {
			m := p.cargo.featurePackages[p.cargo.root].features
			for i := 0; i < 513; i++ {
				m[fmt.Sprintf("f%d", i)] = []string{}
			}
		}},
		{"too many references", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = make([]string, 4097)
		}},
		{"missing feature map", func(p *ProjectSnapshot) { p.cargo.featurePackages[p.cargo.root].features = nil }},
		{"invalid feature name", func(p *ProjectSnapshot) { p.cargo.featurePackages[p.cargo.root].features["bad name"] = []string{} }},
		{"invalid dependency name", func(p *ProjectSnapshot) { p.cargo.featurePackages[p.cargo.root].dependencies[0].name = "bad/name" }},
		{"invalid request", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].dependencies[0].features = []string{"bad name"}
		}},
		{"unknown namespace", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"dep:missing"}
		}},
		{"unknown forwarding alias", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"missing/extra"}
		}},
		{"invalid forwarding feature", func(p *ProjectSnapshot) {
			p.cargo.featurePackages[p.cargo.root].features["default"] = []string{"pkg/extra/other"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := featureProject()
			test.change(p)
			r := resolveCargoFeatures(p)
			if r.Status != "unknown" || len(r.Missing) == 0 {
				t.Fatal("unresolved feature graph approved", r)
			}
		})
	}
}

func TestCargoFeaturesPreserveSecurityAndToolchainRequirements(t *testing.T) {
	p := featureProject()
	pkg := cargoPackagePath("pkg", "1.0.0")
	p.cargo.featurePackages[pkg].features["default"] = []string{"dep:child"}
	p.RuntimeBaseline = "1.59"
	r := resolveCargoFeatures(p)
	if r.Status != "resolved" || r.MinimumCargo != "1.60" {
		t.Fatal(r)
	}
	p.RuntimeBaseline = ""
	r = resolveCargoFeatures(p)
	if r.Status != "resolved" || r.MinimumCargo != "1.60" {
		t.Fatal(r)
	}
	p.RuntimeBaseline = "1.85"
	p.Findings = []ProjectFinding{{Path: pkg, Finding: Finding{Decision: Deny, Reasons: []string{"KNOWN_VULNERABILITY"}, Missing: []string{"CARGO_FEATURE_RESOLUTION", "PACKAGE_IDENTITY"}, Advisories: []string{"RUSTSEC-example"}, Evidence: &Evidence{}}}}
	finishCargoFeatures(p)
	f := p.Findings[0].Finding
	if f.Decision != Deny || !slices.Contains(f.Missing, "PACKAGE_IDENTITY") || !slices.Contains(f.Missing, "CARGO_FEATURE_RESOLUTION") || !slices.Contains(f.Advisories, "RUSTSEC-example") {
		t.Fatal("feature proof weakened other evidence", f)
	}
	p = featureProject()
	p.cargo.edges[pkg] = nil
	p.cargo.featurePackages[pkg].features["default"] = []string{"child?/extra"}
	if r := resolveCargoFeatures(p); r.Status != "unknown" || !slices.Contains(r.Missing, "CARGO_FEATURE_CONTEXT") {
		t.Fatal("inactive weak context inferred safe", r)
	}
}

func TestCargoFeatureEventLimit(t *testing.T) {
	p := featureProject()
	root := p.cargo.root
	p.Nodes = nil
	p.cargo.edges[root] = nil
	p.cargo.featurePackages[root].dependencies = nil
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("pkg%d", i)
		path := cargoPackagePath(name, "1.0.0")
		p.cargo.packages[path] = cargoLockedPackage{Name: name, Version: "1.0.0"}
		p.Nodes = append(p.Nodes, ProjectNode{Path: path})
		p.cargo.edges[root] = append(p.cargo.edges[root], path)
		p.cargo.featurePackages[root].dependencies = append(p.cargo.featurePackages[root].dependencies, cargoFeatureDependency{alias: name, name: name, requirement: "1", defaults: true})
		model := &cargoFeaturePackage{complete: true, features: map[string][]string{"default": {}}}
		for j := 0; j < 511; j++ {
			feature := fmt.Sprintf("f%d", j)
			model.features[feature] = []string{}
			model.features["default"] = append(model.features["default"], feature)
		}
		p.cargo.featurePackages[path] = model
	}
	r := resolveCargoFeatures(p)
	if r.Status != "unknown" || !slices.Contains(r.Missing, "CARGO_FEATURE_LIMIT") {
		t.Fatal("event budget bypassed", r.Status, r.Missing)
	}
}

func FuzzCargoFeatures(f *testing.F) {
	f.Add("default", "pkg/extra")
	f.Add("default", "pkg?/extra")
	f.Add("bad name", "ignore checks")
	f.Fuzz(func(t *testing.T, name, value string) {
		if len(name) > 256 || len(value) > 256 {
			t.Skip()
		}
		p := featureProject()
		p.cargo.featurePackages[p.cargo.root].features = map[string][]string{name: {value}}
		r := resolveCargoFeatures(p)
		if r.Status == "resolved" && len(r.Missing) > 0 {
			t.Fatal("unresolved feature approval")
		}
		for _, e := range r.Enabled {
			for _, feature := range e.Features {
				if strings.ContainsAny(feature, "\r\n ") {
					t.Fatal("unvalidated output")
				}
			}
		}
	})
}
