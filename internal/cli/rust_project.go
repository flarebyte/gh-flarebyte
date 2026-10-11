// purpose: Discover locked Cargo projects and map shared configuration to Rust packages and targets.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type cargoTarget struct {
	Name string   `json:"name"`
	Kind []string `json:"kind"`
}

type cargoPackage struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	ManifestPath string        `json:"manifest_path"`
	DefaultRun   string        `json:"default_run"`
	Targets      []cargoTarget `json:"targets"`
}

type cargoProject struct {
	Packages []cargoPackage `json:"packages"`
	Members  []string       `json:"workspace_members"`
	Root     string         `json:"workspace_root"`
}

func cargoMetadata(manifest string, env []string) (cargoProject, error) {
	var project cargoProject
	info, err := os.Stat(manifest)
	if err != nil || !info.Mode().IsRegular() {
		return project, fmt.Errorf("Rust requires a Cargo.toml manifest at %s", manifest)
	}
	out, err := cargoRun([]string{"metadata", "--locked", "--no-deps", "--format-version", "1", "--manifest-path", manifest}, env)
	if err != nil {
		return project, err
	}
	if err := json.Unmarshal([]byte(out), &project); err != nil {
		return project, fmt.Errorf("invalid Cargo metadata JSON: %w", err)
	}
	if project.Root == "" || len(project.Members) == 0 {
		return project, fmt.Errorf("Cargo metadata requires a workspace root and member packages")
	}
	if info, err := os.Stat(filepath.Join(project.Root, "Cargo.lock")); err != nil || !info.Mode().IsRegular() {
		return project, fmt.Errorf("Rust requires Cargo.lock at the workspace root; run cargo generate-lockfile and commit it")
	}
	return project, nil
}

func rustPackages(project cargoProject, selections []string) ([]cargoPackage, error) {
	var packages []cargoPackage
	all := len(selections) == 1 && selections[0] == "./..."
	if !all && contains(selections, "./...") {
		return nil, fmt.Errorf("build.packages: ./... cannot be mixed with Rust crate names")
	}
	for _, p := range project.Packages {
		if contains(project.Members, p.ID) && (all || contains(selections, p.Name)) {
			packages = append(packages, p)
		}
	}
	if len(packages) == 0 {
		return nil, fmt.Errorf("build.packages did not select any Rust workspace crates")
	}
	if !all {
		for _, name := range selections {
			found := false
			for _, p := range packages {
				found = found || p.Name == name
			}
			if !found {
				return nil, fmt.Errorf("build.packages: Rust crate %q is not a workspace member", name)
			}
		}
	}
	return packages, nil
}

func rustPackageArgs(packages []cargoPackage) []string {
	var args []string
	for _, p := range packages {
		args = append(args, "--package", p.Name)
	}
	return args
}

func rustBinaryPackage(project cargoProject, directory string) (cargoPackage, string, error) {
	manifest, err := rustManifestPath(filepath.Join(directory, "Cargo.toml"))
	if err != nil {
		return cargoPackage{}, "", err
	}
	for _, p := range project.Packages {
		packageManifest, err := rustManifestPath(p.ManifestPath)
		if err != nil {
			return cargoPackage{}, "", err
		}
		if packageManifest != manifest || !contains(project.Members, p.ID) {
			continue
		}
		var bins []string
		for _, target := range p.Targets {
			if contains(target.Kind, "bin") {
				bins = append(bins, target.Name)
			}
		}
		if p.DefaultRun != "" && contains(bins, p.DefaultRun) {
			return p, p.DefaultRun, nil
		}
		if len(bins) == 1 {
			return p, bins[0], nil
		}
		return p, "", fmt.Errorf("build.mainPackage %q must have one binary or set package.default-run in Cargo.toml", directory)
	}
	return cargoPackage{}, "", fmt.Errorf("build.mainPackage %q does not identify a Rust workspace package", directory)
}

func rustTargetTriple(target string) (string, error) {
	triples := map[string]string{
		"linux-amd64": "x86_64-unknown-linux-gnu", "linux-arm64": "aarch64-unknown-linux-gnu",
		"darwin-amd64": "x86_64-apple-darwin", "darwin-arm64": "aarch64-apple-darwin",
		"windows-amd64": "x86_64-pc-windows-msvc", "windows-arm64": "aarch64-pc-windows-msvc",
	}
	if triple, ok := triples[target]; ok {
		return triple, nil
	}
	return "", fmt.Errorf("unsupported Rust target %q: use linux/darwin/windows with amd64/arm64", target)
}

func rustCustomHarness(packages []cargoPackage) (bool, error) {
	for _, p := range packages {
		data, err := os.ReadFile(p.ManifestPath)
		if err != nil {
			return false, err
		}
		var manifest struct {
			Lib struct {
				Harness *bool `toml:"harness"`
			} `toml:"lib"`
			Bin []struct {
				Harness *bool `toml:"harness"`
			} `toml:"bin"`
			Test []struct {
				Harness *bool `toml:"harness"`
			} `toml:"test"`
		}
		if err := toml.Unmarshal(data, &manifest); err != nil {
			return false, err
		}
		if manifest.Lib.Harness != nil && !*manifest.Lib.Harness {
			return true, nil
		}
		for _, b := range manifest.Bin {
			if b.Harness != nil && !*b.Harness {
				return true, nil
			}
		}
		for _, test := range manifest.Test {
			if test.Harness != nil && !*test.Harness {
				return true, nil
			}
		}
	}
	return false, nil
}

func rustProjectVersion(manifest string) (string, error) {
	project, err := cargoMetadata(manifest, os.Environ())
	if err != nil {
		return "", err
	}
	abs, err := rustManifestPath(manifest)
	if err != nil {
		return "", err
	}
	for _, p := range project.Packages {
		packageManifest, err := rustManifestPath(p.ManifestPath)
		if err != nil {
			return "", err
		}
		if packageManifest == abs {
			if !isSemver(p.Version) {
				return "", fmt.Errorf("invalid Rust package version %q: semantic version required", p.Version)
			}
			return p.Version, nil
		}
	}
	return "", fmt.Errorf("release.versionSource must identify a package Cargo.toml; a virtual workspace has no release version")
}

func rustManifestPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Cargo canonicalizes manifests; resolve aliases such as /var and /private/var too.
	return filepath.EvalSymlinks(abs)
}
