// purpose: Compare packaged Cargo.toml with the approved registry metadata without executing it.
// responsibilities: Reject source changes, undeclared dependencies/features and unsafe manifest file paths.
package dependencies

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func verifyCargoManifest(data []byte, a CargoArtifact, files map[string]bool) (CargoArchiveInfo, error) {
	var m map[string]any
	if toml.Unmarshal(data, &m) != nil {
		return CargoArchiveInfo{}, errors.New("invalid crate manifest TOML")
	}
	for _, key := range []string{"workspace", "patch", "replace", "target", "cargo-features", "dev_dependencies", "build_dependencies", "project"} {
		if _, exists := m[key]; exists {
			return CargoArchiveInfo{}, fmt.Errorf("%w: crate resolution form", ErrInstallUnsupported)
		}
	}
	pkg, ok := m["package"].(map[string]any)
	if !ok || pkg["name"] != a.Node.Name || pkg["version"] != a.Node.Version {
		return CargoArchiveInfo{}, errors.New("crate manifest identity mismatch")
	}
	if _, exists := pkg["workspace"]; exists {
		return CargoArchiveInfo{}, fmt.Errorf("%w: crate workspace", ErrInstallUnsupported)
	}
	actual := &cargoFeaturePackage{complete: true, features: map[string][]string{}}
	if value, present := pkg["rust-version"]; present {
		actual.rustVersion, ok = value.(string)
		if !ok {
			return CargoArchiveInfo{}, errors.New("invalid crate compiler declaration")
		}
	}
	if value, present := pkg["edition"]; present {
		actual.edition, ok = value.(string)
		if !ok {
			return CargoArchiveInfo{}, errors.New("invalid crate edition declaration")
		}
	}
	expected := a.expected.metadata
	if !cargoEditionBaseline(pkg, actual.rustVersion) || !sameRustDeclaration(actual.rustVersion, expected.rustVersion) || defaultCargoEdition(actual.edition) != defaultCargoEdition(expected.edition) {
		return CargoArchiveInfo{}, errors.New("crate runtime metadata mismatch")
	}
	decls, ok := cargoDeclarations(m)
	if !ok {
		return CargoArchiveInfo{}, fmt.Errorf("%w: crate dependency declaration", ErrInstallUnsupported)
	}
	for _, d := range decls {
		if d.kind != "dev" {
			actual.dependencies = append(actual.dependencies, cargoFeatureDependency{alias: d.alias, name: d.name, kind: d.kind, requirement: d.requirement, optional: d.optional, defaults: d.defaultFeatures, features: d.requested})
		}
	}
	if value, present := m["features"]; present {
		entries, ok := value.(map[string]any)
		if !ok {
			return CargoArchiveInfo{}, errors.New("invalid crate feature map")
		}
		for name, value := range entries {
			list, ok := value.([]any)
			if !ok {
				return CargoArchiveInfo{}, errors.New("invalid crate feature declaration")
			}
			actual.features[name] = []string{}
			for _, value := range list {
				text, ok := value.(string)
				if !ok {
					return CargoArchiveInfo{}, errors.New("invalid crate feature value")
				}
				actual.features[name] = append(actual.features[name], text)
			}
		}
	}
	if !sameCargoMetadata(actual, expected) {
		return CargoArchiveInfo{}, errors.New("crate dependency or feature metadata mismatch")
	}
	return cargoManifestFiles(m, pkg, files)
}

func sameRustDeclaration(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	x, xOK := rustVersion(a)
	y, yOK := rustVersion(b)
	return xOK && yOK && compareNumbers(x, y) == 0
}
func defaultCargoEdition(edition string) string {
	if edition == "" {
		return "2015"
	}
	return edition
}

func sameCargoMetadata(a, b *cargoFeaturePackage) bool {
	x, xOK, _ := normaliseCargoFeatures(a)
	y, yOK, _ := normaliseCargoFeatures(b)
	if !xOK || !yOK {
		return false
	}
	for _, p := range []*cargoFeaturePackage{x, y} {
		for key, values := range p.features {
			p.features[key] = unique(values)
		}
	}
	if !reflect.DeepEqual(x.features, y.features) || len(x.dependencies) != len(y.dependencies) {
		return false
	}
	keys := func(deps []cargoFeatureDependency) ([]string, bool) {
		out := []string{}
		for _, d := range deps {
			req, ok := cargoRequirementText(d.requirement)
			if !ok {
				return nil, false
			}
			out = append(out, fmt.Sprintf("%q %q %q %q %q %t %t %q", d.alias, d.name, d.kind, d.target, req, d.optional, d.defaults, unique(append([]string{}, d.features...))))
		}
		sort.Strings(out)
		return out, true
	}
	xKeys, xOK := keys(x.dependencies)
	yKeys, yOK := keys(y.dependencies)
	return xOK && yOK && reflect.DeepEqual(xKeys, yKeys)
}

