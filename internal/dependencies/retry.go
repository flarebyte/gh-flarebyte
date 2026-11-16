// purpose: Retry transient read-only provider queries within a small fixed budget.
// responsibilities: Respect bounded Retry-After delays and cancellation without replaying mutations.
// architecture notes: All callers issue GETs or read-only OSV query POSTs; request bodies are recreated for retries.
package dependencies

import (
	"net/http"
	"strconv"
	"time"
)

func (c *Client) request(req *http.Request) (*http.Response, int, error) {
	if err := reserveRequest(req.Context()); err != nil {
		return nil, 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return resp, 1, err
	}
	if resp.StatusCode != 429 && resp.StatusCode != 500 && resp.StatusCode != 502 && resp.StatusCode != 503 && resp.StatusCode != 504 {
		return resp, 1, nil
	}
	delay, ok := retryDelay(resp.Header.Get("Retry-After"), time.Now())
	if !ok {
		return resp, 1, nil
	}
	next := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody == nil {
			return resp, 1, nil
		}
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return resp, 1, nil
		}
		next.Body = body
	}
	resp.Body.Close()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-req.Context().Done():
		if next.Body != nil {
			next.Body.Close()
		}
		return nil, 1, req.Context().Err()
	case <-timer.C:
	}
	if err := reserveRequest(req.Context()); err != nil {
		if next.Body != nil {
			next.Body.Close()
		}
		return nil, 1, err
	}
	resp, err = c.HTTP.Do(next)
	return resp, 2, err
}

func retryDelay(raw string, now time.Time) (time.Duration, bool) {
	if raw == "" {
		return 100 * time.Millisecond, true
	}
	var delay time.Duration
	if seconds, err := strconv.ParseUint(raw, 10, 32); err == nil {
		if seconds > 1 {
			return 0, false
		}
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(raw); err == nil {
		delay = date.Sub(now)
	} else {
		return 0, false
	}
	if delay > time.Second {
		return 0, false
	}
	if delay < 100*time.Millisecond {
		delay = 100 * time.Millisecond
	}
	return delay, true
}
