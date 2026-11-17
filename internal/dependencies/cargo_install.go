// purpose: Prepare an exact Cargo dependency pin for the supported default-target context.
// responsibilities: Preserve TOML formatting and bind installation to unchanged local resolution inputs.
// architecture notes: Only ordinary library graphs without build/proc-macro contexts are applicable.
package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type CargoInstallPlan struct {
	Manifest  []byte
	Lockfile  []byte
	before    []byte
	snapshot  *ProjectSnapshot
	context   string
	sources   []string
	pinDigest string
}

func PrepareCargoInstall(dir string, r Report) (CargoInstallPlan, error) {
	p := CargoInstallPlan{}
	artifacts, err := PrepareCargoArtifacts(dir, r)
	if err != nil {
		return p, err
	}
	for _, a := range artifacts {
		for _, d := range a.expected.metadata.dependencies {
			if d.kind != "normal" || d.target != "" {
				return p, fmt.Errorf("%w: Cargo build or target dependencies", ErrInstallUnsupported)
			}
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return p, err
	}
	defer root.Close()
	m, err := projectFile(root, "Cargo.toml", 1<<20)
	if err != nil {
		return p, err
	}
	l, err := projectFile(root, "Cargo.lock", maxBody)
	if err != nil {
		return p, err
	}
	if digestBytes(m) != r.Details.Project.ManifestDigest || digestBytes(l) != r.Details.Project.LockDigest {
		return p, fmt.Errorf("Cargo inputs changed after assessment")
	}
	context, sources, err := cargoInstallContext(dir, m)
	if err != nil {
		return p, err
	}
	var manifest map[string]any
	if toml.Unmarshal(m, &manifest) != nil {
		return p, fmt.Errorf("invalid Cargo manifest")
	}
	_, name, _ := strings.Cut(r.Package, ":")
	decls, ok := cargoDeclarations(manifest)
	if !ok {
		return p, fmt.Errorf("unsupported Cargo dependencies")
	}
	alias := ""
	for _, d := range decls {
		if d.runtime && d.name == name {
			if alias != "" {
				return p, fmt.Errorf("%w: ambiguous runtime alias", ErrInstallUnsupported)
			}
			alias = d.alias
		}
	}
	if alias == "" {
		return p, fmt.Errorf("%w: existing locked runtime dependency required", ErrInstallUnsupported)
	}
	pinned, err := pinCargoRequirement(m, alias, r.Recommendation.Version)
	if err != nil {
		return p, err
	}
	p = CargoInstallPlan{Manifest: pinned, Lockfile: l, before: m, snapshot: ReadProject(dir, "cargo"), context: context, sources: sources, pinDigest: digestBytes(pinned)}
	if !p.Unchanged(dir) {
		return CargoInstallPlan{}, fmt.Errorf("Cargo inputs changed while preparing pin")
	}
	return p, nil
}

// Scan parsed scalar ranges rather than rewriting TOML: comments, quoting of
// keys, dependency options and every unrelated byte remain untouched.
func pinCargoRequirement(data []byte, alias, version string) ([]byte, error) {
	var before map[string]any
	if toml.Unmarshal(data, &before) != nil {
		return nil, fmt.Errorf("invalid Cargo TOML")
	}
	var parser unstable.Parser
	parser.Reset(data)
	table := []string{}
	var found *unstable.Range
	keys := func(n *unstable.Node) []string {
		out := []string{}
		it := n.Key()
		for it.Next() {
			out = append(out, string(it.Node().Data))
		}
		return out
	}
	var visit func(*unstable.Node, []string)
	visit = func(n *unstable.Node, prefix []string) {
		path := append(slices.Clone(prefix), keys(n)...)
		v := n.Value()
		if v.Kind == unstable.InlineTable {
			it := v.Children()
			for it.Next() {
				visit(it.Node(), path)
			}
			return
		}
		if slices.Equal(path, []string{"dependencies", alias}) || slices.Equal(path, []string{"dependencies", alias, "version"}) {
			if v.Kind == unstable.String {
				if found != nil {
					found.Length = 0
					return
				}
				raw := v.Raw
				found = &raw
			}
		}
	}
	for parser.NextExpression() {
		n := parser.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			table = keys(n)
		case unstable.KeyValue:
			visit(n, table)
		}
	}
	if parser.Error() != nil || found == nil || found.Length == 0 {
		return nil, fmt.Errorf("%w: Cargo version declaration cannot be pinned", ErrInstallUnsupported)
	}
	start, end := int(found.Offset), int(found.Offset+found.Length)
	if start < 0 || end > len(data) || end <= start {
		return nil, fmt.Errorf("invalid Cargo scalar range")
	}
	pinned := append([]byte{}, data[:start]...)
	pinned = append(pinned, []byte(strconv.Quote("="+version))...)
	pinned = append(pinned, data[end:]...)
	var after map[string]any
	if toml.Unmarshal(pinned, &after) != nil {
		return nil, fmt.Errorf("invalid pinned Cargo TOML")
	}
	deps, ok := before["dependencies"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing runtime dependencies")
	}
	switch value := deps[alias].(type) {
	case string:
		deps[alias] = "=" + version
	case map[string]any:
		value["version"] = "=" + version
	default:
		return nil, fmt.Errorf("unsupported runtime declaration")
	}
	if !reflect.DeepEqual(before, after) {
		return nil, fmt.Errorf("pin changed unrelated Cargo semantics")
	}
	return pinned, nil
}

