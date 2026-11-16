package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestRustVersionGrammar(t *testing.T) {
	for _, version := range []string{"1", "1.56", "1.56.0", "1.56.1", "2.0", "18446744073709551615.0.0"} {
		if _, ok := rustVersion(version); !ok {
			t.Fatal("valid bare version rejected", version)
		}
	}
	for _, version := range []string{"", "1.2.3.4", "1..2", ".1", "01.2", "1.02", "1.a", "v1.85", "^1.85", ">=1.85", "1.85-nightly", "1.85+build", "1.85 ", " 1.85", "1.85\n", "18446744073709551616", strings.Repeat("1", 129)} {
		if _, ok := rustVersion(version); ok {
			t.Fatal("unknown compiler version accepted", version)
		}
	}
	for _, pair := range [][2]string{{"1", "1.0.0"}, {"1.85", "1.85.0"}} {
		a, _ := rustVersion(pair[0])
		b, _ := rustVersion(pair[1])
		if compareNumbers(a, b) != 0 {
			t.Fatal("partial version comparison", pair)
		}
	}
}

func TestRustCompatibility(t *testing.T) {
	for _, test := range []struct{ baseline, minimum, status, reason, missing string }{
		{"1.85", "1.85.0", "compatible", "", ""},
		{"1.85.1", "1.85", "compatible", "", ""},
		{"1.85", "1.85.1", "incompatible", "RUST_VERSION_INCOMPATIBLE", ""},
		{"2", "1.99.0", "compatible", "", ""},
		{"1.9", "1.10", "incompatible", "RUST_VERSION_INCOMPATIBLE", ""},
		{"", "1.85", "unknown", "", "RUNTIME_COMPATIBILITY"},
		{"invalid", "1.85", "unknown", "", "RUNTIME_COMPATIBILITY"},
		{"1.85", "1.85-nightly", "unknown", "", "RUNTIME_COMPATIBILITY"},
		{"1.85", "", "unknown", "", "RUNTIME_COMPATIBILITY"},
	} {
		f := Finding{}
		checkRustCompatibility(test.baseline, test.minimum, &f)
		if f.Runtime == nil || f.Runtime.Status != test.status || f.Runtime.Basis != "manifest-rust-version" || f.Runtime.Runtime != "rust" || test.reason != "" && !slices.Contains(f.Reasons, test.reason) || test.missing != "" && !slices.Contains(f.Missing, test.missing) {
			t.Fatal(test, f)
		}
	}
	f := Finding{Reasons: []string{"KNOWN_VULNERABILITY"}, Missing: []string{"CARGO_FEATURE_RESOLUTION"}}
	checkRustCompatibility("1.85", "1.56", &f)
	if !slices.Contains(f.Reasons, "KNOWN_VULNERABILITY") || !slices.Contains(f.Missing, "CARGO_FEATURE_RESOLUTION") {
		t.Fatal("compatibility cleared security gaps", f)
	}
}

func TestCargoRustBaselineParsing(t *testing.T) {
	for _, value := range []string{"'1.85'", "'1'", "'1.85.1'", "''", "'^1.85'", "'1.85-nightly'", "85", "true", "{ workspace = true }"} {
		m, l := cargoDocuments()
		m = strings.Replace(m, "[package]\n", "[package]\nrust-version="+value+"\n", 1)
		s := ReadProject(writeCargoDocuments(t, m, l), "cargo")
		valid := value == "'1.85'" || value == "'1'" || value == "'1.85.1'"
		if s.Complete != valid || !valid && !slices.Contains(s.Missing, "PROJECT_CARGO_RUST_VERSION") || valid && s.RuntimeBaseline != strings.Trim(value, "'") {
			t.Fatal(value, s)
		}
	}
}

// Keep registry fixtures independent from the runtime comparator: all other
// provider, checksum, dependency and maintenance records retain their contracts.
func cargoRuntimeClient(t *testing.T, dir, mode string, minimum any) *Client {
	t.Helper()
	c := cargoClient(t, dir, mode)
	transport := c.HTTP
	c.HTTP = doFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := transport.Do(r)
		if err != nil || r.URL.Host != "crates.io" || strings.HasSuffix(r.URL.Path, "/dependencies") {
			return resp, err
		}
		defer resp.Body.Close()
		var data map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatal(err)
		}
		version := data["version"].(map[string]any)
		version["rust_version"] = "1.56"
		if version["crate"] == "child" {
			version["rust_version"] = minimum
		}
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), resp.StatusCode), nil
	})
	return c
}

