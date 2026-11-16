package dependencies

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReadOnlyRetry(t *testing.T) {
	calls := 0
	c := &Client{HTTP: doFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		b, err := io.ReadAll(r.Body)
		if err != nil || string(b) != `{"query":true}` {
			t.Fatalf("request body %s: %v", b, err)
		}
		if calls == 1 {
			return response(`{}`, 503), nil
		}
		return response(`{}`, 200), nil
	})}
	req, err := http.NewRequestWithContext(context.Background(), "POST", "https://api.osv.dev/v1/query", strings.NewReader(`{"query":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, attempts, err := c.request(req)
	if err != nil || attempts != 2 || resp.StatusCode != 200 || calls != 2 {
		t.Fatal(resp, attempts, err)
	}
	resp.Body.Close()
	for _, retryAfter := range []string{"30", "bad"} {
		c.HTTP = doFunc(func(*http.Request) (*http.Response, error) {
			r := response(`{}`, 429)
			r.Header = http.Header{"Retry-After": []string{retryAfter}}
			return r, nil
		})
		r, attempts, err := c.request(req)
		if err != nil || attempts != 1 {
			t.Fatal(err, attempts)
		}
		r.Body.Close()
	}
	c.HTTP = doFunc(func(*http.Request) (*http.Response, error) { return response(`{}`, 503), nil })
	noReplay := req.Clone(context.Background())
	noReplay.GetBody = nil
	if r, attempts, err := c.request(noReplay); err != nil || attempts != 1 {
		t.Fatal(r, attempts, err)
	} else {
		r.Body.Close()
	}
	badReplay := req.Clone(context.Background())
	badReplay.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("cannot replay") }
	if r, attempts, err := c.request(badReplay); err != nil || attempts != 1 {
		t.Fatal(r, attempts, err)
	} else {
		r.Body.Close()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, attempts, err := c.request(req.Clone(ctx)); err == nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
	get, _ := http.NewRequestWithContext(ctx, "GET", "https://api.deps.dev/v3/query", nil)
	if _, _, err := c.request(get); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		raw string
		ok  bool
	}{
		{"", true}, {"0", true}, {"1", true}, {"2", false}, {"999999999999", false}, {"invalid", false},
		{now.Add(-time.Hour).Format(http.TimeFormat), true}, {now.Add(time.Hour).Format(http.TimeFormat), false},
	} {
		delay, ok := retryDelay(tt.raw, now)
		if ok != tt.ok || ok && (delay < 100*time.Millisecond || delay > time.Second) {
			t.Fatal(tt, delay, ok)
		}
	}
}
