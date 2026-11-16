package dependencies

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestRegistryReleaseEvidence(t *testing.T) {
	for _, tt := range []struct {
		system, body     string
		status           int
		known, withdrawn bool
		reason           string
	}{
		{"npm", `{"name":"pkg","version":"1.0.0","description":"package"}`, 200, true, false, ""},
		{"npm", `{"name":"pkg","version":"1.0.0","deprecated":"abandoned"}`, 200, true, false, "DEPRECATED_RELEASE"},
		{"npm", `{"name":"other","version":"1.0.0"}`, 200, false, false, "IDENTITY_CONFLICT"},
		{"npm", `{}`, 200, false, false, ""},
		{"npm", `{}`, 404, true, true, ""},
		{"npm", `{}`, 503, false, false, ""},
		{"pypi", `{"info":{"name":"PKG","version":"1.0.0","summary":"package"},"urls":[{"filename":"pkg.whl","yanked":false}]}`, 200, true, false, ""},
		{"pypi", `{"info":{"name":"pkg","version":"1.0.0"},"urls":[{"filename":"pkg.whl","yanked":true}]}`, 200, true, true, ""},
		{"pypi", `{"info":{"name":"pkg","version":"1.0.0"},"urls":[{"yanked":true}]}`, 200, false, true, ""},
		{"pypi", `{"info":{"name":"pkg","version":"1.0.0"}}`, 200, false, false, ""},
		{"pypi", `{"info":{"name":"other","version":"1.0.0"}}`, 200, false, false, "IDENTITY_CONFLICT"},
		{"pypi", `{}`, 200, false, false, ""},
		{"cargo", `{"version":{"crate":"pkg","num":"1.0.0","yanked":false,"description":"package"}}`, 200, true, false, ""},
		{"cargo", `{"version":{"crate":"pkg","num":"1.0.0","yanked":true}}`, 200, true, true, ""},
		{"cargo", `{"version":{"crate":"pkg","num":"1.0.0"}}`, 200, false, false, ""},
		{"cargo", `{"version":{"crate":"other","num":"1.0.0"}}`, 200, false, false, "IDENTITY_CONFLICT"},
		{"cargo", `{}`, 200, false, false, ""},
		{"go", `{}`, 200, false, false, ""},
	} {
		t.Run(tt.system+tt.body+http.StatusText(tt.status), func(t *testing.T) {
			c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
				if !strings.Contains(r.URL.Path, "pkg") || !strings.Contains(r.URL.Path, "1.0.0") {
					t.Fatal(r.URL)
				}
				return response(tt.body, tt.status), nil
			})}
			sources := []Source{}
			e, _, _, reasons := c.releaseEvidence(context.Background(), tt.system, "pkg", "1.0.0", &sources)
			if e.LifecycleKnown != tt.known || e.Withdrawn != tt.withdrawn || tt.reason != "" && !slices.Contains(reasons, tt.reason) {
				t.Fatalf("%+v %v", e, reasons)
			}
			if tt.reason == "" && len(reasons) > 0 {
				t.Fatal(reasons)
			}
		})
	}
	if !equivalentName("cargo", "test_crate", "Test-Crate") || !equivalentName("pypi", "Foo.BAR", "foo-bar") || equivalentName("npm", "Foo", "foo") {
		t.Fatal("registry normalization")
	}
}
