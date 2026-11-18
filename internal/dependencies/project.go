// purpose: Inspect a concrete local npm lock graph without invoking package tooling.
// responsibilities: Bound file reads, verify manifest/lock agreement and identify unsupported resolution changes.
// architecture notes: Inspect locked versions and dependency-free additions/upgrades; general changes require a separate safe resolver.
package dependencies

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
)

const maxProjectNodes = 256

type ProjectNode struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Integrity string `json:"integrity"`
	Resolved  string `json:"resolved"`
}

type ProjectSnapshot struct {
	System            string             `json:"system"`
	Resolution        string             `json:"resolution,omitempty"`
	RuntimeBaseline   string             `json:"runtime_baseline,omitempty"`
	ManifestDigest    string             `json:"manifest_digest,omitempty"`
	LockDigest        string             `json:"lock_digest,omitempty"`
	Complete          bool               `json:"complete"`
	InventoryComplete bool               `json:"inventory_complete,omitempty"`
	Missing           []string           `json:"missing"`
	Nodes             []ProjectNode      `json:"nodes"`
	Findings          []ProjectFinding   `json:"findings,omitempty"`
	Plans             []ProjectPlan      `json:"plans,omitempty"`
	Features          *FeatureResolution `json:"features,omitempty"`
	runtime           map[string]string
	packages          map[string]npmPackage
	cargo             *cargoGraph
	pypi              map[string]pypiLockedArtifact
}

