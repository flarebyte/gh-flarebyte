// purpose: Parse and validate independent script/e2e runtime environments.
package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/scanner"
)

type ToolingConfig struct {
	Runtime      string              `json:"runtime"`
	Root         string              `json:"root"`
	Dependencies *DependenciesConfig `json:"dependencies,omitempty"`
}

type DependenciesConfig struct {
	Manager string `json:"manager"`
	Root    string `json:"root"`
}

// Tooling objects deliberately accept only literal strings and nested objects.
// Unlike regex extraction, this preserves field scope and rejects wrong types.
func parseTooling(raw, section string) (*ToolingConfig, error) {
	present := false
	for _, field := range extractTopLevelFieldNames(raw) {
		if field == section {
			present = true
		}
	}
	if !present {
		return nil, nil
	}
	header := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(section) + `[ \t]*:`).FindStringIndex(raw)
	if header == nil {
		return nil, fmt.Errorf("missing %s object", section)
	}
	value := strings.TrimSpace(raw[header[1]:])
	if !strings.HasPrefix(value, "{") {
		return nil, fmt.Errorf("%s requires an object", section)
	}
	end, ok := findMatchingBrace(value, 0)
	if !ok {
		return nil, fmt.Errorf("unclosed %s object", section)
	}
	block := value[1:end]
	var s scanner.Scanner
	s.Init(strings.NewReader(block))
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanComments | scanner.SkipComments
	var scanErr error
	s.Error = func(_ *scanner.Scanner, msg string) { scanErr = fmt.Errorf("%s: %s", section, msg) }
	var object func(bool) (map[string]any, error)
	object = func(nested bool) (map[string]any, error) {
		values := map[string]any{}
		for token := s.Scan(); token != scanner.EOF; token = s.Scan() {
			if token == '}' && nested {
				return values, scanErr
			}
			if token == ',' {
				continue
			}
			if token != scanner.Ident {
				return nil, fmt.Errorf("invalid %s field near %q", section, s.TokenText())
			}
			name := s.TokenText()
			if _, exists := values[name]; exists {
				return nil, fmt.Errorf("duplicate %s.%s", section, name)
			}
			if s.Scan() != ':' {
				return nil, fmt.Errorf("expected ':' after %s.%s", section, name)
			}
			switch s.Scan() {
			case scanner.String:
				value, err := strconv.Unquote(s.TokenText())
				if err != nil {
					return nil, err
				}
				values[name] = value
			case '{':
				value, err := object(true)
				if err != nil {
					return nil, err
				}
				values[name] = value
			default:
				return nil, fmt.Errorf("%s.%s requires a string or object", section, name)
			}
		}
		if nested {
			return nil, fmt.Errorf("unclosed object in %s", section)
		}
		return values, scanErr
	}
	values, err := object(false)
	if err != nil {
		return nil, err
	}
	cfg := &ToolingConfig{Root: section}
	for _, name := range toolingKeys(values) {
		value := values[name]
		switch name {
		case "runtime", "root":
			str, ok := value.(string)
			if !ok || str == "" {
				return nil, fmt.Errorf("%s.%s requires a nonempty string", section, name)
			}
			if name == "runtime" {
				cfg.Runtime = str
			} else {
				cfg.Root = str
			}
		case "dependencies":
			deps, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.dependencies requires an object", section)
			}
			cfg.Dependencies = &DependenciesConfig{Root: "."}
			for _, key := range toolingKeys(deps) {
				val := deps[key]
				str, ok := val.(string)
				if !ok || str == "" {
					return nil, fmt.Errorf("%s.dependencies.%s requires a nonempty string", section, key)
				}
				switch key {
				case "manager":
					cfg.Dependencies.Manager = str
				case "root":
					cfg.Dependencies.Root = str
				default:
					return nil, fmt.Errorf("unknown field %q in %s.dependencies", key, section)
				}
			}
		default:
			return nil, fmt.Errorf("unknown field %q in %s", name, section)
		}
	}
	if cfg.Dependencies != nil && cfg.Dependencies.Manager == "" && cfg.Runtime == "bun" {
		cfg.Dependencies.Manager = "bun"
	}
	return cfg, validateTooling(section, cfg)
}

func validateTooling(section string, cfg *ToolingConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.Runtime != "bun" && cfg.Runtime != "python3" {
		return fmt.Errorf("%s.runtime must be bun or python3", section)
	}
	if !repositoryRelative(cfg.Root) {
		return fmt.Errorf("%s.root must be a repository-relative path", section)
	}
	if deps := cfg.Dependencies; deps != nil {
		if !repositoryRelative(deps.Root) {
			return fmt.Errorf("%s.dependencies.root must be a repository-relative path", section)
		}
		if filepath.Clean(deps.Root) != "." && filepath.Clean(deps.Root) != filepath.Clean(cfg.Root) {
			return fmt.Errorf("%s.dependencies.root must be the repository root or %s.root", section, section)
		}

		if (cfg.Runtime == "bun" && deps.Manager != "bun" && deps.Manager != "npm") || (cfg.Runtime == "python3" && deps.Manager != "uv") {
			return fmt.Errorf("invalid %s.dependencies.manager for runtime %s", section, cfg.Runtime)
		}
	}
	return nil
}

func repositoryRelative(path string) bool {
	clean := filepath.Clean(path)
	return path != "" && !filepath.IsAbs(path) && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) && !strings.HasPrefix(path, "~")
}

func validateSharedToolingDependencies(cfg Config) error {
	managers := map[string]string{}
	if cfg.Build.Language == "typescript" {
		managers["."] = "npm"
	}
	for _, entry := range []*ToolingConfig{cfg.E2E, cfg.Script} {
		if entry == nil || entry.Dependencies == nil || entry.Dependencies.Manager == "uv" {
			continue
		}
		deps := entry.Dependencies
		root := filepath.Clean(deps.Root)
		if manager, exists := managers[root]; exists && manager != deps.Manager {
			return fmt.Errorf("shared tooling dependencies.root must use one manager; %s already uses %s", root, manager)
		}
		managers[root] = deps.Manager
	}
	return nil
}

func toolingKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
