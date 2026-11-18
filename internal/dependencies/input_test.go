package dependencies

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCoordinates(t *testing.T) {
	for _, tt := range []struct{ system, arg, name, version string }{
		{"npm", "lodash", "lodash", ""}, {"npm", "@scope/pkg@1.2.3", "@scope/pkg", "1.2.3"},
		{"cargo", "SerDe@1.0.2", "serde", "1.0.2"}, {"pypi", "A_b.C@2.1", "a-b-c", "2.1"},
		{"go", "github.com/org/mod@v1.2.3", "github.com/org/mod", "v1.2.3"},
		{"maven", "org.example:lib@1.2", "org.example:lib", "1.2"},
		{"pub", "archive@3.3.0", "archive", "3.3.0"},
		{"swift", "https://github.com/apple/swift-nio@2.40.0", "https://github.com/apple/swift-nio", "2.40.0"},
	} {
		t.Run(tt.arg, func(t *testing.T) {
			n, v, err := Parse(tt.system, tt.arg)
			if err != nil || n != tt.name {
				t.Fatalf("%s %v", n, err)
			}
			if tt.version == "" {
				if v != nil {
					t.Fatal(v)
				}
			} else if v == nil || *v != tt.version {
				t.Fatal(v)
			}
		})
	}
	for _, arg := range []string{"", "-x", "pkg@", "pkg@^1", "pkg@1.2 || 2", "a\n", "a/../b", "@scope/pkg@1/2", "pkg@a$b", "pkg@" + strings.Repeat("a", 129), strings.Repeat("a", 513)} {
		if _, _, err := Parse("npm", arg); err == nil {
			t.Fatalf("accepted %q", arg)
		}
	}
	for _, s := range []string{"other", "go", "maven", "swift"} {
		if _, _, err := Parse(s, "bad"); err == nil {
			t.Fatal(s)
		}
	}
	for _, repo := range []string{"github.com/org/repo?x=1", "evil.com/org/repo", "github.com/../repo", "github.com/org/..", "github.com/org/repo/extra"} {
		if Repository(repo) != "" {
			t.Fatal(repo)
		}
	}
}

func TestVersionOrdering(t *testing.T) {
	for _, tt := range []struct {
		system, version string
		valid           bool
	}{
		{"npm", "1.2.3", true}, {"npm", "1.2", false}, {"cargo", "1.2.3-rc1", false},
		{"npm", "1.2.3+build.1", true}, {"cargo", "1.2.3+build", true}, {"cargo", "1.2.3+", false}, {"cargo", "1.2.3+a..b", false},
		{"go", "v2.0.0+incompatible", true}, {"go", "2.0.0", false},
		{"pypi", "1.2.3.4", true}, {"pypi", "1.2rc1", false}, {"pypi", "1..2", false},
		{"npm", "01.2.3", false}, {"npm", "1.a.3", false}, {"npm", "99999999999999999999999999.2.3", false},
		{"maven", "1.2.3", true}, {"swift", "1.2.3", true}, {"pypi", "1.2.3.4.5", true},
	} {
		_, ok := stableVersion(tt.system, tt.version)
		if ok != tt.valid {
			t.Fatalf("%+v: %v", tt, ok)
		}
	}
	if !newer("npm", "1.10.0", "1.9.0") || newer("npm", "1.2.0", "1.3.0") || !newer("npm", "2.0.0", "1.99.99") || newer("npm", "1.0.0", "1.0.0") {
		t.Fatal("incorrect semantic order")
	}
	_ = newer("maven", "b", "a")
}

func TestDescription(t *testing.T) {
	if Description(" \n\t ") == nil {
	} else {
		t.Fatal("expected null")
	}
	if s := Description("hello\x1b\n world\u202e"); s == nil || *s != "hello world" {
		t.Fatal(s)
	}
	s := Description(strings.Repeat("界", 201))
	if utf8.RuneCountInString(*s) != 200 || !strings.HasSuffix(*s, "…") {
		t.Fatal(*s)
	}
	p := DefaultPolicy(14)
	if p.Digest() != DefaultPolicy(14).Digest() || p.Digest() == DefaultPolicy(0).Digest() {
		t.Fatal("policy digest")
	}
}

func FuzzCoordinates(f *testing.F) {
	f.Add("npm", "@scope/pkg@1.2.3")
	f.Add("pypi", "Foo_BAR")
	f.Add("pypi", "Example_Pkg@1!2.0.post1")
	f.Add("go", "../../x")
	f.Fuzz(func(t *testing.T, system, arg string) {
		name, version, err := Parse(system, arg)
		if err != nil {
			return
		}
		if name == "" || strings.ContainsAny(name, "\r\n\t") {
			t.Fatal("invalid identity")
		}
		if version != nil && *version == "" {
			t.Fatal("empty exact version")
		}
	})
}

func FuzzDescription(f *testing.F) {
	f.Add("hello\x1b[31m")
	f.Add("界\u202e")
	f.Fuzz(func(t *testing.T, raw string) {
		s := Description(raw)
		if s != nil && (!utf8.ValidString(*s) || utf8.RuneCountInString(*s) > 200) {
			t.Fatal("invalid bounded description")
		}
	})
}
