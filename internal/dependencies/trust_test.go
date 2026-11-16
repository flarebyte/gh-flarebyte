package dependencies

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func goodCard() Scorecard {
	s := Scorecard{Date: checked.Add(-time.Hour), Score: 0, Checks: []Check{}}
	s.Repo.Name = "github.com/org/repo"
	s.Repo.Commit = strings.Repeat("a", 40)
	s.Engine.Version = "v5.5.0"
	for _, rule := range DefaultPolicy(14).ScorecardChecks {
		s.Checks = append(s.Checks, Check{Name: rule.Name, Score: 10})
	}
	return s
}

func completeTrust() TrustEvidence {
	stars, maintainers := 4200, 2
	return TrustEvidence{Identity: "verified", Repository: "github.com/org/repo", Stars: &stars, IndependentMaintainers: &maintainers, LifecycleKnown: true, NameScreened: true, ProjectResolved: true, Scorecards: []Scorecard{goodCard()}}
}

func TestTrustEvidence(t *testing.T) {
	p := DefaultPolicy(14)
	for _, tt := range []struct {
		name            string
		change          func(*TrustEvidence)
		reason, missing string
	}{
		{"complete", func(*TrustEvidence) {}, "", ""},
		{"low stars", func(e *TrustEvidence) { n := 20; e.Stars = &n }, "LOW_REPUTATION", ""},
		{"unknown stars", func(e *TrustEvidence) { e.Stars = nil }, "", "REPOSITORY_REPUTATION"},
		{"invalid stars", func(e *TrustEvidence) { n := -1; e.Stars = &n }, "", "REPOSITORY_REPUTATION"},
		{"single", func(e *TrustEvidence) { n := 1; e.IndependentMaintainers = &n }, "SINGLE_MAINTAINER", ""},
		{"none", func(e *TrustEvidence) { n := 0; e.IndependentMaintainers = &n }, "INSUFFICIENT_MAINTAINERS", ""},
		{"unknown maintainers", func(e *TrustEvidence) { e.IndependentMaintainers = nil }, "", "ACTIVE_MAINTAINERS"},
		{"invalid maintainers", func(e *TrustEvidence) { n := -1; e.IndependentMaintainers = &n }, "", "ACTIVE_MAINTAINERS"},
		{"identity conflict", func(e *TrustEvidence) { e.Identity = "conflict" }, "IDENTITY_CONFLICT", ""},
		{"unverified identity", func(e *TrustEvidence) { e.Identity = "unverified" }, "", "PACKAGE_IDENTITY"},
		{"withdrawn", func(e *TrustEvidence) { e.Withdrawn = true }, "WITHDRAWN_RELEASE", ""},
		{"unknown lifecycle", func(e *TrustEvidence) { e.LifecycleKnown = false }, "", "RELEASE_LIFECYCLE"},
		{"lookalike", func(e *TrustEvidence) { e.NameConflict = true }, "SUSPICIOUS_NAME", ""},
		{"name coverage", func(e *TrustEvidence) { e.NameScreened = false }, "", "TYPOSQUATTING_COVERAGE"},
		{"resolution", func(e *TrustEvidence) { e.ProjectResolved = false }, "", "PROJECT_RESOLUTION"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := completeTrust()
			tt.change(&e)
			r, m := e.Evaluate(p, checked)
			if tt.reason != "" && !slices.Contains(r, tt.reason) {
				t.Fatalf("reasons %v missing %v", r, m)
			}
			if tt.missing != "" && !slices.Contains(m, tt.missing) {
				t.Fatalf("reasons %v missing %v", r, m)
			}
			if tt.name == "complete" && (len(r) > 0 || len(m) > 0) {
				t.Fatalf("reasons %v missing %v", r, m)
			}
		})
	}
	e := completeTrust()
	e.Identity = "unverified"
	n := 20
	e.Stars = &n
	r, m := e.Evaluate(p, checked)
	if slices.Contains(r, "LOW_REPUTATION") || !slices.Contains(m, "SCORECARD_IDENTITY") {
		t.Fatalf("borrowed reputation: %v %v", r, m)
	}
	v := Version{Key: VersionKey{System: "npm", Name: "pkg", Version: "1.0.0"}, PublishedAt: checked.Add(-14 * 24 * time.Hour)}
	f := evaluateRelease(v, nil, nil, completeTrust(), p, checked)
	if f.Decision != Allow || f.Evidence.Maintainers != "multiple" {
		t.Fatal(f)
	}
	v.PublishedAt = v.PublishedAt.Add(time.Nanosecond)
	if !slices.Contains(evaluateRelease(v, nil, nil, completeTrust(), p, checked).Reasons, "RELEASE_TOO_NEW") {
		t.Fatal("fractional age bypass")
	}
	e = completeTrust()
	one := 1
	e.IndependentMaintainers = &one
	f = evaluateRelease(v, nil, nil, e, p, checked)
	if f.Evidence.Maintainers != "single" {
		t.Fatal(f)
	}
}

