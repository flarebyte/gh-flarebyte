package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func cargoDocuments() (string, string) {
	return "[package]\nname = 'local'\nversion = '1.0.0'\n[dependencies]\npkg = '1.0'\n", fmt.Sprintf(`version = 4
[[package]]
name = "local"
version = "1.0.0"
dependencies = ["pkg"]
[[package]]
name = "pkg"
version = "1.0.0"
source = %q
checksum = %q
dependencies = ["child"]
[[package]]
name = "child"
version = "1.0.0"
source = %q
checksum = %q
`, cargoRegistry, strings.Repeat("a", 64), cargoRegistry, strings.Repeat("a", 64))
}

func writeCargoDocuments(t *testing.T, m, l string) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range map[string]string{"Cargo.toml": m, "Cargo.lock": l} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReadCargoProject(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(string, string) (string, string)
		missing string
	}{
		{"valid", func(m, l string) (string, string) { return m, l }, ""},
		{"v3", func(m, l string) (string, string) { return m, strings.Replace(l, "version = 4", "version = 3", 1) }, ""},
		{"explicit edge", func(m, l string) (string, string) {
			return m, strings.Replace(l, `["pkg"]`, `["pkg 1.0.0 (`+cargoRegistry+`)"]`, 1)
		}, ""},
		{"renamed", func(m, l string) (string, string) {
			return strings.Replace(m, "pkg = '1.0'", "alias = { package = 'pkg', version = '1.0' }", 1), l
		}, ""},
		{"bad toml", func(m, l string) (string, string) { return m + "[dependencies]\n", l }, "PROJECT_TOML"},
		{"bad lock toml", func(m, l string) (string, string) { return m, l + "version = 5\nversion = 6\n" }, "PROJECT_TOML"},
		{"old lock", func(m, l string) (string, string) { return m, strings.Replace(l, "version = 4", "version = 1", 1) }, "PROJECT_LOCK_FORMAT"},
		{"workspace", func(m, l string) (string, string) { return m + "[workspace]\n", l }, "PROJECT_CARGO_UNSUPPORTED_MANIFEST"},
		{"patch", func(m, l string) (string, string) { return m + "[patch.crates-io]\n", l }, "PROJECT_CARGO_UNSUPPORTED_MANIFEST"},
		{"target", func(m, l string) (string, string) { return m + "[target.'cfg(unix)'.dependencies]\n", l }, "PROJECT_CARGO_UNSUPPORTED_MANIFEST"},
		{"path", func(m, l string) (string, string) {
			return strings.Replace(m, "pkg = '1.0'", "pkg = {path = '../pkg', version = '1.0'}", 1), l
		}, "PROJECT_CARGO_UNSUPPORTED_MANIFEST"},
		{"git", func(m, l string) (string, string) {
			return m, strings.Replace(l, cargoRegistry, "git+https://example.com/pkg", 1)
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"checksum", func(m, l string) (string, string) { return m, strings.Replace(l, strings.Repeat("a", 64), "bad", 1) }, "PROJECT_ARTIFACT_IDENTITY"},
		{"duplicate", func(m, l string) (string, string) {
			return m, l + fmt.Sprintf("[[package]]\nname='pkg'\nversion='1.0.0'\nsource=%q\nchecksum=%q\n", cargoRegistry, strings.Repeat("a", 64))
		}, "PROJECT_CARGO_AMBIGUOUS_PACKAGE"},
		{"missing root", func(m, l string) (string, string) {
			return strings.Replace(m, "name = 'local'", "name = 'different'", 1), l
		}, "PROJECT_NONREGISTRY_DEPENDENCY"},
		{"stale range", func(m, l string) (string, string) { return strings.Replace(m, "pkg = '1.0'", "pkg = '2.0'", 1), l }, "PROJECT_LOCK_STALE"},
		{"unknown range", func(m, l string) (string, string) { return strings.Replace(m, "pkg = '1.0'", "pkg = 'latest'", 1), l }, "PROJECT_RANGE_SEMANTICS"},
		{"missing edge", func(m, l string) (string, string) { return m, strings.Replace(l, `["child"]`, `["missing"]`, 1) }, "PROJECT_GRAPH_INCOMPLETE"},
		{"duplicate edge", func(m, l string) (string, string) { return m, strings.Replace(l, `["child"]`, `["child", "child"]`, 1) }, "PROJECT_GRAPH_INCOMPLETE"},
		{"extra root edge", func(m, l string) (string, string) { return m, strings.Replace(l, `["pkg"]`, `["pkg", "child"]`, 1) }, "PROJECT_LOCK_STALE"},
		{"prerelease lock", func(m, l string) (string, string) {
			return m, strings.ReplaceAll(l, `version = "1.0.0"`, `version = "1.0.0-rc1"`)
		}, "PROJECT_VERSION_SEMANTICS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, l := cargoDocuments()
			m, l = test.change(m, l)
			dir := writeCargoDocuments(t, m, l)
			s := ReadProject(dir, "cargo")
			if test.missing != "" {
				if s.Complete || !slices.Contains(s.Missing, test.missing) {
					t.Fatalf("got %+v, want %s", s, test.missing)
				}
				return
			}
			if !s.Complete || !s.binds("pkg", "1.0.0") || len(s.Nodes) != 2 || !s.unchanged(dir) || s.Resolution != "locked-superset" {
				t.Fatalf("%+v", s)
			}
		})
	}
}

