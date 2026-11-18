// purpose: Evaluate independently established release and repository trust evidence.
// responsibilities: Enforce reputation, maintenance and per-check security policy without averaging snapshots.
// architecture notes: Facts are explicit; publisher authority never follows from contribution counts.
package dependencies

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type TrustEvidence struct {
	registryAuthority        *registryAuthorityObservation
	registryAuthorityChecked bool
	cargoMaintenance         *cargoMaintenanceProof
	cargoMaintenanceChecked  bool
	Identity                 string
	Repository               string
	Stars                    *int
	IndependentMaintainers   *int
	LifecycleKnown           bool
	Withdrawn                bool
	NameScreened             bool
	NameConflict             bool
	ProjectResolved          bool
	Scorecards               []Scorecard
	RepositoryDescription    *string
	MaintenanceChecked       bool
}

func (e TrustEvidence) Evaluate(p Policy, now time.Time) ([]string, []string) {
	if p.ID == PackageAdvicePolicyID {
		return e.evaluatePackageAdvice(p)
	}
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
		reasons = append(reasons, "SUSPICIOUS_NAME")
	}
	if !e.NameScreened {
		missing = append(missing, "TYPOSQUATTING_COVERAGE")
	}
	if !e.ProjectResolved {
		missing = append(missing, "PROJECT_RESOLUTION")
	}
	_, publicActive := e.cargoMaintenance.evidence(p, now)
	if (e.IndependentMaintainers == nil || *e.IndependentMaintainers < 0) && !publicActive {
		missing = append(missing, "ACTIVE_MAINTAINERS")
	} else if e.IndependentMaintainers != nil && *e.IndependentMaintainers >= 0 && *e.IndependentMaintainers < p.MinMaintainers && !publicActive {
		if *e.IndependentMaintainers == 1 {
			reasons = append(reasons, "SINGLE_MAINTAINER")
		} else {
			reasons = append(reasons, "INSUFFICIENT_MAINTAINERS")
		}
	}
	// Unverified metadata cannot borrow a repository's reputation, positive or negative.
	if e.Identity != "verified" {
		missing = append(missing, "REPOSITORY_REPUTATION", "SCORECARD_IDENTITY")
		return unique(reasons), unique(missing)
	}
	if e.Stars == nil || *e.Stars < 0 {
		missing = append(missing, "REPOSITORY_REPUTATION")
	} else if *e.Stars < p.MinStars {
		reasons = append(reasons, "LOW_REPUTATION")
	}
	r, m := evaluateScorecards(e.Scorecards, e.Repository, p, now)
	return unique(append(reasons, r...)), unique(append(missing, m...))
}

func evaluateScorecards(cards []Scorecard, repo string, p Policy, now time.Time) ([]string, []string) {
	var latest *Scorecard
	conflict := false
	for i := range cards {
		s := &cards[i]
		if !strings.EqualFold(s.Repo.Name, repo) || s.Repo.Commit == "" || s.Engine.Version == "" || s.Date.IsZero() || s.Date.After(now) {
			continue
		}
		if latest == nil || s.Date.After(latest.Date) {
			latest = s
			conflict = false
		} else if s.Date.Equal(latest.Date) {
			a, _ := json.Marshal(latest)
			b, _ := json.Marshal(s)
			if string(a) != string(b) {
				conflict = true
			}
		}
	}
	if latest == nil {
		return nil, []string{"SCORECARD_LOOKUP"}
	}
	if conflict {
		return nil, []string{"SCORECARD_CONFLICT"}
	}
	if now.Sub(latest.Date) > time.Duration(p.MaxScorecardAgeDays)*24*time.Hour {
		return nil, []string{"SCORECARD_STALE"}
	}
	checks := map[string]int{}
	for _, c := range latest.Checks {
		if _, duplicate := checks[c.Name]; duplicate {
			return nil, []string{"SCORECARD_SCHEMA"}
		}
		checks[c.Name] = c.Score
	}
	reasons, missing := []string{}, []string{}
	for _, rule := range p.ScorecardChecks {
		score, ok := checks[rule.Name]
		if !ok || score < 0 || score > 10 {
			missing = append(missing, "SCORECARD_CHECK:"+rule.Name)
		} else if score < rule.MinScore {
			reasons = append(reasons, "SECURITY_PRACTICES")
		}
	}
	return unique(reasons), unique(missing)
}

var commitHash = regexp.MustCompile(`^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$`)

func sourceRepository(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host != "github.com" {
		return ""
	}
	return Repository("github.com/" + strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"))
}

func versionRepository(v Version) (string, string) {
	verified := []string{}
	attestations := append([]Attestation{}, v.Attestations...)
	for _, legacy := range v.Provenances {
		legacy.Type = "https://slsa.dev/provenance/v0.2"
		attestations = append(attestations, legacy)
	}
	for _, a := range attestations {
		known := a.Type == "https://slsa.dev/provenance/v0.2" || a.Type == "https://slsa.dev/provenance/v1" || a.Type == "https://docs.pypi.org/attestations/publish/v1"
		if repo := sourceRepository(a.SourceRepository); known && a.Verified && commitHash.MatchString(a.Commit) && repo != "" {
			verified = append(verified, strings.ToLower(repo))
		}
	}
	verified = unique(verified)
	if len(verified) > 1 {
		return "", "conflict"
	}
	if len(verified) == 1 {
		return verified[0], "verified"
	}
	repos := []string{}
	for _, related := range v.Projects {
		if related.Type == "SOURCE_REPO" && Repository(related.Key.ID) != "" {
			repos = append(repos, strings.ToLower(related.Key.ID))
		}
	}
	repos = unique(repos)
	if len(repos) == 1 {
		return repos[0], "unverified"
	}
	return "", "unknown"
}
