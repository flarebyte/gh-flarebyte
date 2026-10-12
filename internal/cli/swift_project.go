// purpose: Discover SwiftPM root targets, source files, products, and locked dependencies.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type swiftTarget struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Path    string   `json:"path"`
	Sources []string `json:"sources"`
}
type swiftProduct struct {
	Name string                     `json:"name"`
	Type map[string]json.RawMessage `json:"type"`
}
type swiftProject struct {
	Path         string         `json:"path"`
	ToolsVersion string         `json:"tools_version"`
	Targets      []swiftTarget  `json:"targets"`
	Products     []swiftProduct `json:"products"`
}

func swiftRun(args, env []string) (string, string, error) {
	out, diagnostics, err := runCommandCapture("swift", args, env)
	if err != nil {
		return out, diagnostics, fmt.Errorf("swift %s failed (requires Swift 6+, its SDK, and resolved dependencies): %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return out, diagnostics, nil
}

func loadSwiftProject(env []string) (swiftProject, error) {
	var project swiftProject
	if info, err := os.Stat("Package.swift"); err != nil || !info.Mode().IsRegular() {
		return project, fmt.Errorf("Swift requires Package.swift in the project root")
	}
	version, _, err := swiftRun([]string{"--version"}, env)
	if err != nil {
		return project, err
	}
	if !regexp.MustCompile(`Swift version ([6-9]|[1-9][0-9]+)\.`).MatchString(version) {
		return project, fmt.Errorf("Swift support requires a Swift 6+ toolchain")
	}
	// Evaluating the manifest does not resolve dependencies or rewrite Package.resolved.
	out, _, err := swiftRun([]string{"package", "dump-package"}, env)
	if err != nil {
		return project, err
	}
	var manifest struct {
		Dependencies []map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(out), &manifest); err != nil {
		return project, fmt.Errorf("invalid Swift manifest JSON: %w", err)
	}
	for _, dependency := range manifest.Dependencies {
		if _, local := dependency["fileSystem"]; !local {
			if info, err := os.Stat("Package.resolved"); err != nil || !info.Mode().IsRegular() {
				return project, fmt.Errorf("Swift remote dependencies require Package.resolved; run swift package resolve and commit it")
			}
		}
	}
	out, _, err = swiftRun([]string{"package", "--force-resolved-versions", "describe", "--type", "json"}, env)
	if err != nil {
		return project, err
	}
	if err := json.Unmarshal([]byte(out), &project); err != nil {
		return project, fmt.Errorf("invalid Swift package description JSON: %w", err)
	}
	if project.Path == "" || len(project.Targets) == 0 {
		return project, fmt.Errorf("Swift package description has no root path or targets")
	}
	return project, nil
}

func swiftSelectedTargets(project swiftProject, selections []string) ([]string, error) {
	if len(selections) == 1 && selections[0] == "./..." {
		return nil, nil
	}
	if contains(selections, "./...") {
		return nil, fmt.Errorf("build.packages: ./... cannot be mixed with Swift target names")
	}
	var selected []string
	for _, name := range selections {
		found := false
		for _, target := range project.Targets {
			if name == target.Name && target.Type != "test" && target.Type != "plugin" {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("build.packages: %q is not a root-package Swift build target", name)
		}
		if !contains(selected, name) {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("build.packages must select Swift build targets")
	}
	return selected, nil
}

func swiftExecutable(project swiftProject, name string) (string, error) {
	var names []string
	for _, product := range project.Products {
		if _, ok := product.Type["executable"]; ok {
			names = append(names, product.Name)
		}
	}
	if name == "" && len(names) == 1 {
		return names[0], nil
	}
	if name != "" && contains(names, name) {
		return name, nil
	}
	return "", fmt.Errorf("build.mainPackage must name an executable product when Swift package selection is ambiguous (available: %s)", strings.Join(names, ", "))
}

func swiftSourceFiles(project swiftProject, productionOnly bool) (map[string]string, error) {
	files := map[string]string{}
	root, err := filepath.EvalSymlinks(project.Path)
	if err != nil {
		return nil, err
	}
	add := func(path string) error {
		abs, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("Swift source %s is outside the root package", path)
		}
		files[abs] = filepath.ToSlash(rel)
		return nil
	}
	for _, target := range project.Targets {
		if productionOnly && (target.Type == "test" || target.Type == "plugin") {
			continue
		}
		for _, source := range target.Sources {
			if err := add(filepath.Join(root, target.Path, source)); err != nil {
				return nil, err
			}
		}
	}
	if !productionOnly {
		if err := add(filepath.Join(root, "Package.swift")); err != nil {
			return nil, err
		}
	}
	return files, nil
}
