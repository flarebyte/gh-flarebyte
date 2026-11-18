// purpose: Classify dependency markers over a declared inclusive Python lower-bound range.
// responsibilities: Bound parsing, preserve unknown environments, and never skip partially applicable dependencies.
package dependencies

import "strings"

type pythonMarkerState uint8

const (
	markerUnknown pythonMarkerState = iota
	markerFalse
	markerTrue
)

func pythonMarkedRequirement(raw, manifest string) (string, string, pythonMarkerState) {
	if len(raw) > 512 {
		return "", "", markerUnknown
	}
	base, marker, hasMarker := strings.Cut(raw, ";")
	name, constraint, ok := pythonDistributionRequirement(strings.TrimSpace(base))
	if !ok {
		return "", "", markerUnknown
	}
	if !hasMarker {
		return name, constraint, markerTrue
	}
	return name, constraint, pythonMarkerRange(marker, manifest)
}

type pythonMarkerParser struct {
	tokens     []string
	pos, atoms int
	valid      bool
	baseline   []uint64
}

func pythonMarkerRange(raw, manifest string) pythonMarkerState {
	tokens, ok := pythonMarkerTokens(raw)
	if !ok {
		return markerUnknown
	}
	p := pythonMarkerParser{tokens: tokens, valid: true}
	manifest = strings.TrimSpace(manifest)
	if strings.HasPrefix(manifest, ">=") {
		p.baseline, _ = pythonRuntimeVersion(strings.TrimSpace(strings.TrimPrefix(manifest, ">=")))
	}
	state := p.expression(0)
	if !p.valid || p.pos != len(tokens) {
		return markerUnknown
	}
	return state
}

func pythonMarkerTokens(raw string) ([]string, bool) {
	if len(raw) == 0 || len(raw) > 512 {
		return nil, false
	}
	var tokens []string
	for i := 0; i < len(raw); {
		ch := raw[i]
		if ch == ' ' || ch == '\t' {
			i++
			continue
		}
		start := i
		switch {
		case ch == '\'' || ch == '"':
			i++
			for i < len(raw) && raw[i] != ch {
				if raw[i] < 32 || raw[i] > 126 || raw[i] == '\\' {
					return nil, false
				}
				i++
			}
			if i == len(raw) {
				return nil, false
			}
			i++
		case ch == '(' || ch == ')':
			i++
		case strings.ContainsRune("<>=!", rune(ch)):
			i++
			if i < len(raw) && raw[i] == '=' {
				i++
			}
		case ch >= 'a' && ch <= 'z' || ch == '_':
			for i < len(raw) && (raw[i] >= 'a' && raw[i] <= 'z' || raw[i] == '_') {
				i++
			}
		default:
			return nil, false
		}
		tokens = append(tokens, raw[start:i])
		if len(tokens) > 128 {
			return nil, false
		}
	}
	return tokens, len(tokens) > 0
}

func (p *pythonMarkerParser) take(token string) bool {
	if p.pos < len(p.tokens) && p.tokens[p.pos] == token {
		p.pos++
		return true
	}
	return false
}

func (p *pythonMarkerParser) expression(depth int) pythonMarkerState {
	left := p.conjunction(depth)
	for p.take("or") {
		right := p.conjunction(depth)
		switch {
		case left == markerTrue || right == markerTrue:
			left = markerTrue
		case left == markerFalse && right == markerFalse:
			left = markerFalse
		default:
			left = markerUnknown
		}
	}
	return left
}

func (p *pythonMarkerParser) conjunction(depth int) pythonMarkerState {
	left := p.atom(depth)
	for p.take("and") {
		right := p.atom(depth)
		switch {
		case left == markerFalse || right == markerFalse:
			left = markerFalse
		case left == markerTrue && right == markerTrue:
			left = markerTrue
		default:
			left = markerUnknown
		}
	}
	return left
}

func (p *pythonMarkerParser) atom(depth int) pythonMarkerState {
	if depth > 8 || p.atoms >= 32 || p.pos >= len(p.tokens) {
		p.valid = false
		return markerUnknown
	}
	if p.take("(") {
		value := p.expression(depth + 1)
		if !p.take(")") {
			p.valid = false
		}
		return value
	}
	p.atoms++
	if p.pos+3 > len(p.tokens) {
		p.valid = false
		return markerUnknown
	}
	left, op, right := p.tokens[p.pos], p.tokens[p.pos+1], p.tokens[p.pos+2]
	p.pos += 3
	switch op {
	case "==", "!=", ">=", "<=", ">", "<":
	default:
		p.valid = false
		return markerUnknown
	}
	quoted := func(s string) bool { return len(s) >= 2 && (s[0] == '\'' || s[0] == '"') }
	if quoted(left) && !quoted(right) {
		left, right = right, left
		switch op {
		case ">":
			op = "<"
		case "<":
			op = ">"
		case ">=":
			op = "<="
		case "<=":
			op = ">="
		}
	}
	if quoted(left) || !quoted(right) {
		p.valid = false
		return markerUnknown
	}
	switch left {
	case "python_version", "python_full_version":
	case "os_name", "sys_platform", "platform_release", "platform_system", "platform_version", "platform_machine", "platform_python_implementation", "implementation_name", "implementation_version", "extra", "extras", "dependency_groups":
		return markerUnknown
	default:
		p.valid = false
		return markerUnknown
	}
	literal := right[1 : len(right)-1]
	required, ok := pythonRuntimeVersion(literal)
	if !ok {
		p.valid = false
		return markerUnknown
	}
	if len(p.baseline) == 0 {
		return markerUnknown
	}
	base := append([]uint64{}, p.baseline...)
	if left == "python_version" {
		base[2] = 0
	}
	cmp := compareNumbers(base, required)
	switch op {
	case ">=":
		if cmp >= 0 {
			return markerTrue
		}
	case ">":
		if cmp > 0 {
			return markerTrue
		}
	case "<":
		if cmp >= 0 {
			return markerFalse
		}
	case "<=":
		if cmp > 0 {
			return markerFalse
		}
	case "==":
		if cmp > 0 {
			return markerFalse
		}
	case "!=":
		if cmp > 0 {
			return markerTrue
		}
	}
	return markerUnknown
}