func TestScorecardSnapshotPolicy(t *testing.T) {
	p := DefaultPolicy(14)
	for _, tt := range []struct {
		name            string
		cards           func() []Scorecard
		reason, missing string
	}{
		{"missing", func() []Scorecard { return nil }, "", "SCORECARD_LOOKUP"},
		{"wrong repository", func() []Scorecard { s := goodCard(); s.Repo.Name = "github.com/other/repo"; return []Scorecard{s} }, "", "SCORECARD_LOOKUP"},
		{"future", func() []Scorecard { s := goodCard(); s.Date = checked.Add(time.Second); return []Scorecard{s} }, "", "SCORECARD_LOOKUP"},
		{"no commit", func() []Scorecard { s := goodCard(); s.Repo.Commit = ""; return []Scorecard{s} }, "", "SCORECARD_LOOKUP"},
		{"no engine", func() []Scorecard { s := goodCard(); s.Engine.Version = ""; return []Scorecard{s} }, "", "SCORECARD_LOOKUP"},
		{"no date", func() []Scorecard { s := goodCard(); s.Date = time.Time{}; return []Scorecard{s} }, "", "SCORECARD_LOOKUP"},
		{"stale", func() []Scorecard {
			s := goodCard()
			s.Date = checked.Add(-30*24*time.Hour - time.Nanosecond)
			return []Scorecard{s}
		}, "", "SCORECARD_STALE"},
		{"conflict", func() []Scorecard {
			a, b := goodCard(), goodCard()
			b.Repo.Commit = "different"
			return []Scorecard{a, b}
		}, "", "SCORECARD_CONFLICT"},
		{"duplicate check", func() []Scorecard { s := goodCard(); s.Checks = append(s.Checks, s.Checks[0]); return []Scorecard{s} }, "", "SCORECARD_SCHEMA"},
		{"failed check", func() []Scorecard { s := goodCard(); s.Checks[0].Score = 0; return []Scorecard{s} }, "SECURITY_PRACTICES", ""},
		{"inconclusive check", func() []Scorecard { s := goodCard(); s.Checks[0].Score = -1; return []Scorecard{s} }, "", "SCORECARD_CHECK:Branch-Protection"},
		{"invalid score", func() []Scorecard { s := goodCard(); s.Checks[0].Score = 11; return []Scorecard{s} }, "", "SCORECARD_CHECK:Branch-Protection"},
		{"no cherry picking", func() []Scorecard {
			a, b := goodCard(), goodCard()
			b.Date = checked
			b.Checks = nil
			return []Scorecard{a, b}
		}, "", "SCORECARD_CHECK:Branch-Protection"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, m := evaluateScorecards(tt.cards(), "github.com/org/repo", p, checked)
			if tt.reason != "" && !slices.Contains(r, tt.reason) {
				t.Fatalf("%v %v", r, m)
			}
			if tt.missing != "" && !slices.Contains(m, tt.missing) {
				t.Fatalf("%v %v", r, m)
			}
		})
	}
	s := goodCard()
	s.Date = checked.Add(-30 * 24 * time.Hour)
	if r, m := evaluateScorecards([]Scorecard{s, s}, s.Repo.Name, p, checked); len(r) > 0 || len(m) > 0 {
		t.Fatalf("identical snapshots or boundary: %v %v", r, m)
	}
	a, b, newer := goodCard(), goodCard(), goodCard()
	b.Repo.Commit = "conflict"
	newer.Date = checked
	if r, m := evaluateScorecards([]Scorecard{a, b, newer}, a.Repo.Name, p, checked); len(r) > 0 || len(m) > 0 {
		t.Fatalf("newer snapshot did not resolve tie %v %v", r, m)
	}
}

