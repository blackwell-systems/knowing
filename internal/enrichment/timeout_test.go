package enrichment

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithSymbolTimeout_FastCall(t *testing.T) {
	err := WithSymbolTimeout(context.Background(), 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWithSymbolTimeout_SlowCall(t *testing.T) {
	err := WithSymbolTimeout(context.Background(), 50*time.Millisecond, func(ctx context.Context) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if !errors.Is(err, ErrSymbolTimeout) {
		t.Fatalf("expected ErrSymbolTimeout, got %v", err)
	}
}

func TestWithSymbolTimeout_FnError(t *testing.T) {
	customErr := errors.New("custom error")
	err := WithSymbolTimeout(context.Background(), 5*time.Second, func(ctx context.Context) error {
		return customErr
	})
	if !errors.Is(err, customErr) {
		t.Fatalf("expected custom error, got %v", err)
	}
}

func TestWithSymbolTimeout_ParentCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := WithSymbolTimeout(ctx, 5*time.Second, func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWithSymbolTimeout_ZeroTimeout(t *testing.T) {
	err := WithSymbolTimeout(context.Background(), 0, func(ctx context.Context) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if !errors.Is(err, ErrSymbolTimeout) {
		t.Fatalf("expected ErrSymbolTimeout, got %v", err)
	}
}

func TestDefaultSymbolTimeout_Value(t *testing.T) {
	if DefaultSymbolTimeout != 10*time.Second {
		t.Fatalf("expected DefaultSymbolTimeout == 10s, got %v", DefaultSymbolTimeout)
	}
}
