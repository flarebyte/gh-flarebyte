package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func authorityClient(t *testing.T, mode string) *Client {
	t.Helper()
	return &Client{GitHubToken: func(context.Context) (string, error) { return "private-test-token", nil }, HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer private-test-token" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Fatal("unsafe GitHub request", r.URL)
		}
		if mode == "denied" {
			return response(`{"message":"private-test-token"}`, 403), nil
		}
		var value any
		switch {
		case strings.Contains(r.URL.Path, "/collaborators"):
			rows := []map[string]any{
				{"id": 1, "login": "first", "type": "User", "permissions": map[string]bool{"push": true}},
				{"id": 2, "login": "second", "type": "User", "permissions": map[string]bool{"push": true}},
				{"id": 3, "login": "third", "type": "User", "permissions": map[string]bool{"push": false}},
				{"id": 4, "login": "automation[bot]", "type": "Bot", "permissions": map[string]bool{"push": true}},
			}
			if mode == "single" {
				rows = rows[:1]
			}
			if mode == "badrole" {
				delete(rows[0], "permissions")
			}
			if mode == "badpermission" {
				rows[0]["permissions"] = map[string]bool{}
			}
			if mode == "duplicate" {
				rows[1]["id"] = 1
			}
			if mode == "rolelimit" || mode == "incomplete" {
				for len(rows) < 100 {
					rows = append(rows, rows[0])
				}
				if mode == "rolelimit" {
					rows = append(rows, rows[0])
				}
				if mode == "incomplete" {
					for i := range rows {
						rows[i] = rows[0]
					}
				}
			}
			value = rows
		case strings.Contains(r.URL.Path, "/reviews"):
			user := map[string]any{"id": 2, "login": "second", "type": "User"}
			row := map[string]any{"user": user, "state": "APPROVED", "submitted_at": checked.Add(-time.Hour)}
			if mode == "self" {
				user["id"] = 1
			}
			if mode == "bot" {
				user["type"] = "Bot"
			}
			if mode == "nonmaintainer" {
				user["id"] = 3
			}
			if mode == "future" {
				row["submitted_at"] = checked.Add(time.Hour)
			}
			if mode == "stale" {
				row["submitted_at"] = checked.AddDate(-1, 0, 0)
			}
			if mode == "comment" {
				row["state"] = "COMMENTED"
			}
			if mode == "changes" {
				row["state"] = "CHANGES_REQUESTED"
			}
			rows := []map[string]any{row}
			if mode == "reviewlimit" || mode == "reviewpages" {
				for len(rows) < 100 {
					rows = append(rows, row)
				}
				if mode == "reviewlimit" {
					rows = append(rows, row)
				} else {
					user["id"] = 1
				}
			}
			value = rows
		default:
			rows := []map[string]any{{"number": 1, "user": map[string]any{"id": 1, "login": "first", "type": "User"}, "created_at": checked.Add(-time.Hour), "updated_at": checked.Add(-time.Hour)}}
			if mode == "oldpull" {
				rows[0]["updated_at"] = checked.AddDate(-1, 0, 0)
			}
			if mode == "futurepull" {
				rows[0]["updated_at"] = checked.Add(time.Hour)
			}
			if mode == "badpull" {
				rows[0]["number"] = 0
			}
			if mode == "twopulls" {
				rows = append(rows, map[string]any{"number": 2, "user": map[string]any{"id": 2, "type": "User"}, "created_at": checked.Add(-time.Hour), "updated_at": checked.Add(-time.Hour)})
			}
			if mode == "pulllimit" {
				for len(rows) <= 10 {
					rows = append(rows, rows[0])
				}
			}
			value = rows
		}
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return response(string(b), 200), nil
	})}
}

func TestGitHubMaintenanceAuthority(t *testing.T) {
	for _, mode := range []string{"normal", "changes", "single", "badrole", "badpermission", "duplicate", "rolelimit", "incomplete", "self", "bot", "nonmaintainer", "future", "stale", "comment", "oldpull", "futurepull", "badpull", "twopulls", "pulllimit", "reviewlimit", "reviewpages", "denied"} {
		t.Run(mode, func(t *testing.T) {
			c := authorityClient(t, mode)
			sources := []Source{}
			count, err := c.maintainerCount(context.Background(), "github.com/org/repo", DefaultPolicy(14), checked, &sources)
			pass := mode == "normal" || mode == "changes" || mode == "twopulls"
			if pass && (err != nil || count == nil || *count != 2) {
				t.Fatal(count, err)
			}
			if !pass && (err == nil || count != nil) {
				t.Fatalf("unproven maintenance accepted: %v %v", count, err)
			}
			if err != nil && strings.Contains(err.Error(), "private-test-token") {
				t.Fatal("credential leaked")
			}
		})
	}
	sources := []Source{}
	if _, err := authorityClient(t, "normal").maintainerCount(context.Background(), "evil.test/org/repo", DefaultPolicy(14), checked, &sources); err == nil {
		t.Fatal("foreign repo accepted")
	}
}

func TestGitHubCredentialBoundary(t *testing.T) {
	for _, mode := range []string{"none", "error", "empty", "newline", "oversized", "wronghost", "post"} {
		t.Run(mode, func(t *testing.T) {
			c := authorityClient(t, "normal")
			c.GitHubToken = func(context.Context) (string, error) {
				switch mode {
				case "error":
					return "", errors.New("private-token")
				case "empty":
					return "", nil
				case "newline":
					return "secret\n", nil
				case "oversized":
					return strings.Repeat("x", 4097), nil
				}
				return "secret", nil
			}
			if mode == "none" {
				c.GitHubToken = nil
			}
			sources := []Source{}
			endpoint := "https://api.github.com/repos/org/repo/collaborators"
			if mode == "wronghost" {
				endpoint = "https://evil.test/path"
			}
			var body any
			if mode == "post" {
				body = map[string]string{"x": "y"}
			}
			var out githubArray[githubCollaborator]
			if err := c.read(context.Background(), "github", endpoint, body, &out, &sources); err == nil || strings.Contains(err.Error(), "private-token") {
				t.Fatal(err)
			}
		})
	}
	c := authorityClient(t, "normal")
	c.HTTP = doFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("token sent to another provider")
		}
		return response(`{}`, 200), nil
	})
	sources := []Source{}
	_, _ = c.Package(context.Background(), "npm", "pkg", &sources)
	var out githubArray[githubCollaborator]
	if json.Unmarshal([]byte(`{}`), &out) == nil || json.Unmarshal([]byte(`[ {"id":1,"id":2} ]`), &out) == nil {
		t.Fatal("invalid role evidence accepted")
	}
}

func TestMaintenanceEvidenceCache(t *testing.T) {
	c := authorityClient(t, "normal")
	r := NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
	cache := map[string]TrustEvidence{}
	e := TrustEvidence{Identity: "unverified", Repository: "github.com/org/repo"}
	c.fillMaintainers(context.Background(), &e, DefaultPolicy(14), checked, &r, cache)
	if len(r.Details.Sources) != 0 {
		t.Fatal("unverified identity borrowed authority")
	}
	e.Identity = "verified"
	c.fillMaintainers(context.Background(), &e, DefaultPolicy(14), checked, &r, cache)
	if e.IndependentMaintainers == nil {
		t.Fatal("authority not populated")
	}
	before := len(r.Details.Sources)
	c.fillMaintainers(context.Background(), &e, DefaultPolicy(14), checked, &r, cache)
	if len(r.Details.Sources) != before {
		t.Fatal("authority cache missed")
	}
}