type ProjectPlan struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Kind            string `json:"kind"`
	Integrity       string `json:"integrity,omitempty"`
	Resolved        string `json:"resolved,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
}

type ProjectFinding struct {
	Path    string  `json:"path"`
	Package string  `json:"package"`
	Finding Finding `json:"finding"`
}

type npmPackage struct {
	Name                 string                     `json:"name"`
	Version              string                     `json:"version"`
	Dependencies         map[string]string          `json:"dependencies"`
	DevDependencies      map[string]string          `json:"devDependencies"`
	OptionalDependencies map[string]string          `json:"optionalDependencies"`
	PeerDependencies     map[string]string          `json:"peerDependencies"`
	PeerMeta             map[string]PeerRequirement `json:"peerDependenciesMeta"`
	Workspaces           json.RawMessage            `json:"workspaces"`
	Overrides            json.RawMessage            `json:"overrides"`
	Link                 bool                       `json:"link"`
	InBundle             bool                       `json:"inBundle"`
	HasShrinkwrap        bool                       `json:"hasShrinkwrap"`
	Bin                  json.RawMessage            `json:"bin"`
	Resolved             string                     `json:"resolved"`
	Integrity            string                     `json:"integrity"`
}

type PeerRequirement struct {
	Optional bool `json:"optional"`
}

func digestBytes(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

func projectFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("unsupported project file")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("project file exceeds size limit")
	}
	return b, nil
}

// uniqueJSON refuses duplicate object fields at any depth rather than relying
// on a parser's last-value-wins behavior for security-sensitive manifests.
func uniqueJSON(b []byte, out any) error {
	return strictJSON(b, out, '{')
}

func decodeGitHubArray(b []byte, out any) error { return strictJSON(b, out, '[') }

func strictJSON(b []byte, out any, opening byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	var walk func() error
	walk = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, err := d.Token()
					if err != nil {
						return err
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return fmt.Errorf("duplicate JSON field")
					}
					seen[key] = true
					if err := walk(); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err := walk(); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("unexpected JSON delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	if b = bytes.TrimSpace(b); len(b) == 0 || b[0] != opening {
		return fmt.Errorf("unexpected JSON container")
	}
	return json.Unmarshal(b, out)
}

func ReadProject(dir, system string) *ProjectSnapshot {
	if system == "pypi" {
		return readPyPIProject(dir)
	}
	if system == "cargo" {
		return readCargoProject(dir)
	}
	s := &ProjectSnapshot{System: system, Missing: []string{}, Nodes: []ProjectNode{}, runtime: map[string]string{}}
	fail := func(code string) *ProjectSnapshot { s.Missing = unique(append(s.Missing, code)); return s }
	if system != "npm" {
		return fail("PROJECT_RESOLVER_UNSUPPORTED")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fail("PROJECT_READ")
	}
	defer root.Close()
	manifest, err := projectFile(root, "package.json", 1<<20)
	if err != nil {
		return fail("PROJECT_MANIFEST")
	}
	lock, err := projectFile(root, "package-lock.json", maxBody)
	if err != nil {
		return fail("PROJECT_LOCKFILE")
	}
	s.ManifestDigest = digestBytes(manifest)
	s.LockDigest = digestBytes(lock)
	var m npmPackage
	var l struct {
		Version  int                   `json:"lockfileVersion"`
		Packages map[string]npmPackage `json:"packages"`
	}
	if uniqueJSON(manifest, &m) != nil || uniqueJSON(lock, &l) != nil {
		return fail("PROJECT_JSON")
	}
	s.packages = l.Packages
	if l.Version != 2 && l.Version != 3 {
		return fail("PROJECT_LOCK_FORMAT")
	}
	lroot, ok := l.Packages[""]
	if !ok {
		return fail("PROJECT_LOCK_ROOT")
	}
	if len(m.Workspaces) > 0 || len(m.Overrides) > 0 {
		return fail("PROJECT_WORKSPACES_OR_OVERRIDES")
	}
	if m.Name != lroot.Name || m.Version != lroot.Version || !sameDeps(m, lroot) {
		return fail("PROJECT_LOCK_STALE")
	}
	if len(l.Packages) > maxProjectNodes+1 {
		return fail("PROJECT_GRAPH_LIMIT")
	}
	paths := make([]string, 0, len(l.Packages))
	for path := range l.Packages {
		if path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		p := l.Packages[path]
		name := lockName(path)
		if name == "" || p.Link || p.InBundle || (p.Name != "" && p.Name != name) {
			return fail("PROJECT_NONREGISTRY_DEPENDENCY")
		}
		if p.HasShrinkwrap {
			return fail("PROJECT_SHRINKWRAP")
		}
		if _, ok := stableVersion("npm", p.Version); !ok {
			return fail("PROJECT_VERSION_SEMANTICS")
		}
		if !npmArtifact(p.Resolved, p.Integrity) {
			return fail("PROJECT_ARTIFACT_IDENTITY")
		}
		s.Nodes = append(s.Nodes, ProjectNode{Path: path, Name: name, Version: p.Version, Integrity: p.Integrity, Resolved: p.Resolved})
	}
	for path, p := range l.Packages {
		maps := []map[string]string{p.Dependencies, p.OptionalDependencies, p.PeerDependencies}
		if path == "" {
			maps = append(maps, p.DevDependencies)
		}
		for group, dependencies := range maps {
			for name, requirement := range dependencies {
				location := resolveLocked(l.Packages, path, name)
				if location == "" {
					if group == 2 && p.PeerMeta[name].Optional {
						continue
					}
					s.Missing = append(s.Missing, "PROJECT_GRAPH_INCOMPLETE")
					continue
				}
				satisfied, known := npmSatisfies(requirement, l.Packages[location].Version)
				if !known {
					s.Missing = append(s.Missing, "PROJECT_RANGE_SEMANTICS")
				} else if !satisfied {
					s.Missing = append(s.Missing, "PROJECT_LOCK_STALE")
				}
			}
		}
	}
	s.Missing = unique(s.Missing)
	s.Complete = len(s.Missing) == 0
	for name := range m.Dependencies {
		if path := resolveLocked(l.Packages, "", name); path != "" {
			s.runtime[name] = l.Packages[path].Version
		}
	}
	return s
}

func sameDeps(a, b npmPackage) bool {
	// Empty and absent maps have the same native meaning.
	equal := func(x, y map[string]string) bool {
		if len(x) == 0 && len(y) == 0 {
			return true
		}
		return reflect.DeepEqual(x, y)
	}
	peers := func(m map[string]PeerRequirement) map[string]bool {
		out := map[string]bool{}
		for name, p := range m {
			if p.Optional {
				out[name] = true
			}
		}
		return out
	}
	return equal(a.Dependencies, b.Dependencies) && equal(a.DevDependencies, b.DevDependencies) && equal(a.OptionalDependencies, b.OptionalDependencies) && equal(a.PeerDependencies, b.PeerDependencies) && reflect.DeepEqual(peers(a.PeerMeta), peers(b.PeerMeta))
}

func lockName(path string) string {
	if !strings.HasPrefix(path, "node_modules/") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, "node_modules/"), "/node_modules/")
	for _, name := range parts {
		if !npmName.MatchString(name) {
			return ""
		}
	}
	return parts[len(parts)-1]
}

func resolveLocked(packages map[string]npmPackage, from, name string) string {
	if !npmName.MatchString(name) {
		return ""
	}
	for {
		candidate := "node_modules/" + name
		if from != "" {
			candidate = from + "/" + candidate
		}
		if _, ok := packages[candidate]; ok {
			return candidate
		}
		if from == "" {
			return ""
		}
		if at := strings.LastIndex(from, "/node_modules/"); at >= 0 {
			from = from[:at]
		} else {
			from = ""
		}
	}
}

func npmArtifact(raw, integrity string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "registry.npmjs.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.Contains(u.Path, "/-/") {
		return false
	}
	if !strings.HasPrefix(integrity, "sha512-") {
		return false
	}
	if strings.ContainsAny(integrity, " \r\n\t") {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(integrity, "sha512-"))
	return err == nil && len(b) == 64
}

func (s *ProjectSnapshot) binds(name, version string) bool {
	return s != nil && s.Complete && s.runtime[name] == version
}

// planLeaf covers a deliberately narrow addition that cannot introduce new
// dependency edges. Existing versions and artifact identities remain pinned.
func (s *ProjectSnapshot) planLeaf(release *NPMRelease) bool {
	if s == nil || !s.Complete || release == nil || !npmArtifact(release.Dist.Tarball, release.Dist.Integrity) {
		return false
	}
	if !npmName.MatchString(release.Name) {
		return false
	}
	if _, ok := stableVersion("npm", release.Version); !ok {
		return false
	}
	if len(release.Dependencies) > 0 || len(release.OptionalDependencies) > 0 || len(release.PeerDependencies) > 0 || hasBundled(release.BundleDependencies) || hasBundled(release.BundledDependencies) {
		return false
	}
	if release.HasShrinkwrap || release.Shrinkwrap {
		return false
	}
	if _, occupied := s.packages["node_modules/"+release.Name]; occupied {
		return false
	}
	for path, p := range s.packages {
		requirement, present := p.PeerDependencies[release.Name]
		if !present || resolveLocked(s.packages, path, release.Name) != "" {
			continue
		}
		if match, known := npmSatisfies(requirement, release.Version); !known || !match {
			return false
		}
	}
	s.Plans = append(s.Plans, ProjectPlan{Name: release.Name, Version: release.Version, Kind: "add-leaf", Integrity: release.Dist.Integrity, Resolved: release.Dist.Tarball})
	return true
}

// planReplacement preserves every other locked node and every incoming edge.
// Only the root runtime declaration may change; no fresh resolution is inferred.
func (s *ProjectSnapshot) planReplacement(release *NPMRelease) bool {
	if s == nil || !s.Complete || release == nil {
		return false
	}
	path := "node_modules/" + release.Name
	old, exists := s.packages[path]
	if !exists || s.runtime[release.Name] != old.Version {
		return false
	}
	before, beforeOK := stableVersion("npm", old.Version)
	after, afterOK := stableVersion("npm", release.Version)
	if !beforeOK || !afterOK || compareNumbers(after, before) <= 0 || len(old.Dependencies)+len(old.OptionalDependencies)+len(old.PeerDependencies) > 0 {
		return false
	}
	// Reuse addition validation on a private view without the old root slot.
	view := *s
	view.packages = make(map[string]npmPackage, len(s.packages))
	view.Plans = nil
	for location, p := range s.packages {
		if strings.HasPrefix(location, path+"/") {
			return false
		}
		if location != path {
			view.packages[location] = p
		}
		groups := []map[string]string{p.Dependencies, p.OptionalDependencies, p.PeerDependencies}
		if location == "" {
			if _, found := p.DevDependencies[release.Name]; found {
				return false
			}
			if _, found := p.OptionalDependencies[release.Name]; found {
				return false
			}
		}
		for group, declarations := range groups {
			requirement, present := declarations[release.Name]
			if !present || location == "" && group == 0 || resolveLocked(s.packages, location, release.Name) != path {
				continue
			}
			if match, known := npmSatisfies(requirement, release.Version); !known || !match {
				return false
			}
		}
	}
	if !view.planLeaf(release) {
		return false
	}
	plan := view.Plans[0]
	plan.Kind = "replace-leaf"
	plan.PreviousVersion = old.Version
	s.Plans = append(s.Plans, plan)
	return true
}

func (s *ProjectSnapshot) unchanged(dir string) bool {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return false
	}
	defer root.Close()
	manifest, lock := "package.json", "package-lock.json"
	if s.System == "cargo" {
		manifest, lock = "Cargo.toml", "Cargo.lock"
		if cargoConfigPresent(root) {
			return false
		}
	}
	if s.System == "pypi" {
		manifest, lock = "pyproject.toml", "pylock.toml"
	}
	m, err := projectFile(root, manifest, 1<<20)
	if err != nil {
		return false
	}
	l, err := projectFile(root, lock, maxBody)
	return err == nil && digestBytes(m) == s.ManifestDigest && digestBytes(l) == s.LockDigest
}
