package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestPythonStableVersionOrdering(t *testing.T) {
	ordered := []string{"0.9", "1.0", "1.0.post0", "1.0.post1", "1.0.post10", "1.0.0.1", "1.0.1", "9999.0", "1!0", "1!0.post0", "1!1.0", "2!0"}
	for i, version := range ordered {
		key, ok := stableVersion("pypi", version)
		if !ok {
			t.Fatal(version)
		}
		for j, other := range ordered {
			otherKey, ok := stableVersion("pypi", other)
			if !ok {
				t.Fatal(other)
			}
			comparison := compareNumbers(key, otherKey)
			if i < j && comparison >= 0 || i == j && comparison != 0 || i > j && comparison <= 0 {
				t.Fatalf("wrong order: %s vs %s", version, other)
			}
		}
	}
	for _, pair := range [][2]string{{"1", "1.0.0"}, {"0!1.0", "1"}, {"01.002.0", "1.2"}, {"001!01.0.post002", "1!1.post2"}} {
		a, aOK := stableVersion("pypi", pair[0])
		b, bOK := stableVersion("pypi", pair[1])
		if !aOK || !bOK || compareNumbers(a, b) != 0 {
			t.Fatal("padding or integer normalization differs", pair)
		}
	}
	if !newer("pypi", "1!0", "9999") || !newer("pypi", "1.post10", "1.post2") {
		t.Fatal("numeric precedence lost")
	}
}

func TestPythonVersionBoundaries(t *testing.T) {
	for _, version := range []string{"", "1..0", "1!", "!1", "1!!1", "-1.0", "1.post", "1.post-1", "1.0+local", "v1.0", "1.0-1", "1.0POST1", "1.0.post1.extra", "1.0a1", "1!1.0rc1", "1.0.post1.dev2", "18446744073709551616!1", "1.18446744073709551616", "1.post18446744073709551616", strings.Repeat("1.", 64) + "1"} {
		if _, ok := stableVersion("pypi", version); ok {
			t.Fatal("unsupported version ordered", version)
		}
	}
	for _, version := range []string{"1.0a1", "1.0b2", "1.0rc3", "1!1.0rc1.post2", "1.0.dev1", "1.0.post1.dev2", "2!1.0rc1.post2.dev3+local.1"} {
		if !knownPrerelease("pypi", version) {
			t.Fatal("prerelease not excluded", version)
		}
		if _, ok := stableVersion("pypi", version); ok {
			t.Fatal("prerelease eligible", version)
		}
	}
	for _, version := range []string{"1.0", "1!1.0.post1", "1.0+local", "1.0rc", strings.Repeat("1", 129)} {
		if knownPrerelease("pypi", version) {
			t.Fatal("stable or unknown classified prerelease", version)
		}
	}
	if _, ok := stableVersion("pypi", strings.Repeat("1.", 63)+"1"); !ok {
		t.Fatal("valid bounded release tuple rejected")
	}
	if _, ok := stableVersion("pypi", "18446744073709551615!1.post18446744073709551615"); !ok {
		t.Fatal("uint64 boundary rejected")
	}
	for system, name := range map[string]string{"npm": "pkg", "cargo": "pkg", "go": "example.com/pkg", "maven": "org:pkg", "pub": "pkg", "swift": "https://github.com/org/pkg"} {
		if _, _, err := Parse(system, name+"@1!1.0"); err == nil {
			t.Fatal("epoch accepted outside PyPI", system)
		}
	}
	name, v, err := Parse("pypi", "Example_Pkg@1!2.0.post1")
	if err != nil || name != "example-pkg" || v == nil || *v != "1!2.0.post1" {
		t.Fatal(name, v, err)
	}
}

