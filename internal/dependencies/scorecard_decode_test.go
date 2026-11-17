package dependencies

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestScorecardScanDateFormats(t *testing.T) {
	for _, test := range []struct {
		date, want string
		valid      bool
	}{
		{`"2026-11-02"`, "2026-11-02T00:00:00Z", true},
		{`"2026-11-02T13:24:15Z"`, "2026-11-02T13:24:15Z", true},
		{`"2026-11-02T13:24:15+02:00"`, "2026-11-02T11:24:15Z", true},
		{`null`, "", true},
		{`"2026-02-30"`, "", false},
		{`"2026-1-02"`, "", false},
		{`" 2026-11-02"`, "", false},
		{`"2026-11-02T13:24:15"`, "", false},
		{`123`, "", false},
		{`{}`, "", false},
		{`""`, "", false},
	} {
		t.Run(test.date, func(t *testing.T) {
			var card Scorecard
			err := json.Unmarshal([]byte(`{"date":`+test.date+`,"repo":{"name":"github.com/org/repo"},"score":7.2,"checks":[{"name":"Maintained","score":10}]}`), &card)
			if (err == nil) != test.valid {
				t.Fatal(err)
			}
			if !test.valid {
				return
			}
			if test.want == "" {
				if !card.Date.IsZero() {
					t.Fatal(card.Date)
				}
			} else if card.Date.UTC().Format(time.RFC3339) != test.want {
				t.Fatal(card.Date)
			}
			if card.Repo.Name != "github.com/org/repo" || len(card.Checks) != 1 || card.Checks[0].Score != 10 || card.Score != 7.2 {
				t.Fatal("other fields lost", card)
			}
		})
	}
	var absent Scorecard
	if json.Unmarshal([]byte(`{"score":1}`), &absent) != nil || !absent.Date.IsZero() {
		t.Fatal("missing date must remain unknown")
	}
}

func TestScorecardPublicDateContract(t *testing.T) {
	// Public api.scorecard.dev responses observed on 2026-11-17 use YYYY-MM-DD,
	// whereas embedded deps.dev scorecards use RFC3339 timestamps.
	card := goodCard()
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	payload["date"] = "2026-11-02"
	data, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.scorecard.dev/projects/github.com/org/repo" {
			t.Fatal(r.URL)
		}
		return response(string(data), 200), nil
	})}
	sources := []Source{}
	got, err := c.Scorecard(context.Background(), "github.com/org/repo", &sources)
	if err != nil || len(sources) != 1 || sources[0].Status != "ok" || got.Date.Format(time.RFC3339) != "2026-11-02T00:00:00Z" {
		t.Fatal(got, err, sources)
	}
}