// Cargo normalises bare requirements to caret form in registry records. Do not
// guess equivalence for other differences in the approved requirement spelling.
func cargoRequirementText(requirement string) (string, bool) {
	if _, ok := cargoSatisfies(requirement, "0.0.0"); !ok {
		return "", false
	}
	parts := strings.Split(requirement, ",")
	for i, part := range parts {
		parts[i] = strings.TrimPrefix(strings.Join(strings.Fields(part), ""), "^")
	}
	return strings.Join(unique(parts), ","), true
}

func cargoManifestFiles(m, pkg map[string]any, files map[string]bool) (CargoArchiveInfo, error) {
	info := CargoArchiveInfo{BuildScript: files["build.rs"]}
	file := func(value any) bool { name, ok := value.(string); return ok && cargoPortablePath(name) && files[name] }
	for _, key := range []string{"readme", "license-file"} {
		if value, present := pkg[key]; present {
			if key == "readme" {
				if _, ok := value.(bool); ok {
					continue
				}
			}
			if !file(value) {
				return CargoArchiveInfo{}, errors.New("unsafe or missing crate metadata file")
			}
		}
	}
	if value, present := pkg["build"]; present {
		switch v := value.(type) {
		case bool:
			info.BuildScript = v
			if v && !files["build.rs"] {
				return CargoArchiveInfo{}, errors.New("crate build script missing")
			}
		case string:
			if !file(v) {
				return CargoArchiveInfo{}, errors.New("unsafe or missing crate build script")
			}
			info.BuildScript = true
		default:
			return CargoArchiveInfo{}, errors.New("invalid crate build declaration")
		}
	}
	for _, key := range []string{"lib", "bin", "example", "test", "bench"} {
		value, present := m[key]
		if !present {
			continue
		}
		targets := []map[string]any{}
		if key == "lib" {
			target, ok := value.(map[string]any)
			if !ok {
				return CargoArchiveInfo{}, errors.New("invalid crate library target")
			}
			targets = append(targets, target)
		} else {
			list, ok := value.([]any)
			if !ok {
				return CargoArchiveInfo{}, errors.New("invalid crate targets")
			}
			for _, value := range list {
				target, ok := value.(map[string]any)
				if !ok {
					return CargoArchiveInfo{}, errors.New("invalid crate target")
				}
				targets = append(targets, target)
			}
		}
		for _, target := range targets {
			for _, alias := range []string{"proc_macro", "crate_type", "required_features"} {
				if _, present := target[alias]; present {
					return CargoArchiveInfo{}, fmt.Errorf("%w: legacy crate target declaration", ErrInstallUnsupported)
				}
			}
			if value, present := target["name"]; present {
				name, ok := value.(string)
				if !ok || !cargoName.MatchString(name) {
					return CargoArchiveInfo{}, fmt.Errorf("%w: crate target name", ErrInstallUnsupported)
				}
			}
			if value, present := target["path"]; present && !file(value) {
				return CargoArchiveInfo{}, errors.New("unsafe or missing crate target path")
			}
			if value, present := target["proc-macro"]; present {
				flag, ok := value.(bool)
				if !ok || key != "lib" {
					return CargoArchiveInfo{}, errors.New("invalid crate proc-macro declaration")
				}
				info.ProcMacro = flag
			}
			if value, present := target["crate-type"]; present {
				list, ok := value.([]any)
				if !ok {
					return CargoArchiveInfo{}, errors.New("invalid crate type")
				}
				for _, value := range list {
					text, ok := value.(string)
					if !ok {
						return CargoArchiveInfo{}, errors.New("invalid crate type")
					}
					if text == "proc-macro" {
						info.ProcMacro = true
					}
				}
			}
		}
	}
	return info, nil
}
