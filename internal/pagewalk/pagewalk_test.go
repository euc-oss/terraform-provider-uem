package pagewalk

import (
	"context"
	"fmt"
	"testing"
)

// item is the generic test payload: an id (the dedupe key) and which fake
// page it was generated on, purely for diagnostics.
type item struct {
	id int
}

func idKey(it item) (string, error) {
	if it.id == 0 {
		return "", fmt.Errorf("item has no id")
	}
	return fmt.Sprintf("%d", it.id), nil
}

func idRange(start, n int) []item {
	out := make([]item, n)
	for i := range out {
		out[i] = item{id: start + i}
	}
	return out
}

// pagedFake is a test-only Fetcher backed by a page->response map, keyed by
// the actual page index the fixture's base uses (0 or 1). A page with no
// entry returns an empty page with a nil Total -- the live-verified
// out-of-range/HTTP-204 shape -- never an error.
type pagedFake struct {
	pages          map[int]Page[item]
	err            error
	pagesRequested []int
}

func (f *pagedFake) fetch(_ context.Context, page, _ int) (Page[item], error) {
	f.pagesRequested = append(f.pagesRequested, page)
	if f.err != nil {
		return Page[item]{}, f.err
	}
	p, ok := f.pages[page]
	if !ok {
		return Page[item]{}, nil
	}
	return p, nil
}

func withTotal(items []item, total int) Page[item] { return Page[item]{Items: items, Total: &total} }

func TestWalkTwiceVerified_ZeroBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = 1200
	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal(idRange(1, 500), total),
		1: withTotal(idRange(501, 500), total),
		2: withTotal(idRange(1001, 200), total), // short: 200 < pageSize(500)
	}}

	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != total {
		t.Fatalf("expected %d items, got %d", total, len(res.Items))
	}
	if res.Base != 0 {
		t.Fatalf("expected base 0, got %d", res.Base)
	}
	if fake.pagesRequested[0] != 0 {
		t.Fatalf("expected the probe to request page 0 first, got: %v", fake.pagesRequested)
	}
}

func TestWalkTwiceVerified_OneBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = 1200
	fake := &pagedFake{pages: map[int]Page[item]{
		// Deliberately no entry for page 0: probing it must come back empty,
		// forcing the probe to fall through to page 1.
		1: withTotal(idRange(1, 500), total),
		2: withTotal(idRange(501, 500), total),
		3: withTotal(idRange(1001, 200), total), // short
	}}

	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != total {
		t.Fatalf("expected %d items, got %d", total, len(res.Items))
	}
	if res.Base != 1 {
		t.Fatalf("expected base 1, got %d", res.Base)
	}
	if len(fake.pagesRequested) < 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected the probe to try page 0 then page 1, got: %v", fake.pagesRequested)
	}
}

// TestWalkTwiceVerified_ZeroBasedPageOneReturns204 proves the exact live
// shape behind release blocker B1's diagnosis: a 0-based tenant whose item
// count is pageSize-aligned (500 items at pageSize 500) returns a FULL page 0
// that reaches the total exactly, and page 1 -- the confirming page the
// hardened walk always fetches after a full page reaches total -- comes back
// as an HTTP-204-shaped empty page (no items, no total). This must be treated
// as the end-of-results confirmation, not an error and not "zero results"
// (page 0 already proved items exist).
func TestWalkTwiceVerified_ZeroBasedPageOneReturns204(t *testing.T) {
	t.Parallel()

	const total = 500
	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal(idRange(1, 500), total),
		// Deliberately no entry for page 1: the confirming fetch comes back
		// as an empty 204-shaped page.
	}}

	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != total {
		t.Fatalf("expected %d items, got %d", total, len(res.Items))
	}
	// pages=2 (page 0 + the confirming page 1) is a multi-page result, so
	// WalkTwiceVerified walks it a SECOND, fully independent time (this fake
	// is deterministic, so the second walk agrees): page 0 then page 1,
	// twice over.
	want := []int{0, 1, 0, 1}
	if len(fake.pagesRequested) != len(want) {
		t.Fatalf("expected page 0 then a confirming page 1, walked twice (%v), got: %v", want, fake.pagesRequested)
	}
	for i, p := range want {
		if fake.pagesRequested[i] != p {
			t.Fatalf("expected page 0 then a confirming page 1, walked twice (%v), got: %v", want, fake.pagesRequested)
		}
	}
}