func TestCargoProjectFilesAndChanges(t *testing.T) {
	m, l := cargoDocuments()
	for _, name := range []string{"Cargo.toml", "Cargo.lock"} {
		dir := writeCargoDocuments(t, m, l)
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if s := ReadProject(dir, "cargo"); s.Complete {
			t.Fatal("missing file accepted")
		}
		if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if s := ReadProject(dir, "cargo"); s.Complete {
			t.Fatal("symlink accepted")
		}
	}
	dir := writeCargoDocuments(t, m, l)
	s := ReadProject(dir, "cargo")
	if err := os.WriteFile(filepath.Join(dir, "Cargo.lock"), []byte(l+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.unchanged(dir) {
		t.Fatal("lock edit missed")
	}
	dir = writeCargoDocuments(t, m, l)
	s = ReadProject(dir, "cargo")
	if err := os.Mkdir(filepath.Join(dir, ".cargo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cargo", "config.toml"), []byte("[source.crates-io]\nreplace-with='elsewhere'"), 0600); err != nil {
		t.Fatal(err)
	}
	if s.unchanged(dir) || !slices.Contains(ReadProject(dir, "cargo").Missing, "PROJECT_CARGO_CONFIG") {
		t.Fatal("source override missed")
	}
	if s := ReadProject(filepath.Join(t.TempDir(), "absent"), "cargo"); s.Complete {
		t.Fatal("absent root accepted")
	}
}

func TestCargoRanges(t *testing.T) {
	for _, test := range []struct {
		r, v         string
		match, known bool
	}{
		{"1.2.3", "1.9.0", true, true}, {"1.2.3", "2.0.0", false, true}, {"1.2", "1.2.0", true, true}, {"1", "1.99.0", true, true},
		{"0.2", "0.3.0", false, true}, {"0.0", "0.0.9", true, true}, {"0", "0.99.0", true, true}, {"0.0.3", "0.0.4", false, true},
		{"~1.2", "1.2.9", true, true}, {"~1.2", "1.3.0", false, true}, {"~1", "1.9.0", true, true}, {"^ 1.2.3", "1.5.0", true, true},
		{"*", "1.2.3", true, true}, {"1.*", "2.0.0", false, true}, {"1.2.*", "1.2.9", true, true},
		{">= 1.0.0, < 2.0.0", "1.9.0", true, true}, {"= 1.2.3", "1.2.4", false, true}, {"1.2, <1.3.0", "1.4.0", false, true},
		{"1.x", "1.2.3", false, false}, {">1", "2.0.0", false, false}, {"1 || 2", "1.0.0", false, false}, {"", "1.0.0", false, false},
		{"1,", "1.0.0", false, false}, {"1.*.2", "1.0.0", false, false}, {"^1.*", "1.0.0", false, false}, {"1.0.0-rc1", "1.0.0", false, false},
		{"*.*", "1.0.0", false, false}, {"1.*.*", "1.0.0", false, false},
		{"18446744073709551615", "1.0.0", false, false}, {"1.2.3.4", "1.2.3", false, false}, {"*", "1.0.0-rc1", false, false},
		{strings.Repeat("1", 4097), "1.0.0", false, false}, {strings.Repeat("1,", 16) + "1", "1.0.0", false, false},
	} {
		match, known := cargoSatisfies(test.r, test.v)
		if match != test.match || known != test.known {
			t.Fatal(test, match, known)
		}
	}
}

func FuzzCargoProjectTOML(f *testing.F) {
	m, l := cargoDocuments()
	f.Add(m, l)
	f.Add("[dependencies]", "version=4")
	f.Fuzz(func(t *testing.T, m, l string) {
		if len(m) > 8192 || len(l) > 8192 {
			t.Skip()
		}
		dir := writeCargoDocuments(t, m, l)
		s := ReadProject(dir, "cargo")
		if s.Complete {
			if s.cargo == nil || !s.unchanged(dir) {
				t.Fatal("invalid complete snapshot")
			}
			for _, n := range s.Nodes {
				if !strings.HasPrefix(n.Integrity, "sha256-") {
					t.Fatal("missing identity")
				}
			}
		}
	})
}

func FuzzCargoRange(f *testing.F) {
	f.Add("1.2", "1.9.0")
	f.Add(">=1.0.0, <2.0.0", "1.2.3")
	f.Fuzz(func(t *testing.T, r, v string) {
		if len(r) > 4096 || len(v) > 128 {
			t.Skip()
		}
		match, known := cargoSatisfies(r, v)
		if match && !known {
			t.Fatal("unknown approved")
		}
	})
}

func TestCargoDeclarationSchema(t *testing.T) {
	for _, test := range []struct {
		raw             string
		valid, features bool
	}{
		{"[dependencies]\npkg='1.0'", true, false},
		{"[dependencies]\npkg={version='1.0',optional=false,default-features=true,features=[]}", true, false},
		{"[dependencies]\nalias={version='1.0',package='pkg',features=['derive']}", true, true},
		{"[dependencies]\npkg={version='1.0',default-features=false}", true, true},
		{"[dependencies]\npkg={version='1.0',optional=true}", true, true},
		{"[dev-dependencies]\npkg='1'\n[build-dependencies]\npkg='2'", true, false},
		{"dependencies=[]", false, false},
		{"[dependencies]\n'bad.name'='1'", false, false},
		{"[dependencies]\npkg=1", false, false},
		{"[dependencies]\npkg={version=1}", false, false},
		{"[dependencies]\npkg={version='1',package=false}", false, false},
		{"[dependencies]\npkg={version='1',package='bad/name'}", false, false},
		{"[dependencies]\npkg={version='1',optional='true'}", false, false},
		{"[dependencies]\npkg={version='1',features='derive'}", false, false},
		{"[dependencies]\npkg={version='1',features=[true]}", false, false},
		{"[dependencies]\npkg={workspace=true}", false, false},
		{"[dependencies]\npkg={package='pkg'}", false, false},
	} {
		t.Run(test.raw, func(t *testing.T) {
			var m map[string]any
			if err := toml.Unmarshal([]byte(test.raw), &m); err != nil {
				t.Fatal(err)
			}
			decls, valid := cargoDeclarations(m)
			if valid != test.valid {
				t.Fatal(valid, decls)
			}
			if valid && len(decls) == 1 && (decls[0].optional || !decls[0].defaultFeatures || len(decls[0].requested) > 0) != test.features {
				t.Fatal("feature gap lost", decls)
			}
		})
	}
	m := map[string]any{"dependencies": map[string]any{}}
	for i := 0; i <= maxProjectNodes; i++ {
		m["dependencies"].(map[string]any)[fmt.Sprintf("pkg%d", i)] = "1"
	}
	if _, ok := cargoDeclarations(m); ok {
		t.Fatal("declaration limit ignored")
	}
}

func TestCargoAmbiguityAndBounds(t *testing.T) {
	g := &cargoGraph{packages: map[string]cargoLockedPackage{"one": {Name: "pkg", Version: "1.0.0"}, "two": {Name: "pkg", Version: "2.0.0"}}}
	for _, edge := range []string{"", "pkg", "pkg 1.0.0 (git+https://example.com/pkg)", "pkg 1.0.0 extra tokens"} {
		if g.resolve(edge) != "" {
			t.Fatal("ambiguous edge accepted", edge)
		}
	}
	if g.resolve("pkg 1.0.0") != "one" {
		t.Fatal("explicit identity not resolved")
	}
	m, l := cargoDocuments()
	for _, suffix := range []string{"[features]\nbad=true\n", "[features]\nbad=[true]\n", "features=true\n", "[dev_dependencies]\nother='1'\n", "[package.workspace]\nname='parent'\n"} {
		dir := writeCargoDocuments(t, m+suffix, l)
		if s := ReadProject(dir, "cargo"); s.Complete {
			t.Fatal("unsupported manifest accepted", suffix)
		}
	}
	for _, count := range []int{maxProjectNodes + 2} {
		lock := struct {
			Version  int                  `toml:"version"`
			Packages []cargoLockedPackage `toml:"package"`
		}{Version: 4}
		for i := 0; i < count; i++ {
			lock.Packages = append(lock.Packages, cargoLockedPackage{Name: fmt.Sprintf("pkg%d", i), Version: "1.0.0"})
		}
		b, err := toml.Marshal(lock)
		if err != nil {
			t.Fatal(err)
		}
		if s := ReadProject(writeCargoDocuments(t, m, string(b)), "cargo"); !slices.Contains(s.Missing, "PROJECT_GRAPH_LIMIT") {
			t.Fatal(s)
		}
	}
	parts := strings.Split(l, "[[package]]")
	if s := ReadProject(writeCargoDocuments(t, m, parts[0]+"[[package]]"+strings.Join(parts[2:], "[[package]]")), "cargo"); !slices.Contains(s.Missing, "PROJECT_LOCK_STALE") {
		t.Fatal("missing root accepted", s)
	}
}
