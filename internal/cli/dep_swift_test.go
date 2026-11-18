package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDepSwiftAdviceAndPrintOnly(t *testing.T) {
	for _, mode := range []string{"check", "explain", "install", "exact", "blocked", "flags", "explain install"} {
		t.Run(mode, func(t *testing.T) {
			localDependencyRepo(t, "")
			const manifest = "// untouched Package.swift\n"
			if err := os.WriteFile("Package.swift", []byte(manifest), 0644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", t.TempDir())
			stubPackageAdviceAPI(t, mode == "blocked")
			args := []string{"dep", "install", "swift", "https://github.com/Org/Repo.git"}
			if mode == "exact" {
				args[3] += "@v1.2.3"
			}
			if mode == "check" || mode == "explain" {
				args[1] = "check"
				args = append(args, "--json")
			}
			if mode == "explain" || mode == "explain install" {
				args = append(args, "--explain")
			}
			if mode == "flags" {
				args = append(args, "--json")
			}
			var out, stderr bytes.Buffer
			r := Run(args, &out, &stderr)
			if mode == "blocked" || mode == "flags" || mode == "explain install" {
				if r.ExitCode == 0 || out.Len() != 0 || stderr.Len() == 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
			} else {
				if r.ExitCode != 0 || stderr.Len() != 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
				if mode == "check" || mode == "explain" {
					if !strings.Contains(out.String(), `"decision":"allow"`) || !strings.Contains(out.String(), `"description":"Swift fixture"`) {
						t.Fatal(out.String())
					}
				} else if !strings.Contains(out.String(), "PRINT ONLY — nothing installed.") || !strings.Contains(out.String(), `.package(url: "https://github.com/org/repo", exact: "1.2.3")`) {
					t.Fatal(out.String())
				}
			}
			data, err := os.ReadFile("Package.swift")
			if err != nil || string(data) != manifest {
				t.Fatal(string(data), err)
			}
		})
	}
}

func TestDepSwiftPrintRejectsInvalidCoordinates(t *testing.T) {
	for _, arg := range []string{"https://evil.test/org/repo", "https://github.com/org/repo\n"} {
		var out, stderr bytes.Buffer
		if r := printSwiftDependency(arg, "1.0.0", &out, &stderr); r.ExitCode != ExitUsage || out.Len() != 0 {
			t.Fatal(r, out.String())
		}
	}
}