func cargoInstallContext(dir string, m []byte) (string, []string, error) {
	unsupported := func() (string, []string, error) {
		return "", nil, fmt.Errorf("%w: Cargo install requires default source targets without build/dev contexts or inherited configuration", ErrInstallUnsupported)
	}
	var manifest map[string]any
	if toml.Unmarshal(m, &manifest) != nil {
		return unsupported()
	}
	for _, key := range []string{"lib", "bin", "example", "test", "bench", "workspace", "target", "patch", "replace", "cargo-features"} {
		if _, ok := manifest[key]; ok {
			return unsupported()
		}
	}
	pkg, ok := manifest["package"].(map[string]any)
	if !ok {
		return unsupported()
	}
	for _, key := range []string{"workspace", "links", "metabuild", "autolib", "autobins", "autoexamples", "autotests", "autobenches"} {
		if _, ok := pkg[key]; ok {
			return unsupported()
		}
	}
	if build, ok := pkg["build"]; ok && build != false {
		return unsupported()
	}
	if resolver, ok := pkg["resolver"]; ok && resolver != "1" && resolver != "2" && resolver != "3" {
		return unsupported()
	}
	for _, key := range []string{"build-dependencies", "dev-dependencies"} {
		if value, ok := manifest[key]; ok {
			entries, ok := value.(map[string]any)
			if !ok || len(entries) != 0 {
				return unsupported()
			}
		}
	}
	for _, key := range []string{"readme", "license-file"} {
		if v, exists := pkg[key]; exists {
			if key == "readme" {
				if _, ok := v.(bool); ok {
					continue
				}
			}
			name, ok := v.(string)
			if !ok || !cargoPortablePath(name) {
				return unsupported()
			}
		}
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", nil, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", nil, err
	}
	for parent := dir; ; parent = filepath.Dir(parent) {
		for _, name := range []string{".cargo/config", ".cargo/config.toml"} {
			if _, err := os.Lstat(filepath.Join(parent, name)); !os.IsNotExist(err) {
				return unsupported()
			}
		}
		if parent != dir {
			if _, err := os.Lstat(filepath.Join(parent, "Cargo.toml")); !os.IsNotExist(err) {
				return unsupported()
			}
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", nil, err
	}
	defer root.Close()
	for _, name := range []string{"build.rs", "src/bin", "examples", "tests", "benches"} {
		if _, err := root.Lstat(name); !os.IsNotExist(err) {
			return unsupported()
		}
	}
	info, err := root.Lstat("src")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return unsupported()
	}
	sources := []string{}
	for _, name := range []string{"src/lib.rs", "src/main.rs"} {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return unsupported()
		}
		sources = append(sources, name)
	}
	if len(sources) == 0 {
		return unsupported()
	}
	return strings.Join(sources, "\n"), sources, nil
}

func (p CargoInstallPlan) OriginalManifest() []byte { return slices.Clone(p.before) }
func (p CargoInstallPlan) Unchanged(dir string) bool {
	return p.snapshot != nil && p.snapshot.unchanged(dir) && p.ContextUnchanged(dir)
}

// ContextUnchanged excludes the manifest, which is moved aside during commit.
func (p CargoInstallPlan) ContextUnchanged(dir string) bool {
	if p.snapshot == nil {
		return false
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false
	}
	defer root.Close()
	l, err := projectFile(root, "Cargo.lock", maxBody)
	if err != nil || digestBytes(l) != p.snapshot.LockDigest {
		return false
	}
	context, _, err := cargoInstallContext(dir, p.before)
	return err == nil && context == p.context
}

func (s *CargoStage) UseInstallPlan(p CargoInstallPlan) error {
	if p.snapshot == nil || p.snapshot.ManifestDigest != s.original.ManifestDigest || p.snapshot.LockDigest != s.original.LockDigest || digestBytes(p.Manifest) != p.pinDigest || digestBytes(p.Lockfile) != s.original.LockDigest {
		return fmt.Errorf("Cargo stage/pin binding invalid")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range []string{"project/Cargo.toml", "project/src/lib.rs"} {
		b, err := projectFile(root, name, 1<<20)
		if err != nil || digestBytes(b) != s.files[name] {
			return fmt.Errorf("Cargo stage changed before context binding")
		}
		if err := root.Remove(name); err != nil {
			return err
		}
		delete(s.files, name)
	}
	if err := s.write("project/Cargo.toml", p.Manifest); err != nil {
		return err
	}
	for _, name := range p.sources {
		if err := s.write(filepath.Join("project", name), nil); err != nil {
			return err
		}
	}
	s.installSources = slices.Clone(p.sources)
	var manifest map[string]any
	if err := toml.Unmarshal(p.before, &manifest); err != nil {
		return err
	}
	edition, _ := manifest["package"].(map[string]any)["edition"].(string)
	s.installEdition = defaultCargoEdition(edition)
	return nil
}
