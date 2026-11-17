// purpose: Inspect single-package crates.io lock graphs without executing Cargo or project code.
// responsibilities: Bind manifests to exact locked identities and reject ambiguous sources, graphs and overrides.
// architecture notes: This assesses a conservative locked superset, not target/feature activation or fresh resolution.
package dependencies

import (
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const cargoRegistry = "registry+https://github.com/rust-lang/crates.io-index"

var cargoName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var cargoChecksum = regexp.MustCompile(`^[a-f0-9]{64}$`)

type cargoLockedPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Source       string   `toml:"source"`
	Checksum     string   `toml:"checksum"`
	Dependencies []string `toml:"dependencies"`
	Replace      string   `toml:"replace"`
}
type cargoGraph struct {
	packages        map[string]cargoLockedPackage
	edges           map[string][]string
	root            string
	featurePackages map[string]*cargoFeaturePackage
}
type cargoDeclaration struct {
	name, requirement string
	kind              string
	runtime           bool
	alias             string
	optional          bool
	defaultFeatures   bool
	requested         []string
}

func cargoConfigPresent(root *os.Root) bool {
	for _, path := range []string{".cargo/config", ".cargo/config.toml"} {
		if _, err := root.Lstat(path); !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func readCargoProject(dir string) *ProjectSnapshot {
	s := &ProjectSnapshot{System: "cargo", Resolution: "locked-superset", Missing: []string{}, Nodes: []ProjectNode{}, runtime: map[string]string{}}
	fail := func(code string) *ProjectSnapshot { s.Missing = unique(append(s.Missing, code)); return s }
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fail("PROJECT_READ")
	}
	defer root.Close()
	if cargoConfigPresent(root) {
		return fail("PROJECT_CARGO_CONFIG")
	}
	m, err := projectFile(root, "Cargo.toml", 1<<20)
	if err != nil {
		return fail("PROJECT_MANIFEST")
	}
	l, err := projectFile(root, "Cargo.lock", maxBody)
	if err != nil {
		return fail("PROJECT_LOCKFILE")
	}
	s.ManifestDigest, s.LockDigest = digestBytes(m), digestBytes(l)
	var manifest map[string]any
	var lock struct {
		Version  int                  `toml:"version"`
		Packages []cargoLockedPackage `toml:"package"`
	}
	if toml.Unmarshal(m, &manifest) != nil || toml.Unmarshal(l, &lock) != nil {
		return fail("PROJECT_TOML")
	}
	if lock.Version != 3 && lock.Version != 4 {
		return fail("PROJECT_LOCK_FORMAT")
	}
	for _, key := range []string{"workspace", "patch", "replace", "target", "dev_dependencies", "build_dependencies", "cargo-features"} {
		if _, found := manifest[key]; found {
			return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
		}
	}
	pkg, ok := manifest["package"].(map[string]any)
	if !ok {
		return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
	}
	if _, found := pkg["workspace"]; found {
		return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
	}
	name, nameOK := pkg["name"].(string)
	version, versionOK := pkg["version"].(string)
	if !nameOK || !versionOK || !cargoName.MatchString(name) {
		return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
	}
	if value, present := pkg["rust-version"]; present {
		baseline, ok := value.(string)
		if !ok {
			return fail("PROJECT_CARGO_RUST_VERSION")
		}
		if _, ok := rustVersion(baseline); !ok {
			return fail("PROJECT_CARGO_RUST_VERSION")
		}
		s.RuntimeBaseline = baseline
	}
	if !cargoEditionBaseline(pkg, s.RuntimeBaseline) {
		return fail("PROJECT_CARGO_EDITION")
	}
	declarations, ok := cargoDeclarations(manifest)
	if !ok {
		return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
	}
	if len(lock.Packages) > maxProjectNodes+1 {
		return fail("PROJECT_GRAPH_LIMIT")
	}
	g := &cargoGraph{packages: map[string]cargoLockedPackage{}, edges: map[string][]string{}}
	rootFeatures := map[string][]string{}
	if features, present := manifest["features"]; present {
		entries, ok := features.(map[string]any)
		if !ok {
			return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
		}
		for name, entry := range entries {
			items, ok := entry.([]any)
			if !ok {
				return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
			}
			for _, item := range items {
				text, ok := item.(string)
				if !ok {
					return fail("PROJECT_CARGO_UNSUPPORTED_MANIFEST")
				}
				rootFeatures[name] = append(rootFeatures[name], text)
			}
			if rootFeatures[name] == nil {
				rootFeatures[name] = []string{}
			}
		}
	}
	s.cargo = g
	rootPath := ""
	for _, p := range lock.Packages {
		if !cargoName.MatchString(p.Name) || p.Replace != "" {
			return fail("PROJECT_NONREGISTRY_DEPENDENCY")
		}
		if _, ok := stableVersion("cargo", p.Version); !ok {
			return fail("PROJECT_VERSION_SEMANTICS")
		}
		path := cargoPackagePath(p.Name, p.Version)
		if _, exists := g.packages[path]; exists {
			return fail("PROJECT_CARGO_AMBIGUOUS_PACKAGE")
		}
		g.packages[path] = p
		if p.Source == "" && p.Name == name && p.Version == version && p.Checksum == "" {
			rootPath = path
			continue
		}
		if p.Source != cargoRegistry {
			return fail("PROJECT_NONREGISTRY_DEPENDENCY")
		}
		if !cargoChecksum.MatchString(p.Checksum) {
			return fail("PROJECT_ARTIFACT_IDENTITY")
		}
		s.Nodes = append(s.Nodes, ProjectNode{Path: path, Name: p.Name, Version: p.Version, Resolved: "https://static.crates.io/crates/" + p.Name + "/" + p.Name + "-" + p.Version + ".crate", Integrity: "sha256-" + p.Checksum})
	}
	if rootPath == "" {
		return fail("PROJECT_LOCK_STALE")
	}
	g.root = rootPath
	g.featurePackages = map[string]*cargoFeaturePackage{rootPath: {features: rootFeatures, complete: true}}
	for _, d := range declarations {
		g.featurePackages[rootPath].dependencies = append(g.featurePackages[rootPath].dependencies, cargoFeatureDependency{alias: d.alias, name: d.name, requirement: d.requirement, kind: d.kind, optional: d.optional, defaults: d.defaultFeatures, features: d.requested})
	}
	paths := make([]string, 0, len(g.packages))
	for path := range g.packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		p := g.packages[path]
		if len(p.Dependencies) > maxProjectNodes {
			return fail("PROJECT_GRAPH_LIMIT")
		}
		seen := map[string]bool{}
		for _, edge := range p.Dependencies {
			resolved := g.resolve(edge)
			if resolved == "" || resolved == rootPath || resolved == path || seen[resolved] {
				return fail("PROJECT_GRAPH_INCOMPLETE")
			}
			seen[resolved] = true
			g.edges[path] = append(g.edges[path], resolved)
		}
		sort.Strings(g.edges[path])
	}
	for _, d := range declarations {
		matches := []string{}
		for _, edge := range g.edges[rootPath] {
			p := g.packages[edge]
			if p.Name != d.name {
				continue
			}
			match, known := cargoSatisfies(d.requirement, p.Version)
			if !known {
				return fail("PROJECT_RANGE_SEMANTICS")
			}
			if match {
				matches = append(matches, p.Version)
			}
		}
		if len(matches) == 0 {
			return fail("PROJECT_LOCK_STALE")
		}
		if d.runtime && len(matches) == 1 {
			if previous := s.runtime[d.name]; previous != "" && previous != matches[0] {
				return fail("PROJECT_CARGO_AMBIGUOUS_PACKAGE")
			}
			s.runtime[d.name] = matches[0]
		}
	}
	for _, edge := range g.edges[rootPath] {
		p := g.packages[edge]
		found := false
		for _, d := range declarations {
			if p.Name == d.name {
				match, known := cargoSatisfies(d.requirement, p.Version)
				found = found || (known && match)
			}
		}
		if !found {
			return fail("PROJECT_LOCK_STALE")
		}
	}
	sort.Slice(s.Nodes, func(i, j int) bool { return s.Nodes[i].Path < s.Nodes[j].Path })
	s.Complete = true
	return s
}

