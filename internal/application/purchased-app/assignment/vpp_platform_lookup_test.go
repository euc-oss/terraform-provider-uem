package assignment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// fakePurchasedAppSearchAPI is the test double for purchasedAppSearchAPI: a
// single fixed result returned regardless of the requested page, for tests
// where every fetch (both probe calls, and both independent
// pagewalk.WalkTwiceVerified walks for a multi-page result) must see the
// same, stable single-page catalog.
type fakePurchasedAppSearchAPI struct {
	result       *sdk.PurchasedApplicationSearchResultV1
	err          error
	lastOpts     *sdk.PurchasedAppsV1VppAppSearchAsyncOptions
	capturedOpts bool
	calls        int
}

func (f *fakePurchasedAppSearchAPI) VppAppSearchAsync(_ context.Context, opts *sdk.PurchasedAppsV1VppAppSearchAsyncOptions) (http.Header, *sdk.PurchasedApplicationSearchResultV1, error) {
	f.lastOpts = opts
	f.capturedOpts = true
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func intPtr(v int) *int { return &v }

// TestDefaultVppPlatformLookup_MatchesByUUIDAmongSeveralResults pins B24: no
// bundle id or org group filter is sent (the V2 GET that used to supply them
// is gone — see vpp_platform_lookup.go's file-level comment), yet the
// correct entity is still found and mapped purely by uuid. The fixed result
// fits on one page, so pagewalk.WalkTwiceVerified's base probe (page 0,
// non-empty) resolves it in a single call and there is no second walk.
func TestDefaultVppPlatformLookup_MatchesByUUIDAmongSeveralResults(t *testing.T) {
	appUUID := "596b30c4-5fd8-f8a4-2f40-553c312b9f1a"
	total := 3
	search := &fakePurchasedAppSearchAPI{result: &sdk.PurchasedApplicationSearchResultV1{
		Application: []sdk.PurchasedApplicationEntityV1{
			{UUID: "bafde89c-041e-1756-082b-933aaf16cad8", Platform: intPtr(2)},
			{UUID: appUUID, Platform: intPtr(10)},
			{UUID: "05d17100-b346-c29d-6760-a0fdedcf8623", Platform: intPtr(2)},
		},
		Total: &total,
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), appUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformMacOS {
		t.Fatalf("expected macOS, got %v", platform)
	}
	if search.calls != 1 {
		t.Fatalf("expected exactly 1 call (base-0 probe resolves a single-page result), got %d", search.calls)
	}
	if !search.capturedOpts || search.lastOpts == nil {
		t.Fatal("expected search to be called")
	}
	if search.lastOpts.Bundleid != nil {
		t.Fatalf("B24: expected NO bundle id filter (unfiltered search), got %v", *search.lastOpts.Bundleid)
	}
	if search.lastOpts.OrganizationGroupUUID != nil {
		t.Fatalf("B24: expected NO org group filter (unfiltered search), got %v", *search.lastOpts.OrganizationGroupUUID)
	}
}

func TestDefaultVppPlatformLookup_MatchesUUIDCaseInsensitively(t *testing.T) {
	total := 1
	search := &fakePurchasedAppSearchAPI{result: &sdk.PurchasedApplicationSearchResultV1{
		Application: []sdk.PurchasedApplicationEntityV1{
			{UUID: "596b30c4-5fd8-f8a4-2f40-553c312b9f1a", Platform: intPtr(2)},
		},
		Total: &total,
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), "596b30c4-5fd8-f8a4-2f40-553c312b9f1a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformIOS {
		t.Fatalf("expected iOS, got %v", platform)
	}
}

func TestDefaultVppPlatformLookup_PlatformMapping(t *testing.T) {
	appUUID := "app-uuid"
	tests := []struct {
		name     string
		platform *int
		want     vppPlatform
	}{
		{"2 is iOS", intPtr(2), vppPlatformIOS},
		{"10 is macOS", intPtr(10), vppPlatformMacOS},
		{"other code is unknown", intPtr(99), vppPlatformUnknown},
		{"nil is unknown", nil, vppPlatformUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := 1
			search := &fakePurchasedAppSearchAPI{result: &sdk.PurchasedApplicationSearchResultV1{
				Application: []sdk.PurchasedApplicationEntityV1{{UUID: appUUID, Platform: tt.platform}},
				Total:       &total,
			}}
			l := &defaultVppPlatformLookup{search: search}

			got, err := l.LookupVppPlatform(context.Background(), appUUID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDefaultVppPlatformLookup_NoMatchIsUnknown: a COMPLETE walk that simply
// never contains appUUID is the one case allowed to report Unknown with no
// error (see vppPlatformLookupAPI's doc comment).
func TestDefaultVppPlatformLookup_NoMatchIsUnknown(t *testing.T) {
	total := 1
	search := &fakePurchasedAppSearchAPI{result: &sdk.PurchasedApplicationSearchResultV1{
		Application: []sdk.PurchasedApplicationEntityV1{
			{UUID: "not-the-app", Platform: intPtr(2)},
		},
		Total: &total,
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), "app-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformUnknown {
		t.Fatalf("expected unknown for no match, got %v", platform)
	}
}

func TestDefaultVppPlatformLookup_SearchErrorPropagates(t *testing.T) {
	search := &fakePurchasedAppSearchAPI{err: errors.New("search failed")}
	l := &defaultVppPlatformLookup{search: search}

	_, err := l.LookupVppPlatform(context.Background(), "app-uuid")
	if err == nil {
		t.Fatal("expected error to propagate from the V1 search call")
	}
}

// TestVppAppSearchOptions_NoFilters pins B24: the options builder never sets
// a bundle id or org group filter -- that's the whole point of the fix (see
// vpp_platform_lookup.go's file-level comment). Fails on revert: adding
// either filter back makes this fail.
func TestVppAppSearchOptions_NoFilters(t *testing.T) {
	opts := vppAppSearchOptions(0, vppSearchPageSize)
	if opts.Bundleid != nil {
		t.Fatalf("expected no bundle id filter, got %v", *opts.Bundleid)
	}
	if opts.OrganizationGroupUUID != nil {
		t.Fatalf("expected no org group filter, got %v", *opts.OrganizationGroupUUID)
	}
}

// TestVppAppSearchOptions_CarriesPageAndPageSize proves the page/pagesize
// values passed in are carried onto the options unchanged (internal-task minor
// 3).
func TestVppAppSearchOptions_CarriesPageAndPageSize(t *testing.T) {
	opts := vppAppSearchOptions(3, 500)
	if opts.Page == nil || *opts.Page != 3 {
		t.Fatalf("expected Page = 3, got %v", opts.Page)
	}
	if opts.PageSize == nil || *opts.PageSize != 500 {
		t.Fatalf("expected PageSize = 500, got %v", opts.PageSize)
	}
}

func TestNewDefaultVppPlatformLookup_BuildsFromSDKClient(t *testing.T) {
	l := newDefaultVppPlatformLookup(&sdk.Client{})
	if l == nil {
		t.Fatal("expected non-nil lookup")
	}
	if _, ok := l.(*defaultVppPlatformLookup); !ok {
		t.Fatalf("expected *defaultVppPlatformLookup, got %T", l)
	}
}

func TestDefaultVppPlatformLookupFactory(t *testing.T) {
	l := defaultVppPlatformLookupFactory(&sdk.Client{})
	if l == nil {
		t.Fatal("expected non-nil lookup")
	}
}

// --- pagewalk-backed pagination tests (B24) ---------------------------------
//
// Live-verified contract for /api/mam/apps/purchased/search: pages are
// 0-indexed, the response's Total field is the count of items IN THAT PAGE
// (never the overall total, so pagewalk trusts it only across non-empty
// pages), and the default page size when unset is 500.
//
// funcPurchasedAppSearchAPI's fn is keyed by the REQUESTED page number (not a
// raw call ordinal): pagewalk.WalkTwiceVerified probes page 0 (and page 1 if
// page 0 is empty) independently for EACH of up to two full walks, so a fake
// keyed by call order would desync on any multi-page scenario. Keying by the
// actual page argument makes the fake behave like a real, unchanging catalog
// -- both walks see identical data for identical page requests.

// funcPurchasedAppSearchAPI is a test double for purchasedAppSearchAPI whose
// response is computed per call from the REQUESTED page index.
type funcPurchasedAppSearchAPI struct {
	fn         func(page int) *sdk.PurchasedApplicationSearchResultV1
	err        error
	calls      int
	optsByCall []*sdk.PurchasedAppsV1VppAppSearchAsyncOptions
}

func (f *funcPurchasedAppSearchAPI) VppAppSearchAsync(_ context.Context, opts *sdk.PurchasedAppsV1VppAppSearchAsyncOptions) (http.Header, *sdk.PurchasedApplicationSearchResultV1, error) {
	f.optsByCall = append(f.optsByCall, opts)
	f.calls++
	if f.err != nil {
		return nil, nil, f.err
	}
	page := 0
	if opts.Page != nil {
		page = *opts.Page
	}
	return nil, f.fn(page), nil
}

// fullNonMatchingPage returns n entities for the given page index, none of
// which are uuid, with keys that are unique both within the page and across
// different page indices (so a multi-page walk's dedup count grows
// correctly, and a second independent walk requesting the same page indices
// again sees IDENTICAL entities).
func fullNonMatchingPage(n, page int, uuid string) []sdk.PurchasedApplicationEntityV1 {
	entities := make([]sdk.PurchasedApplicationEntityV1, n)
	for i := range entities {
		entities[i] = sdk.PurchasedApplicationEntityV1{UUID: fmt.Sprintf("non-match-p%d-%d-%s", page, i, uuid), Platform: intPtr(2)}
	}
	return entities
}

// TestDefaultVppPlatformLookup_FoundOnSecondPageAfterFullFirstPage: page 0 is
// a full, non-matching page; page 1 holds the single matching entity. Total
// pages of DATA walked is 2, so pagewalk.WalkTwiceVerified performs a
// second, fully independent walk to confirm the result -- doubling the call
// count to 4 (each walk re-probes page 0, then fetches page 1).
func TestDefaultVppPlatformLookup_FoundOnSecondPageAfterFullFirstPage(t *testing.T) {
	appUUID := "596b30c4-5fd8-f8a4-2f40-553c312b9f1a"
	total := vppSearchPageSize + 1
	search := &funcPurchasedAppSearchAPI{fn: func(page int) *sdk.PurchasedApplicationSearchResultV1 {
		switch page {
		case 0:
			// A full page (== vppSearchPageSize), none matching: pagination
			// must continue to page 1.
			return &sdk.PurchasedApplicationSearchResultV1{Application: fullNonMatchingPage(vppSearchPageSize, 0, appUUID), Total: &total}
		case 1:
			return &sdk.PurchasedApplicationSearchResultV1{Application: []sdk.PurchasedApplicationEntityV1{
				{UUID: appUUID, Platform: intPtr(10)},
			}, Total: &total}
		default:
			t.Fatalf("expected only pages 0 and 1 to be requested, got page %d", page)
			return nil
		}
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), appUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformMacOS {
		t.Fatalf("expected macOS, got %v", platform)
	}
	if search.calls != 4 {
		t.Fatalf("expected exactly 4 calls (2 independent walks x page 0 + page 1), got %d", search.calls)
	}
	if search.optsByCall[0].Page == nil || *search.optsByCall[0].Page != 0 {
		t.Fatalf("expected first call's Page = 0, got %v", search.optsByCall[0].Page)
	}
	if search.optsByCall[1].Page == nil || *search.optsByCall[1].Page != 1 {
		t.Fatalf("expected second call's Page = 1, got %v", search.optsByCall[1].Page)
	}
	for i, opts := range search.optsByCall {
		if opts.PageSize == nil || *opts.PageSize != vppSearchPageSize {
			t.Fatalf("call %d: expected PageSize = %d, got %v", i, vppSearchPageSize, opts.PageSize)
		}
	}
}

// TestDefaultVppPlatformLookup_EmptyResultIsUnknown: both base-probe
// candidates (page 0, page 1) come back empty with no reported total --
// pagewalk confirms zero results without error, and LookupVppPlatform
// reports Unknown with no error (the "complete walk, genuinely not found"
// case).
func TestDefaultVppPlatformLookup_EmptyResultIsUnknown(t *testing.T) {
	search := &funcPurchasedAppSearchAPI{fn: func(int) *sdk.PurchasedApplicationSearchResultV1 {
		return &sdk.PurchasedApplicationSearchResultV1{Application: nil}
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), "app-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformUnknown {
		t.Fatalf("expected unknown for an empty result, got %v", platform)
	}
	if search.calls != 2 {
		t.Fatalf("expected exactly 2 calls (probe page 0, then page 1, both empty), got %d", search.calls)
	}
}

func TestDefaultVppPlatformLookup_ShortPageStopsWithoutNextRequest(t *testing.T) {
	total := 2
	search := &funcPurchasedAppSearchAPI{fn: func(page int) *sdk.PurchasedApplicationSearchResultV1 {
		if page != 0 {
			t.Fatalf("expected only page 0 to be requested, got page %d", page)
		}
		// Fewer than vppSearchPageSize items, matching the reported total:
		// must stop here, no probe of page 1 needed (page 0 was non-empty).
		return &sdk.PurchasedApplicationSearchResultV1{Application: []sdk.PurchasedApplicationEntityV1{
			{UUID: "not-the-app", Platform: intPtr(2)},
			{UUID: "also-not-the-app", Platform: intPtr(10)},
		}, Total: &total}
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), "app-uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformUnknown {
		t.Fatalf("expected unknown, got %v", platform)
	}
	if search.calls != 1 {
		t.Fatalf("expected exactly 1 call (a short, non-empty page 0 both confirms base=0 and ends the walk), got %d", search.calls)
	}
}

// TestDefaultVppPlatformLookup_PageCapIsAnError (B21, adapted to
// pagewalk.WalkTwiceVerified in B24): every page is full and none match, and
// the reported total is never reachable within the page cap. The walk must
// error, never return a silent Unknown. Fail-on-revert: reverting
// LookupVppPlatform to the old hand-rolled loop (or any change that swallows
// pagewalk's cap error) makes this fail.
func TestDefaultVppPlatformLookup_PageCapIsAnError(t *testing.T) {
	appUUID := "app-uuid"
	unreachableTotal := vppSearchMaxPages*vppSearchPageSize + 1
	search := &funcPurchasedAppSearchAPI{fn: func(page int) *sdk.PurchasedApplicationSearchResultV1 {
		// Always a new full page, never matching, with a total no amount of
		// paging within the cap can reach: the walk only ends at the page
		// cap, which must be reported, not returned as a silent Unknown.
		return &sdk.PurchasedApplicationSearchResultV1{Application: fullNonMatchingPage(vppSearchPageSize, page, appUUID), Total: &unreachableTotal}
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), appUUID)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("(max=%d)", vppSearchMaxPages)) {
		t.Fatalf("expected the page-cap error, got: %v", err)
	}
	if platform != vppPlatformUnknown {
		t.Fatalf("expected unknown when the page cap is hit, got %v", platform)
	}
	if search.calls != vppSearchMaxPages {
		t.Fatalf("expected exactly %d calls (the page cap; a cap hit is a first-walk error, so there is no second walk), got %d", vppSearchMaxPages, search.calls)
	}
}

// TestDefaultVppPlatformLookup_RepeatedFullPageIsAnError (B21, adapted to
// pagewalk.WalkTwiceVerified in B24): the server returns the SAME full page
// for both page 0 and page 1 (ignoring the page parameter) while claiming
// more items exist than have been seen -- no progress is made, which must
// error rather than silently report Unknown.
func TestDefaultVppPlatformLookup_RepeatedFullPageIsAnError(t *testing.T) {
	appUUID := "app-uuid"
	claimedTotal := vppSearchPageSize + 1 // more than the one distinct page's worth of items
	search := &funcPurchasedAppSearchAPI{fn: func(int) *sdk.PurchasedApplicationSearchResultV1 {
		// The same full page every time (page argument ignored): the server
		// is not advancing.
		return &sdk.PurchasedApplicationSearchResultV1{Application: fullNonMatchingPage(vppSearchPageSize, 0, appUUID), Total: &claimedTotal}
	}}
	l := &defaultVppPlatformLookup{search: search}

	_, err := l.LookupVppPlatform(context.Background(), appUUID)
	if err == nil || !strings.Contains(err.Error(), "made no progress on page 1") {
		t.Fatalf("expected a no-progress error on page 1, got: %v", err)
	}
	if search.calls != 2 {
		t.Fatalf("expected 2 calls (page 0, then a repeated page 1), got %d", search.calls)
	}
}

// TestDefaultVppPlatformLookup_BaseProbeFallsBackToPageOne is the fail-on-
// revert test for the base probe (B24): page 0 is out of range (empty, no
// total) but page 1 holds the real, single-page catalog including the
// matching app. The old hand-rolled loop hardcoded `for page := 0; ...` and
// would have stopped at the first empty page, never trying page 1 -- this
// test fails immediately under that shape (Unknown instead of the real
// platform) and passes only because pagewalk.WalkTwiceVerified's probeBase
// tries page 1 when page 0 comes back empty.
func TestDefaultVppPlatformLookup_BaseProbeFallsBackToPageOne(t *testing.T) {
	appUUID := "596b30c4-5fd8-f8a4-2f40-553c312b9f1a"
	total := 1
	search := &funcPurchasedAppSearchAPI{fn: func(page int) *sdk.PurchasedApplicationSearchResultV1 {
		if page == 0 {
			return &sdk.PurchasedApplicationSearchResultV1{Application: nil}
		}
		return &sdk.PurchasedApplicationSearchResultV1{Application: []sdk.PurchasedApplicationEntityV1{
			{UUID: appUUID, Platform: intPtr(2)},
		}, Total: &total}
	}}
	l := &defaultVppPlatformLookup{search: search}

	platform, err := l.LookupVppPlatform(context.Background(), appUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform != vppPlatformIOS {
		t.Fatalf("expected iOS (found on page 1 after an empty page 0 probe), got %v", platform)
	}
	if search.calls != 2 {
		t.Fatalf("expected exactly 2 calls (empty page-0 probe, then page 1), got %d", search.calls)
	}
	if search.optsByCall[0].Page == nil || *search.optsByCall[0].Page != 0 {
		t.Fatalf("expected the first call to probe page 0, got %v", search.optsByCall[0].Page)
	}
	if search.optsByCall[1].Page == nil || *search.optsByCall[1].Page != 1 {
		t.Fatalf("expected the second call to probe page 1, got %v", search.optsByCall[1].Page)
	}
}
