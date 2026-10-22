// Package notfound provides Confirm, a small generic helper that protects a
// Terraform Read from dropping real state on a single flaky not-found
// response from UEM.
//
// Root cause (the 7fx incident): under concurrent live-tenant load, as<internal-env>
// was observed intermittently answering a GET for a genuinely-existing
// object with a not-found shape (a plain 404, or a resource-specific "gone"
// shape such as the profile resource's 400 "Invalid Profile <id>."). The
// classifiers that recognize these shapes are narrow and correct — this is
// not a misclassification bug — but a flaky not-found is thinkable in
// production too, not just under artificial concurrent test load. If the
// flake happens during a `terraform plan` refresh (Read), the provider
// drops real Terraform state, and the next `apply` creates a duplicate
// object on the tenant.
//
// Confirm defends against exactly one flaky response: call it only at the
// point a caller is about to RemoveResource on a not-found classification.
// It waits Delay, then re-fetches once more; only if that confirming fetch
// ALSO classifies as not-found does the caller proceed with the drop.
package notfound

import (
	"context"
	"time"
)

// Delay is how long Confirm waits before its confirming re-GET. 1.5s is
// long enough to ride out the kind of transient backend blip this package
// defends against (a single request landing on a momentarily inconsistent
// backend node) without meaningfully slowing down a `terraform plan`
// refresh, and short enough that it does not turn a genuine deletion's
// Read into a noticeable pause.
//
// This is a package var, not a parameter threaded through every call site,
// deliberately: Confirm has call sites across nine resource packages, and a
// configurable delay would be a parameter that only tests ever set to a
// non-default value. A package-level var is the standard escape hatch for a
// test-only knob on a helper with many call sites — the same pattern
// internal/scripts/mac/crud_support.go already uses for
// scriptListPageSize/scriptListMaxPages. Tests should set this to 0 (e.g.
// in a TestMain or an init) so Confirm runs with no wall-clock delay.
var Delay = 1500 * time.Millisecond

// Confirm re-fetches once, after waiting Delay, to protect against a single
// flaky not-found response. Call it ONLY at the point a caller is about to
// RemoveResource on a not-found classification: err was already classified
// as not-found by classify (checking that is the caller's job, not
// Confirm's — Confirm never inspects the original error).
//
//   - If the confirming refetch ALSO classifies as not-found (per classify),
//     Confirm returns (zero value, true, nil) — the caller should
//     RemoveResource, exactly as it would have without this helper.
//   - If the confirming refetch SUCCEEDS, Confirm returns (result, false,
//     nil) — the caller should continue with result exactly as if the
//     ORIGINAL call had succeeded, and should tflog.Warn that this happened
//     (the caller does the logging: only the caller knows its resource
//     type/id for a useful message — Confirm stays generic).
//   - If the confirming refetch fails with some OTHER (non-not-found)
//     error, Confirm returns (zero value, false, err) — the caller should
//     surface err as a normal Diagnostics error, NOT drop state (the
//     ORIGINAL not-found is superseded by this new, different error —
//     don't report both).
//
// Confirm respects ctx cancellation while waiting: if ctx is done before
// Delay elapses, it returns immediately with ctx.Err() as err (the third
// case above), without calling refetch.
func Confirm[T any](ctx context.Context, classify func(error) bool, refetch func(context.Context) (T, error)) (result T, stillNotFound bool, err error) {
	var zero T

	if Delay > 0 {
		timer := time.NewTimer(Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return zero, false, ctx.Err()
		case <-timer.C:
		}
	}

	confirmed, refetchErr := refetch(ctx)
	if refetchErr == nil {
		return confirmed, false, nil
	}
	if classify(refetchErr) {
		return zero, true, nil
	}
	return zero, false, refetchErr
}