func TestProvenanceIdentity(t *testing.T) {
	a := Attestation{Type: "https://slsa.dev/provenance/v1", Verified: true, SourceRepository: "https://github.com/Org/Repo.git", Commit: strings.Repeat("a", 40)}
	v := Version{Attestations: []Attestation{a}}
	if repo, identity := versionRepository(v); repo != "github.com/org/repo" || identity != "verified" {
		t.Fatalf("%s %s", repo, identity)
	}
	for _, change := range []func(*Attestation){func(a *Attestation) { a.Verified = false }, func(a *Attestation) { a.Type = "" }, func(a *Attestation) { a.Type = "unknown" }, func(a *Attestation) { a.Commit = "bad" }, func(a *Attestation) { a.SourceRepository = "https://github.com/Org/Repo?x=1" }} {
		b := a
		change(&b)
		if _, identity := versionRepository(Version{Attestations: []Attestation{b}}); identity == "verified" {
			t.Fatal("invalid provenance trusted")
		}
	}
	b := a
	b.SourceRepository = "https://github.com/other/repo"
	if _, identity := versionRepository(Version{Attestations: []Attestation{a, b}}); identity != "conflict" {
		t.Fatal(identity)
	}
	if repo, identity := versionRepository(Version{Provenances: []Attestation{a}}); repo == "" || identity != "verified" {
		t.Fatal("legacy provenance ignored")
	}
	for _, raw := range []string{"http://github.com/org/repo", "https://evil.test/org/repo", "https://user@github.com/org/repo", "https://github.com/org/repo#fragment", ":bad"} {
		if sourceRepository(raw) != "" {
			t.Fatal(raw)
		}
	}
	unknown := versionFixture(t, `{"relatedProjects":[{"projectKey":{"id":"github.com/org/repo"},"relationType":"SOURCE_REPO"},{"projectKey":{"id":"github.com/other/repo"},"relationType":"SOURCE_REPO"}]}`)
	if repo, identity := versionRepository(unknown); repo != "" || identity != "unknown" {
		t.Fatal(repo, identity)
	}
}

func TestNameScreen(t *testing.T) {
	for _, tt := range []struct {
		system, name      string
		known, suspicious bool
	}{
		{"npm", "lodash", true, false}, {"npm", "lodahs", true, true}, {"npm", "lodas", true, true}, {"npm", "loodash", true, true}, {"npm", "lodasx", true, true}, {"npm", "@unknown/lodash", true, true}, {"npm", "unrelated-package", true, false}, {"swift", "https://github.com/org/repo", false, false},
		{"npm", "@types/react", true, false},
	} {
		known, suspicious := screenName(tt.system, tt.name)
		if known != tt.known || suspicious != tt.suspicious {
			t.Fatal(tt, known, suspicious)
		}
	}
	if nearName("abc", "abc") || nearName("abc", "xyz") || nearName("a", "abcdef") || !nearName("abx", "ab") || nearName("acb", "bac") {
		t.Fatal("name comparison")
	}
}
