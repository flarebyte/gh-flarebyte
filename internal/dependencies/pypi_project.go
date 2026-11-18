// purpose: Audit a restricted public-wheel pylock inventory without executing Python.
// responsibilities: Bind exact manifest pins and immutable artifact identities; reject ambiguous sources and contexts.
// architecture notes: Inventory completeness is distinct from resolution/installation approval.
package dependencies

import (
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var pythonExactRequirement = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)\s*==\s*([A-Za-z0-9][A-Za-z0-9.!+_-]*)$`)

type pypiLockedArtifact struct {
	Filename       string
	Size           int64
	LockPython     string
	ManifestPython string
}

func readPyPIProject(dir string) *ProjectSnapshot {
	s := &ProjectSnapshot{System: "pypi", Resolution: "locked-inventory", Missing: []string{}, Nodes: []ProjectNode{}, runtime: map[string]string{}, pypi: map[string]pypiLockedArtifact{}}
	fail := func(code string) *ProjectSnapshot { s.Missing = unique(append(s.Missing, code)); return s }
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fail("PROJECT_READ")
	}
	defer root.Close()
	m, err := projectFile(root, "pyproject.toml", 1<<20)
	if err != nil {
		return fail("PROJECT_MANIFEST")
	}
	l, err := projectFile(root, "pylock.toml", maxBody)
	if err != nil {
		return fail("PROJECT_LOCKFILE")
	}
	s.ManifestDigest, s.LockDigest = digestBytes(m), digestBytes(l)
	var manifest, lock map[string]any
	if toml.Unmarshal(m, &manifest) != nil || toml.Unmarshal(l, &lock) != nil {
		return fail("PROJECT_TOML")
	}
	project, ok := manifest["project"].(map[string]any)
	if !ok {
		return fail("PROJECT_PYTHON_MANIFEST")
	}
	for _, key := range []string{"dynamic", "optional-dependencies"} {
		if _, exists := project[key]; exists {
			return fail("PROJECT_PYTHON_CONTEXT")
		}
	}
	if _, exists := manifest["dependency-groups"]; exists {
		return fail("PROJECT_PYTHON_CONTEXT")
	}
	if tool, ok := manifest["tool"].(map[string]any); ok {
		for _, key := range []string{"uv", "poetry", "pdm"} {
			if _, exists := tool[key]; exists {
				return fail("PROJECT_PYTHON_CONTEXT")
			}
		}
	}
	if lock["lock-version"] != "1.0" {
		return fail("PROJECT_LOCK_FORMAT")
	}
	created, ok := lock["created-by"].(string)
	if !ok || created == "" || len(created) > 128 {
		return fail("PROJECT_LOCK_FORMAT")
	}
	for _, key := range []string{"environments", "extras", "dependency-groups", "default-groups"} {
		if value, exists := lock[key]; exists {
			items, ok := value.([]any)
			if !ok || len(items) > 0 {
				return fail("PROJECT_PYTHON_CONTEXT")
			}
		}
	}
	entries, ok := lock["packages"].([]any)
	if !ok || len(entries) == 0 || len(entries) > maxProjectNodes {
		return fail("PROJECT_GRAPH_LIMIT")
	}
	locked := map[string]string{}
	manifestPython, _ := project["requires-python"].(string)
	lockPython, lockPythonString := lock["requires-python"].(string)
	if _, exists := lock["requires-python"]; exists && (!lockPythonString || strings.TrimSpace(lockPython) == "") {
		lockPython = "unsupported"
	}
	for _, entry := range entries {
		pkg, ok := entry.(map[string]any)
		if !ok {
			return fail("PROJECT_PYTHON_LOCK")
		}
		for _, key := range []string{"marker", "vcs", "directory", "archive", "sdist", "attestation-identities"} {
			if _, exists := pkg[key]; exists {
				return fail("PROJECT_NONREGISTRY_DEPENDENCY")
			}
		}
		name, nOK := pkg["name"].(string)
		version, vOK := pkg["version"].(string)
		canonical, exact, err := Parse("pypi", name+"@"+version)
		if !nOK || !vOK || err != nil || canonical != name || exact == nil || *exact != version {
			return fail("PROJECT_PYTHON_LOCK")
		}
		if _, ok := stableVersion("pypi", version); !ok {
			return fail("PROJECT_VERSION_SEMANTICS")
		}
		if _, exists := locked[name]; exists {
			return fail("PROJECT_PYTHON_AMBIGUOUS_PACKAGE")
		}
		if value, exists := pkg["index"]; exists {
			index, ok := value.(string)
			if !ok || strings.TrimSuffix(index, "/") != "https://pypi.org/simple" {
				return fail("PROJECT_NONREGISTRY_DEPENDENCY")
			}
		}
		wheels, ok := pkg["wheels"].([]any)
		if !ok || len(wheels) != 1 {
			return fail("PROJECT_PYTHON_ARTIFACT_SELECTION")
		}
		wheel, ok := wheels[0].(map[string]any)
		if !ok {
			return fail("PROJECT_PYTHON_LOCK")
		}
		if _, exists := wheel["path"]; exists {
			return fail("PROJECT_NONREGISTRY_DEPENDENCY")
		}
		resolved, rOK := wheel["url"].(string)
		hashes, hOK := wheel["hashes"].(map[string]any)
		hash, hashOK := hashes["sha256"].(string)
		if !rOK || !hOK || !hashOK || !cargoChecksum.MatchString(hash) {
			return fail("PROJECT_ARTIFACT_IDENTITY")
		}
		filename, ok := pypiWheelURL(resolved)
		if !ok {
			return fail("PROJECT_ARTIFACT_IDENTITY")
		}
		if supplied, exists := wheel["name"]; exists && supplied != filename {
			return fail("PROJECT_ARTIFACT_IDENTITY")
		}
		var size int64
		if supplied, exists := wheel["size"]; exists {
			var ok bool
			size, ok = supplied.(int64)
			if !ok || size <= 0 {
				return fail("PROJECT_ARTIFACT_IDENTITY")
			}
		}
		id := "pylock.toml#" + name + "@" + version
		locked[name] = version
		s.Nodes = append(s.Nodes, ProjectNode{Path: id, Name: name, Version: version, Resolved: resolved, Integrity: "sha256-" + hash})
		s.pypi[id] = pypiLockedArtifact{Filename: filename, Size: size, ManifestPython: manifestPython, LockPython: lockPython}
	}
	declarations, ok := project["dependencies"].([]any)
	if !ok || len(declarations) == 0 || len(declarations) > maxProjectNodes {
		return fail("PROJECT_PYTHON_REQUIREMENTS")
	}
	for _, entry := range declarations {
		text, ok := entry.(string)
		if !ok || len(text) > 512 {
			return fail("PROJECT_PYTHON_REQUIREMENTS")
		}
		match := pythonExactRequirement.FindStringSubmatch(text)
		if match == nil {
			return fail("PROJECT_PYTHON_REQUIREMENTS")
		}
		name, version, err := Parse("pypi", match[1]+"@"+match[2])
		if err != nil || version == nil || s.runtime[name] != "" {
			return fail("PROJECT_PYTHON_REQUIREMENTS")
		}
		if locked[name] != *version {
			return fail("PROJECT_LOCK_STALE")
		}
		s.runtime[name] = *version
	}
	sort.Slice(s.Nodes, func(i, j int) bool { return s.Nodes[i].Path < s.Nodes[j].Path })
	s.InventoryComplete = true
	// The lock's dependency table is informational in PEP 751. File-specific
	// dependency metadata and target compatibility must still be proven.
	return fail("PYPI_RESOLUTION_UNVERIFIED")
}

func pypiWheelURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "files.pythonhosted.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/packages/") || path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\") {
		return "", false
	}
	name := path.Base(u.Path)
	if len(name) > 255 || !strings.HasSuffix(name, ".whl") {
		return "", false
	}
	for _, r := range u.Path {
		if r < 33 || r > 126 {
			return "", false
		}
	}
	return name, true
}
