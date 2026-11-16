// purpose: Bound total provider work for one assessment, including transitive lookups.
// responsibilities: Cap request count and cumulative decoded input independently of provider response limits.
// architecture notes: Context-scoped budgets are shared by nested clients and safe for concurrent callers.
package dependencies

import (
	"context"
	"errors"
	"sync"
)

var errBudget = errors.New("assessment evidence budget exhausted")

type budgetKey struct{}
type evidenceBudget struct {
	mu       sync.Mutex
	requests int
	bytes    int64
}

func withBudget(ctx context.Context) context.Context {
	if ctx.Value(budgetKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, budgetKey{}, &evidenceBudget{requests: 128, bytes: 32 << 20})
}

func reserveRequest(ctx context.Context) error {
	if b, ok := ctx.Value(budgetKey{}).(*evidenceBudget); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.requests <= 0 {
			return errBudget
		}
		b.requests--
	}
	return nil
}

func remainingBytes(ctx context.Context) int64 {
	if b, ok := ctx.Value(budgetKey{}).(*evidenceBudget); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.bytes
	}
	return maxBody
}

func consumeBytes(ctx context.Context, n int64) error {
	if b, ok := ctx.Value(budgetKey{}).(*evidenceBudget); ok {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.bytes -= n
		if b.bytes < 0 {
			return errBudget
		}
	}
	return nil
}
