package dependencies

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestEvidenceBudget(t *testing.T) {
	ctx := withBudget(context.Background())
	if withBudget(ctx) != ctx {
		t.Fatal("budget reset")
	}
	if remainingBytes(ctx) != 32<<20 || remainingBytes(context.Background()) != maxBody {
		t.Fatal("byte limits")
	}
	if err := consumeBytes(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := reserveRequest(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if !errors.Is(reserveRequest(ctx), errBudget) {
		t.Fatal("unbounded requests")
	}
	if err := consumeBytes(ctx, 32<<20); err != nil {
		t.Fatal(err)
	}
	if remainingBytes(ctx) != 0 || !errors.Is(consumeBytes(ctx, 1), errBudget) {
		t.Fatal("unbounded response budget")
	}
}

func TestClientBudgetExhaustion(t *testing.T) {
	for _, tt := range []struct {
		name     string
		requests int
		bytes    int64
		body     string
	}{
		{"request", 0, 100, `{}`}, {"zero bytes", 2, 0, `{}`}, {"bytes", 2, 1, `{}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), budgetKey{}, &evidenceBudget{requests: tt.requests, bytes: tt.bytes})
			calls := 0
			c := &Client{HTTP: doFunc(func(*http.Request) (*http.Response, error) { calls++; return response(tt.body, 200), nil })}
			sources := []Source{}
			_, err := c.Package(ctx, "npm", "pkg", &sources)
			if err == nil || len(sources) != 1 || sources[0].Status != "budget_exhausted" {
				t.Fatal(err, sources)
			}
			if tt.requests == 0 && calls != 0 {
				t.Fatal("request sent after exhaustion")
			}
		})
	}
	ctx := context.WithValue(context.Background(), budgetKey{}, &evidenceBudget{requests: 1, bytes: 100})
	c := &Client{HTTP: doFunc(func(*http.Request) (*http.Response, error) { return response(`{}`, 503), nil })}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.osv.dev/v1/query", strings.NewReader(`{}`))
	if _, attempts, err := c.request(req); !errors.Is(err, errBudget) || attempts != 1 {
		t.Fatal(attempts, err)
	}
}
