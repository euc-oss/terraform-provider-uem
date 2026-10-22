package scripts

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- Pagination (internal-ticket, release blocker B12):
// GetScriptsByOrganizationGroupAsync paginates via Page/PageSize + SearchResults/RecordCount,
// no cursor. Mirrors internal/profile/data_source_test.go's pagination test shape, adapted to
// the scripts SDK service and to calling scriptSearch.List directly (the seam
// scriptlookup.Lister exposes GetScriptsByOrganizationGroupAsync, not the framework-level
// Read). The heavy-lifting walk/probe logic itself is exhaustively covered generically in
// internal/pagewalk; these tests pin scriptSearch's wiring onto it: dedupe key
// (lower-cased ScriptUUID), filter propagation, OG-UUID pass-through, and the fail-closed
// guards surfacing as errors through List. ---

func intPtr(i int) *int { return &i }

func namesFor(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return out
}

func scriptPage(names []string, total int) *sdk.ScriptsSearchResultV1 {
	items := make([]sdk.ScriptResourceLiteV1, len(names))
	for i, n := range names {
		items[i] = sdk.ScriptResourceLiteV1{
			Name:                  n,
			ScriptUUID:            n, // names are unique per test fixture; used directly as the dedupe key
			OrganizationGroupUUID: "og-1",
			Platform:              "APPLE_OSX",
			ScriptType:            "BASH",
			Version:               "1",
		}
	}
	return &sdk.ScriptsSearchResultV1{SearchResults: items, RecordCount: intPtr(total)}
}

// pagedScriptFake is a test-only fake satisfying scriptlookup.Lister whose
// response depends on the requested page (keyed by the ACTUAL page index the
// fixture's base uses -- 0 or 1). A page with no entry returns an empty body
// with RecordCount 0 -- the live-verified out-of-range shape on both as<internal-env>
// and paul-2609 -- never an error.
type pagedScriptFake struct {
	pages          map[int]*sdk.ScriptsSearchResultV1
	err            error
	pagesRequested []int
	lastOGUUID     string
	lastNames      []*string
}

func (f *pagedScriptFake) GetScriptsByOrganizationGroupAsync(_ context.Context, ogUUID string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	f.lastOGUUID = ogUUID
	if f.err != nil {
		return nil, nil, f.err
	}
	if opts.Page == nil {
		return nil, nil, fmt.Errorf("test fake: called without a Page option set -- the fix must always set one explicitly")
	}
	f.lastNames = append(f.lastNames, opts.Name)
	p := *opts.Page
	f.pagesRequested = append(f.pagesRequested, p)
	res, ok := f.pages[p]
	if !ok {
		return nil, &sdk.ScriptsSearchResultV1{SearchResults: nil, RecordCount: intPtr(0)}, nil
	}
	return nil, res, nil
}

func listScripts(t *testing.T, s *scriptSearch) ([]ScriptSummary, error) {
	t.Helper()
	return s.List(context.Background(), scriptFilters{
		OrganizationGroupUuid: types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
	})
}

// TestScriptSearch_List_ZeroBasedMultiPage proves a 2-page, 0-INDEXED result
// (live-confirmed shape on both as<internal-env> and paul-2609) accumulates correctly
// across both pages: page 0 is a full page (pageSize items), so the walk
// continues to page 1 to confirm the end.
func TestScriptSearch_List_ZeroBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = scriptSearchPageSize + 200
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: scriptPage(namesFor("p0", scriptSearchPageSize), total),
		1: scriptPage(namesFor("p1", 200), total), // short: 200 < pageSize
	}}
	out, err := listScripts(t, &scriptSearch{svc: fake})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != total {
		t.Fatalf("expected %d scripts, got %d", total, len(out))
	}
	if fake.pagesRequested[0] != 0 {
		t.Fatalf("expected the probe to request page 0 first, got: %v", fake.pagesRequested)
	}
	if fake.lastOGUUID != "bafde89c-041e-1756-082b-933aaf16cad8" {
		t.Fatalf("expected the OG uuid to reach the SDK call, got %q", fake.lastOGUUID)
	}
}

