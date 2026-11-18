// purpose: Select recommendations only from fully assessed eligible releases.
// responsibilities: Preserve exact-version verdicts and avoid claiming newest when higher candidates are unresolved.
// architecture notes: Unknown candidates are not equivalent to policy rejection.
package dependencies

import (
	"sort"
	"strings"
	"time"
)

func finishAssessment(r *Report, published map[string]time.Time, now time.Time) {
	findings := r.Details.Candidates
	if len(findings) == 0 {
		r.Evidence.Missing = []string{"VERSION_CANDIDATES"}
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		return
	}
	ordered := append([]Finding{}, findings...)
	system, _, _ := strings.Cut(r.Package, ":")
	sort.SliceStable(ordered, func(i, j int) bool { return newer(system, ordered[i].Version, ordered[j].Version) })
	higherUnknown := false
	if len(r.Details.RecommendationMissing) > 0 {
		higherUnknown = true
	}
	for _, f := range ordered {
		if f.Decision == Unknown || f.Decision == "" {
			higherUnknown = true
			continue
		}
		if f.Decision != Allow {
			continue
		}
		date := published[f.Version]
		if date.IsZero() || date.After(now) {
			continue
		}
		candidate := Candidate{Version: f.Version, AgeDays: daysOld(now, date), Compatibility: "constraints-satisfied"}
		if r.Policy == PackageAdvicePolicyID {
			candidate.Compatibility = "package-only"
		}
		if r.Recommendation == nil {
			if !higherUnknown {
				r.Recommendation = &candidate
			}
		} else if len(r.Alternatives) < 2 {
			r.Alternatives = append(r.Alternatives, candidate)
		}
	}
	summary := findings[0]
	if r.Requested != nil {
		for _, f := range findings {
			if f.Version == *r.Requested {
				summary = f
				break
			}
		}
		r.Decision = summary.Decision
		r.Reasons = append([]string{}, summary.Reasons...)
	} else if r.Recommendation != nil {
		for _, f := range findings {
			if f.Version == r.Recommendation.Version {
				summary = f
				break
			}
		}
		r.Decision = Allow
		r.Reasons = []string{}
	} else {
		allDenied := !r.Details.LimitReached
		for _, f := range findings {
			if f.Decision != Deny {
				summary = f
				allDenied = false
				break
			}
		}
		if allDenied {
			r.Decision = Deny
			r.Reasons = []string{"NO_ELIGIBLE_VERSION"}
		} else {
			r.Decision = Unknown
			r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		}
	}
	if summary.Evidence != nil {
		r.Evidence = *summary.Evidence
	} else {
		r.Evidence.Missing = append([]string{}, summary.Missing...)
		r.Evidence.Advisories = append([]string{}, summary.Advisories...)
	}
	if r.Policy == PackageAdvicePolicyID {
		r.Warnings = append([]string{}, summary.Warnings...)
		if r.Decision == Deny && r.Requested == nil {
			r.Reasons = unique(append(r.Reasons, summary.Reasons...))
		}
	}
	if r.Decision == Unknown && r.Details.LimitReached {
		r.Evidence.Missing = unique(append(r.Evidence.Missing, "CANDIDATE_SEARCH_INCOMPLETE"))
	}
	if r.Decision == Unknown && r.Recommendation == nil {
		r.Evidence.Missing = unique(append(r.Evidence.Missing, "RECOMMENDATION_UNCERTAIN"))
	}
	if r.Requested == nil && len(r.Details.RecommendationMissing) > 0 {
		r.Decision = Unknown
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
		r.Evidence.Missing = unique(append(r.Evidence.Missing, r.Details.RecommendationMissing...))
	}
	if project := r.Details.Project; project != nil {
		r.Evidence.Missing = unique(append(r.Evidence.Missing, project.Missing...))
		if summary.Resolution != "" || project.binds(strings.TrimPrefix(r.Package, system+":"), summary.Version) {
			r.Scope = "project-resolution"
		}
	}
	if r.Decision == "" {
		r.Decision = Unknown
	}
	if r.Decision == Unknown && len(r.Reasons) == 0 {
		r.Reasons = []string{"INSUFFICIENT_EVIDENCE"}
	}
}
