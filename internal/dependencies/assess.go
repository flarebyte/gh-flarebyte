// purpose: Assess public package evidence while preserving unresolved installation gates.
// responsibilities: Evaluate exact releases, merge advisory findings and explain incomplete coverage.
// architecture notes: Native resolution and maintainer authority are never inferred from metadata or contributor totals.
package dependencies

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxCandidates = 4

func unique(items []string) []string {
	sort.Strings(items)
	out := []string{}
	for _, item := range items {
		if len(out) == 0 || out[len(out)-1] != item {
			out = append(out, item)
		}
	}
	return out
}

func verdict(reasons, missing []string) string {
	if len(reasons) > 0 {
		return Deny
	}
	if len(missing) > 0 {
		return Unknown
	}
	return Allow
}

// EvaluateVersion is deterministic and does not perform I/O. All required evidence
// must be supplied explicitly; no caller can turn missing coverage into success.
func EvaluateVersion(v Version, advisories []Advisory, osvErr error, p Policy, now time.Time) Finding {
	return evaluateRelease(v, advisories, osvErr, TrustEvidence{}, p, now)
}

func evaluateRelease(v Version, advisories []Advisory, osvErr error, e TrustEvidence, p Policy, now time.Time) Finding {
	if !strings.EqualFold(v.Key.System, "cargo") || e.cargoMaintenance != nil && e.cargoMaintenance.name != v.Key.Name {
		e.cargoMaintenance = nil
	}
	f := Finding{Version: v.Key.Version, Reasons: []string{}, Missing: []string{}, Advisories: []string{}}
	if v.PublishedAt.IsZero() || v.PublishedAt.After(now) {
		f.Missing = append(f.Missing, "PUBLICATION_TIME")
	} else if daysOld(now, v.PublishedAt) < int64(p.MinAgeDays) {
		if p.ID == PackageAdvicePolicyID {
			f.Missing = append(f.Missing, "RELEASE_TOO_NEW")
		} else {
			f.Reasons = append(f.Reasons, "RELEASE_TOO_NEW")
		}
	}
	if v.Deprecated {
		f.Reasons = append(f.Reasons, "DEPRECATED_RELEASE")
	}
	if system := strings.ToLower(v.Key.System); knownPrerelease(system, v.Key.Version) {
		f.Reasons = append(f.Reasons, "PRERELEASE")
	} else if _, ok := stableVersion(system, v.Key.Version); !ok {
		f.Missing = append(f.Missing, "STABLE_VERSION_SEMANTICS")
	}
	ids := []string{}
	for _, a := range v.Advisories {
		ids = append(ids, a.ID)
	}
	groups, reasons, missing := groupAdvisories(advisories, ids)
	f.AdvisoryGroups = groups
	for _, g := range groups {
		f.Advisories = append(f.Advisories, g.ID)
	}
	f.Reasons = append(f.Reasons, reasons...)
	f.Missing = append(f.Missing, missing...)
	reasons, missing = e.Evaluate(p, now)
	f.Reasons = unique(append(f.Reasons, reasons...))
	f.Missing = append(f.Missing, missing...)
	if osvErr != nil {
		f.Missing = append(f.Missing, "OSV_LOOKUP")
	}
	f.Missing = unique(f.Missing)
	f.Advisories = unique(f.Advisories)
	f.Decision = verdict(f.Reasons, f.Missing)
	maintainers := "unknown"
	if e.IndependentMaintainers != nil {
		if *e.IndependentMaintainers == 1 {
			maintainers = "single"
		} else if *e.IndependentMaintainers >= p.MinMaintainers {
			maintainers = "multiple"
		}
	}
	identity := e.Identity
	maintenance, publicActive := e.cargoMaintenance.evidence(p, now)
	if maintenance == nil && e.registryAuthority != nil && strings.EqualFold(v.Key.System, e.registryAuthority.system) && equivalentName(e.registryAuthority.system, v.Key.Name, e.registryAuthority.name) {
		maintenance = e.registryAuthority.evidence()
	}
	if publicActive {
		maintainers = "multiple"
		f.cargoMaintenance = e.cargoMaintenance
	}
	if identity == "" {
		identity = "unknown"
	}
	f.Evidence = &Evidence{Identity: identity, Stars: e.Stars, Maintainers: maintainers, Advisories: f.Advisories, Missing: f.Missing, Maintenance: maintenance}
	if p.ID == PackageAdvicePolicyID {
		f.Warnings = packageAdviceWarnings(e, p, now)
	}
	return f
}

