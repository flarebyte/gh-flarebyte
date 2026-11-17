package dependencies

import (
	"archive/tar"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestCrateManifestAgreement(t *testing.T) {
	for _, test := range []struct {
		name, manifest      string
		expected            func(*cargoFeaturePackage)
		extra               []tar.Header
		body                []string
		valid, build, macro bool
	}{
		{"build and macro", "[package]\nname='pkg'\nversion='1.0.0'\nbuild='build.rs'\n[lib]\npath='src/lib.rs'\nproc-macro=true", nil, []tar.Header{{Name: "pkg-1.0.0/build.rs"}}, []string{"fn main() {}"}, true, true, true},
		{"automatic build", simpleCrateManifest, nil, []tar.Header{{Name: "pkg-1.0.0/build.rs"}}, []string{"fn main() {}"}, true, true, false},
		{"disabled build", "[package]\nname='pkg'\nversion='1.0.0'\nbuild=false\n", nil, []tar.Header{{Name: "pkg-1.0.0/build.rs"}}, []string{"fn main() {}"}, true, false, false},
		{"canonical runtime", "[package]\nname='pkg'\nversion='1.0.0'\nrust-version='1.85.0'\nedition='2021'", func(p *cargoFeaturePackage) { p.rustVersion = "1.85"; p.edition = "2021" }, nil, nil, true, false, false},
		{"feature ordering", simpleCrateManifest + "[features]\ndefault=['a','b']\na=[]\nb=[]\n", func(p *cargoFeaturePackage) {
			p.features = map[string][]string{"default": {"b", "a"}, "a": {}, "b": {}}
		}, nil, nil, true, false, false},
		{"caret spelling", simpleCrateManifest + "[dependencies]\nchild='1.0.0'\n", func(p *cargoFeaturePackage) {
			p.dependencies = []cargoFeatureDependency{{name: "child", alias: "child", kind: "normal", requirement: "^1.0.0", defaults: true}}
		}, nil, nil, true, false, false},
		{"inert lock and original", simpleCrateManifest, nil, []tar.Header{{Name: "pkg-1.0.0/Cargo.lock"}, {Name: "pkg-1.0.0/Cargo.toml.orig"}}, []string{"untrusted original resolution", "[workspace]\n"}, true, false, false},
		{"wrong identity", strings.Replace(simpleCrateManifest, "name='pkg'", "name='other'", 1), nil, nil, nil, false, false, false},
		{"invalid toml", "[package", nil, nil, nil, false, false, false},
		{"missing package", "[lib]\npath='src/lib.rs'", nil, nil, nil, false, false, false},
		{"workspace", simpleCrateManifest + "[workspace]\n", nil, nil, nil, false, false, false},
		{"workspace inheritance", "[package]\nname='pkg'\nversion='1.0.0'\nworkspace='..'", nil, nil, nil, false, false, false},
		{"path dependency", simpleCrateManifest + "[dependencies]\nchild={path='../child',version='1'}", nil, nil, nil, false, false, false},
		{"hidden dependency", simpleCrateManifest + "[dependencies]\nchild='1'", nil, nil, nil, false, false, false},
		{"unknown requirement", simpleCrateManifest + "[dependencies]\nchild='latest'", func(p *cargoFeaturePackage) {
			p.dependencies = []cargoFeatureDependency{{alias: "child", name: "child", kind: "normal", requirement: "^1.0.0", defaults: true}}
		}, nil, nil, false, false, false},
		{"unknown feature alias", simpleCrateManifest + "[features]\ndefault=['missing']", nil, nil, nil, false, false, false},
		{"changed dependency kind", simpleCrateManifest + "[build-dependencies]\nchild='1.0.0'", func(p *cargoFeaturePackage) {
			p.dependencies = []cargoFeatureDependency{{name: "child", alias: "child", kind: "normal", requirement: "^1.0.0", defaults: true}}
		}, nil, nil, false, false, false},
		{"changed target condition", simpleCrateManifest + "[dependencies]\nchild='1.0.0'", func(p *cargoFeaturePackage) {
			p.dependencies = []cargoFeatureDependency{{alias: "child", name: "child", kind: "normal", target: "cfg(windows)", requirement: "^1.0.0", defaults: true}}
		}, nil, nil, false, false, false},
		{"hidden feature", simpleCrateManifest + "[features]\nextra=[]", nil, nil, nil, false, false, false},
		{"bad feature map", "features=1\n" + simpleCrateManifest, nil, nil, nil, false, false, false},
		{"bad feature list", simpleCrateManifest + "[features]\nextra=true", nil, nil, nil, false, false, false},
		{"bad feature value", simpleCrateManifest + "[features]\nextra=[true]", nil, nil, nil, false, false, false},
		{"runtime contradiction", "[package]\nname='pkg'\nversion='1.0.0'\nrust-version='1.85'", nil, nil, nil, false, false, false},
		{"invalid rust version type", "[package]\nname='pkg'\nversion='1.0.0'\nrust-version=true", nil, nil, nil, false, false, false},
		{"invalid edition type", "[package]\nname='pkg'\nversion='1.0.0'\nedition=2021", nil, nil, nil, false, false, false},
		{"unknown edition", "[package]\nname='pkg'\nversion='1.0.0'\nedition='future'", nil, nil, nil, false, false, false},
		{"edition contradiction", "[package]\nname='pkg'\nversion='1.0.0'\nedition='2021'", nil, nil, nil, false, false, false},
		{"unsafe build path", "[package]\nname='pkg'\nversion='1.0.0'\nbuild='../outside'", nil, nil, nil, false, false, false},
		{"missing forced build", "[package]\nname='pkg'\nversion='1.0.0'\nbuild=true", nil, nil, nil, false, false, false},
		{"invalid build declaration", "[package]\nname='pkg'\nversion='1.0.0'\nbuild=1", nil, nil, nil, false, false, false},
		{"unsafe readme", "[package]\nname='pkg'\nversion='1.0.0'\nreadme='../outside'", nil, nil, nil, false, false, false},
		{"readme disabled", "[package]\nname='pkg'\nversion='1.0.0'\nreadme=false", nil, nil, nil, true, false, false},
		{"bad library target", "[package]\nname='pkg'\nversion='1.0.0'\nlib=[]", nil, nil, nil, true, false, false},
		{"unsafe library path", strings.Replace(simpleCrateManifest, "src/lib.rs", "../outside", 1), nil, nil, nil, false, false, false},
		{"deprecated macro", simpleCrateManifest + "proc_macro=true", nil, nil, nil, false, false, false},
		{"invalid macro", simpleCrateManifest + "proc-macro='true'", nil, nil, nil, false, false, false},
		{"crate type macro", simpleCrateManifest + "crate-type=['proc-macro']", nil, nil, nil, true, false, true},
		{"bad crate type", simpleCrateManifest + "crate-type=[true]", nil, nil, nil, false, false, false},
		{"unsafe binary path", simpleCrateManifest + "[[bin]]\nname='bin'\npath='../outside'", nil, nil, nil, false, false, false},
		{"unsafe binary name", simpleCrateManifest + "[[bin]]\nname='../outside'", nil, nil, nil, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, a := crateFixture(t, test.manifest, test.extra, test.body, nil)
			if test.expected != nil {
				test.expected(a.expected.metadata)
			}
			info, err := VerifyCargoArtifact(b, a)
			if (err == nil) != test.valid {
				t.Fatal("manifest verification", err)
			}
			if err == nil && (info.BuildScript != test.build || info.ProcMacro != test.macro) {
				t.Fatal(info)
			}
		})
	}
}

