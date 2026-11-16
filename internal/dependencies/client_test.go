package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type doFunc func(*http.Request) (*http.Response, error)

func (f doFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errors.New("read error") }

func TestBoundedHTTP(t *testing.T) {
	client := NewClient()
	h := client.HTTP.(*http.Client)
	if h.Timeout <= 0 || h.CheckRedirect(nil, nil) == nil {
		t.Fatal("unsafe HTTP defaults")
	}
	for _, tt := range []struct {
		name, body string
		status     int
		failure    bool
	}{
		{"ok", "{}", 200, false}, {"not found", "secret", 404, true}, {"rate limit", "secret", 429, true},
		{"bad json", "{", 200, true}, {"null", "null", 200, true}, {"array", "[]", 200, true}, {"empty", "", 200, true},
		{"duplicate fields", `{"verified":false,"verified":true}`, 200, true},
		{"oversized", strings.Repeat(" ", maxBody+1), 200, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{HTTP: doFunc(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("User-Agent") == "" || req.Header.Get("Accept") != "application/json" {
					t.Fatal("missing headers")
				}
				return response(tt.body, tt.status), nil
			})}
			sources := []Source{}
			var out map[string]any
			err := c.read(context.Background(), "test", "https://example.test", nil, &out, &sources)
			if (err != nil) != tt.failure {
				t.Fatal(err)
			}
			if len(sources) != 1 {
				t.Fatal(sources)
			}
			if tt.failure && sources[0].Status == "ok" {
				t.Fatal("failure recorded as success")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("body leaked")
			}
		})
	}
	sources := []Source{}
	c := &Client{HTTP: doFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret token") })}
	if _, err := c.Package(context.Background(), "npm", "pkg", &sources); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(failReader{})}, nil
	})
	if _, err := c.Package(context.Background(), "npm", "pkg", &sources); err == nil {
		t.Fatal("read error lost")
	}
	if err := c.read(context.Background(), "test", ":bad", nil, &struct{}{}, &sources); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if err := c.read(context.Background(), "test", "https://example.test", make(chan int), &struct{}{}, &sources); err == nil {
		t.Fatal("marshal error lost")
	}
	if _, err := c.Scorecard(context.Background(), "github.com/org/repo/../../evil", &sources); err == nil {
		t.Fatal("unsafe repo accepted")
	}
}

func TestOSVPagination(t *testing.T) {
	sources := []Source{}
	if _, err := NewClient().OSV(nil, "npm", "pkg", "1.0.0", &sources); err == nil {
		t.Fatal("nil context accepted")
	}
	for _, mode := range []string{"success", "repeat", "limit", "failure"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			sources := []Source{}
			c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.String() != "https://api.osv.dev/v1/query" || r.Header.Get("Content-Type") != "application/json" {
					t.Fatal(r)
				}
				var query map[string]any
				if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
					t.Fatal(err)
				}
				if query["version"] != "1.0.0" {
					t.Fatal(query)
				}
				if calls == 1 {
					return response(`{"vulns":[{"id":"GHSA-test"}],"next_page_token":"next"}`, 200), nil
				}
				switch mode {
				case "success":
					return response(`{}`, 200), nil
				case "failure":
					return response(``, 503), nil
				case "repeat":
					return response(`{"next_page_token":"next"}`, 200), nil
				default:
					return response(`{"next_page_token":"page`+string(rune('a'+calls))+`"}`, 200), nil
				}
			})}
			items, err := c.OSV(context.Background(), "npm", "pkg", "1.0.0", &sources)
			if len(items) != 1 || (err == nil) != (mode == "success") {
				t.Fatalf("%+v %v", items, err)
			}
			if mode == "limit" && calls != 8 {
				t.Fatal(calls)
			}
		})
	}
}

func TestProviderPathsAndDescriptions(t *testing.T) {
	for _, tt := range []struct{ system, name, url, body, description string }{
		{"npm", "@scope/pkg", "https://registry.npmjs.org/@scope%2Fpkg/latest", `{"description":"hello"}`, "hello"},
		{"pypi", "pkg", "https://pypi.org/pypi/pkg/json", `{"info":{"summary":"summary"}}`, "summary"},
		{"cargo", "pkg", "https://crates.io/api/v1/crates/pkg", `{"crate":{"description":"crate"}}`, "crate"},
	} {
		t.Run(tt.system, func(t *testing.T) {
			c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != tt.url {
					t.Fatalf("%s != %s", r.URL, tt.url)
				}
				return response(tt.body, 200), nil
			})}
			sources := []Source{}
			d := c.registryDescription(context.Background(), tt.system, tt.name, nil, &sources)
			if d == nil || *d != tt.description {
				t.Fatal(d)
			}
		})
	}
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		return response(`{"description":"hello","info":{"summary":"summary"}}`, 200), nil
	})}
	sources := []Source{}
	v := "1.2.3"
	for _, s := range []string{"npm", "pypi"} {
		if c.registryDescription(context.Background(), s, "pkg", &v, &sources) == nil {
			t.Fatal(s)
		}
	}
	if c.registryDescription(context.Background(), "go", "pkg", nil, &sources) != nil {
		t.Fatal("unsupported description")
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if c.registryDescription(context.Background(), "npm", "pkg", nil, &sources) != nil {
		t.Fatal("failed description")
	}
	c.HTTP = doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.deps.dev" && r.URL.Host != "api.scorecard.dev" {
			t.Fatal(r.URL)
		}
		return response(`{}`, 200), nil
	})
	_, _ = c.Version(context.Background(), "npm", "@scope/pkg", "1.2.3", &sources)
	if !strings.Contains(sources[len(sources)-1].URL, "@scope%2Fpkg") {
		t.Fatal(sources)
	}
	_, _ = c.Project(context.Background(), "github.com/org/repo", &sources)
	_, _ = c.Scorecard(context.Background(), "github.com/org/repo", &sources)
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	sources := []Source{}
	if _, err := c.Package(ctx, "npm", "pkg", &sources); err == nil {
		t.Fatal("cancel ignored")
	}
	if sources[0].RetrievedAt.After(time.Now()) {
		t.Fatal("invalid retrieval time")
	}
}
