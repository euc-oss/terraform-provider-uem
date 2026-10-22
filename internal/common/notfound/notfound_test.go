package notfound

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestMain zeroes Delay for every test in this package so the table test
// runs fast; the one case that needs to prove Confirm actually waits sets
// Delay back to a small non-zero value for just that case (see
// TestConfirm/waits_for_Delay).
func TestMain(m *testing.M) {
	Delay = 0
	m.Run()
}

var errBoom = errors.New("boom: a different error, not not-found")

func TestConfirm(t *testing.T) {
	t.Run("flaky then found: refetch succeeds, no error, result kept", func(t *testing.T) {
		Delay = 0
		classifyCalls := 0
		classify := func(error) bool {
			classifyCalls++
			return true // would only matter if refetch failed; it doesn't here.
		}
		result, stillNotFound, err := Confirm(context.Background(), classify, func(context.Context) (string, error) {
			return "found-on-retry", nil
		})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if stillNotFound {
			t.Fatal("expected stillNotFound=false when the confirming refetch succeeds")
		}
		if result != "found-on-retry" {
			t.Fatalf("expected the refetch's result to be returned, got %q", result)
		}
		if classifyCalls != 0 {
			t.Fatalf("classify must not be called when refetch succeeds, got %d calls", classifyCalls)
		}
	})

	t.Run("gone twice: refetch fails and classifies as not-found, dropped with no error", func(t *testing.T) {
		Delay = 0
		notFoundErr := errors.New("still not found")
		classify := func(e error) bool { return errors.Is(e, notFoundErr) }
		result, stillNotFound, err := Confirm(context.Background(), classify, func(context.Context) (string, error) {
			return "", notFoundErr
		})
		if err != nil {
			t.Fatalf("expected no error when the confirming refetch also classifies as not-found, got %v", err)
		}
		if !stillNotFound {
			t.Fatal("expected stillNotFound=true when the confirming refetch also classifies as not-found")
		}
		if result != "" {
			t.Fatalf("expected a zero-value result, got %q", result)
		}
	})

	t.Run("not-found then a different error: surfaced as-is, not dropped", func(t *testing.T) {
		Delay = 0
		classify := func(e error) bool { return false } // the confirming error never classifies as not-found
		result, stillNotFound, err := Confirm(context.Background(), classify, func(context.Context) (string, error) {
			return "", errBoom
		})
		if stillNotFound {
			t.Fatal("expected stillNotFound=false when the confirming refetch fails with a different error")
		}
		if !errors.Is(err, errBoom) {
			t.Fatalf("expected the DIFFERENT confirming error to be returned unchanged, got %v", err)
		}
		if result != "" {
			t.Fatalf("expected a zero-value result, got %q", result)
		}
	})

	t.Run("waits for Delay before refetching", func(t *testing.T) {
		old := Delay
		Delay = 30 * time.Millisecond
		defer func() { Delay = old }()

		var calledAt time.Time
		start := time.Now()
		_, _, err := Confirm(context.Background(), func(error) bool { return true }, func(context.Context) (int, error) {
			calledAt = time.Now()
			return 0, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := calledAt.Sub(start); elapsed < Delay {
			t.Fatalf("expected the refetch to happen after Delay (%s), but it happened after only %s", Delay, elapsed)
		}
	})

	t.Run("respects context cancellation instead of blocking past Delay", func(t *testing.T) {
		old := Delay
		Delay = time.Hour // would hang the test if Confirm ignored ctx
		defer func() { Delay = old }()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		refetchCalled := false
		_, stillNotFound, err := Confirm(ctx, func(error) bool { return true }, func(context.Context) (int, error) {
			refetchCalled = true
			return 0, nil
		})
		if err == nil {
			t.Fatal("expected ctx.Err() to be returned when ctx is already done")
		}
		if stillNotFound {
			t.Fatal("expected stillNotFound=false on ctx cancellation")
		}
		if refetchCalled {
			t.Fatal("expected refetch to never be called once ctx is done")
		}
	})
}