func cargoPackagePath(name, version string) string { return "Cargo.lock#" + name + "@" + version }

func (g *cargoGraph) resolve(edge string) string {
	parts := strings.Fields(edge)
	if len(parts) < 1 || len(parts) > 3 || !cargoName.MatchString(parts[0]) || len(parts) == 3 && parts[2] != "("+cargoRegistry+")" {
		return ""
	}
	match := ""
	for path, p := range g.packages {
		if p.Name != parts[0] || len(parts) > 1 && p.Version != parts[1] {
			continue
		}
		if match != "" {
			return ""
		}
		match = path
	}
	return match
}

func cargoDeclarations(manifest map[string]any) ([]cargoDeclaration, bool) {
	declarations := []cargoDeclaration{}
	for _, group := range []string{"dependencies", "dev-dependencies", "build-dependencies"} {
		value, present := manifest[group]
		if !present {
			continue
		}
		entries, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		for alias, value := range entries {
			d := cargoDeclaration{name: alias, alias: alias, defaultFeatures: true, runtime: group == "dependencies"}
			d.kind = map[string]string{"dependencies": "normal", "dev-dependencies": "dev", "build-dependencies": "build"}[group]
			if !cargoName.MatchString(alias) {
				return nil, false
			}
			switch v := value.(type) {
			case string:
				d.requirement = v
			case map[string]any:
				for key, value := range v {
					switch key {
					case "version":
						d.requirement, ok = value.(string)
						if !ok {
							return nil, false
						}
					case "package":
						d.name, ok = value.(string)
						if !ok {
							return nil, false
						}
					case "optional", "default-features":
						flag, ok := value.(bool)
						if !ok {
							return nil, false
						}
						if key == "optional" {
							d.optional = flag
						} else {
							d.defaultFeatures = flag
						}
					case "features":
						items, ok := value.([]any)
						if !ok {
							return nil, false
						}
						for _, item := range items {
							text, ok := item.(string)
							if !ok {
								return nil, false
							}
							d.requested = append(d.requested, text)
						}
					default:
						return nil, false
					}
				}
			default:
				return nil, false
			}
			if !cargoName.MatchString(d.name) || d.requirement == "" {
				return nil, false
			}
			declarations = append(declarations, d)
			if len(declarations) > maxProjectNodes {
				return nil, false
			}
		}
	}
	sort.Slice(declarations, func(i, j int) bool {
		a, b := declarations[i], declarations[j]
		if a.name != b.name {
			return a.name < b.name
		}
		if a.requirement != b.requirement {
			return a.requirement < b.requirement
		}
		if a.runtime != b.runtime {
			return !a.runtime
		}
		return a.alias < b.alias
	})
	return declarations, true
}