func TestWalkTwiceVerified_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{pages: map[int]Page[item]{}}
	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err != nil {
		t.Fatalf("expected NO error for a confirmed-empty search, got: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(res.Items))
	}
	if len(fake.pagesRequested) != 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected both page 0 and page 1 to be probed before confirming zero, got: %v", fake.pagesRequested)
	}
}

func TestWalkTwiceVerified_NonZeroTotalNoItemsIsError(t *testing.T) {
	t.Parallel()

	five := 5
	fake := &pagedFake{pages: map[int]Page[item]{
		0: {Items: nil, Total: &five},
	}}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err == nil {
		t.Fatal("expected an error: total=5 reported but no items on page 0 or page 1")
	}
}

func TestWalkTwiceVerified_TotalMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal(idRange(1, 5), 10),
		1: withTotal(idRange(6, 5), 20), // total changed from 10 to 20
	}}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 5, 100, "test search")
	if err == nil {
		t.Fatal("expected an error: total changed between page 0 (10) and page 1 (20)")
	}
}

// twiceWalkMismatchFake simulates a list that changes between the first and
// second (confirming) independent walk of a multi-page result: a new walk
// always begins by probing page 0, so this fake tracks how many times page 0
// has been requested and serves a DIFFERENT (self-consistent) 3-page dataset
// on the second walk than the first.
type twiceWalkMismatchFake struct {
	walk           int
	pagesRequested []int
}

func (f *twiceWalkMismatchFake) fetch(_ context.Context, page, _ int) (Page[item], error) {
	if page == 0 {
		f.walk++
	}
	f.pagesRequested = append(f.pagesRequested, page)

	var total, n int
	switch {
	case f.walk <= 1:
		total = 1200
		switch page {
		case 0, 1:
			n = 500
		case 2:
			n = 200
		}
	default:
		total = 1150
		switch page {
		case 0, 1:
			n = 500
		case 2:
			n = 150
		}
	}
	if page > 2 {
		return Page[item]{}, nil
	}
	return withTotal(idRange(page*10000+1, n), total), nil
}

func TestWalkTwiceVerified_TwiceWalkMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &twiceWalkMismatchFake{}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err == nil {
		t.Fatal("expected an error: the second independent walk disagreed with the first")
	}
	if fake.walk < 2 {
		t.Fatalf("expected a second walk to have been attempted (multi-page result), got %d walk(s)", fake.walk)
	}
}

// infiniteFake always returns a FULL (non-short) page with no reported total,
// forever, for any page requested -- simulating a pathological/misbehaving
// API response that never signals end-of-results.
type infiniteFake struct {
	pageSize       int
	pagesRequested []int
}

func (f *infiniteFake) fetch(_ context.Context, page, _ int) (Page[item], error) {
	f.pagesRequested = append(f.pagesRequested, page)
	return Page[item]{Items: idRange(page*f.pageSize+1, f.pageSize)}, nil
}

func TestWalkTwiceVerified_MaxPageCapFailsLoudly(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &infiniteFake{pageSize: 500}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, testMaxPages, "test search")
	if err == nil {
		t.Fatal("expected an error once the max-pages cap is exceeded, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+2 {
		t.Fatalf("expected the loop to stop at or just past the cap (%d), got %d requests -- it kept looping past the cap", testMaxPages, len(fake.pagesRequested))
	}
}

// boundedFake serves fresh, full 0-indexed pages that all report the same
// total (lastPage+1 full pages) and an empty page past lastPage, so a walk is
// internally consistent and only the page cap can stop it before it
// completes.
type boundedFake struct {
	lastPage       int
	pageSize       int
	pagesRequested []int
}

func (f *boundedFake) fetch(_ context.Context, page, _ int) (Page[item], error) {
	f.pagesRequested = append(f.pagesRequested, page)
	if page > f.lastPage {
		return Page[item]{}, nil
	}
	total := (f.lastPage + 1) * f.pageSize
	return withTotal(idRange(page*f.pageSize+1, f.pageSize), total), nil
}

