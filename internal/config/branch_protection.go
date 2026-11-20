package config

import (
	"fmt"
	"strings"
	"text/scanner"
)

// Like the other config readers, accept literal values rather than CUE expressions.
func parseBranchProtection(raw string) (*BranchProtectionConfig, error) {
	var s scanner.Scanner
	s.Init(strings.NewReader(raw))
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanComments | scanner.SkipComments
	var scanErr error
	s.Error = func(_ *scanner.Scanner, msg string) {
		scanErr = fmt.Errorf("invalid repository.branchProtection: %s", msg)
	}
	depth := 0
	var cfg *BranchProtectionConfig
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		if depth == 0 && tok == scanner.Ident && s.TokenText() == "branchProtection" {
			if cfg != nil {
				return nil, fmt.Errorf("duplicate repository.branchProtection")
			}
			if s.Scan() != ':' || s.Scan() != '{' {
				return nil, fmt.Errorf("repository.branchProtection must be an object")
			}
			var err error
			cfg, err = scanBranchProtection(&s)
			if err != nil {
				return nil, err
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
	return cfg, scanErr
}

func scanBranchProtection(s *scanner.Scanner) (*BranchProtectionConfig, error) {
	cfg := &BranchProtectionConfig{AllowAdminCIBypass: true}
	seen := map[string]bool{}
	for tok := s.Scan(); tok != '}'; tok = s.Scan() {
		if tok == ',' {
			continue
		}
		if tok == scanner.EOF {
			return nil, fmt.Errorf("unclosed repository.branchProtection")
		}
		field := s.TokenText()
		path := "repository.branchProtection." + field
		if tok != scanner.Ident || (field != "enabled" && field != "allowAdminCIBypass") {
			return nil, fmt.Errorf("unknown field %q in repository.branchProtection", field)
		}
		if seen[field] {
			return nil, fmt.Errorf("duplicate %s", path)
		}
		seen[field] = true
		if s.Scan() != ':' || s.Scan() != scanner.Ident || (s.TokenText() != "true" && s.TokenText() != "false") {
			return nil, fmt.Errorf("%s must be a boolean", path)
		}
		value := s.TokenText() == "true"
		if field == "enabled" {
			cfg.Enabled = value
		} else {
			cfg.AllowAdminCIBypass = value
		}
	}
	if !seen["enabled"] {
		return nil, fmt.Errorf("repository.branchProtection.enabled is required")
	}
	return cfg, nil
}