func (c *Client) Assess(ctx context.Context, system, name string, requested *string, p Policy, now time.Time) Report {
	return c.assess(ctx, system, name, requested, p, now, nil)
}

func (c *Client) AssessLocal(ctx context.Context, dir, system, name string, requested *string, p Policy, now time.Time) Report {
	ctx = withBudget(ctx)
	project := ReadProject(dir, system)
	r := c.assess(ctx, system, name, requested, p, now, project)
	if project.System == "pypi" && project.InventoryComplete {
		if len(project.Findings) == 0 {
			c.assessProject(ctx, project, p, now, &r, map[string]TrustEvidence{})
		}
		finishPythonGraph(project)
		mergePythonInventory(&r, project)
	}
	if (project.Complete || project.InventoryComplete) && !project.unchanged(dir) {
		project.Complete = false
		project.InventoryComplete = false
		if project.PythonGraph != nil {
			project.PythonGraph.Status = "incomplete"
			project.PythonGraph.Missing = unique(append(project.PythonGraph.Missing, "PROJECT_CHANGED_DURING_ASSESSMENT"))
		}
		project.Missing = unique(append(project.Missing, "PROJECT_CHANGED_DURING_ASSESSMENT"))
		r.Scope = "package"
		r.Recommendation = nil
		r.Alternatives = []Candidate{}
		r.Evidence.Missing = unique(append(r.Evidence.Missing, "PROJECT_CHANGED_DURING_ASSESSMENT"))
		if r.Decision == Allow {
			r.Decision = Unknown
			r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		}
	}
	return r
}

