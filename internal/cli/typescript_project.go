// purpose: Discover one locked TypeScript project and execute its local tools with Bun.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const typescriptCompiler = "./node_modules/typescript/bin/tsc"
const typescriptBiome = "./node_modules/@biomejs/biome/bin/biome"

type typescriptProject struct {
	Root   string
	Files  map[string]string
	Bunfig map[string]any
}

func typescriptRun(args, env []string) (string, error) {
	out, diagnostics, err := runCommandCapture("bun", append([]string{"--no-install"}, args...), env)
	if err != nil {
		return out, fmt.Errorf("bun %s failed (requires Bun 1.3.6+, locked dependencies, TypeScript, and Biome): %w\n%s", args[0], err, strings.TrimSpace(out+"\n"+diagnostics))
	}
	return out, nil
}

func prepareTypeScript(env []string) (typescriptProject, error) {
	project := typescriptProject{Files: map[string]string{}, Bunfig: map[string]any{}}
	for _, path := range []string{"package.json", "bun.lock", "tsconfig.json"} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return project, fmt.Errorf("TypeScript requires %s at the project root; install dependencies with Bun and commit bun.lock", path)
		}
	}
	if _, err := os.Stat("biome.json"); err != nil {
		if _, err := os.Stat("biome.jsonc"); err != nil {
			return project, fmt.Errorf("TypeScript requires biome.json or biome.jsonc at the project root")
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
		return project, fmt.Errorf("TypeScript initially supports one root package, not Bun workspaces")
	}
	version, err := typescriptRun([]string{"--version"}, env)
	if err != nil {
		return project, err
	}
	parts := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(strings.TrimSpace(version))
	if len(parts) != 4 {
		return project, fmt.Errorf("cannot determine Bun version; TypeScript requires Bun 1.3.6+")
	}
	major, _ := strconv.Atoi(parts[1])
	minor, _ := strconv.Atoi(parts[2])
	patch, _ := strconv.Atoi(parts[3])
	if major < 1 || (major == 1 && (minor < 3 || (minor == 3 && patch < 6))) {
		return project, fmt.Errorf("TypeScript requires Bun 1.3.6+")
	}
	if _, err := typescriptRun([]string{"install", "--frozen-lockfile", "--ignore-scripts"}, env); err != nil {
		return project, err
	}
	for _, tool := range []string{typescriptCompiler, typescriptBiome} {
		if info, err := os.Stat(tool); err != nil || !info.Mode().IsRegular() {
			return project, fmt.Errorf("TypeScript requires the project-local tool %s; add typescript and @biomejs/biome to devDependencies", tool)
		}
	}
	project.Root, err = filepath.EvalSymlinks(".")
	if err != nil {
		return project, err
	}
	project.Root, err = filepath.Abs(project.Root)
	if err != nil {
		return project, err
	}
	if data, err := os.ReadFile("bunfig.toml"); err == nil {
		if err := toml.Unmarshal(data, &project.Bunfig); err != nil {
			return project, fmt.Errorf("invalid bunfig.toml: %w", err)
		}
	} else if !os.IsNotExist(err) {
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
		case "node_modules", "test", "tests", "__tests__", "build", "dist", "coverage", ".cache", "generated", "__generated__":
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
		if typescriptProductionFile(label) {
			files[path] = label
		}
	}
	test, _ := project.Bunfig["test"].(map[string]any)
	if patterns, ok := test["coveragePathIgnorePatterns"]; ok {
		if pattern, ok := patterns.(string); ok {
			patterns = []string{pattern}
		}
		payload, err := json.Marshal(map[string]any{"files": files, "patterns": patterns})
		if err != nil {
			return nil, err
		}
		// Bun owns its glob syntax; pass data as one argument, never interpolated code.
		code := `const p=JSON.parse(Bun.argv[1]);const globs=p.patterns.map(x=>new Bun.Glob(x));process.stdout.write(JSON.stringify(Object.fromEntries(Object.entries(p.files).filter(([path,label])=>!globs.some(g=>g.match(label))))));`
		out, err := typescriptRun([]string{"--eval", code, "--", string(payload)}, env)
		if err != nil {
			return nil, err
		}
		selected := map[string]string{}
		if err := json.Unmarshal([]byte(out), &selected); err != nil {
			return nil, fmt.Errorf("invalid Bun coverage source selection: %w", err)
		}
		files = selected
	}
	return files, nil
}
