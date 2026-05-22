// purpose: Scan raw config text for top-level field names and enforce strict schema keys.
// responsibilities: Detect unknown fields in scopes and safely find matching braces while ignoring string literals.
// architecture notes: Scanner is line-oriented and shallow by design to keep behavior deterministic and testable.
package config

import (
	"fmt"
	"unicode"
)

func advanceStringState(ch byte, inString, escaped *bool) bool {
	if !*inString {
		return false
	}
	if *escaped {
		*escaped = false
		return true
	}
	if ch == '\\' {
		*escaped = true
		return true
	}
	if ch == '"' {
		*inString = false
	}
	return true
}

func assertOnlyAllowedFields(scope, raw string, allowed map[string]struct{}) error {
	for _, field := range extractTopLevelFieldNames(raw) {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unknown field %q in %s", field, scope)
		}
	}
	return nil
}

func extractTopLevelFieldNames(raw string) []string {
	var fields []string
	seen := map[string]struct{}{}
	inString := false
	escaped := false
	braceDepth := 0
	bracketDepth := 0
	lineStart := true

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if advanceStringState(ch, &inString, &escaped) {
			continue
		}
		if ch == '"' {
			inString = true
			lineStart = false
			continue
		}
		if ch == '\n' {
			lineStart = true
			continue
		}
		switch ch {
		case '{':
			braceDepth++
			lineStart = false
			continue
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
			lineStart = false
			continue
		case '[':
			bracketDepth++
			lineStart = false
			continue
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			lineStart = false
			continue
		}
		if !lineStart || braceDepth > 0 || bracketDepth > 0 {
			if !unicode.IsSpace(rune(ch)) {
				lineStart = false
			}
			continue
		}
		j := i
		for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
			j++
		}
		if j+1 < len(raw) && raw[j] == '/' && raw[j+1] == '/' {
			lineStart = false
			continue
		}
		start := j
		if j >= len(raw) || (!unicode.IsLetter(rune(raw[j])) && raw[j] != '_') {
			lineStart = false
			continue
		}
		j++
		for j < len(raw) && (unicode.IsLetter(rune(raw[j])) || unicode.IsDigit(rune(raw[j])) || raw[j] == '_') {
			j++
		}
		name := raw[start:j]
		for j < len(raw) && (raw[j] == ' ' || raw[j] == '\t') {
			j++
		}
		if j < len(raw) && raw[j] == ':' {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				fields = append(fields, name)
			}
		}
		lineStart = false
	}
	return fields
}

func findMatchingBrace(raw string, openBraceIdx int) (int, bool) {
	if openBraceIdx < 0 || openBraceIdx >= len(raw) || raw[openBraceIdx] != '{' {
		return -1, false
	}
	inString := false
	escaped := false
	depth := 0
	for i := openBraceIdx; i < len(raw); i++ {
		ch := raw[i]
		if advanceStringState(ch, &inString, &escaped) {
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}