func TestPythonCatalogCandidateOrdering(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "supported", true: "local version"}[unknown], func(t *testing.T) {
			c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "api.deps.dev" && !strings.Contains(r.URL.Path, "/versions/") {
					versions := []string{"9999.0", "1!0.post2", "1!0.post10", "1!0", "2!1.0rc1", "2!1.0.post1.dev1", "0.1.2.3.4"}
					if unknown {
						versions = append(versions, "1!0+local")
					}
					p := Package{}
					p.PackageKey.Name = "example"
					for _, v := range versions {
						p.Versions = append(p.Versions, Version{Key: VersionKey{Version: v}})
					}
					b, err := json.Marshal(p)
					if err != nil {
						t.Fatal(err)
					}
					return response(string(b), 200), nil
				}
				if r.URL.Host == "api.deps.dev" {
					v := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
					data, err := json.Marshal(Version{Key: VersionKey{System: "PYPI", Name: "example", Version: v}, PublishedAt: checked.AddDate(-1, 0, 0)})
					if err != nil {
						t.Fatal(err)
					}
					return response(string(data), 200), nil
				}
				if r.URL.Host == "api.osv.dev" {
					var q struct {
						Version string `json:"version"`
					}
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
						t.Fatal(err)
					}
					if strings.Contains(q.Version, "%21") {
						t.Fatal("OSV received URL encoding instead of exact version")
					}
					return response(`{}`, 200), nil
				}
				if r.URL.Host == "pypi.org" {
					parts := strings.Split(r.URL.Path, "/")
					v := parts[len(parts)-2]
					data, err := json.Marshal(map[string]any{"info": map[string]string{"name": "example", "version": v}, "urls": []any{map[string]any{"filename": "example.whl", "yanked": false}}})
					if err != nil {
						t.Fatal(err)
					}
					return response(string(data), 200), nil
				}
				t.Fatalf("unexpected provider %s", r.URL)
				return nil, nil
			})}
			r := c.Assess(context.Background(), "pypi", "example", nil, DefaultPolicy(14), checked)
			want := []string{"1!0.post10", "1!0.post2", "1!0", "9999.0"}
			if len(r.Details.Candidates) != len(want) {
				t.Fatal(r.Details.Candidates)
			}
			for i, f := range r.Details.Candidates {
				if f.Version != want[i] {
					t.Fatal("wrong candidate order", r.Details.Candidates)
				}
				for _, gap := range []string{"VERSION_LOOKUP", "STABLE_VERSION_SEMANTICS", "RELEASE_LIFECYCLE"} {
					if slices.Contains(f.Missing, gap) {
						t.Fatal("supported exact version lost evidence", f)
					}
				}
				if slices.Contains(f.Reasons, "IDENTITY_CONFLICT") {
					t.Fatal("epoch changed across providers", f)
				}
			}
			if slices.Contains(r.Details.RecommendationMissing, "VERSION_ORDERING") != unknown {
				t.Fatal(r.Details.RecommendationMissing)
			}
			if r.Decision == Allow || r.Recommendation != nil {
				t.Fatal("version support bypassed trust or resolution gates")
			}
		})
	}
}

func TestPythonVersionPolicyBoundaries(t *testing.T) {
	for _, test := range []struct{ version, reason, missing string }{
		{"1!2.0.post0", "", ""},
		{"1!2.0rc1.post1", "PRERELEASE", ""},
		{"1!2.0.post1.dev1", "PRERELEASE", ""},
		{"1!2.0+local", "", "STABLE_VERSION_SEMANTICS"},
		{"1!2.0POST1", "", "STABLE_VERSION_SEMANTICS"},
	} {
		v := Version{Key: VersionKey{System: "PYPI", Name: "example", Version: test.version}, PublishedAt: checked.AddDate(-1, 0, 0)}
		f := evaluateRelease(v, nil, nil, completeTrust(), DefaultPolicy(14), checked)
		if test.reason != "" && !slices.Contains(f.Reasons, test.reason) || test.missing != "" && !slices.Contains(f.Missing, test.missing) {
			t.Fatal(test, f)
		}
		if test.reason == "" && test.missing == "" {
			if f.Decision != Allow {
				t.Fatal("stable post release rejected", f)
			}
		} else if f.Decision == Allow {
			t.Fatal("unsupported version approved", f)
		}
	}
}

func FuzzPythonVersion(f *testing.F) {
	f.Add("1!2.0.post10", "1!2.0.post2")
	f.Add("1.0.post0", "1.0")
	f.Add("1.0rc1.post2.dev3", "1.0")
	f.Fuzz(func(t *testing.T, a, b string) {
		x, xOK := stableVersion("pypi", a)
		y, yOK := stableVersion("pypi", b)
		if xOK && knownPrerelease("pypi", a) {
			t.Fatal("prerelease eligible")
		}
		if !xOK || !yOK {
			return
		}
		if len(x) != len(y) || compareNumbers(x, y) != -compareNumbers(y, x) {
			t.Fatal("inconsistent ordering")
		}
		if a != b && newer("pypi", a, b) == newer("pypi", b, a) {
			t.Fatal("nondeterministic total order")
		}
	})
}
