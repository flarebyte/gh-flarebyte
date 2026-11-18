// purpose: Prove structural closure of a bounded pure-Python wheel inventory.
// responsibilities: Use private registry/metadata bindings, retain unreachable nodes and never grant environment/install approval.
package dependencies

type pythonGraphProof struct {
	Node            ProjectNode
	Dependencies    []string
	MetadataSHA256  string
	RegistryBound   bool
	WheelCompatible bool
}

type PythonGraphClosure struct {
	Basis          string   `json:"basis"`
	Status         string   `json:"status"`
	Roots          int      `json:"roots"`
	Nodes          int      `json:"nodes"`
	Edges          int      `json:"edges"`
	ReachableNodes int      `json:"reachable_nodes"`
	Missing        []string `json:"missing"`
}

func finishPythonGraph(project *ProjectSnapshot) {
	g := &PythonGraphClosure{Basis: "registry-bound-wheel-metadata", Status: "incomplete", Roots: len(project.runtime), Nodes: len(project.Nodes), Missing: []string{}}
	project.PythonGraph = g
	gap := func(code string) { g.Missing = append(g.Missing, code) }
	if !project.InventoryComplete || len(project.Nodes) == 0 || len(project.Nodes) > maxAssessedNodes || len(project.Findings) != len(project.Nodes) {
		gap("PYPI_GRAPH_ASSESSMENT_INCOMPLETE")
		return
	}
	nodes := map[string]ProjectNode{}
	for _, node := range project.Nodes {
		if _, exists := nodes[node.Name]; exists {
			gap("PYPI_GRAPH_AMBIGUOUS_NODE")
		}
		nodes[node.Name] = node
		proof, ok := project.pypiGraph[node.Path]
		if !ok || proof.Node != node || !proof.RegistryBound || !cargoChecksum.MatchString(proof.MetadataSHA256) {
			gap("PYPI_GRAPH_METADATA_UNVERIFIED")
			continue
		}
		if !proof.WheelCompatible {
			gap("PYPI_GRAPH_WHEEL_CONTEXT_UNVERIFIED")
		}
		g.Edges += len(proof.Dependencies)
	}
	queue := []string{}
	for name, version := range project.runtime {
		if node, ok := nodes[name]; !ok || node.Version != version {
			gap("PYPI_GRAPH_ROOT_UNVERIFIED")
			continue
		}
		queue = append(queue, name)
	}
	if len(queue) == 0 {
		gap("PYPI_GRAPH_ROOT_UNVERIFIED")
	}
	seen := map[string]bool{}
	for i := 0; i < len(queue); i++ {
		name := queue[i]
		if seen[name] {
			continue
		}
		seen[name] = true
		node, exists := nodes[name]
		if !exists {
			gap("PYPI_GRAPH_EDGE_UNVERIFIED")
			continue
		}
		proof, ok := project.pypiGraph[node.Path]
		if !ok || proof.Node != node || !proof.RegistryBound {
			continue
		}
		for _, dependency := range proof.Dependencies {
			if _, ok := nodes[dependency]; !ok {
				gap("PYPI_GRAPH_EDGE_UNVERIFIED")
				continue
			}
			if !seen[dependency] {
				queue = append(queue, dependency)
			}
		}
	}
	g.ReachableNodes = len(seen)
	if g.ReachableNodes != len(nodes) {
		gap("PYPI_GRAPH_UNREACHABLE_NODE")
	}
	g.Missing = unique(g.Missing)
	if len(g.Missing) == 0 {
		g.Status = "closed"
	}
}
