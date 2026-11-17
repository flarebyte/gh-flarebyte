// purpose: Establish active crate release authority from public registry-issued account IDs.
// responsibilities: Match current user owners to bounded recent uploads, preserving incomplete evidence as unknown.
// architecture notes: No usernames are joined across providers; teams and unrelated publishers never qualify.
package dependencies

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"
)

const publicMaintenancePolicy = "conservative-v2"
const cargoMaintenanceBasis = "cargo-registry-publishers"

type cargoOwner struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Kind  string `json:"kind"`
}
type cargoPublication struct {
	ID        int64     `json:"id"`
	Crate     string    `json:"crate"`
	Version   string    `json:"num"`
	CreatedAt time.Time `json:"created_at"`
	Publisher *struct {
		ID int64 `json:"id"`
	} `json:"published_by"`
}

type cargoMaintenanceProof struct {
	name, policyDigest                   string
	checkedAt                            time.Time
	owners                               []MaintenanceAuthority
	activities                           []MaintenanceActivity
	ownersKnown, activitiesKnown, sealed bool
	missing                              []string
}

func (p *cargoMaintenanceProof) evidence(policy Policy, now time.Time) (*MaintenanceEvidence, bool) {
	if p == nil || policy.ID != publicMaintenancePolicy || p.policyDigest != policy.Digest() {
		return nil, false
	}
	e := &MaintenanceEvidence{Basis: cargoMaintenanceBasis}
	if p.ownersKnown {
		n := len(p.owners)
		e.AuthorityAccounts = &n
	}
	active := map[int64]bool{}
	owners := map[int64]bool{}
	for _, owner := range p.owners {
		owners[owner.ID] = true
	}
	for _, a := range p.activities {
		if owners[a.OwnerID] && !a.PublishedAt.Before(now.AddDate(0, 0, -policy.ActivityDays)) && !a.PublishedAt.After(now) {
			active[a.OwnerID] = true
		}
	}
	if p.activitiesKnown {
		n := len(active)
		e.ActiveAuthorityAccounts = &n
	}
	ok := p.sealed && p.ownersKnown && p.activitiesKnown && len(active) >= policy.MinMaintainers && policy.MinMaintainers >= 2 && policy.ActivityDays > 0 && !p.checkedAt.IsZero() && !p.checkedAt.After(now) && now.Sub(p.checkedAt) <= 24*time.Hour
	return e, ok
}

func (p *cargoMaintenanceProof) detail() MaintenanceDetail {
	e, _ := p.evidencePolicySnapshot()
	return MaintenanceDetail{Package: "cargo:" + p.name, Evidence: e, Authorities: append([]MaintenanceAuthority{}, p.owners...), Activities: append([]MaintenanceActivity{}, p.activities...), Missing: append([]string{}, p.missing...)}
}

func (p *cargoMaintenanceProof) evidencePolicySnapshot() (MaintenanceEvidence, bool) {
	// Display observed counts only; authorization always re-evaluates the sealed
	// records against the caller's exact policy and time.
	e := MaintenanceEvidence{Basis: cargoMaintenanceBasis}
	if p.ownersKnown {
		n := len(p.owners)
		e.AuthorityAccounts = &n
	}
	if p.activitiesKnown {
		ids := map[int64]bool{}
		for _, a := range p.activities {
			ids[a.OwnerID] = true
		}
		n := len(ids)
		e.ActiveAuthorityAccounts = &n
	}
	return e, p.sealed
}

func (c *Client) cargoOwners(ctx context.Context, name string, sources *[]Source) ([]MaintenanceAuthority, error) {
	var body struct {
		Users *[]cargoOwner `json:"users"`
	}
	if err := c.read(ctx, "cargo-maintenance", "https://crates.io/api/v1/crates/"+url.PathEscape(name)+"/owners", nil, &body, sources); err != nil {
		return nil, err
	}
	if body.Users == nil || len(*body.Users) > 1000 {
		return nil, fmt.Errorf("incomplete Cargo owner roster")
	}
	owners := []MaintenanceAuthority{}
	seen := map[string]bool{}
	logins := map[string]int64{}
	for _, owner := range *body.Users {
		key := fmt.Sprintf("%s:%d", owner.Kind, owner.ID)
		if owner.ID <= 0 || owner.Login == "" || len(owner.Login) > 128 || seen[key] || owner.Kind != "user" && owner.Kind != "team" {
			return nil, fmt.Errorf("invalid Cargo owner record")
		}
		seen[key] = true
		if owner.Kind == "team" || strings.HasSuffix(strings.ToLower(owner.Login), "[bot]") {
			continue
		}
		if !simpleName.MatchString(owner.Login) {
			return nil, fmt.Errorf("invalid Cargo owner login")
		}
		login := strings.ToLower(owner.Login)
		if id, exists := logins[login]; exists && id != owner.ID {
			return nil, fmt.Errorf("ambiguous Cargo owner identity")
		}
		logins[login] = owner.ID
		owners = append(owners, MaintenanceAuthority{ID: owner.ID, Login: owner.Login})
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i].ID < owners[j].ID })
	return owners, nil
}