func (c *Client) assess(ctx context.Context, system, name string, requested *string, p Policy, now time.Time, project *ProjectSnapshot) Report {
	ctx = withBudget(ctx)
	r := NewReport(system, name, requested, p, now)
	r.Details.Project = project
	cache := map[string]TrustEvidence{}
	published := map[string]time.Time{}
	if system == "pub" || system == "swift" {
		r.Reasons = append(r.Reasons, "UNSUPPORTED_COVERAGE")
		r.Evidence.Missing = []string{"RELEASE_CATALOG", "PACKAGE_IDENTITY", "ACTIVE_MAINTAINERS", "PROJECT_RESOLUTION"}
		if requested != nil {
			advisories, err := c.OSV(ctx, systems[system], name, *requested, &r.Details.Sources)
			v := Version{Key: VersionKey{System: system, Name: name, Version: *requested}}
			f := EvaluateVersion(v, advisories, err, p, now)
			r.Details.Candidates = append(r.Details.Candidates, f)
			r.Evidence.Advisories = f.Advisories
			r.Evidence.Missing = unique(append(r.Evidence.Missing, f.Missing...))
			if len(f.Reasons) > 0 {
				r.Decision = Deny
				r.Reasons = unique(append(r.Reasons, f.Reasons...))
			}
		}
		return r
	}
	pack, err := c.Package(ctx, system, name, &r.Details.Sources)
	if err != nil {
		var lookup *LookupError
		if errors.As(err, &lookup) && lookup.Status == http.StatusNotFound {
			r.Error = &ReportError{"PACKAGE_NOT_FOUND", "Package not indexed by deps.dev; existence is unverified"}
		}
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		r.Evidence.Missing = []string{"PACKAGE_LOOKUP"}
		return r
	}
	if pack.PackageKey.Name == "" || len(pack.Versions) == 0 {
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		r.Evidence.Missing = []string{"PACKAGE_SCHEMA"}
		return r
	}
	canonical, _, parseErr := Parse(system, pack.PackageKey.Name)
	if parseErr != nil || !equivalentName(system, canonical, name) {
		r.Reasons = []string{"IDENTITY_CONFLICT"}
		r.Decision = Deny
		r.Evidence.Identity = "conflict"
		return r
	}
	r.Package = system + ":" + canonical
	registryDescriptionSet := false
	var adviceOwners *registryAuthorityObservation
	versions := []string{}
	catalogUnavailable := map[string]bool{}
	for _, v := range pack.Versions {
		if _, ok := stableVersion(system, v.Key.Version); ok {
			if p.ID == PackageAdvicePolicyID && !v.PublishedAt.IsZero() && (v.PublishedAt.After(now) || daysOld(now, v.PublishedAt) < int64(p.MinAgeDays)) {
				continue
			}
			versions = append(versions, v.Key.Version)
		} else if !knownPrerelease(system, v.Key.Version) && !v.Deprecated {
			if system == "pypi" {
				excluded, seen := catalogUnavailable[v.Key.Version]
				if !seen && len(catalogUnavailable) < maxPythonCatalogProbes {
					excluded = c.pythonCatalogUnavailable(ctx, canonical, v.Key.Version, &r.Details.Sources)
					catalogUnavailable[v.Key.Version] = excluded
					if excluded {
						r.Details.CatalogExclusions = append(r.Details.CatalogExclusions, CatalogExclusion{Version: v.Key.Version, Reason: "WITHDRAWN_RELEASE"})
					}
				}
				if excluded {
					continue
				}
			}
			r.Details.RecommendationMissing = []string{"VERSION_ORDERING"}
		}
	}
	versions = unique(versions)
	sort.Slice(r.Details.CatalogExclusions, func(i, j int) bool {
		return r.Details.CatalogExclusions[i].Version < r.Details.CatalogExclusions[j].Version
	})
	sort.Slice(versions, func(i, j int) bool { return newer(system, versions[i], versions[j]) })
	if requested != nil {
		found := false
		for _, v := range pack.Versions {
			found = found || v.Key.Version == *requested
		}
		if !found {
			r.Reasons = []string{"VERSION_NOT_FOUND"}
			r.Error = &ReportError{"VERSION_NOT_FOUND", "Exact version is not indexed; no range expansion was attempted"}
			return r
		}
		filtered := []string{*requested}
		for _, v := range versions {
			if v != *requested {
				if p.ID == PackageAdvicePolicyID && !newer(system, v, *requested) {
					continue
				}
				filtered = append(filtered, v)
			}
		}
		versions = filtered
	}
	if len(versions) > maxCandidates {
		r.Details.LimitReached = true
		versions = versions[:maxCandidates]
	}
	if project != nil {
		if locked := project.runtime[name]; locked != "" {
			listed, assessed := false, false
			for _, v := range pack.Versions {
				listed = listed || v.Key.Version == locked
			}
			for _, v := range versions {
				assessed = assessed || v == locked
			}
			if listed && !assessed {
				if len(versions) < maxCandidates {
					versions = append(versions, locked)
				} else {
					versions[len(versions)-1] = locked
				}
			}
		}
	}
	for _, version := range versions {
		if ctx.Err() != nil {
			r.Details.LimitReached = true
			break
		}
		var advisories []Advisory
		var osvErr error
		if p.ID == PackageAdvicePolicyID {
			advisories, osvErr = c.OSV(ctx, systems[system], name, version, &r.Details.Sources)
		}
		v, err := c.Version(ctx, system, name, version, &r.Details.Sources)
		if err != nil || v.Key.Name != pack.PackageKey.Name || v.Key.Version != version || !strings.EqualFold(v.Key.System, system) {
			f := Finding{Version: version, Decision: Unknown, Reasons: []string{}, Missing: []string{"VERSION_LOOKUP"}, Advisories: []string{}}
			if p.ID == PackageAdvicePolicyID {
				f = evaluateRelease(Version{Key: VersionKey{System: system, Name: name, Version: version}}, advisories, osvErr, TrustEvidence{}, p, now)
				f.Missing = unique(append(f.Missing, "VERSION_LOOKUP"))
				f.Evidence.Missing = f.Missing
			}
			r.Details.Candidates = append(r.Details.Candidates, f)
			continue
		}

		if p.ID != PackageAdvicePolicyID {
			advisories, osvErr = c.OSV(ctx, systems[system], name, version, &r.Details.Sources)
		}
		e, npm, description, releaseReasons := c.releaseEvidence(ctx, system, name, version, &r.Details.Sources)
		c.fillRepository(ctx, v, &e, &r, cache)
		if p.ID == PackageAdvicePolicyID {
			if adviceOwners == nil {
				adviceOwners = c.registryAuthorities(ctx, system, name, &r.Details.Sources)
				r.Details.Maintenance = append(r.Details.Maintenance, adviceOwners.detail())
			}
			e.registryAuthority = adviceOwners
		} else {
			c.fillMaintenance(ctx, system, v.Key.Name, &e, p, now, &r, cache)
		}
		e.NameScreened, e.NameConflict = screenName(system, name)
		resolution := ""
		inventory := project != nil && project.System == "pypi" && project.InventoryComplete && project.runtime[name] == version
		if project.binds(name, version) {
			resolution = "locked"
		} else if project.planLeaf(npm) {
			resolution = "add-leaf"
		} else if project.planReplacement(npm) {
			resolution = "replace-leaf"
		}
		if resolution != "" {
			e.ProjectResolved = true
		}
		if resolution != "" || inventory {
			if len(project.Findings) == 0 {
				c.assessProject(ctx, project, p, now, &r, cache)
			}
		}
		f := evaluateRelease(v, advisories, osvErr, e, p, now)
		f.Resolution = resolution
		f.Reasons = unique(append(f.Reasons, releaseReasons...))
		if e.ProjectResolved || inventory {
			mergeProjectFindings(&f, project, name)
			if npm != nil && (len(npm.Engines) > 0 || len(npm.OS) > 0 || len(npm.CPU) > 0) {
				f.Missing = unique(append(f.Missing, "RUNTIME_COMPATIBILITY"))
			}
		}
		f.Decision = verdict(f.Reasons, f.Missing)
		f.Evidence.Missing = f.Missing
		r.Details.Candidates = append(r.Details.Candidates, f)
		published[version] = v.PublishedAt
		if description != nil && (!registryDescriptionSet || requested != nil && version == *requested) {
			r.Description = description
			registryDescriptionSet = true
		}
	}
	if !registryDescriptionSet {
		if description := c.registryDescription(ctx, system, name, requested, &r.Details.Sources); description != nil {
			r.Description = description
		}
	}
	finishAssessment(&r, published, now)
	for _, source := range r.Details.Sources {
		if source.Status == "budget_exhausted" {
			r.Details.LimitReached = true
		}
	}
	return r
}

