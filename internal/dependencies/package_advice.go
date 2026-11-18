// purpose: Assess named releases without requiring proof of a local installation.
// responsibilities: Separate advisory adoption policy from retained strict installers.
package dependencies

import (
	"context"
	"slices"
	"time"
)

const PackageAdvicePolicyID = "package-advice-v1"

func SupportsPackageAdvice(system string) bool {
	return system == "npm" || system == "go" || system == "maven" || system == "pypi" || system == "cargo"
}

func PackageAdvicePolicy(days int) Policy {
	p := DefaultPolicy(days)
	p.ID = PackageAdvicePolicyID
	p.ActivityDays = 0
	p.ScorecardChecks = nil
	return p
}

func (c *Client) AssessPackage(ctx context.Context, system, name string, requested *string, p Policy, now time.Time) Report {
	if !SupportsPackageAdvice(system) || p.ID != PackageAdvicePolicyID {
		r := NewReport(system, name, requested, p, now)
		r.Reasons = []string{"UNSUPPORTED_PACKAGE_ADVICE"}
		return r
	}
	r := c.assess(ctx, system, name, requested, p, now, nil)
	r.Coverage = "direct-package-only"
	for _, f := range r.Details.Candidates {
		if f.Decision == Deny || len(f.Advisories) > 0 {
			continue
		}
		gap := false
		for _, missing := range f.Missing {
			if !slices.Contains([]string{"PACKAGE_IDENTITY", "REPOSITORY_REPUTATION", "SUSPICIOUS_NAME", "TYPOSQUATTING_COVERAGE"}, missing) {
				gap = true
			}
		}
		if !gap && (r.CandidateVersion == nil || newer(system, f.Version, *r.CandidateVersion)) {
			version := f.Version
			r.CandidateVersion = &version
		}
	}
	return r
}

func (e TrustEvidence) evaluatePackageAdvice(p Policy) ([]string, []string) {
	reasons, missing := []string{}, []string{}
	if e.Identity == "conflict" {
		reasons = append(reasons, "IDENTITY_CONFLICT")
	} else if e.Identity != "verified" {
		missing = append(missing, "PACKAGE_IDENTITY")
	}
	if !e.LifecycleKnown {
		missing = append(missing, "RELEASE_LIFECYCLE")
	}
	if e.Withdrawn {
		reasons = append(reasons, "WITHDRAWN_RELEASE")
	}
	if e.NameConflict {
		missing = append(missing, "SUSPICIOUS_NAME")
	}
	if !e.NameScreened {
		missing = append(missing, "TYPOSQUATTING_COVERAGE")
	}
	if e.Identity == "verified" {
		if e.Stars == nil || *e.Stars < 0 {
			missing = append(missing, "REPOSITORY_REPUTATION")
		} else if *e.Stars < p.MinStars {
			reasons = append(reasons, "LOW_REPUTATION")
		}
	} else {
		missing = append(missing, "REPOSITORY_REPUTATION")
	}
	if o := e.registryAuthority; o != nil && o.known && o.organization == "" && !o.opaqueAuthority && len(o.roles) == 1 {
		reasons = append(reasons, "SINGLE_REGISTRY_OWNER")
	}
	return unique(reasons), unique(missing)
}

func packageAdviceWarnings(e TrustEvidence, p Policy, now time.Time) []string {
	warnings := []string{"MAINTAINER_ACTIVITY_UNKNOWN"}
	if e.registryAuthority == nil || !e.registryAuthority.known || len(e.registryAuthority.roles) == 0 || e.registryAuthority.organization != "" || e.registryAuthority.opaqueAuthority {
		warnings = append(warnings, "MAINTAINER_COUNT_UNKNOWN")
	}
	if len(e.Scorecards) == 0 {
		warnings = append(warnings, "SCORECARD_UNAVAILABLE")
	} else {
		fresh := false
		for _, card := range e.Scorecards {
			if !card.Date.IsZero() && !card.Date.After(now) && daysOld(now, card.Date) <= int64(p.MaxScorecardAgeDays) {
				fresh = true
			}
		}
		if !fresh {
			warnings = append(warnings, "SCORECARD_STALE")
		}
	}
	return unique(warnings)
}
