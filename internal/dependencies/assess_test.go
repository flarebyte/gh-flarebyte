package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

var checked = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func versionFixture(t *testing.T, raw string) Version {
	t.Helper()
	var v Version
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEvaluateVersion(t *testing.T) {
	base := Version{Key: VersionKey{System: "NPM", Name: "pkg", Version: "1.2.3"}, PublishedAt: checked.Add(-14 * 24 * time.Hour)}
	for _, tt := range []struct {
		name            string
		change          func(*Version)
		reason, missing string
	}{
		{"boundary", func(*Version) {}, "", ""},
		{"too new", func(v *Version) { v.PublishedAt = v.PublishedAt.Add(time.Second) }, "RELEASE_TOO_NEW", ""},
		{"future", func(v *Version) { v.PublishedAt = checked.Add(time.Second) }, "", "PUBLICATION_TIME"},
		{"missing time", func(v *Version) { v.PublishedAt = time.Time{} }, "", "PUBLICATION_TIME"},
		{"deprecated", func(v *Version) { v.Deprecated = true }, "DEPRECATED_RELEASE", ""},
		{"prerelease", func(v *Version) { v.Key.Version = "1.2.3-rc1" }, "PRERELEASE", ""},
		{"unsupported version", func(v *Version) { v.Key.Version = "invalid" }, "", "STABLE_VERSION_SEMANTICS"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := base
			tt.change(&v)
			f := EvaluateVersion(v, nil, nil, DefaultPolicy(14), checked)
			if tt.reason != "" && !slices.Contains(f.Reasons, tt.reason) {
				t.Fatal(f)
			}
			if tt.missing != "" && !slices.Contains(f.Missing, tt.missing) {
				t.Fatal(f)
			}
			if verdict(f.Reasons, f.Missing) == Allow {
				t.Fatal("unresolved coverage approved")
			}
		})
	}
	zero := DefaultPolicy(0)
	v := base
	v.PublishedAt = checked
	if slices.Contains(EvaluateVersion(v, nil, nil, zero, checked).Reasons, "RELEASE_TOO_NEW") {
		t.Fatal("zero age rejected")
	}
	v = versionFixture(t, `{"versionKey":{"system":"NPM","name":"pkg","version":"1.2.3"},"advisoryKeys":[{"id":"GHSA-one"},{"id":"MAL-one"},{"id":"withdrawn"},{"id":""}]}`)
	withdrawn := checked.Add(-time.Hour)
	items := []Advisory{{ID: "GHSA-one"}, {ID: "GHSA-alias", Aliases: []string{"MAL-two"}}, {ID: "withdrawn", Withdrawn: &withdrawn}, {}}
	f := EvaluateVersion(v, items, errors.New("partial"), DefaultPolicy(14), checked)
	for _, reason := range []string{"KNOWN_MALWARE", "KNOWN_VULNERABILITY"} {
		if !slices.Contains(f.Reasons, reason) {
			t.Fatal(f)
		}
	}
	for _, missing := range []string{"ADVISORY_SCHEMA", "OSV_LOOKUP", "ADVISORY_WITHDRAWAL_CONFLICT"} {
		if !slices.Contains(f.Missing, missing) {
			t.Fatal(f)
		}
	}
	if len(f.Advisories) != 3 {
		t.Fatal(f.Advisories)
	}
	if verdict(nil, nil) != Allow || verdict(nil, []string{"gap"}) != Unknown || verdict([]string{"bad"}, []string{"gap"}) != Deny {
		t.Fatal("verdict precedence")
	}
}

func fixtureClient(t *testing.T, mode string) *Client {
	t.Helper()
	return &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if mode == "offline" {
			return nil, errors.New("offline")
		}
		switch r.URL.Host {
		case "registry.npmjs.org":
			version := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			return response(`{"name":"pkg","version":"`+version+`","description":"A useful test library"}`, 200), nil
		case "api.osv.dev":
			if mode == "vulnerable" || mode == "pub" {
				return response(`{"vulns":[{"id":"GHSA-example"}]}`, 200), nil
			}
			return response(`{}`, 200), nil
		case "api.scorecard.dev":
			return response(`{"date":"2026-11-10T00:00:00Z","repo":{"name":"github.com/org/repo","commit":"new"},"score":6.6}`, 200), nil
		case "api.deps.dev":
			if strings.Contains(r.URL.Path, "/projects/") {
				return response(`{"projectKey":{"id":"github.com/org/repo"},"starsCount":4200,"scorecard":{"date":"2026-08-24T00:00:00Z","repository":{"name":"github.com/org/repo","commit":"old"},"overallScore":7.5}}`, 200), nil
			}
			if strings.Contains(r.URL.Path, "/versions/") {
				if mode == "versionfailure" {
					return response(`{}`, 503), nil
				}
				v := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				return response(`{"versionKey":{"system":"NPM","name":"pkg","version":"`+v+`"},"publishedAt":"2025-01-01T00:00:00Z","relatedProjects":[{"projectKey":{"id":"github.com/org/repo"},"relationType":"SOURCE_REPO","relationProvenance":"UNVERIFIED_METADATA"}]}`, 200), nil
			}
			switch mode {
			case "notfound":
				return response(`{}`, 404), nil
			case "schema":
				return response(`{}`, 200), nil
			case "conflict":
				return response(`{"packageKey":{"name":"other"},"versions":[{"versionKey":{"version":"1.0.0"}}]}`, 200), nil
			}
			return response(`{"packageKey":{"name":"pkg"},"versions":[{"versionKey":{"version":"1.0.0"}},{"versionKey":{"version":"1.10.0"}},{"versionKey":{"version":"1.2.0"}},{"versionKey":{"version":"2.0.0"}},{"versionKey":{"version":"3.0.0"}}]}`, 200), nil
		}
		t.Fatalf("unexpected provider %s", r.URL)
		return nil, errors.New("unexpected provider")
	})}
}

