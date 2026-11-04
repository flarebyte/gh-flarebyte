// purpose: Discover one locked TypeScript project and execute its local tools with Node/npm.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const typescriptCompiler = "./node_modules/typescript/bin/tsc"
const typescriptBiome = "./node_modules/@biomejs/biome/bin/biome"

type typescriptProject struct {
	Root         string
	Files        map[string]string
	ToolingRoots []string
}

func bunRun(args, env []string) (string, error) {
	out, diagnostics, err := runCommandCapture("bun", append([]string{"--no-install"}, args...), env)
	if err != nil {
		return out, fmt.Errorf("bun %s failed (requires Bun 1.3.6+ for standalone CLI compilation): %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return out, nil
}

func typescriptRun(args, env []string) (string, error) {
	out, diagnostics, err := runCommandCapture("node", args, env)
	if err != nil {
		return out, fmt.Errorf("node %s failed: %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return out, nil
}

func npmRun(args, env []string) (string, error) {
	out, diagnostics, err := runCommandCapture("npm", args, env)
	if err != nil {
		return out, fmt.Errorf("npm %s failed: %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return out, nil
}

func prepareTypeScript(env []string) (typescriptProject, error) {
	project := typescriptProject{Files: map[string]string{}}
	for _, path := range []string{"package.json", "package-lock.json", "tsconfig.json"} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return project, fmt.Errorf("TypeScript requires %s at the project root; install dependencies with npm and commit package-lock.json", path)
		}
	}
	data, err := os.ReadFile("package.json")
	if err != nil {
		return project, err
	}
	var manifest struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return project, fmt.Errorf("invalid TypeScript package.json: %w", err)
	}
	if len(manifest.Workspaces) > 0 {
		return project, fmt.Errorf("TypeScript initially supports one root package, not npm workspaces")
	}
	if _, err := npmRun([]string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}, env); err != nil {
		return project, err
	}
	if info, err := os.Stat(typescriptCompiler); err != nil || !info.Mode().IsRegular() {
		return project, fmt.Errorf("TypeScript requires the project-local tool %s; add typescript to devDependencies", typescriptCompiler)
	}
	project.Root, err = filepath.EvalSymlinks(".")
	if err != nil {
		return project, err
	}
	project.Root, err = filepath.Abs(project.Root)
	if err != nil {
		return project, err
	}
	out, err := typescriptRun([]string{typescriptCompiler, "--project", "tsconfig.json", "--showConfig"}, env)
	if err != nil {
		return project, err
	}
	var resolved struct {
		Files      []string          `json:"files"`
		References []json.RawMessage `json:"references"`
	}
	if err := json.Unmarshal([]byte(out), &resolved); err != nil {
		return project, fmt.Errorf("invalid TypeScript project configuration: %w", err)
	}
	if len(resolved.References) > 0 {
		return project, fmt.Errorf("TypeScript project references are outside initial single-project support")
	}
	for _, name := range resolved.Files {
		if !filepath.IsAbs(name) {
			name = filepath.Join(project.Root, name)
		}
		canonical, err := filepath.EvalSymlinks(name)
		if err != nil {
			return project, err
		}
		relative, err := filepath.Rel(project.Root, canonical)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return project, fmt.Errorf("TypeScript source %s is outside the root package", name)
		}
		project.Files[canonical] = filepath.ToSlash(relative)
	}
	if len(project.Files) == 0 {
		return project, fmt.Errorf("TypeScript project contains no source files")
	}
	return project, nil
}

func typescriptTypecheck(env []string) error {
	_, err := typescriptRun([]string{typescriptCompiler, "--project", "tsconfig.json", "--noEmit"}, env)
	return err
}

func typescriptProductionFile(name string) bool {
	for _, part := range strings.Split(name, "/") {
		switch part {
		case "node_modules", "e2e", "script", "test", "tests", "__tests__", "build", "dist", "coverage", ".cache", "generated", "__generated__":
			return false
		}
	}
	base := filepath.Base(name)
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasSuffix(base, ".d.ts") || strings.HasSuffix(base, ".d.mts") || strings.HasSuffix(base, ".d.cts") {
		return false
	}
	return contains([]string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}, filepath.Ext(base))
}

func (project typescriptProject) coverageFiles(env []string) (map[string]string, error) {
	files := map[string]string{}
	for path, label := range project.Files {
		toolingFile := false
		for _, root := range project.ToolingRoots {
			if label == root || strings.HasPrefix(label, root+"/") {
				toolingFile = true
			}
		}
		if typescriptProductionFile(label) && !toolingFile {
			files[path] = label
		}
	}

	return files, nil
}
