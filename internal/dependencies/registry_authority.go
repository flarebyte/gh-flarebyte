// purpose: Report current npm/PyPI registry roles without inferring active maintenance.
// responsibilities: Validate package-scoped rosters, distinguish author credits and preserve opaque organization access.
// architecture notes: Observations never populate authority/activity approval proofs or change policy decisions.
package dependencies

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type registryAuthorityObservation struct {
	system, name, basis, organization string
	roles                             []RegistryRole
	known                             bool
	opaqueAuthority                   bool
	missing                           []string
}

func (o *registryAuthorityObservation) evidence() *MaintenanceEvidence {
	e := &MaintenanceEvidence{Basis: o.basis}
	if o.known && len(o.roles) > 0 {
		n := len(o.roles)
		e.AuthorityAccounts = &n
	}
	// Even a valid current roster establishes no matched recent activity.
	return e
}

func (o *registryAuthorityObservation) detail() MaintenanceDetail {
	return MaintenanceDetail{Package: o.system + ":" + o.name, Evidence: *o.evidence(), Authorities: []MaintenanceAuthority{}, Activities: []MaintenanceActivity{}, Missing: append([]string{}, o.missing...), RegistryRoles: append([]RegistryRole{}, o.roles...), Organization: o.organization}
}

func (c *Client) registryAuthorities(ctx context.Context, system, name string, sources *[]Source) *registryAuthorityObservation {
	o := &registryAuthorityObservation{system: system, name: name, basis: system + "-registry-roles", roles: []RegistryRole{}, missing: []string{"REGISTRY_ACTIVE_AUTHORITY_UNSUPPORTED"}}
	fail := func(code string) *registryAuthorityObservation { o.missing = unique(append(o.missing, code)); return o }
	parsed, version, err := Parse(system, name)
	if err != nil || version != nil || parsed != name || system != "npm" && system != "pypi" && system != "cargo" && system != "pub" {
		return fail("REGISTRY_AUTHORITY_UNSUPPORTED")
	}
	var roles []RegistryRole
	switch system {
	case "pub":
		o.basis = "pub-publisher-context"
		var body struct {
			PublisherID *string `json:"publisherId"`
		}
		if c.read(ctx, "pub-publisher", "https://pub.dev/api/packages/"+url.PathEscape(name)+"/publisher", nil, &body, sources) != nil {
			return fail("REGISTRY_AUTHORITY_LOOKUP")
		}
		if body.PublisherID != nil && len(*body.PublisherID) <= 253 && pubPublisherID.MatchString(*body.PublisherID) {
			o.organization = *body.PublisherID
		}
		return fail("PUB_MAINTAINER_COUNT_UNKNOWN")
	case "cargo":
		o.basis = "cargo-registry-owners"
		var body struct {
			Users *[]cargoOwner `json:"users"`
		}
		if c.read(ctx, "cargo-authority", "https://crates.io/api/v1/crates/"+url.PathEscape(name)+"/owners", nil, &body, sources) != nil {
			return fail("REGISTRY_AUTHORITY_LOOKUP")
		}
		if body.Users == nil || len(*body.Users) > 1000 {
			return fail("REGISTRY_AUTHORITY_SCHEMA")
		}
		seen := map[string]bool{}
		logins := map[string]int64{}
		for _, owner := range *body.Users {
			key := owner.Kind + ":" + strconv.FormatInt(owner.ID, 10)
			if owner.ID <= 0 || owner.Login == "" || len(owner.Login) > 128 || seen[key] || owner.Kind != "user" && owner.Kind != "team" {
				return fail("REGISTRY_AUTHORITY_SCHEMA")
			}
			seen[key] = true
			if owner.Kind == "team" {
				o.opaqueAuthority = true
				o.missing = unique(append(o.missing, "REGISTRY_TEAM_MEMBERS_UNKNOWN"))
				continue
			}
			login := strings.ToLower(owner.Login)
			if previous, exists := logins[login]; exists && previous != owner.ID {
				return fail("REGISTRY_AUTHORITY_SCHEMA")
			}
			logins[login] = owner.ID
			roles = append(roles, RegistryRole{Account: owner.Login, Role: "Owner"})
		}
	case "npm":
		o.basis = "npm-registry-owners"
		var body struct {
			Name        string `json:"name"`
			Maintainers *[]struct {
				Name string `json:"name"`
			} `json:"maintainers"`
		}
		if err := c.read(ctx, "npm-authority", "https://registry.npmjs.org/"+url.PathEscape(name), nil, &body, sources); err != nil {
			return fail("REGISTRY_AUTHORITY_LOOKUP")
		}
		if body.Name != name {
			return fail("REGISTRY_AUTHORITY_IDENTITY")
		}
		if body.Maintainers == nil || len(*body.Maintainers) > 1000 {
			return fail("REGISTRY_AUTHORITY_SCHEMA")
		}
		for _, user := range *body.Maintainers {
			roles = append(roles, RegistryRole{Account: user.Name, Role: "Owner"})
		}
	case "pypi":
		var body struct {
			Info struct {
				Name string `json:"name"`
			} `json:"info"`
			Ownership *struct {
				Roles *[]struct {
					Role string `json:"role"`
					User string `json:"user"`
				} `json:"roles"`
				Organization json.RawMessage `json:"organization"`
			} `json:"ownership"`
		}
		if err := c.read(ctx, "pypi-authority", "https://pypi.org/pypi/"+url.PathEscape(name)+"/json", nil, &body, sources); err != nil {
			return fail("REGISTRY_AUTHORITY_LOOKUP")
		}
		if !equivalentName("pypi", body.Info.Name, name) || body.Info.Name == "" {
			return fail("REGISTRY_AUTHORITY_IDENTITY")
		}
		if body.Ownership == nil || body.Ownership.Roles == nil || len(*body.Ownership.Roles) > 1000 || len(body.Ownership.Organization) == 0 {
			return fail("REGISTRY_AUTHORITY_SCHEMA")
		}
		if !bytes.Equal(body.Ownership.Organization, []byte("null")) {
			if json.Unmarshal(body.Ownership.Organization, &o.organization) != nil || !registryAccount(o.organization) {
				o.organization = ""
				return fail("REGISTRY_AUTHORITY_SCHEMA")
			}
		}
		for _, user := range *body.Ownership.Roles {
			if user.Role != "Owner" && user.Role != "Maintainer" {
				return fail("REGISTRY_AUTHORITY_SCHEMA")
			}
			roles = append(roles, RegistryRole{Account: user.User, Role: user.Role})
		}
	}
	normalized := map[string]RegistryRole{}
	for _, role := range roles {
		if !registryAccount(role.Account) {
			return fail("REGISTRY_AUTHORITY_SCHEMA")
		}
		key := strings.ToLower(role.Account)
		if previous, exists := normalized[key]; exists {
			if previous.Role != role.Role {
				return fail("REGISTRY_AUTHORITY_SCHEMA")
			}
			// Stable output even when duplicate aliases arrive in another order.
			if previous.Account < role.Account {
				continue
			}
		}
		normalized[key] = role
	}
	for _, role := range normalized {
		o.roles = append(o.roles, role)
	}
	sort.Slice(o.roles, func(i, j int) bool { return strings.ToLower(o.roles[i].Account) < strings.ToLower(o.roles[j].Account) })
	o.known = true
	if len(o.roles) == 0 {
		if o.organization != "" || o.opaqueAuthority {
			return fail("REGISTRY_ORGANIZATION_AUTHORITY")
		}
		return fail("REGISTRY_AUTHORITY_ROSTER_EMPTY")
	}
	return o
}

func registryAccount(name string) bool { return len(name) <= 128 && simpleName.MatchString(name) }
