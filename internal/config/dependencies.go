// purpose: Parse dependency policy literals without silently accepting malformed thresholds.
// responsibilities: Preserve explicit zero; reject duplicate, unknown and noninteger policy fields.
// architecture notes: Token scanning avoids matching policy text inside comments, strings or nested objects.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
)

// ParseDependencies reads only the optional top-level dependency policy. It also
// supports a policy-only file without requiring repository administration fields.
func ParseDependencies(raw string) (DependencyPolicyConfig, error) {
	cfg := DependencyPolicyConfig{MinAgeDays: 14}
	var s scanner.Scanner
	s.Init(strings.NewReader(raw))
	var scanErr error
	s.Error = func(_ *scanner.Scanner, msg string) {
		scanErr = fmt.Errorf("invalid dependency configuration: %s", msg)
	}
	depth, seen := 0, false
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		if depth == 0 && tok == scanner.Ident && s.TokenText() == "dependencies" {
			if seen || s.Scan() != ':' || s.Scan() != '{' {
				return cfg, fmt.Errorf("invalid or duplicate dependencies block")
			}
			seen = true
			fieldSeen := false
			for tok = s.Scan(); tok != '}'; tok = s.Scan() {
				if tok == ',' {
					continue
				}
				if tok != scanner.Ident || s.TokenText() != "minAgeDays" || fieldSeen || s.Scan() != ':' {
					return cfg, fmt.Errorf("dependencies: expected a single minAgeDays integer field")
				}
				fieldSeen = true
				if s.Scan() != scanner.Int {
					return cfg, fmt.Errorf("dependencies.minAgeDays: expected a nonnegative integer")
				}
				value, err := strconv.ParseUint(s.TokenText(), 10, 31)
				if err != nil {
					return cfg, fmt.Errorf("dependencies.minAgeDays: invalid integer")
				}
				cfg.MinAgeDays = int(value)
			}
			continue
		}
		switch tok {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	if scanErr != nil {
		return cfg, scanErr
	}
	return cfg, nil
}
