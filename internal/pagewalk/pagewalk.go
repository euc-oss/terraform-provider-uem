// Package pagewalk holds the hardened, page-index-agnostic list walk shared by
// every hand-written paginated Terraform data source in this provider
// (internal-ticket, release blockers B1/B12). It generalizes
// internal/profile/data_source.go's walkProfiles/probeProfilePageBase (B1) --
// itself mirroring internal/scripts/scriptlookup.AbsentFromOrgGroup's
// hardened-walk rules -- over any item type T, so uem_scripts, uem_applications
// and uem_mac_applications share exactly one implementation of the paging
// defenses instead of three near-identical copies.
//
// None of this changes what a data source filters or returns; it only decides
// how many pages to fetch and how hard to verify the result is complete.
package pagewalk

import (
	"context"
	"fmt"
)

// Page is one fetched page: its items, and the grand total the server
// reported FOR THIS PAGE (nil when the page did not report one -- the live
// shape of an out-of-range/HTTP-204-style page on every endpoint this package
// has been used against).
type Page[T any] struct {
	Items []T
	Total *int
}

// Fetcher fetches the page at the given (endpoint-native) page index and page
// size. Pagewalk never assumes what "page 0" or "page 1" means to the
// underlying API; see probeBase.
type Fetcher[T any] func(ctx context.Context, page, pageSize int) (Page[T], error)

// KeyFunc returns a stable, unique, non-empty key identifying item, or an
// error if the item has none. An item with no usable key can neither be
// deduped nor trusted to make progress, so it is always an error rather than
// silently skipped or counted as a duplicate.
type KeyFunc[T any] func(item T) (string, error)

// Result is one independent walk's outcome.
type Result[T any] struct {
	// Items holds every matched item, in first-seen order, deduped by KeyFunc.
	Items []T
	// Total is the grand total the walk trusts, learned from the first
	// non-empty page and confirmed unchanged on every later non-empty page.
	Total int
	// Base is the page index the walk's first page was fetched at (0 or 1;
	// see probeBase). Meaningless (always 0) when Items is empty and Total is
	// 0 -- there is no data to have a base.
	Base int
	// Pages is how many page fetches this walk made, including the base
	// probe's first (successful) page but not a second, empty probe of the
	// other candidate base once a base is already confirmed.
	Pages int
}

// WalkTwiceVerified walks fetch under the rules documented on walkOnce. A
// result spanning more than one page is walked a SECOND, fully independent
// time (including its own base probe), and the two outcomes -- the set of
// item keys, the trusted Total, and the detected Base -- must agree exactly,
// mirroring internal/organizationgroup and internal/smartgroup's search walks
// and internal/profile's listAllProfiles/walkProfiles. This guards against a
// list that shifts (or a page-index base that somehow differs) between the
// two walks. A single-page walk makes exactly one round trip (plus its own
// probe) and has no window for that to happen, so it is not repeated.
//
// Subject names the search in error messages (e.g. "script search",
// "application search").
func WalkTwiceVerified[T any](ctx context.Context, fetch Fetcher[T], key KeyFunc[T], pageSize, maxPages int, subject string) (Result[T], error) {
	first, err := walkOnce(ctx, fetch, key, pageSize, maxPages, subject)
	if err != nil || first.Pages <= 1 {
		return first, err
	}
	second, err := walkOnce(ctx, fetch, key, pageSize, maxPages, subject)
	switch {
	case err != nil:
		return Result[T]{}, fmt.Errorf("second walk of the %s disagrees with the first, which found %d items: %w", subject, len(first.Items), err)
	case !sameKeySet(first.Items, second.Items, key):
		return Result[T]{}, fmt.Errorf("%s changed between two walks: the first found %d items, the second found %d with a different set of ids", subject, len(first.Items), len(second.Items))
	case second.Total != first.Total:
		return Result[T]{}, fmt.Errorf("%s changed between two walks: total was %d then %d; refusing to return a possibly inconsistent result", subject, first.Total, second.Total)
	case second.Base != first.Base:
		return Result[T]{}, fmt.Errorf("%s page-index base changed between two walks: %d then %d; refusing to return a possibly inconsistent result", subject, first.Base, second.Base)
	}
	return first, nil
}

