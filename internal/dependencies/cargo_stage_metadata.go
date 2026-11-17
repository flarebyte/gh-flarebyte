// purpose: Compare offline Cargo metadata with a frozen approved locked graph.
// responsibilities: Reject changed staged files and discrepancies in package identities, edges and default feature activation.
package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

type cargoNativeMetadata struct {
	Packages []struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Version  string  `json:"version"`
		Source   *string `json:"source"`
		Manifest string  `json:"manifest_path"`
	} `json:"packages"`
	WorkspaceMembers []string `json:"workspace_members"`
	Resolve          *struct {
		Root  string `json:"root"`
		Nodes []struct {
			ID           string   `json:"id"`
			Dependencies []string `json:"dependencies"`
			Features     []string `json:"features"`
		} `json:"nodes"`
	} `json:"resolve"`
}

// VerifyMetadata validates only the staged default-feature union. It does not
// assert target/unit build equivalence or authorize applying project changes.
func (s *CargoStage) VerifyMetadata(data []byte) error {
	if len(s.written) != len(s.artifacts) {
		return fmt.Errorf("Cargo stage is incomplete")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for name, digest := range s.files {
		b, err := projectFile(root, name, maxUnpackedBytes)
		if err != nil || digestBytes(b) != digest {
			return fmt.Errorf("Cargo changed staged inputs")
		}
	}
	var m cargoNativeMetadata
	if len(data) > maxBody || uniqueJSON(data, &m) != nil || m.Resolve == nil || len(m.Packages) != len(s.features) || len(m.Resolve.Nodes) != len(s.features) {
		return fmt.Errorf("invalid Cargo metadata graph")
	}
	ids := map[string]string{}
	paths := map[string]bool{}
	for _, p := range m.Packages {
		path := cargoPackagePath(p.Name, p.Version)
		_, active := s.features[path]
		if p.ID == "" || ids[p.ID] != "" || paths[path] || !active {
			return fmt.Errorf("unexpected Cargo metadata package")
		}
		manifest := filepath.Join(s.dir, "vendor", p.Name+"-"+p.Version, "Cargo.toml")
		if path == s.original.cargo.root {
			if p.Source != nil {
				return fmt.Errorf("Cargo root source changed")
			}
			manifest = filepath.Join(s.dir, "project", "Cargo.toml")
		} else if p.Source == nil || *p.Source != cargoRegistry {
			return fmt.Errorf("Cargo dependency source changed")
		}
		if p.Manifest != manifest {
			return fmt.Errorf("Cargo manifest location changed")
		}
		ids[p.ID], paths[path] = path, true
	}
	if ids[m.Resolve.Root] != s.original.cargo.root || len(m.WorkspaceMembers) != 1 || m.WorkspaceMembers[0] != m.Resolve.Root {
		return fmt.Errorf("Cargo workspace changed")
	}
	seen := map[string]bool{}
	for _, node := range m.Resolve.Nodes {
		path := ids[node.ID]
		if path == "" || seen[path] || !sameCargoSet(node.Features, s.features[path]) {
			return fmt.Errorf("Cargo feature activation changed")
		}
		seen[path] = true
		actual := []string{}
		for _, id := range node.Dependencies {
			if ids[id] == "" {
				return fmt.Errorf("Cargo dependency edge missing")
			}
			actual = append(actual, ids[id])
		}
		expected := []string{}
		for _, edge := range s.original.cargo.edges[path] {
			if paths[edge] {
				expected = append(expected, edge)
			}
		}
		if !sameCargoSet(actual, expected) {
			return fmt.Errorf("Cargo dependency edges changed")
		}
	}
	return nil
}

func sameCargoSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y) && len(slices.Compact(x)) == len(a) && len(slices.Compact(y)) == len(b)
}
