// purpose: Decode the date-only scan dates returned by the public Scorecard API.
// responsibilities: Preserve RFC3339 compatibility and reject malformed or ambiguous dates.
// architecture notes: Date-only scans use midnight UTC, conservatively retaining the existing freshness policy.
package dependencies

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Scorecard) UnmarshalJSON(data []byte) error {
	type fields Scorecard
	var raw struct {
		fields
		Date json.RawMessage `json:"date"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var date time.Time
	if len(raw.Date) > 0 && !bytes.Equal(raw.Date, []byte("null")) {
		var text string
		if json.Unmarshal(raw.Date, &text) != nil {
			return fmt.Errorf("invalid Scorecard scan date")
		}
		var err error
		date, err = time.Parse(time.RFC3339, text)
		if err != nil {
			date, err = time.Parse("2006-01-02", text)
		}
		if err != nil {
			return fmt.Errorf("invalid Scorecard scan date")
		}
	}
	*s = Scorecard(raw.fields)
	s.Date = date
	return nil
}