// walkOnce is one independent walk: it first probes the page-index base (see
// probeBase), then pages forward from that base. It returns the matched items
// in first-seen order, the trusted Total, the detected Base, and how many
// pages it fetched.
//
// Progress is counted in unique keys (via KeyFunc), never in raw items per
// page, so a server that repeats a page, or a list that shifts under the
// walk, cannot reach Total through duplicates. An item with no usable key is
// an error at once.
//
// Total is the grand total only on pages that hold items; an empty page never
// overwrites a total learned from an earlier non-empty page, and a non-empty
// page with a nil Total is an error. Per page, with n items, unique the
// distinct keys seen so far and total the last trusted Total:
//
//   - unique > total: error -- more distinct items exist than the server
//     claims, so Total cannot be trusted to bound the walk.
//   - unique == total and n < pageSize (a short or empty page): done -- every
//     item the server counts has been seen, and the server has signalled
//     there is no more data. The walk NEVER stops on the total alone; it
//     always requires this short/empty page too.
//   - unique == total and a full page that just reached total: fetch one more
//     page to confirm the end (a total that is an exact multiple of pageSize
//     is legitimate and ends on a full page).
//   - unique == total and a full page when total was already reached before
//     it: error -- the server keeps returning full pages past the end.
//   - unique < total and an empty page: error -- the list ran out early.
//   - unique < total and a page that added no new key: error -- the server is
//     not advancing (for example ignoring the page parameter); continuing
//     would only re-read the same items.
//
// The maxPages cap is a last-resort guard against an endless walk; exceeding
// it is an error, never a truncated result.
func walkOnce[T any](ctx context.Context, fetch Fetcher[T], key KeyFunc[T], pageSize, maxPages int, subject string) (Result[T], error) {
	base, first, err := probeBase(ctx, fetch, pageSize, subject)
	if err != nil {
		return Result[T]{}, err
	}
	if first == nil {
		// Confirmed zero results (see probeBase): nothing to page through.
		return Result[T]{Items: []T{}, Total: 0, Base: base, Pages: 1}, nil
	}
	return walkFromFirstPage(ctx, fetch, key, pageSize, maxPages, base, first, subject)
}

// walkOnceFixedBase is walkOnce for an endpoint whose page-index base is
// already known and fixed rather than probed -- see
// WalkTwiceVerifiedFromBase's doc comment for why probing is sometimes
// unsafe. It fetches page=base directly: an empty result there is a
// confirmed zero (there is no second candidate base to fall back to, unlike
// probeBase), and a fetch error propagates as-is.
func walkOnceFixedBase[T any](ctx context.Context, fetch Fetcher[T], key KeyFunc[T], pageSize, maxPages, base int, subject string) (Result[T], error) {
	first, err := fetch(ctx, base, pageSize)
	if err != nil {
		return Result[T]{}, fmt.Errorf("%s (page %d): %w", subject, base, err)
	}
	if len(first.Items) == 0 {
		if first.Total != nil && *first.Total != 0 {
			return Result[T]{}, fmt.Errorf(
				"%s reported a total of %d but page %d returned no items; refusing to treat this as zero results",
				subject, *first.Total, base,
			)
		}
		return Result[T]{Items: []T{}, Total: 0, Base: base, Pages: 1}, nil
	}
	return walkFromFirstPage(ctx, fetch, key, pageSize, maxPages, base, &first, subject)
}