func TestCrateManifestTargetSchema(t *testing.T) {
	for _, fragment := range []string{"lib=1", "bin=true", "bin=[true]", "lib={path='../outside'}", "lib={proc-macro=true}", "lib={proc_macro=true}", "lib={crate-type=true}", "lib={crate-type=[true]}", "bin=[{name='bin',proc-macro=true}]", "lib={name='../outside'}"} {
		m := map[string]any{}
		if err := toml.Unmarshal([]byte(fragment), &m); err != nil {
			t.Fatal(err)
		}
		// A lib proc-macro boolean alone is a valid informational declaration.
		_, err := cargoManifestFiles(m, map[string]any{}, map[string]bool{})
		if fragment == "lib={proc-macro=true}" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("invalid target accepted", fragment)
		}
	}
}

func FuzzCrateManifest(f *testing.F) {
	f.Add(simpleCrateManifest)
	f.Add("[package]\nname='pkg'\nversion='1.0.0'\nbuild='../outside'")
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 8192 {
			t.Skip()
		}
		a := sealedCrate(ProjectNode{Name: "pkg", Version: "1.0.0"}, &cargoFeaturePackage{complete: true, features: map[string][]string{}}, nil)
		_, _ = verifyCargoManifest([]byte(text), a, map[string]bool{"src/lib.rs": true, "build.rs": true})
	})
}