// TestWalkTwiceVerified_MaxPageCapStopsConsistentWalk proves the cap fires
// even against an internally-consistent, well-behaved walk that would
// otherwise succeed after more pages than the cap allows: removing the cap
// makes this test's error expectation fail.
func TestWalkTwiceVerified_MaxPageCapStopsConsistentWalk(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &boundedFake{lastPage: 19, pageSize: 500}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, testMaxPages, "test search")
	if err == nil {
		t.Fatal("expected the max-pages cap to stop a consistent 20-page walk with an error, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+1 {
		t.Fatalf("expected at most %d page requests before the cap fired, got %d: %v", testMaxPages+1, len(fake.pagesRequested), fake.pagesRequested)
	}
}

func TestWalkTwiceVerified_NoProgressIsError(t *testing.T) {
	t.Parallel()

	const total = 1000
	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal(idRange(1, 500), total),
		1: withTotal(idRange(1, 500), total), // same ids as page 0: no progress
	}}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 5, "test search")
	if err == nil {
		t.Fatal("expected an error: page 1 repeated page 0's ids and made no progress toward the total")
	}
	// The no-progress guard must fire IMMEDIATELY on page 1 (exactly 2
	// requests: the page-0 probe, then the repeated page 1) rather than
	// continuing on to page 2+.
	if len(fake.pagesRequested) != 2 {
		t.Fatalf("expected the no-progress guard to stop the walk after exactly 2 requests, got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

func TestWalkTwiceVerified_DedupsAcrossPages(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal([]item{{id: 1}, {id: 2}}, 3),
		1: withTotal([]item{{id: 2}, {id: 3}}, 3), // 2 overlaps with page 0
	}}
	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 2, 100, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	seen := map[int]int{}
	for _, it := range res.Items {
		seen[it.id]++
	}
	if seen[2] != 1 {
		t.Fatalf("expected item 2 (overlapping across pages 0 and 1) to appear exactly once, got %d: %+v", seen[2], res.Items)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 deduped items, got %d: %+v", len(res.Items), res.Items)
	}
}

// TestWalkTwiceVerified_NoKeyIsError proves an item with no usable key errors
// at once rather than being silently skipped or breaking dedup.
func TestWalkTwiceVerified_NoKeyIsError(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal([]item{{id: 0}}, 1), // id 0 -> idKey returns an error
	}}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err == nil {
		t.Fatal("expected an error: item has no usable key")
	}
}

// TestWalkTwiceVerified_FetchErrorPropagates proves a transport/API error
// from the fetcher is returned as-is (wrapped), not swallowed.
func TestWalkTwiceVerified_FetchErrorPropagates(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{err: fmt.Errorf("boom")}
	_, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err == nil {
		t.Fatal("expected the fetch error to propagate")
	}
}

