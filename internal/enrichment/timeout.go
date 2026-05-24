package enrichment

import (
	"context"
	"errors"
	"time"
)

// ErrSymbolTimeout is returned when a per-symbol LSP call exceeds
// its timeout.
var ErrSymbolTimeout = errors.New("enrichment: symbol timeout exceeded")

// DefaultSymbolTimeout is the default per-symbol timeout (10 seconds).
const DefaultSymbolTimeout = 10 * time.Second

// WithSymbolTimeout executes fn with a per-symbol timeout. If fn
// does not return within timeout, the context is cancelled and
// ErrSymbolTimeout is returned. The parent context is NOT cancelled.
func WithSymbolTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	// If parent context is already cancelled, return its error immediately.
	if err := ctx.Err(); err != nil {
		return err
	}

	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fn(childCtx)
	}()

	select {
	case err := <-done:
		return err
	case <-childCtx.Done():
		// Check if it was the parent that was cancelled.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrSymbolTimeout
	}
}