func (c *Client) cargoMaintainers(ctx context.Context, name string, policy Policy, now time.Time, sources *[]Source) *cargoMaintenanceProof {
	p := &cargoMaintenanceProof{name: name, policyDigest: policy.Digest(), checkedAt: now, missing: []string{}}
	fail := func(code string) *cargoMaintenanceProof { p.missing = unique(append(p.missing, code)); return p }
	if policy.ID != publicMaintenancePolicy || !cargoName.MatchString(name) {
		return fail("CARGO_MAINTENANCE_UNSUPPORTED")
	}
	owners, err := c.cargoOwners(ctx, name, sources)
	if err != nil {
		return fail("CARGO_OWNER_LOOKUP")
	}
	p.owners, p.ownersKnown = owners, true
	if len(owners) < policy.MinMaintainers {
		return fail("CARGO_AUTHORITY_ACCOUNTS")
	}
	var body struct {
		Versions *[]cargoPublication `json:"versions"`
	}
	// The public API supports seek pagination, not numeric page= parameters.
	// A bounded first page supplies positive evidence only, never an absence claim.
	endpoint := "https://crates.io/api/v1/crates/" + url.PathEscape(name) + "/versions?per_page=100&sort=date"
	if err := c.read(ctx, "cargo-maintenance", endpoint, nil, &body, sources); err != nil {
		return fail("CARGO_PUBLISHER_LOOKUP")
	}
	if body.Versions == nil || len(*body.Versions) > 100 {
		return fail("CARGO_PUBLISHER_SCHEMA")
	}
	seenIDs := map[int64]bool{}
	seenVersions := map[string]bool{}
	ownerIDs := map[int64]bool{}
	for _, owner := range owners {
		ownerIDs[owner.ID] = true
	}
	activities := []MaintenanceActivity{}
	for _, v := range *body.Versions {
		_, stable := stableVersion("cargo", v.Version)
		if v.ID <= 0 || seenIDs[v.ID] || seenVersions[v.Version] || v.Crate != name || !stable && !knownPrerelease("cargo", v.Version) || v.CreatedAt.IsZero() || v.CreatedAt.After(now) {
			return fail("CARGO_PUBLISHER_SCHEMA")
		}
		seenIDs[v.ID], seenVersions[v.Version] = true, true
		if v.Publisher == nil {
			continue
		}
		if v.Publisher.ID <= 0 {
			return fail("CARGO_PUBLISHER_SCHEMA")
		}
		if !ownerIDs[v.Publisher.ID] || v.CreatedAt.Before(now.AddDate(0, 0, -policy.ActivityDays)) {
			continue
		}
		activities = append(activities, MaintenanceActivity{OwnerID: v.Publisher.ID, Version: v.Version, PublishedAt: v.CreatedAt})
	}
	p.activities, p.activitiesKnown = activities, true
	// Re-read authority after activity retrieval. Revoked or changed-to-team
	// accounts cannot remain eligible through a cached first roster.
	current, err := c.cargoOwners(ctx, name, sources)
	if err != nil {
		return fail("CARGO_OWNER_RECHECK")
	}
	p.owners = current
	currentIDs := map[int64]bool{}
	for _, owner := range current {
		currentIDs[owner.ID] = true
	}
	p.activities = nil
	for _, a := range activities {
		if currentIDs[a.OwnerID] {
			p.activities = append(p.activities, a)
		}
	}
	sort.Slice(p.activities, func(i, j int) bool {
		a, b := p.activities[i], p.activities[j]
		if a.OwnerID != b.OwnerID {
			return a.OwnerID < b.OwnerID
		}
		return a.Version < b.Version
	})
	p.sealed = true
	if _, ok := p.evidence(policy, now); !ok {
		return fail("CARGO_ACTIVE_AUTHORITY_EVIDENCE")
	}
	return p
}

func validCargoMaintenanceFinding(f Finding, name string, report Report) bool {
	if f.cargoMaintenance == nil {
		return f.Evidence == nil || f.Evidence.Maintenance == nil
	}
	if f.Evidence == nil {
		return false
	}
	p := report.Details.Policy
	observed, ok := f.cargoMaintenance.evidence(p, report.CheckedAt)
	return ok && f.cargoMaintenance.name == name && p.ID == report.Policy && p.Digest() == report.PolicyDigest && reflect.DeepEqual(observed, f.Evidence.Maintenance)
}

func (p *cargoMaintenanceProof) clone() *cargoMaintenanceProof {
	if p == nil {
		return nil
	}
	frozen := *p
	frozen.owners = append([]MaintenanceAuthority{}, p.owners...)
	frozen.activities = append([]MaintenanceActivity{}, p.activities...)
	frozen.missing = append([]string{}, p.missing...)
	return &frozen
}