// TestScriptSearch_List_OneBasedMultiPage proves the SAME shape walks
// correctly when 1-INDEXED: page 0 is probed first, comes back empty (no
// entry in the fixture), so page 1 is probed next and detected as the base.
func TestScriptSearch_List_OneBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = scriptSearchPageSize + 50
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		// no entry for page 0: forces the fallback probe to page 1.
		1: scriptPage(namesFor("p1", scriptSearchPageSize), total),
		2: scriptPage(namesFor("p2", 50), total), // short
	}}
	out, err := listScripts(t, &scriptSearch{svc: fake})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != total {
		t.Fatalf("expected %d scripts, got %d", total, len(out))
	}
	if len(fake.pagesRequested) < 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected the probe to try page 0 then page 1, got: %v", fake.pagesRequested)
	}
}

// TestScriptSearch_List_ZeroBasedPageOneReturns200Empty proves the live
// out-of-range shape observed on both as<internal-env> and paul-2609 (HTTP 200 with an
// empty SearchResults and RecordCount 0, not a 204): a page-size-aligned
// result's confirming page must be treated as end-of-results, not an error.
func TestScriptSearch_List_ZeroBasedPageOneReturns200Empty(t *testing.T) {
	t.Parallel()

	const total = scriptSearchPageSize
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: scriptPage(namesFor("p0", scriptSearchPageSize), total),
		// no entry for page 1: served as {SearchResults: nil, RecordCount: 0}
	}}
	out, err := listScripts(t, &scriptSearch{svc: fake})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != total {
		t.Fatalf("expected %d scripts, got %d", total, len(out))
	}
}

func TestScriptSearch_List_UsesScriptUUIDAsDedupeKey(t *testing.T) {
	t.Parallel()

	items := []sdk.ScriptResourceLiteV1{
		{Name: "a", ScriptUUID: "UUID-A", OrganizationGroupUUID: "og-1", Platform: "APPLE_OSX", ScriptType: "BASH", Version: "1"},
	}
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: {SearchResults: items, RecordCount: intPtr(1)},
	}}
	out, err := listScripts(t, &scriptSearch{svc: fake})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].ScriptUuid.ValueString() != "UUID-A" {
		t.Fatalf("expected 1 script with uuid UUID-A, got %+v", out)
	}
}

func TestScriptSearch_List_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{}}
	out, err := listScripts(t, &scriptSearch{svc: fake})
	if err != nil {
		t.Fatalf("expected no error for a confirmed-empty search, got: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected 0 scripts, got %d", len(out))
	}
	if len(fake.pagesRequested) != 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected both page 0 and page 1 to be probed before confirming zero, got: %v", fake.pagesRequested)
	}
}

// TestScriptSearch_List_TotalMismatchIsError proves that a RecordCount which
// changes between two non-empty pages of the SAME walk is an error.
func TestScriptSearch_List_TotalMismatchIsError(t *testing.T) {
	t.Parallel()

	full := scriptPage(namesFor("p0", scriptSearchPageSize), scriptSearchPageSize)
	page1 := scriptPage(namesFor("p1", 1), 999) // RecordCount changed
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: full,
		1: page1,
	}}
	_, err := listScripts(t, &scriptSearch{svc: fake})
	if err == nil {
		t.Fatalf("expected an error: RecordCount changed between page 0 (%d) and page 1 (999)", scriptSearchPageSize)
	}
}

