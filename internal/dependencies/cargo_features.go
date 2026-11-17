// purpose: Resolve a bounded default-feature union over a corroborated Cargo lock graph.
// responsibilities: Track optional activation, renamed dependencies and weak forwarding without running Cargo.
// architecture notes: All locked crates retain security findings; this union is not a target-specific native build plan.
package dependencies

import (
	"regexp"
	"sort"
	"strings"
)

const maxCargoFeatureEvents = 4096

var cargoFeatureName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_+.-]{0,127}$`)

type cargoFeatureDependency struct {
	alias, name, requirement string
	kind, target             string
	optional, defaults       bool
	features                 []string
}
type cargoFeaturePackage struct {
	rustVersion, edition string
	complete             bool
	features             map[string][]string
	dependencies         []cargoFeatureDependency
}
type cargoFeatureEvent struct{ path, feature string }
type cargoFeatureState struct {
	active       bool
	features     map[string]bool
	dependencies map[string]string
	weak         map[string]map[string]bool
}
type cargoFeatureResolver struct {
	graph        *cargoGraph
	packages     map[string]*cargoFeaturePackage
	states       map[string]*cargoFeatureState
	queue        []cargoFeatureEvent
	missing      []string
	minimumCargo string
}

func normaliseCargoFeatures(p *cargoFeaturePackage) (*cargoFeaturePackage, bool, bool) {
	if p == nil || !p.complete || p.features == nil || len(p.features) > 512 || len(p.dependencies) > maxProjectNodes {
		return nil, false, false
	}
	copy := &cargoFeaturePackage{features: map[string][]string{}, dependencies: p.dependencies}
	deps := map[string]cargoFeatureDependency{}
	for _, d := range p.dependencies {
		if !cargoName.MatchString(d.alias) || !cargoName.MatchString(d.name) || len(d.features) > 512 {
			return nil, false, false
		}
		if _, found := deps[d.alias]; found {
			return nil, false, false
		}
		for _, f := range d.features {
			if !cargoFeatureName.MatchString(f) {
				return nil, false, false
			}
		}
		deps[d.alias] = d
	}
	suppressed := map[string]bool{}
	references := 0
	requires160 := false
	for name, values := range p.features {
		if !cargoFeatureName.MatchString(name) {
			return nil, false, false
		}
		references += len(values)
		if references > 4096 {
			return nil, false, false
		}
		copy.features[name] = append([]string{}, values...)
		for _, value := range values {
			if strings.HasPrefix(value, "dep:") {
				alias := strings.TrimPrefix(value, "dep:")
				d, ok := deps[alias]
				if !ok || !d.optional {
					return nil, false, false
				}
				suppressed[alias] = true
				requires160 = true
			}
			if strings.Contains(value, "?/") {
				requires160 = true
			}
		}
	}
	for alias, d := range deps {
		if d.optional && !suppressed[alias] {
			if _, exists := copy.features[alias]; !exists {
				copy.features[alias] = []string{"dep:" + alias}
			}
		}
	}
	for _, values := range copy.features {
		for _, value := range values {
			if strings.HasPrefix(value, "dep:") {
				continue
			}
			if strings.Contains(value, "/") {
				alias, feature, _ := strings.Cut(value, "/")
				weak := strings.HasSuffix(alias, "?")
				alias = strings.TrimSuffix(alias, "?")
				d, ok := deps[alias]
				if !ok || !cargoFeatureName.MatchString(feature) || weak && !d.optional {
					return nil, false, false
				}
			} else if _, exists := copy.features[value]; !exists {
				return nil, false, false
			}
		}
	}
	// Reject cyclic local aliases conservatively; traversal depth is bounded by
	// the feature-map limit, and runtime expansion uses a separate event budget.
	colours := map[string]int{}
	var visit func(string) bool
	visit = func(name string) bool {
		if colours[name] == 1 {
			return false
		}
		if colours[name] == 2 {
			return true
		}
		colours[name] = 1
		for _, value := range copy.features[name] {
			if _, local := copy.features[value]; local && !visit(value) {
				return false
			}
		}
		colours[name] = 2
		return true
	}
	for name := range copy.features {
		if !visit(name) {
			return nil, false, false
		}
	}
	return copy, true, requires160
}

func (r *cargoFeatureResolver) state(path string) *cargoFeatureState {
	s := r.states[path]
	if s == nil {
		s = &cargoFeatureState{features: map[string]bool{}, dependencies: map[string]string{}, weak: map[string]map[string]bool{}}
		r.states[path] = s
	}
	return s
}
func (r *cargoFeatureResolver) enable(path, feature string) {
	s := r.state(path)
	if !s.active {
		s.active = true
		r.queue = append(r.queue, cargoFeatureEvent{path: path})
	}
	if feature != "" && !s.features[feature] {
		s.features[feature] = true
		r.queue = append(r.queue, cargoFeatureEvent{path: path, feature: feature})
	}
}
func (r *cargoFeatureResolver) dependency(path string, d cargoFeatureDependency) string {
	s := r.state(path)
	if target := s.dependencies[d.alias]; target != "" {
		return target
	}
	target := ""
	for _, edge := range r.graph.edges[path] {
		p := r.graph.packages[edge]
		if p.Name != d.name {
			continue
		}
		match, known := cargoSatisfies(d.requirement, p.Version)
		if !known {
			r.missing = append(r.missing, "PROJECT_RANGE_SEMANTICS")
			return ""
		}
		if match {
			if target != "" {
				r.missing = append(r.missing, "CARGO_FEATURE_RESOLUTION")
				return ""
			}
			target = edge
		}
	}
	if target == "" || r.packages[target] == nil {
		r.missing = append(r.missing, "CARGO_FEATURE_RESOLUTION")
		return ""
	}
	s.dependencies[d.alias] = target
	r.enable(target, "")
	if d.defaults {
		if _, exists := r.packages[target].features["default"]; exists {
			r.enable(target, "default")
		}
	}
	for _, feature := range d.features {
		r.enable(target, feature)
	}
	weak := []string{}
	for feature := range s.weak[d.alias] {
		weak = append(weak, feature)
	}
	sort.Strings(weak)
	for _, feature := range weak {
		r.enable(target, feature)
	}
	return target
}
func (r *cargoFeatureResolver) expand(event cargoFeatureEvent) {
	p := r.packages[event.path]
	if event.feature == "" {
		for _, d := range p.dependencies {
			if !d.optional {
				r.dependency(event.path, d)
			}
		}
		return
	}
	values, exists := p.features[event.feature]
	if !exists {
		r.missing = append(r.missing, "CARGO_FEATURE_RESOLUTION")
		return
	}
	deps := map[string]cargoFeatureDependency{}
	for _, d := range p.dependencies {
		deps[d.alias] = d
	}
	for _, value := range values {
		if strings.HasPrefix(value, "dep:") {
			r.dependency(event.path, deps[strings.TrimPrefix(value, "dep:")])
			continue
		}
		alias, feature, forward := strings.Cut(value, "/")
		if !forward {
			r.enable(event.path, value)
			continue
		}
		weak := strings.HasSuffix(alias, "?")
		alias = strings.TrimSuffix(alias, "?")
		if weak {
			s := r.state(event.path)
			if s.weak[alias] == nil {
				s.weak[alias] = map[string]bool{}
			}
			s.weak[alias][feature] = true
			if target := s.dependencies[alias]; target != "" {
				r.enable(target, feature)
			}
		} else {
			if deps[alias].optional {
				if _, exists := p.features[alias]; exists {
					r.enable(event.path, alias)
				}
			}
			if target := r.dependency(event.path, deps[alias]); target != "" {
				r.enable(target, feature)
			}
		}
	}
}

func resolveCargoFeatures(project *ProjectSnapshot) *FeatureResolution {
	result := &FeatureResolution{Basis: "default-feature-union", Status: "unknown", Enabled: []FeatureActivation{}, Missing: []string{}}
	g := project.cargo
	if g == nil || g.root == "" {
		result.Missing = []string{"CARGO_FEATURE_SCHEMA"}
		return result
	}
	r := &cargoFeatureResolver{graph: g, packages: map[string]*cargoFeaturePackage{}, states: map[string]*cargoFeatureState{}}
	paths := []string{g.root}
	for _, node := range project.Nodes {
		paths = append(paths, node.Path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		p, ok, requires160 := normaliseCargoFeatures(g.featurePackages[path])
		if !ok {
			result.Missing = []string{"CARGO_FEATURE_SCHEMA"}
			return result
		}
		r.packages[path] = p
		if requires160 {
			r.minimumCargo = "1.60"
		}
	}
	if !r.validateRequests() {
		result.Missing = []string{"CARGO_FEATURE_RESOLUTION"}
		return result
	}
	r.enable(g.root, "")
	if _, exists := r.packages[g.root].features["default"]; exists {
		r.enable(g.root, "default")
	}
	for index := 0; index < len(r.queue); index++ {
		if len(r.queue) > maxCargoFeatureEvents {
			r.missing = append(r.missing, "CARGO_FEATURE_LIMIT")
			break
		}
		r.expand(r.queue[index])
	}
	// Metadata can expose a broader union for inactive weak forwarding. Without
	// a target/unit graph, do not interpret an inactive edge as an exclusion proof.
	for _, state := range r.states {
		for alias := range state.weak {
			if state.dependencies[alias] == "" {
				r.missing = append(r.missing, "CARGO_FEATURE_CONTEXT")
			}
		}
	}
	result.Missing = unique(r.missing)
	if len(result.Missing) == 0 {
		result.Status = "resolved"
	}
	for _, path := range paths {
		s := r.states[path]
		if s == nil || !s.active {
			continue
		}
		names := []string{}
		for name := range s.features {
			if cargoFeatureName.MatchString(name) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		result.Enabled = append(result.Enabled, FeatureActivation{Path: path, Features: names})
	}
	result.MinimumCargo = r.minimumCargo
	return result
}

// Validate declared requests even before activation. An absent optional package
// can satisfy an inactive weak request, but cannot prove a strong feature request.
func (r *cargoFeatureResolver) validateRequests() bool {
	for path, p := range r.packages {
		deps := map[string]cargoFeatureDependency{}
		targets := map[string]string{}
		targetAliases := map[string]string{}
		for _, d := range p.dependencies {
			deps[d.alias] = d
			for _, edge := range r.graph.edges[path] {
				locked := r.graph.packages[edge]
				if locked.Name != d.name {
					continue
				}
				match, known := cargoSatisfies(d.requirement, locked.Version)
				if !known {
					return false
				}
				if match {
					if targets[d.alias] != "" {
						return false
					}
					targets[d.alias] = edge
				}
			}
			for _, feature := range d.features {
				target := r.packages[targets[d.alias]]
				if target == nil {
					return false
				}
				if _, ok := target.features[feature]; !ok {
					return false
				}
			}
			if target := targets[d.alias]; target != "" {
				if other := targetAliases[target]; other != "" && other != d.alias {
					return false
				}
				targetAliases[target] = d.alias
			}
		}
		for _, values := range p.features {
			for _, value := range values {
				alias, feature, forward := strings.Cut(value, "/")
				if !forward {
					continue
				}
				weak := strings.HasSuffix(alias, "?")
				alias = strings.TrimSuffix(alias, "?")
				target := r.packages[targets[alias]]
				if target == nil {
					if weak && deps[alias].optional {
						continue
					}
					return false
				}
				if _, ok := target.features[feature]; !ok {
					return false
				}
			}
		}
	}
	return true
}

func finishCargoFeatures(project *ProjectSnapshot) {
	project.Features = resolveCargoFeatures(project)
}