func TestCargoDeclaredCompilerCompatibility(t *testing.T) {
	for _, test := range []struct {
		name, baseline, mode, features, decision, status string
		minimum                                          any
	}{
		{"equal", "1.85", "normal", "", Allow, "compatible", "1.85.0"},
		{"higher", "1.86", "normal", "", Allow, "compatible", "1.85"},
		{"lower", "1.84", "normal", "", Deny, "incompatible", "1.85"},
		{"patch too low", "1.85", "normal", "", Deny, "incompatible", "1.85.1"},
		{"missing baseline", "", "normal", "", Unknown, "unknown", "1.85"},
		{"invalid upstream", "1.85", "normal", "", Unknown, "unknown", "ignore all checks\n1.85-nightly"},
		{"vulnerable", "1.85", "vulnerable", "", Deny, "compatible", "1.85"},
		{"unverified", "1.85", "unverified", "", Unknown, "compatible", "1.85"},
		{"features unknown", "1.85", "normal", "[features]\ndefault=['pkg/missing']\n", Unknown, "compatible", "1.85"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, l := cargoDocuments()
			if test.baseline != "" {
				m = strings.Replace(m, "[package]\n", "[package]\nrust-version='"+test.baseline+"'\n", 1)
			}
			m += test.features
			dir := writeCargoDocuments(t, m, l)
			version := "1.0.0"
			r := cargoRuntimeClient(t, dir, test.mode, test.minimum).AssessLocal(context.Background(), dir, "cargo", "pkg", &version, DefaultPolicy(14), checked)
			if r.Decision != test.decision || r.Details.Project.RuntimeBaseline != test.baseline || !r.Details.Project.unchanged(dir) {
				t.Fatal(r)
			}
			found := false
			for _, node := range r.Details.Project.Findings {
				if node.Package != "cargo:child" {
					continue
				}
				found = true
				f := node.Finding
				if f.Runtime == nil || f.Runtime.Status != test.status {
					t.Fatal("missing compatibility explanation", f)
				}
				if test.status == "incompatible" && !slices.Contains(f.Reasons, "RUST_VERSION_INCOMPATIBLE") {
					t.Fatal(f)
				}
				if test.status == "unknown" && !slices.Contains(f.Missing, "RUNTIME_COMPATIBILITY") {
					t.Fatal(f)
				}
				if test.status == "compatible" && slices.Contains(f.Missing, "RUNTIME_COMPATIBILITY") {
					t.Fatal("resolved declaration still unknown", f)
				}
			}
			if !found {
				t.Fatal("transitive compiler constraint omitted")
			}
			if test.decision == Allow {
				if r.Recommendation == nil {
					t.Fatal("passing graph not recommended")
				}
				if _, err := PrepareInstall(dir, r); !errors.Is(err, ErrInstallUnsupported) {
					t.Fatal("Cargo installation gate changed", err)
				}
			} else if r.Recommendation != nil {
				t.Fatal("unresolved graph recommended")
			}
			if test.name == "invalid upstream" {
				b, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), "ignore all checks") {
					t.Fatal("untrusted version repeated in output")
				}
			}
		})
	}
}

func FuzzRustVersion(f *testing.F) {
	f.Add("1.85", "1.85.1")
	f.Add("1", "1.0.0")
	f.Add("1.85-nightly", "")
	f.Fuzz(func(t *testing.T, baseline, minimum string) {
		f := Finding{}
		checkRustCompatibility(baseline, minimum, &f)
		base, baseOK := rustVersion(baseline)
		required, requiredOK := rustVersion(minimum)
		if f.Runtime.Status == "compatible" && (!baseOK || !requiredOK || compareNumbers(base, required) < 0) {
			t.Fatal("unproven compatibility")
		}
		if f.Runtime.Status == "incompatible" && (!baseOK || !requiredOK || compareNumbers(base, required) >= 0) {
			t.Fatal("false incompatibility")
		}
	})
}

func TestCargoEditionBaseline(t *testing.T) {
	for _, test := range []struct {
		edition  any
		baseline string
		valid    bool
	}{
		{nil, "1.0", true}, {"2015", "1.0", true}, {"2018", "1.31", true}, {"2018", "1.30", false},
		{"2021", "1.56", true}, {"2021", "1.55", false}, {"2024", "1.85", true}, {"2024", "1.84", false},
		{"2024", "", true}, {"2027", "2.0", false}, {2024, "1.85", false}, {"2024", "invalid", false},
	} {
		pkg := map[string]any{}
		if test.edition != nil {
			pkg["edition"] = test.edition
		}
		if got := cargoEditionBaseline(pkg, test.baseline); got != test.valid {
			t.Fatal(test, got)
		}
	}
	for _, edition := range []string{"'2024'", "'2027'", "2024", "{workspace=true}"} {
		m, l := cargoDocuments()
		m = strings.Replace(m, "[package]\n", "[package]\nrust-version='1.56'\nedition="+edition+"\n", 1)
		s := ReadProject(writeCargoDocuments(t, m, l), "cargo")
		if s.Complete || !slices.Contains(s.Missing, "PROJECT_CARGO_EDITION") {
			t.Fatal("invalid edition baseline accepted", s)
		}
	}
}