// TestScriptSearch_List_NoProgressIsError proves the no-progress guard: a
// full page that adds no new (unique) script uuid must error rather than loop
// forever or silently stop short.
func TestScriptSearch_List_NoProgressIsError(t *testing.T) {
	t.Parallel()

	names := namesFor("p0", scriptSearchPageSize)
	full := scriptPage(names, scriptSearchPageSize*2)
	repeated := scriptPage(names, scriptSearchPageSize*2) // same names/uuids as page 0
	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: full,
		1: repeated,
	}}
	_, err := listScripts(t, &scriptSearch{svc: fake, maxPages: 5})
	if err == nil {
		t.Fatal("expected an error: page 1 repeated page 0's scripts and made no progress toward RecordCount")
	}
	if len(fake.pagesRequested) != 2 {
		t.Fatalf("expected the no-progress guard to stop the walk after exactly 2 requests, got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// twiceWalkMismatchScriptFake simulates a script list that changes between
// the first and second (confirming) independent walk of a multi-page result.
type twiceWalkMismatchScriptFake struct {
	walk           int
	pagesRequested []int
}

func (f *twiceWalkMismatchScriptFake) GetScriptsByOrganizationGroupAsync(_ context.Context, _ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	p := *opts.Page
	if p == 0 {
		f.walk++
	}
	f.pagesRequested = append(f.pagesRequested, p)

	var total, n int
	switch {
	case f.walk <= 1:
		total = scriptSearchPageSize + 200
		switch p {
		case 0:
			n = scriptSearchPageSize
		case 1:
			n = 200
		}
	default:
		total = scriptSearchPageSize + 150
		switch p {
		case 0:
			n = scriptSearchPageSize
		case 1:
			n = 150
		}
	}
	if p > 1 {
		return nil, &sdk.ScriptsSearchResultV1{SearchResults: nil, RecordCount: intPtr(0)}, nil
	}
	names := namesFor(fmt.Sprintf("w%d-p%d", f.walk, p), n)
	return nil, scriptPage(names, total), nil
}

func TestScriptSearch_List_TwiceWalkMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &twiceWalkMismatchScriptFake{}
	_, err := listScripts(t, &scriptSearch{svc: fake})
	if err == nil {
		t.Fatal("expected an error: the second independent walk disagreed with the first")
	}
	if fake.walk < 2 {
		t.Fatalf("expected a second walk to have been attempted (multi-page result), got %d walk(s)", fake.walk)
	}
}

// infiniteScriptFake always returns a FULL (non-short) page, forever,
// simulating a pathological/misbehaving API response that never signals
// end-of-results.
type infiniteScriptFake struct {
	pagesRequested []int
}

func (f *infiniteScriptFake) GetScriptsByOrganizationGroupAsync(_ context.Context, _ string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	p := 0
	if opts.Page != nil {
		p = *opts.Page
	}
	f.pagesRequested = append(f.pagesRequested, p)
	names := namesFor(fmt.Sprintf("page%d", p), scriptSearchPageSize)
	items := make([]sdk.ScriptResourceLiteV1, len(names))
	for i, n := range names {
		items[i] = sdk.ScriptResourceLiteV1{Name: n, ScriptUUID: fmt.Sprintf("%d-%s", p, n), OrganizationGroupUUID: "og-1"}
	}
	return nil, &sdk.ScriptsSearchResultV1{SearchResults: items}, nil
}

func TestScriptSearch_List_MaxPageCapFailsLoudly(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &infiniteScriptFake{}
	_, err := listScripts(t, &scriptSearch{svc: fake, maxPages: testMaxPages})
	if err == nil {
		t.Fatal("expected an error once the injected maxPages cap is exceeded, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+2 {
		t.Fatalf("expected the loop to stop at or just past the cap (%d), got %d requests: %v", testMaxPages, len(fake.pagesRequested), fake.pagesRequested)
	}
}

func TestScriptSearch_List_EmptyOrgGroupUuid_ReturnsError(t *testing.T) {
	t.Parallel()

	fake := &pagedScriptFake{}
	s := &scriptSearch{svc: fake}
	_, err := s.List(context.Background(), scriptFilters{OrganizationGroupUuid: types.StringValue("")})
	if err == nil {
		t.Fatal("expected an error for an empty organization_group_uuid")
	}
	if len(fake.pagesRequested) != 0 {
		t.Fatal("expected no SDK calls when organization_group_uuid is empty")
	}
}

// TestScriptSearch_List_NameFilterPropagatedToEveryPage proves the name
// filter (an existing, pre-B12 filter) is carried onto every page's request
// options unchanged, not just the first.
func TestScriptSearch_List_NameFilterPropagatedToEveryPage(t *testing.T) {
	t.Parallel()

	fake := &pagedScriptFake{pages: map[int]*sdk.ScriptsSearchResultV1{
		0: scriptPage(namesFor("p0", scriptSearchPageSize), scriptSearchPageSize+1),
		1: scriptPage(namesFor("p1", 1), scriptSearchPageSize+1),
	}}
	s := &scriptSearch{svc: fake}
	_, err := s.List(context.Background(), scriptFilters{
		OrganizationGroupUuid: types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
		Name:                  types.StringValue("needle"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Multi-page (2 pages) results are walked TWICE (an independent
	// confirmation, see internal/pagewalk.WalkTwiceVerified), so this is 4
	// calls: page 0 + confirming page 1, twice over.
	if len(fake.lastNames) != 4 {
		t.Fatalf("expected 4 calls (page 0 + confirming page 1, walked twice), got %d", len(fake.lastNames))
	}
	for i, n := range fake.lastNames {
		if n == nil || *n != "needle" {
			t.Fatalf("expected the name filter to reach every page's opts, call %d got %v", i, n)
		}
	}
}