// TestWalkTwiceVerified_SinglePageIsNotWalkedTwice proves the common case (a
// single-page result, no multi-page window for the list to shift under) makes
// exactly one page-0 request and is not repeated.
func TestWalkTwiceVerified_SinglePageIsNotWalkedTwice(t *testing.T) {
	t.Parallel()

	fake := &pagedFake{pages: map[int]Page[item]{
		0: withTotal(idRange(1, 3), 3), // short page: 3 < pageSize(500)
	}}
	res, err := WalkTwiceVerified(context.Background(), fake.fetch, idKey, 500, 100, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Items))
	}
	if len(fake.pagesRequested) != 1 {
		t.Fatalf("expected exactly 1 request for a single-page result, got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// --- WalkTwiceVerifiedFromBase (B13) ----------------------------------------

// pageZeroErrorsFake is a Fetcher for an endpoint that REJECTS page 0
// outright (a validation error, not an empty/HTTP-204-shaped page) --
// exactly the live shape WalkTwiceVerifiedFromBase exists for (B13:
// /api/mdm/updates returns HTTP 422 "Page must be a positive numeric value"
// for page=0). Real pages start at fixedBase and behave like pagedFake.
type pageZeroErrorsFake struct {
	pages          map[int]Page[item]
	pagesRequested []int
}

func (f *pageZeroErrorsFake) fetch(_ context.Context, page, _ int) (Page[item], error) {
	f.pagesRequested = append(f.pagesRequested, page)
	if page == 0 {
		return Page[item]{}, fmt.Errorf("API error 422: Page must be a positive numeric value")
	}
	p, ok := f.pages[page]
	if !ok {
		return Page[item]{}, nil
	}
	return p, nil
}

// TestWalkTwiceVerifiedFromBase_NeverProbesPageZero is the fail-on-revert
// test for the base probe (B13): against pageZeroErrorsFake,
// WalkTwiceVerified itself would fail immediately (its probe always tries
// page 0 first). WalkTwiceVerifiedFromBase must succeed, having never
// requested page 0 at all.
func TestWalkTwiceVerifiedFromBase_NeverProbesPageZero(t *testing.T) {
	t.Parallel()

	fake := &pageZeroErrorsFake{pages: map[int]Page[item]{
		1: withTotal(idRange(1, 3), 3), // short page: 3 < pageSize(500)
	}}

	res, err := WalkTwiceVerifiedFromBase(context.Background(), fake.fetch, idKey, 500, 100, 1, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(res.Items))
	}
	if res.Base != 1 {
		t.Fatalf("expected base 1, got %d", res.Base)
	}
	for _, p := range fake.pagesRequested {
		if p == 0 {
			t.Fatalf("page 0 must never be requested, got requests: %v", fake.pagesRequested)
		}
	}

	// Sanity check the fake itself: WalkTwiceVerified (which DOES probe page
	// 0 first) must fail against it, proving pageZeroErrorsFake really does
	// reject page 0 rather than silently tolerating it.
	probingFake := &pageZeroErrorsFake{pages: fake.pages}
	if _, err := WalkTwiceVerified(context.Background(), probingFake.fetch, idKey, 500, 100, "test search"); err == nil {
		t.Fatal("expected WalkTwiceVerified's own page-0 probe to fail against pageZeroErrorsFake")
	}
}

// TestWalkTwiceVerifiedFromBase_MultiPageWalkedTwice confirms a multi-page
// result under a fixed base still gets the full independent second walk,
// with the SAME fixed base reused (never re-probed) both times.
func TestWalkTwiceVerifiedFromBase_MultiPageWalkedTwice(t *testing.T) {
	t.Parallel()

	const total = 700
	fake := &pageZeroErrorsFake{pages: map[int]Page[item]{
		1: withTotal(idRange(1, 500), total),
		2: withTotal(idRange(501, 200), total), // short: 200 < pageSize(500)
	}}

	res, err := WalkTwiceVerifiedFromBase(context.Background(), fake.fetch, idKey, 500, 100, 1, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != total {
		t.Fatalf("expected %d items, got %d", total, len(res.Items))
	}
	if len(fake.pagesRequested) != 4 {
		t.Fatalf("expected 4 requests (2 pages x 2 independent walks), got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// TestWalkTwiceVerifiedFromBase_EmptyIsZeroResults: an empty page at the
// fixed base, with no (or zero) reported total, is a confirmed zero-result
// walk -- not an error, and (unlike probeBase) never tries a second
// candidate base.
func TestWalkTwiceVerifiedFromBase_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pageZeroErrorsFake{pages: map[int]Page[item]{}}
	res, err := WalkTwiceVerifiedFromBase(context.Background(), fake.fetch, idKey, 500, 100, 1, "test search")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(res.Items))
	}
	if len(fake.pagesRequested) != 1 {
		t.Fatalf("expected exactly 1 request (no second candidate base to try), got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// TestWalkTwiceVerifiedFromBase_NonZeroTotalNoItemsIsError mirrors
// probeBase's own anomaly check: a nonzero total with no items at the fixed
// base is a genuine inconsistency, not a legitimate empty answer.
func TestWalkTwiceVerifiedFromBase_NonZeroTotalNoItemsIsError(t *testing.T) {
	t.Parallel()

	total := 5
	fake := &pageZeroErrorsFake{pages: map[int]Page[item]{
		1: {Items: nil, Total: &total},
	}}
	_, err := WalkTwiceVerifiedFromBase(context.Background(), fake.fetch, idKey, 500, 100, 1, "test search")
	if err == nil {
		t.Fatal("expected an error for a nonzero total with no items")
	}
}

// TestWalkTwiceVerifiedFromBase_StrictTotalStillEnforced confirms the shared
// hardening (strict total check) applies through the fixed-base path too,
// not just the probed one.
func TestWalkTwiceVerifiedFromBase_StrictTotalStillEnforced(t *testing.T) {
	t.Parallel()

	fake := &pageZeroErrorsFake{pages: map[int]Page[item]{
		1: withTotal(idRange(1, 3), 2), // 3 distinct items, total claims 2
	}}
	_, err := WalkTwiceVerifiedFromBase(context.Background(), fake.fetch, idKey, 500, 100, 1, "test search")
	if err == nil {
		t.Fatal("expected an error when distinct items exceed the reported total")
	}
}