func (c *Client) fillRepository(ctx context.Context, v Version, e *TrustEvidence, r *Report, cache map[string]TrustEvidence) {
	repo, identity := versionRepository(v)
	e.Repository = repo
	e.Identity = identity
	if repo == "" {
		return
	}
	if prior, ok := cache[repo]; ok {
		e.Stars = prior.Stars
		e.Scorecards = prior.Scorecards
		if r.Description == nil && identity == "verified" {
			r.Description = prior.RepositoryDescription
		}
		return
	}
	evidence := TrustEvidence{}
	project, err := c.Project(ctx, repo, &r.Details.Sources)
	if err == nil && strings.EqualFold(project.Key.ID, repo) {
		evidence.RepositoryDescription = Description(project.Description)
		evidence.Stars = project.Stars
		if !project.Scorecard.Date.IsZero() {
			card := Scorecard{Date: project.Scorecard.Date, Score: project.Scorecard.Score, Checks: project.Scorecard.Checks}
			card.Repo = project.Scorecard.Repository
			card.Engine = project.Scorecard.Engine
			evidence.Scorecards = append(evidence.Scorecards, card)
		}
		if r.Description == nil && identity == "verified" {
			r.Description = evidence.RepositoryDescription
		}
	}
	if r.Policy == PackageAdvicePolicyID && evidence.Stars == nil {
		var public struct {
			FullName    string `json:"full_name"`
			Stars       *int   `json:"stargazers_count"`
			Description string `json:"description"`
		}
		if c.read(ctx, "github-public", "https://api.github.com/repos/"+strings.TrimPrefix(repo, "github.com/"), nil, &public, &r.Details.Sources) == nil && strings.EqualFold("github.com/"+public.FullName, repo) {
			evidence.Stars = public.Stars
			evidence.RepositoryDescription = Description(public.Description)
		}
	}
	if r.Policy != PackageAdvicePolicyID || len(evidence.Scorecards) == 0 {
		card, err := c.Scorecard(ctx, repo, &r.Details.Sources)
		if err == nil && strings.EqualFold(card.Repo.Name, repo) && !card.Date.IsZero() {
			evidence.Scorecards = append(evidence.Scorecards, card)
		}
	}
	cache[repo] = evidence
	e.Stars = evidence.Stars
	e.Scorecards = evidence.Scorecards
	r.Details.Scorecards = append(r.Details.Scorecards, evidence.Scorecards...)
}
