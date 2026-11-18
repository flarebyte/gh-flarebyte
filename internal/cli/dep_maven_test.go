package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDepMavenAdviceAndPrintOnly(t *testing.T) {
	for _, mode := range []string{"check", "kotlin", "groovy", "xml", "missing", "ambiguous", "symlink", "blocked", "exact", "flags"} {
		t.Run(mode, func(t *testing.T) {
			root := localDependencyRepo(t, "")
			dir := filepath.Join(root, "app")
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{}
			switch mode {
			case "kotlin", "exact", "blocked", "flags":
				files["build.gradle.kts"] = "// unchanged\n"
			case "groovy":
				files["build.gradle"] = "// unchanged\n"
			case "xml":
				files["pom.xml"] = "<project/>\n"
			case "ambiguous":
				files["build.gradle.kts"] = "// unchanged\n"
				files["pom.xml"] = "<project/>\n"
			}
			for name, body := range files {
				if err := os.WriteFile(name, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink" {
				if err := os.Symlink(filepath.Join(root, "absent"), "build.gradle.kts"); err != nil {
					t.Skip(err)
				}
			}
			// No SDK or native executable is available/needed for this workflow.
			t.Setenv("PATH", t.TempDir())
			stubPackageAdviceAPI(t, mode == "blocked")
			args := []string{"dep", "install", "maven", "org.example:lib"}
			if mode == "check" {
				args = []string{"dep", "check", "maven", "org.example:lib", "--json"}
			}
			if mode == "exact" {
				args[3] += "@1.2.3"
			}
			if mode == "flags" {
				args = append(args, "--json")
			}
			var out, stderr bytes.Buffer
			r := Run(args, &out, &stderr)
			if mode == "blocked" || mode == "flags" {
				if r.ExitCode == 0 || out.Len() != 0 || stderr.Len() == 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
			} else {
				if r.ExitCode != 0 || stderr.Len() != 0 {
					t.Fatal(r, out.String(), stderr.String())
				}
				if mode == "check" {
					if !strings.Contains(out.String(), `"scope":"package"`) || !strings.Contains(out.String(), `"decision":"allow"`) {
						t.Fatal(out.String())
					}
				} else {
					if !strings.Contains(out.String(), "PRINT ONLY — nothing installed.") || !strings.Contains(out.String(), "org.example") || !strings.Contains(out.String(), "1.2.3") {
						t.Fatal(out.String())
					}
					switch mode {
					case "kotlin", "exact":
						if !strings.Contains(out.String(), `implementation("org.example:lib:1.2.3")`) {
							t.Fatal(out.String())
						}
					case "groovy":
						if !strings.Contains(out.String(), "implementation 'org.example:lib:1.2.3'") {
							t.Fatal(out.String())
						}
					case "xml":
						if !strings.Contains(out.String(), "<groupId>org.example</groupId>") || !strings.Contains(out.String(), "<version>1.2.3</version>") {
							t.Fatal(out.String())
						}
					default:
						if !strings.Contains(out.String(), "missing or ambiguous") {
							t.Fatal(out.String())
						}
					}
				}
			}
			for name, body := range files {
				b, err := os.ReadFile(name)
				if err != nil || string(b) != body {
					t.Fatal("build file changed", name)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := len(files)
			if mode == "symlink" {
				want++
			}
			if len(entries) != want {
				t.Fatal("unexpected file mutation", entries)
			}
		})
	}
}

func TestDepMavenRejectsUnsafeDeclaration(t *testing.T) {
	var out, stderr bytes.Buffer
	r := printMavenDependency(t.TempDir(), "org.example:lib", `1.2.3");evil`, &out, &stderr)
	if r.ExitCode != ExitUsage || out.Len() != 0 {
		t.Fatal(r, out.String())
	}
}
