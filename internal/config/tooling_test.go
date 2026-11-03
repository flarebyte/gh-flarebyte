package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolingConfiguration(t *testing.T) {
	base, err := os.ReadFile("testdata/valid.cue")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, extra, want string }{
		{"wrong object", `e2e: "bun"`, "requires an object"},
		{"unrelated dependency root", `e2e: {runtime: "bun", dependencies: {root: "other"}}`, "repository root"},
		{"defaults", `e2e: {runtime: "bun"}`, ""},
		{"shared npm", `e2e: {runtime: "bun", dependencies: {manager: "npm", root: "."}}
script: {runtime: "bun", dependencies: {manager: "npm", root: "."}}`, ""},
		{"isolated", `e2e: {runtime: "bun", root: "checks", dependencies: {root: "checks"}}`, ""},
		{"python", `script: {runtime: "python3"}`, ""},
		{"uv", `e2e: {runtime: "python3", dependencies: {manager: "uv", root: "e2e"}}`, ""},
		{"unknown runtime", `e2e: {runtime: "node"}`, "runtime"},
		{"wrong type", `e2e: {runtime: true}`, "requires a string"},
		{"unknown field", `e2e: {runtime: "bun", typo: "bad"}`, "unknown field"},
		{"unknown nested field", `e2e: {runtime: "bun", dependencies: {typo: "bad"}}`, "unknown field"},
		{"escape", `e2e: {runtime: "bun", root: "../outside"}`, "repository-relative"},
		{"absolute deps", `e2e: {runtime: "bun", dependencies: {root: "/tmp"}}`, "repository-relative"},
		{"wrong manager", `e2e: {runtime: "python3", dependencies: {manager: "npm"}}`, "manager"},
		{"duplicate", `e2e: {runtime: "bun", runtime: "python3"}`, "duplicate"},
		{"conflicting shared managers", `e2e: {runtime: "bun", dependencies: {manager: "bun"}}
script: {runtime: "bun", dependencies: {manager: "npm"}}`, "one manager"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".gh-flarebyte.cue")
			if err := os.WriteFile(path, append(append([]byte{}, base...), []byte("\n"+tc.extra+"\n")...), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want %q, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "defaults" && (cfg.E2E.Root != "e2e" || cfg.E2E.Dependencies != nil) {
				t.Fatalf("unexpected defaults: %+v", cfg.E2E)
			}
			if tc.name == "isolated" && (cfg.E2E.Root != "checks" || cfg.E2E.Dependencies.Root != "checks" || cfg.E2E.Dependencies.Manager != "bun") {
				t.Fatalf("nested root leaked: %+v", cfg.E2E)
			}
		})
	}
}