// walkFromFirstPage applies the shared hardened-walk rules (see walkOnce's
// doc comment) starting from first, an already-fetched non-empty page at
// index base. Shared by walkOnce (after its base probe) and
// walkOnceFixedBase (after its direct fetch at a known base).
func walkFromFirstPage[T any](ctx context.Context, fetch Fetcher[T], key KeyFunc[T], pageSize, maxPages, base int, first *Page[T], subject string) (Result[T], error) {
	var out []T
	seen := make(map[string]bool)
	reached := false
	total, haveTotal, totalPage := 0, false, -1

	// applyPage folds one page's results (already fetched) into the walk
	// state and reports whether the walk is done, or an error.
	applyPage := func(page int, items []T, totalResults *int) (bool, error) {
		n := len(items)
		switch {
		case n > 0 && totalResults == nil:
			return false, fmt.Errorf("%s response (page %d) has %d items but no reported total", subject, page, n)
		case n > 0 && totalPage >= 0 && *totalResults != total:
			return false, fmt.Errorf("%s total changed from %d to %d between page %d and page %d; the list changed during the walk", subject, total, *totalResults, totalPage, page)
		case n > 0:
			total, haveTotal = *totalResults, true
			if totalPage < 0 {
				totalPage = page
			}
		case !haveTotal:
			t := 0
			if totalResults != nil {
				t = *totalResults
			}
			total, haveTotal = t, true
		}

		before := len(seen)
		for _, item := range items {
			k, kerr := key(item)
			if kerr != nil {
				return false, fmt.Errorf("%s: %w", subject, kerr)
			}
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, item)
		}
		unique := len(seen)
		added := unique - before

		switch {
		case unique > total:
			return false, fmt.Errorf("%s returned %d distinct items by page %d but the reported total is %d", subject, unique, page, total)
		case unique == total && n < pageSize:
			return true, nil
		case unique == total && reached:
			return false, fmt.Errorf("%s returned a full page %d after all %d items were already seen", subject, page, total)
		case unique == total:
			reached = true
		case n == 0:
			return false, fmt.Errorf("%s returned an empty page %d after %d of %d items", subject, page, unique, total)
		case added == 0:
			return false, fmt.Errorf("%s made no progress on page %d: page was full but contained only already-seen items", subject, page)
		}
		return false, nil
	}

	pages := 1
	done, err := applyPage(base, first.Items, first.Total)
	if err != nil {
		return Result[T]{}, err
	}

	for !done {
		pages++
		if pages > maxPages {
			return Result[T]{}, fmt.Errorf(
				"walked %d %s pages (max=%d) without reaching a short/empty page or the reported total; "+
					"aborting rather than silently returning a truncated result set (this likely indicates a misbehaving API response)",
				pages, subject, maxPages,
			)
		}
		page := base + pages - 1
		p, ferr := fetch(ctx, page, pageSize)
		if ferr != nil {
			return Result[T]{}, fmt.Errorf("%s (page %d): %w", subject, page, ferr)
		}
		done, err = applyPage(page, p.Items, p.Total)
		if err != nil {
			return Result[T]{}, err
		}
	}

	if out == nil {
		out = []T{}
	}
	return Result[T]{Items: out, Total: total, Base: base, Pages: pages}, nil
}

