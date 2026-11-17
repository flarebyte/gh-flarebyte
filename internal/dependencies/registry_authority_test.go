package dependencies

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func registryRoleClient(t *testing.T, body string, status int) (*Client, *int) {
	t.Helper()
	calls := 0
	c := &Client{GitHubToken: func(context.Context) (string, error) { t.Fatal("public roster loaded credentials"); return "", nil }, HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Scheme != "https" || r.Header.Get("Authorization") != "" || r.URL.Host != "registry.npmjs.org" && r.URL.Host != "pypi.org" {
			t.Fatal("unsafe roster request", r.URL)
		}
		if status == 0 {
			return nil, errors.New("private diagnostic must not escape")
		}
		return response(body, status), nil
	})}
	return c, &calls
}

func TestPublicRegistryRoles(t *testing.T) {
	for _, test := range []struct {
		name, system, body string
		count              int
		organization       string
	}{
		{"npm", "npm", `{"name":"pkg","maintainers":[{"name":"second","email":"do-not-report@example.test"},{"name":"first"}],"versions":{"1.0.0":{"maintainers":[{"name":"untrusted"}]}}}`, 2, ""},
		{"npm aliases", "npm", `{"name":"pkg","maintainers":[{"name":"First"},{"name":"first"},{"name":"second"}]}`, 2, ""},
		{"npm ignores credits", "npm", `{"name":"pkg","author":"invented","_npmUser":{"name":"unproven"},"maintainers":[{"name":"first"}]}`, 1, ""},
		{"pypi", "pypi", `{"info":{"name":"PKG","author":"invented","maintainer":"not-a-role"},"ownership":{"organization":null,"roles":[{"role":"Owner","user":"second"},{"role":"Maintainer","user":"first"}]}}`, 2, ""},
		{"pypi duplicates", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[{"role":"Owner","user":"First"},{"role":"Owner","user":"first"}]}}`, 1, ""},
		{"organization only", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":"example-org","roles":[]}}`, -1, "example-org"},
		{"organization and users", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":"example-org","roles":[{"role":"Owner","user":"first"}]}}`, 1, "example-org"},
		{"empty npm roster", "npm", `{"name":"pkg","maintainers":[]}`, -1, ""},
		{"empty pypi roster", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[]}}`, -1, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, calls := registryRoleClient(t, test.body, 200)
			sources := []Source{}
			o := c.registryAuthorities(context.Background(), test.system, "pkg", &sources)
			if !o.known || *calls != 1 || len(sources) != 1 {
				t.Fatal(o, *calls, sources)
			}
			e := o.evidence()
			if e.ActiveAuthorityAccounts != nil {
				t.Fatal("roles became activity", e)
			}
			if test.count < 0 {
				if e.AuthorityAccounts != nil {
					t.Fatal("opaque/empty roles became zero authorities", e)
				}
			} else if e.AuthorityAccounts == nil || *e.AuthorityAccounts != test.count {
				t.Fatal(e)
			}
			d := o.detail()
			if d.Organization != test.organization || !slices.Contains(d.Missing, "REGISTRY_ACTIVE_AUTHORITY_UNSUPPORTED") {
				t.Fatal(d)
			}
			b, _ := json.Marshal(d)
			if strings.Contains(string(b), "do-not-report") || strings.Contains(string(b), "untrusted") || strings.Contains(string(b), "invented") || strings.Contains(string(b), "not-a-role") {
				t.Fatal("credits or emails leaked", string(b))
			}
		})
	}
}

func TestInvalidRegistryRoles(t *testing.T) {
	for _, test := range []struct{ name, system, body string }{
		{"npm identity", "npm", `{"name":"other","maintainers":[{"name":"first"}]}`},
		{"npm missing", "npm", `{"name":"pkg","versions":{"1.0.0":{"maintainers":[{"name":"first"}]}}}`},
		{"npm null", "npm", `{"name":"pkg","maintainers":null}`},
		{"npm bad account", "npm", `{"name":"pkg","maintainers":[{"name":"bad\naccount"}]}`},
		{"npm duplicate JSON", "npm", `{"name":"pkg","maintainers":[],"maintainers":[{"name":"first"}]}`},
		{"pypi credits only", "pypi", `{"info":{"name":"pkg","maintainer":"first"}}`},
		{"pypi identity", "pypi", `{"info":{"name":"other"},"ownership":{"organization":null,"roles":[]}}`},
		{"pypi missing roles", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null}}`},
		{"pypi missing org", "pypi", `{"info":{"name":"pkg"},"ownership":{"roles":[]}}`},
		{"pypi invalid org", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":{"name":"invented"},"roles":[]}}`},
		{"pypi unsafe org", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":"../other","roles":[]}}`},
		{"pypi invalid role", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[{"role":"Manager","user":"first"}]}}`},
		{"pypi bad user", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[{"role":"Owner","user":"../first"}]}}`},
		{"pypi conflicting aliases", "pypi", `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[{"role":"Owner","user":"first"},{"role":"Maintainer","user":"FIRST"}]}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := registryRoleClient(t, test.body, 200)
			sources := []Source{}
			o := c.registryAuthorities(context.Background(), test.system, "pkg", &sources)
			if o.known || o.evidence().AuthorityAccounts != nil || len(o.roles) != 0 {
				t.Fatal("invalid roster became evidence", o)
			}
		})
	}
	for _, system := range []string{"npm", "pypi"} {
		t.Run(system+" limit", func(t *testing.T) {
			users := []any{}
			for i := 0; i < 1001; i++ {
				if system == "npm" {
					users = append(users, map[string]string{"name": "first"})
				} else {
					users = append(users, map[string]string{"role": "Owner", "user": "first"})
				}
			}
			var payload any = map[string]any{"name": "pkg", "maintainers": users}
			if system == "pypi" {
				payload = map[string]any{"info": map[string]string{"name": "pkg"}, "ownership": map[string]any{"roles": users, "organization": nil}}
			}
			b, _ := json.Marshal(payload)
			c, _ := registryRoleClient(t, string(b), 200)
			sources := []Source{}
			if c.registryAuthorities(context.Background(), system, "pkg", &sources).known {
				t.Fatal("unbounded roster accepted")
			}
		})
	}
	for _, status := range []int{0, 403} {
		c, _ := registryRoleClient(t, `{"secret":"private diagnostic"}`, status)
		sources := []Source{}
		o := c.registryAuthorities(context.Background(), "npm", "pkg", &sources)
		b, _ := json.Marshal(o.detail())
		if o.known || strings.Contains(string(b), "private diagnostic") {
			t.Fatal(o)
		}
	}
	for _, name := range []string{"../bad", "pkg@1.0.0"} {
		c, calls := registryRoleClient(t, "", 200)
		sources := []Source{}
		if c.registryAuthorities(context.Background(), "npm", name, &sources).known || *calls != 0 {
			t.Fatal("invalid coordinate fetched")
		}
	}
}

func TestRegistryRolesCannotApproveMaintenance(t *testing.T) {
	for _, system := range []string{"npm", "pypi"} {
		t.Run(system, func(t *testing.T) {
			body := `{"name":"pkg","maintainers":[{"name":"first"},{"name":"second"},{"name":"third"}]}`
			if system == "pypi" {
				body = `{"info":{"name":"pkg"},"ownership":{"organization":null,"roles":[{"role":"Owner","user":"first"},{"role":"Owner","user":"second"}]}}`
			}
			c, calls := registryRoleClient(t, body, 200)
			p := DefaultPolicy(14)
			r := NewReport(system, "pkg", nil, p, checked)
			cache := map[string]TrustEvidence{}
			v := Version{Key: VersionKey{System: strings.ToUpper(system), Name: "pkg", Version: "1.0.0"}, PublishedAt: checked.AddDate(-1, 0, 0)}
			e := completeTrust()
			e.IndependentMaintainers = nil
			baseline := evaluateRelease(v, nil, nil, e, p, checked)
			// Keep repository identity absent during collection to exercise public
			// observations without requesting privileged GitHub APIs.
			observation := TrustEvidence{}
			for i := 0; i < 2; i++ {
				c.fillMaintenance(context.Background(), system, "pkg", &observation, p, checked, &r, cache)
			}
			e.registryAuthority = observation.registryAuthority
			f := evaluateRelease(v, nil, nil, e, p, checked)
			if *calls != 1 || len(r.Details.Maintenance) != 1 || !reflect.DeepEqual(baseline.Missing, f.Missing) || f.Decision != Unknown || f.Evidence.Maintainers != "unknown" || f.Evidence.Maintenance == nil || !slices.Contains(f.Missing, "ACTIVE_MAINTAINERS") {
				t.Fatal("roster changed maintenance verdict", f, *calls)
			}
			v.Key.Name = "other"
			if evaluateRelease(v, nil, nil, e, p, checked).Evidence.Maintenance != nil {
				t.Fatal("evidence borrowed across packages")
			}
		})
	}
}

func TestRegistryAuthorityCoordinates(t *testing.T) {
	for _, test := range []struct{ system, name, endpoint, body string }{
		{"npm", "@scope/pkg", "https://registry.npmjs.org/@scope%2Fpkg", `{"name":"@scope/pkg","maintainers":[{"name":"first"}]}`},
		{"pypi", "my-pkg", "https://pypi.org/pypi/my-pkg/json", `{"info":{"name":"My_Pkg"},"ownership":{"roles":[{"role":"Owner","user":"first"}],"organization":null}}`},
	} {
		t.Run(test.system, func(t *testing.T) {
			c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != test.endpoint {
					t.Fatal(r.URL)
				}
				return response(test.body, 200), nil
			})}
			sources := []Source{}
			if !c.registryAuthorities(context.Background(), test.system, test.name, &sources).known {
				t.Fatal("valid coordinate not recognized")
			}
		})
	}
	c, calls := registryRoleClient(t, "", 200)
	sources := []Source{}
	if c.registryAuthorities(context.Background(), "go", "example.com/module", &sources).known || *calls != 0 {
		t.Fatal("unsupported registry fetched")
	}
}
