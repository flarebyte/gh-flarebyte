package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/flarebyte/gh-flarebyte/internal/dependencies"
)

func TestDependencyReportHumanVerdictsAndJSONContract(t *testing.T) {
	for _, tt := range []struct{ decision, label string }{
		{dependencies.Allow, "RECOMMEND"}, {dependencies.Unknown, "REVIEW"}, {dependencies.Deny, "AVOID"},
	} {
		t.Run(tt.decision, func(t *testing.T) {
			requested := "1.0.0"
			r := dependencies.NewReport("npm", "example", &requested, dependencies.PackageAdvicePolicy(14), time.Now())
			r.Decision = tt.decision
			r.Details = nil
			if tt.decision == dependencies.Allow {
				r.Recommendation = &dependencies.Candidate{Version: requested}
			} else if tt.decision == dependencies.Deny {
				r.Recommendation = &dependencies.Candidate{Version: "2.0.0"}
			} else {
				candidate := "2.0.0"
				r.CandidateVersion = &candidate
			}
			var text, machine bytes.Buffer
			if err := writeDependencyReport(&text, r, false); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(text.String(), tt.label+" npm:example@1.0.0\n") {
				t.Fatal(text.String())
			}
			switch tt.decision {
			case dependencies.Allow:
				if !strings.Contains(text.String(), "Recommended: 1.0.0\n") {
					t.Fatal(text.String())
				}
			case dependencies.Unknown:
				if !strings.Contains(text.String(), "Candidate (review required): 2.0.0\n") || strings.Contains(text.String(), "Recommended:") {
					t.Fatal(text.String())
				}
			case dependencies.Deny:
				if !strings.Contains(text.String(), "Recommended alternative: 2.0.0\n") {
					t.Fatal(text.String())
				}
			}
			if err := writeDependencyReport(&machine, r, true); err != nil {
				t.Fatal(err)
			}
			var decoded dependencies.Report
			if err := json.Unmarshal(machine.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Decision != tt.decision || decoded.Schema != r.Schema || decoded.Requested == nil || *decoded.Requested != requested {
				t.Fatal(machine.String())
			}
			if strings.Contains(machine.String(), `"recommended_version"`) || strings.Contains(machine.String(), `"requested_version"`) {
				t.Fatal("unexpected schema migration", machine.String())
			}
		})
	}
}