// WalkTwiceVerifiedFromBase is WalkTwiceVerified for an endpoint whose
// page-index base is already known and FIXED, never probed. Use this
// instead of WalkTwiceVerified when probeBase's own page-0 probe is itself
// unsafe: some endpoints don't just return an empty/HTTP-204-shaped page for
// an out-of-range index, they reject it outright with a validation error
// (live-confirmed, B13: /api/mdm/updates returns HTTP 422 "Page must be a
// positive numeric value" for page=0, not an empty page) -- calling
// WalkTwiceVerified against such an endpoint would fail the walk on its own
// probe before ever reaching real data. The base is trusted as given, with no
// detection or disagreement check between the two independent walks (there
// is nothing to detect: it was never probed in the first place). Every
// other hardening rule is identical to WalkTwiceVerified: dedup by key,
// strict total (an inconsistent or exceeded total is always an error), the
// maxPages cap, and a full independent second walk for any multi-page
// result.
func WalkTwiceVerifiedFromBase[T any](ctx context.Context, fetch Fetcher[T], key KeyFunc[T], pageSize, maxPages, base int, subject string) (Result[T], error) {
	first, err := walkOnceFixedBase(ctx, fetch, key, pageSize, maxPages, base, subject)
	if err != nil || first.Pages <= 1 {
		return first, err
	}
	second, err := walkOnceFixedBase(ctx, fetch, key, pageSize, maxPages, base, subject)
	switch {
	case err != nil:
		return Result[T]{}, fmt.Errorf("second walk of the %s disagrees with the first, which found %d items: %w", subject, len(first.Items), err)
	case !sameKeySet(first.Items, second.Items, key):
		return Result[T]{}, fmt.Errorf("%s changed between two walks: the first found %d items, the second found %d with a different set of ids", subject, len(first.Items), len(second.Items))
	case second.Total != first.Total:
		return Result[T]{}, fmt.Errorf("%s changed between two walks: total was %d then %d; refusing to return a possibly inconsistent result", subject, first.Total, second.Total)
	}
	return first, nil
}

// probeBase determines whether the endpoint is 0-indexed or 1-indexed FOR
// THIS WALK -- it is never cached across walks/reads, since different
// tenants (potentially served by the same provider process, e.g. across two
// `uem` provider configurations) can disagree.
//
// It requests page 0 first. A non-empty response means the endpoint is
// 0-based, and that page's data is returned as the first page of the walk (so
// the caller never re-fetches it). An empty (or HTTP-204-shaped, i.e. zero
// items and nil Total) response means page 0 is out of range, so page 1 is
// probed next; a non-empty response there means the endpoint is 1-based.
//
// If BOTH probes come back empty, the search is confirmed to have zero
// results ONLY when neither probe reported a non-zero Total. A non-zero Total
// with no items on either probe is a genuine silent-partial-result condition
// (the API claims items exist but neither candidate first page produced any),
// not a legitimate empty answer, and is reported as an error rather than
// silently treated as zero.
//
// Returns (base, nil, nil) when the search is confirmed to have zero
// results -- callers must check for a nil Page, not an empty-but-non-nil
// one, to distinguish "zero results" from "page `base` happened to come back
// empty" (which cannot happen once a base is confirmed, but is kept explicit
// here for clarity).
func probeBase[T any](ctx context.Context, fetch Fetcher[T], pageSize int, subject string) (int, *Page[T], error) {
	p0, err := fetch(ctx, 0, pageSize)
	if err != nil {
		return 0, nil, fmt.Errorf("probing %s page 0: %w", subject, err)
	}
	if len(p0.Items) > 0 {
		return 0, &p0, nil
	}

	p1, err := fetch(ctx, 1, pageSize)
	if err != nil {
		return 0, nil, fmt.Errorf("probing %s page 1: %w", subject, err)
	}
	if len(p1.Items) > 0 {
		return 1, &p1, nil
	}

	nonZero := func(t *int) bool { return t != nil && *t != 0 }
	if nonZero(p0.Total) || nonZero(p1.Total) {
		reported := p0.Total
		if reported == nil {
			reported = p1.Total
		}
		return 0, nil, fmt.Errorf(
			"%s reported a total of %d but page 0 and page 1 both returned no items; refusing to treat this as zero results",
			subject, *reported,
		)
	}
	return 0, nil, nil
}

// sameKeySet reports whether a and b contain exactly the same set of keys
// (same count, same keys; order does not matter). Both slices are the output
// of walkOnce, so key(x) is guaranteed not to error for any x in them.
func sameKeySet[T any](a, b []T, key KeyFunc[T]) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[string]bool, len(a))
	for _, x := range a {
		k, err := key(x)
		if err != nil {
			return false
		}
		ids[k] = true
	}
	for _, x := range b {
		k, err := key(x)
		if err != nil {
			return false
		}
		if !ids[k] {
			return false
		}
	}
	return true
}
