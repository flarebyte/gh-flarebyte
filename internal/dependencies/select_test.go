package dependencies

import (
	"slices"
	"testing"
	"time"
)

func selectionFinding(version, decision string) Finding {
	f := Finding{Version: version, Decision: decision, Reasons: []string{}, Missing: []string{}, Advisories: []string{}}
	if decision == Deny {
		f.Reasons = []string{"KNOWN_VULNERABILITY"}
	} else if decision != Allow {
		f.Missing = []string{"ACTIVE_MAINTAINERS"}
	}
	f.Evidence = &Evidence{Identity: "verified", Maintainers: "multiple", Advisories: f.Advisories, Missing: f.Missing}
	return f
}

func TestRecommendationSelection(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		findings              []Finding
		requested             *string
		limited               bool
		decision, recommended string
	}{
		{"newest passing", []Finding{selectionFinding("3.0.0", Deny), selectionFinding("2.0.0", Allow), selectionFinding("1.0.0", Allow)}, nil, false, Allow, "2.0.0"},
		{"higher unknown", []Finding{selectionFinding("3.0.0", Unknown), selectionFinding("2.0.0", Allow)}, nil, false, Unknown, ""},
		{"all denied", []Finding{selectionFinding("2.0.0", Deny), selectionFinding("1.0.0", Deny)}, nil, false, Deny, ""},
		{"bounded denial", []Finding{selectionFinding("2.0.0", Deny)}, nil, true, Unknown, ""},
		{"unknown", []Finding{selectionFinding("2.0.0", Unknown)}, nil, false, Unknown, ""},
		{"requested denied", []Finding{selectionFinding("1.0.0", Deny), selectionFinding("2.0.0", Allow)}, stringPointer("1.0.0"), false, Deny, "2.0.0"},
		{"requested passing", []Finding{selectionFinding("1.0.0", Allow)}, stringPointer("1.0.0"), false, Allow, "1.0.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReport("npm", "pkg", tt.requested, DefaultPolicy(14), checked)
			r.Details.Candidates = tt.findings
			r.Details.LimitReached = tt.limited
			published := map[string]time.Time{}
			for _, f := range tt.findings {
				published[f.Version] = checked.Add(-30 * 24 * time.Hour)
			}
			finishAssessment(&r, published, checked)
			if r.Decision != tt.decision {
				t.Fatal(r)
			}
			if tt.recommended == "" {
				if r.Recommendation != nil {
					t.Fatal(r.Recommendation)
				}
			} else if r.Recommendation == nil || r.Recommendation.Version != tt.recommended || r.Recommendation.AgeDays != 30 {
				t.Fatal(r.Recommendation)
			}
		})
	}
	r := NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
	finishAssessment(&r, nil, checked)
	if !slices.Contains(r.Evidence.Missing, "VERSION_CANDIDATES") {
		t.Fatal(r)
	}
	for _, v := range []string{"5.0.0", "4.0.0", "3.0.0", "2.0.0", "1.0.0"} {
		r.Details.Candidates = append(r.Details.Candidates, selectionFinding(v, Allow))
	}
	dates := map[string]time.Time{"5.0.0": {}, "4.0.0": checked.Add(time.Hour), "3.0.0": checked.Add(-time.Hour), "2.0.0": checked.Add(-time.Hour), "1.0.0": checked.Add(-time.Hour)}
	finishAssessment(&r, dates, checked)
	if r.Recommendation == nil || r.Recommendation.Version != "3.0.0" || len(r.Alternatives) != 2 {
		t.Fatal(r)
	}
	r = NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
	r.Details.Candidates = []Finding{{Version: "1.0.0", Decision: "", Missing: []string{"gap"}, Advisories: []string{}}}
	finishAssessment(&r, nil, checked)
	if r.Decision != Unknown {
		t.Fatal(r)
	}
	r = NewReport("npm", "pkg", nil, DefaultPolicy(14), checked)
	r.Details.Candidates = []Finding{selectionFinding("1.0.0", Allow)}
	r.Details.RecommendationMissing = []string{"VERSION_ORDERING"}
	finishAssessment(&r, map[string]time.Time{"1.0.0": checked.Add(-30 * 24 * time.Hour)}, checked)
	if r.Decision != Unknown || r.Recommendation != nil || !slices.Contains(r.Evidence.Missing, "VERSION_ORDERING") {
		t.Fatal("unordered catalog recommended", r)
	}
}

func stringPointer(s string) *string { return &s }
