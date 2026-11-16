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

func TestAssessCargoFeatures(t *testing.T) {
	for _, test := range []struct {
		name, root, mode string
		pkg              map[string][]string
		optional         bool
		decision         string
	}{
		{"empty default", "[features]\ndefault=[]\n", "normal", map[string][]string{}, false, Allow},
		{"root forwarding", "[features]\ndefault=['pkg/extra']\n", "normal", map[string][]string{"extra": {}}, false, Allow},
		{"unknown forwarding", "[features]\ndefault=['pkg/missing']\n", "normal", map[string][]string{"extra": {}}, false, Unknown},
		{"activated optional", "", "normal", map[string][]string{"default": {"dep:child"}}, true, Allow},
		{"activated weak", "", "normal", map[string][]string{"default": {"child?/extra", "dep:child"}}, true, Allow},
		{"inactive weak context", "", "normal", map[string][]string{"default": {"child?/extra"}}, true, Unknown},
		{"inactive vulnerable crate", "", "vulnerable", map[string][]string{}, true, Deny},
		{"feature schema absent", "", "normal", nil, false, Unknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, l := cargoDocuments()
			dir := writeCargoDocuments(t, m+test.root, l)
			c := cargoClient(t, dir, test.mode)
			transport := c.HTTP
			c.HTTP = doFunc(func(r *http.Request) (*http.Response, error) {
				resp, err := transport.Do(r)
				if err != nil || r.URL.Host != "crates.io" {
					return resp, err
				}
				defer resp.Body.Close()
				var data map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(r.URL.Path, "/dependencies") {
					if strings.Contains(r.URL.Path, "/pkg/") && test.optional {
						data["dependencies"].([]any)[0].(map[string]any)["optional"] = true
					}
				} else {
					v := data["version"].(map[string]any)
					if v["crate"] == "pkg" {
						if test.pkg == nil {
							delete(v, "features")
						} else {
							v["features"] = test.pkg
						}
					} else {
						v["features"] = map[string][]string{"extra": {}}
					}
				}
				b, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				return response(string(b), resp.StatusCode), nil
			})
			version := "1.0.0"
			r := c.AssessLocal(context.Background(), dir, "cargo", "pkg", &version, DefaultPolicy(14), checked)
			if r.Decision != test.decision || !r.Details.Project.unchanged(dir) {
				t.Fatal(r)
			}
			if r.Details.Project.Features == nil {
				t.Fatal("missing feature evidence")
			}
			if test.decision == Allow {
				if r.Recommendation == nil || r.Details.Project.Features.Status != "resolved" {
					t.Fatal(r)
				}
				if _, err := PrepareInstall(dir, r); !errors.Is(err, ErrInstallUnsupported) {
					t.Fatal("feature union enabled native install", err)
				}
			}
			if test.name == "inactive vulnerable crate" && !slices.Contains(r.Reasons, "KNOWN_VULNERABILITY") {
				t.Fatal("inactive crate excluded from security assessment")
			}
			if test.name == "inactive weak context" && !slices.Contains(r.Evidence.Missing, "CARGO_FEATURE_CONTEXT") {
				t.Fatal(r.Evidence)
			}
		})
	}
}