func TestAssessEvidence(t *testing.T) {
	for _, mode := range []string{"normal", "vulnerable", "offline", "notfound", "schema", "conflict", "versionfailure"} {
		t.Run(mode, func(t *testing.T) {
			v := "1.0.0"
			r := fixtureClient(t, mode).Assess(context.Background(), "npm", "pkg", &v, DefaultPolicy(14), checked)
			switch mode {
			case "vulnerable":
				if r.Decision != Deny || !slices.Contains(r.Reasons, "KNOWN_VULNERABILITY") {
					t.Fatal(r)
				}
			case "conflict":
				if r.Decision != Deny || r.Evidence.Identity != "conflict" {
					t.Fatal(r)
				}
			default:
				if r.Decision != Unknown {
					t.Fatal(r)
				}
			}
			if r.Recommendation != nil || len(r.Alternatives) != 0 {
				t.Fatal("incomplete evidence recommended")
			}
			if mode == "normal" {
				if r.Description == nil || *r.Description != "A useful test library" || r.Evidence.Stars == nil || *r.Evidence.Stars != 4200 || r.Evidence.Identity != "unverified" || len(r.Details.Scorecards) != 2 {
					t.Fatalf("%+v", r)
				}
				if r.Details.Candidates[0].Version != v {
					t.Fatal("requested version lost")
				}
			}
		})
	}
	c := fixtureClient(t, "normal")
	r := c.Assess(context.Background(), "npm", "pkg", nil, DefaultPolicy(14), checked)
	if !r.Details.LimitReached || r.Details.Candidates[0].Version != "3.0.0" || len(r.Details.Candidates) != 4 {
		t.Fatal(r.Details)
	}
	v := "123"
	r = c.Assess(context.Background(), "npm", "pkg", &v, DefaultPolicy(14), checked)
	if r.Error == nil || r.Error.Code != "VERSION_NOT_FOUND" {
		t.Fatal(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = c.Assess(ctx, "npm", "pkg", nil, DefaultPolicy(14), checked)
	if !r.Details.LimitReached || r.Decision == Allow {
		t.Fatal(r)
	}
}

func TestUnsupportedCoverage(t *testing.T) {
	for _, system := range []string{"pub", "swift"} {
		for _, mode := range []string{"normal", "pub"} {
			v := "3.3.0"
			c := fixtureClient(t, mode)
			r := c.Assess(context.Background(), system, "archive", &v, DefaultPolicy(14), checked)
			if !slices.Contains(r.Reasons, "UNSUPPORTED_COVERAGE") {
				t.Fatal(r)
			}
			if mode == "pub" && r.Decision != Deny {
				t.Fatal(r)
			}
			r = c.Assess(context.Background(), system, "archive", nil, DefaultPolicy(14), checked)
			if len(r.Details.Sources) != 0 || r.Decision != Unknown {
				t.Fatal(r)
			}
		}
	}
}

func TestRepositoryEvidenceBoundaries(t *testing.T) {
	for _, raw := range []string{
		`{"relatedProjects":[{"projectKey":{"id":"evil.test/org/repo"},"relationType":"SOURCE_REPO"}]}`,
		`{"relatedProjects":[{"projectKey":{"id":"github.com/org/repo"},"relationType":"ISSUE_TRACKER"}]}`,
		`{"relatedProjects":[{"projectKey":{"id":"github.com/org/repo"},"relationType":"SOURCE_REPO"}]}`,
	} {
		r := NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
		e := TrustEvidence{}
		fixtureClient(t, "offline").fillRepository(context.Background(), versionFixture(t, raw), &e, &r, map[string]TrustEvidence{})
		if e.Identity == "verified" {
			t.Fatal("borrowed reputation")
		}
	}
}
