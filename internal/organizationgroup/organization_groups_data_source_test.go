package organizationgroup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- fakes implementing the narrow injected interfaces ---

// fakeSearch answers LocationGroupSearch from a fixed set of pages, keyed by
// the requested page number. It also supports simulating a server that
// ignores the page parameter (always answers page 0), a hard error, and --
// when sequence is set -- answering strictly in CALL order rather than by
// requested page number, so a test can make the two independent walks a
// double-walk performs (see listAllSearch) see different responses. This is
// needed only by the double-walk disagreement tests; every other test uses
// the page-keyed `pages` map, which naturally replays identically across
// both walks of a multi-page double-walk.
type fakeSearch struct {
	pages      map[int]*sdk.LocationGroupSearchResultV1
	sequence   []*sdk.LocationGroupSearchResultV1
	ignorePage bool
	err        error

	pagesRequested []int
}

func (f *fakeSearch) LocationGroupSearch(_ context.Context, opts *sdk.OrganizationGroupsLocationGroupSearchOptions) (http.Header, *sdk.LocationGroupSearchResultV1, error) {
	page := 0
	if opts != nil && opts.Page != nil {
		page = *opts.Page
	}
	call := len(f.pagesRequested)
	f.pagesRequested = append(f.pagesRequested, page)
	if f.err != nil {
		return nil, nil, f.err
	}
	if f.sequence != nil {
		if call >= len(f.sequence) {
			return nil, &sdk.LocationGroupSearchResultV1{}, nil
		}
		return nil, f.sequence[call], nil
	}
	if f.ignorePage {
		page = 0
	}
	res, ok := f.pages[page]
	if !ok {
		return nil, &sdk.LocationGroupSearchResultV1{}, nil
	}
	return nil, res, nil
}

// fakeChildren answers GetChildLocationGroups with a fixed list or error.
type fakeChildren struct {
	children []sdk.LocationGroupV1
	err      error
	idSeen   int
}

func (f *fakeChildren) GetChildLocationGroups(_ context.Context, id int) (http.Header, *[]sdk.LocationGroupV1, error) {
	f.idSeen = id
	if f.err != nil {
		return nil, nil, f.err
	}
	out := f.children
	return nil, &out, nil
}

// fakeParents answers GetParents with a fixed collection or error.
type fakeParents struct {
	items   []string
	err     error
	uuidReq string
}

func (f *fakeParents) GetParents(_ context.Context, uuid string) (http.Header, *sdk.OrganizationGroupCollectionV1Model, error) {
	f.uuidReq = uuid
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, &sdk.OrganizationGroupCollectionV1Model{Items: f.items}, nil
}

func intPtr(v int) *int { return &v }

// mustUnmarshalLG builds an sdk.LocationGroupV1 from a JSON literal. This is
// necessary (rather than a plain struct literal) because LocationGroupV1's
// ID field (and EntityReferenceV1's, used by ParentLocationGroup) is a
// pointer to an SDK-internal type this package cannot name directly --
// exactly the same reason the SDK's own generated tests build fixtures this
// way (see terraform-sdk-uem's tests/generated_systemv1_organizationgroups_test.go,
// which round-trips JSON through a mock HTTP server rather than constructing
// LocationGroupV1 literals), and json.Unmarshal works around it because it
// only needs reflected field access, not type identity across packages.
func mustUnmarshalLG(raw string) sdk.LocationGroupV1 {
	var v sdk.LocationGroupV1
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		panic(fmt.Sprintf("mustUnmarshalLG(%s): %v", raw, err))
	}
	return v
}

// lg builds a minimal organization group fixture with just an id and a name
// -- everything listAllSearch's paging/dedup logic needs.
func lg(id int64, name string) sdk.LocationGroupV1 {
	return mustUnmarshalLG(fmt.Sprintf(`{"Id":{"Value":%d},"Name":%q}`, id, name))
}

// lgWithParent builds an organization group fixture carrying a uuid, an
// LgLevel, and a ParentLocationGroup -- the shape a GetChildLocationGroups
// self entry has (live-verified), used by the searchAndHydrate tests.
func lgWithParent(id int64, name, uuid string, parentID int64, parentUUID string, lgLevel int) sdk.LocationGroupV1 {
	return mustUnmarshalLG(fmt.Sprintf(`{
		"Id": {"Value": %d},
		"Uuid": %q,
		"Name": %q,
		"LgLevel": %d,
		"ParentLocationGroup": {"Id": {"Value": %d}, "Uuid": %q}
	}`, id, uuid, name, lgLevel, parentID, parentUUID))
}

// lgRoot builds an organization group fixture like lgWithParent but with no
// ParentLocationGroup at all, matching a root group's self entry.
func lgRoot(id int64, name, uuid string, lgLevel int) sdk.LocationGroupV1 {
	return mustUnmarshalLG(fmt.Sprintf(`{"Id":{"Value":%d},"Uuid":%q,"Name":%q,"LgLevel":%d}`, id, uuid, name, lgLevel))
}

func searchResult(items []sdk.LocationGroupV1, page, pageSize int, total *int) *sdk.LocationGroupSearchResultV1 {
	return &sdk.LocationGroupSearchResultV1{
		LocationGroups: items,
		Page:           intPtr(page),
		PageSize:       intPtr(pageSize),
		Total:          total,
	}
}

// --- listAllSearch (pagination) ---
//
// These pin the walkSearch/double-walk contract documented on listAllSearch
// (mirroring internal/scripts/scriptlookup.AbsentFromOrgGroup). The old
// "Total is only a backstop" tests are gone: that behaviour (stopping once
// len(out) >= Total after a full page) is now forbidden -- Total must match
// the walk's own unique-id count exactly, never merely bound it.

func TestListAllSearch_SingleShortPageStopsImmediately(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 3, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0}) {
		t.Fatalf("expected exactly one request for page 0 (a single-page walk is never repeated), got %v", fs.pagesRequested)
	}
}

// TestListAllSearch_MultiPageHappyPathTriggersSecondWalk covers the ordinary
// multi-page case (full pages then a natural short final page) and confirms
// the double-walk: since the walk fetched more than one page, listAllSearch
// repeats it once more from page 0, doubling the call count.
func TestListAllSearch_MultiPageHappyPathTriggersSecondWalk(t *testing.T) {
	t.Parallel()
	total := 5
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total),
		1: searchResult([]sdk.LocationGroupV1{lg(3, "C"), lg(4, "D")}, 1, 2, &total),
		2: searchResult([]sdk.LocationGroupV1{lg(5, "E")}, 2, 2, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearch: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0, 1, 2, 0, 1, 2}) {
		t.Fatalf("expected the 3-page walk to run twice (double-walk), got %v", fs.pagesRequested)
	}
}

// TestListAllSearch_TotalExactMultipleOfPageSizeFetchesConfirmingEmptyPage
// covers the "Total is an exact multiple of pageSize" case: the walk reaches
// total on a FULL page, so it must fetch one more (empty) page to confirm
// the end before it can return -- and, having fetched 3 pages, must then
// repeat the whole walk once more.
func TestListAllSearch_TotalExactMultipleOfPageSizeFetchesConfirmingEmptyPage(t *testing.T) {
	t.Parallel()
	total := 4
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total),
		1: searchResult([]sdk.LocationGroupV1{lg(3, "C"), lg(4, "D")}, 1, 2, &total),
		2: searchResult(nil, 2, 2, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearch: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0, 1, 2, 0, 1, 2}) {
		t.Fatalf("expected the confirming empty page 2 to be fetched, and the whole 3-page walk repeated, got %v", fs.pagesRequested)
	}
}

func TestListAllSearch_EmptyPageZeroReturnsEmptyResult(t *testing.T) {
	t.Parallel()
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearch: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0}) {
		t.Fatalf("expected a single page-0 request (a single-page walk is never repeated), got %v", fs.pagesRequested)
	}
}

func TestListAllSearch_DedupsRepeatedIDAcrossPages(t *testing.T) {
	t.Parallel()
	total := 4
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total),
		1: searchResult([]sdk.LocationGroupV1{lg(2, "B"), lg(3, "C")}, 1, 2, &total), // 2 (B) repeated, 3 (C) new
		2: searchResult([]sdk.LocationGroupV1{lg(4, "D")}, 2, 2, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearch: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 deduplicated groups (A,B,C,D), got %d: %+v", len(got), got)
	}
	seen := map[int64]int{}
	for _, g := range got {
		seen[*g.ID.Value]++
	}
	if seen[2] != 1 {
		t.Fatalf("expected id 2 to appear exactly once, appeared %d times", seen[2])
	}
}

func TestListAllSearch_TotalTooSmallErrors(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B"), lg(3, "C")}, 0, 3, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatalf("expected an error when more distinct groups exist than Total claims, got %d groups", len(got))
	}
	if !strings.Contains(err.Error(), "Total") {
		t.Fatalf("expected a Total-related error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected a nil (not partial) result, got %d groups", len(got))
	}
}

func TestListAllSearch_TotalTooLargeWithEmptyPageErrors(t *testing.T) {
	t.Parallel()
	total := 10
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B"), lg(3, "C")}, 0, 3, &total),
		1: searchResult(nil, 1, 3, &total),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 3}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "empty page") {
		t.Fatalf("expected an empty-page-before-Total-reached error, got %v", err)
	}
}

// An empty page's Total must never replace the total learned from an earlier
// non-empty page: here the empty page's smaller Total would equal the ids
// already seen and make a truncated walk look complete.
func TestListAllSearch_EmptyPageTotalNeverOverridesNonEmptyTotal(t *testing.T) {
	t.Parallel()
	total5, total3 := 5, 3
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B"), lg(3, "C")}, 0, 3, &total5),
		1: searchResult(nil, 1, 3, &total3),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "empty page") {
		t.Fatalf("expected an empty-page-before-Total-reached error, got %v (%d groups)", err, len(got))
	}
	if got != nil {
		t.Fatalf("expected a nil (not partial) result, got %d groups", len(got))
	}
}

func TestListAllSearch_TotalChangesBetweenNonEmptyPagesErrors(t *testing.T) {
	t.Parallel()
	total4, total5 := 4, 5
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total4),
		1: searchResult([]sdk.LocationGroupV1{lg(3, "C"), lg(4, "D")}, 1, 2, &total5),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected a Total-changed-during-the-walk error, got %v", err)
	}
}

func TestListAllSearch_FullPageWithNoNewIDsErrors(t *testing.T) {
	t.Parallel()
	total := 4 // never reachable via duplicates alone, so the walk cannot end any other way
	fs := &fakeSearch{
		pages: map[int]*sdk.LocationGroupSearchResultV1{
			0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total),
		},
		ignorePage: true, // every request answers page 0's content again
	}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("expected a did-not-advance error, got %v", err)
	}
}

// TestListAllSearch_FullPageAfterTotalReachedErrors covers the server
// returning a full page of already-seen groups AFTER total was already
// reached on an earlier full page -- distinct from
// TestListAllSearch_FullPageWithNoNewIDsErrors, where total is never reached
// at all.
func TestListAllSearch_FullPageAfterTotalReachedErrors(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, &total), // full page reaches total exactly
		1: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 1, 2, &total), // full page again, all already seen
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "already seen") {
		t.Fatalf("expected a full-page-after-total-reached error, got %v", err)
	}
}

func TestListAllSearch_MaxPagesCapErrorsNeverTruncates(t *testing.T) {
	t.Parallel()
	total := maxSearchPages + 1 // never reachable within maxSearchPages, forcing the cap
	pages := map[int]*sdk.LocationGroupSearchResultV1{}
	for i := 0; i < maxSearchPages; i++ {
		pages[i] = searchResult([]sdk.LocationGroupV1{lg(int64(i)+1, fmt.Sprintf("G%d", i))}, i, 1, &total)
	}
	fs := &fakeSearch{pages: pages}
	d := &organizationGroupsDataSource{search: fs, pageSize: 1}

	got, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatalf("expected an error once the walk hit maxSearchPages, got %d groups with no error", len(got))
	}
	if !strings.Contains(err.Error(), "maxSearchPages") {
		t.Fatalf("expected the maxSearchPages cap error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected a nil (not truncated/partial) result once the cap errors, got %d groups", len(got))
	}
	if len(fs.pagesRequested) != maxSearchPages {
		t.Fatalf("expected exactly %d page requests, got %d", maxSearchPages, len(fs.pagesRequested))
	}
}

func TestListAllSearch_NilTotalOnNonEmptyPageErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, nil),
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "missing Total") {
		t.Fatalf("expected a missing-Total error, got %v", err)
	}
}

func TestListAllSearch_SearchErrorPropagates(t *testing.T) {
	t.Parallel()
	fs := &fakeSearch{err: fmt.Errorf("boom")}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	if _, err := d.listAllSearch(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("expected the search error to propagate")
	}
}

// TestListAllSearch_SecondWalkDifferentIDSetErrors uses fakeSearch.sequence
// so the two independent walks a multi-page result triggers see DIFFERENT
// data: both walks report the same Total (3) and find 3 distinct groups, but
// the actual id sets differ ({1,2,3} vs {1,2,4}). This proves the double-walk
// check compares the id sets themselves, not just their count/Total.
func TestListAllSearch_SecondWalkDifferentIDSetErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSearch{sequence: []*sdk.LocationGroupSearchResultV1{
		searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, intPtr(3)), // walk 1, page 0
		searchResult([]sdk.LocationGroupV1{lg(3, "C")}, 1, 2, intPtr(3)),             // walk 1, page 1 (short, done)
		searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 2, intPtr(3)), // walk 2, page 0
		searchResult([]sdk.LocationGroupV1{lg(4, "D")}, 1, 2, intPtr(3)),             // walk 2, page 1 (short, done, but id 4 not 3)
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "different set of ids") {
		t.Fatalf("expected a different-id-set error, got %v", err)
	}
	if len(fs.pagesRequested) != 4 {
		t.Fatalf("expected exactly 4 calls (2 per walk), got %d", len(fs.pagesRequested))
	}
}

// TestListAllSearch_SecondWalkTotalDiffersErrors exercises "the second walk
// disagrees on Total". Note: once a walk SUCCEEDS, its returned total always
// equals the number of unique ids it collected (that is the success
// condition), so two successful walks that found the very same id set can
// never report different totals -- a same-ids-different-total outcome is not
// reachable through the public listAllSearch contract. What IS reachable,
// and is exactly what a real "the list changed under us" server would
// produce, is the second walk itself hitting a Total-changed-mid-walk
// inconsistency (the same one TestListAllSearch_TotalChangesBetweenNonEmptyPagesErrors
// pins for a single walk); listAllSearch must surface that as an overall
// error rather than silently keeping the first walk's result.
func TestListAllSearch_SecondWalkTotalDiffersErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSearch{sequence: []*sdk.LocationGroupSearchResultV1{
		searchResult([]sdk.LocationGroupV1{lg(1, "A")}, 0, 1, intPtr(2)), // walk 1, page 0 (full, reaches total on page 1)
		searchResult([]sdk.LocationGroupV1{lg(2, "B")}, 1, 1, intPtr(2)), // walk 1, page 1 (full, total reached, needs confirming page)
		searchResult(nil, 2, 1, intPtr(2)),                               // walk 1, page 2 (empty, confirms, done)
		searchResult([]sdk.LocationGroupV1{lg(1, "A")}, 0, 1, intPtr(2)), // walk 2, page 0
		searchResult([]sdk.LocationGroupV1{lg(2, "B")}, 1, 1, intPtr(5)), // walk 2, page 1: Total flips to 5 mid-walk
	}}
	d := &organizationGroupsDataSource{search: fs, pageSize: 1}

	_, err := d.listAllSearch(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "second walk") {
		t.Fatalf("expected the second walk's internal Total inconsistency to fail the overall call, got %v", err)
	}
	if len(fs.pagesRequested) != 5 {
		t.Fatalf("expected exactly 5 calls (3 for walk 1, 2 for walk 2 before it errors), got %d", len(fs.pagesRequested))
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- searchAndHydrate (hydration fan-out via children lookup) ---

// TestSearchAndHydrate_FillsFromSelfEntryNotFirstInList pins the id-matching
// contract: the self entry is picked out of the children-lookup result by
// numeric id, not by position, so it must be found even when it is NOT the
// first entry (mirroring the live children(138883) response, where 138883
// itself appears after some of its descendants).
func TestSearchAndHydrate_FillsFromSelfEntryNotFirstInList(t *testing.T) {
	t.Parallel()
	total := 1
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(138883, "Self")}, 0, 500, &total),
	}}
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgWithParent(149104, "Descendant", "uuid-149104", 138883, "uuid-self", 1),
		lgWithParent(138883, "Self", "uuid-self", 571, "uuid-571", 0), // self entry, deliberately not first
	}}
	d := &organizationGroupsDataSource{search: fs, children: fc, pageSize: 500}

	got, err := d.searchAndHydrate(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("searchAndHydrate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	g := got[0]
	if g.UUID.ValueString() != "uuid-self" || g.ParentID.ValueInt64() != 571 || g.ParentUUID.ValueString() != "uuid-571" {
		t.Fatalf("hydration did not fill uuid/parent from the self entry: %+v", g)
	}
	if fc.idSeen != 138883 {
		t.Fatalf("expected GetChildLocationGroups(138883), got %d", fc.idSeen)
	}
}

// TestSearchAndHydrate_LgLevelAlwaysNullInSearchMode pins that lg_level is
// null in search mode even though the self entry itself carries a (always 0,
// meaningless) LgLevel.
func TestSearchAndHydrate_LgLevelAlwaysNullInSearchMode(t *testing.T) {
	t.Parallel()
	total := 1
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A")}, 0, 500, &total),
	}}
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgWithParent(1, "A", "uuid-1", 2, "uuid-2", 0),
	}}
	d := &organizationGroupsDataSource{search: fs, children: fc, pageSize: 500}

	got, err := d.searchAndHydrate(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("searchAndHydrate: %v", err)
	}
	if len(got) != 1 || !got[0].LgLevel.IsNull() {
		t.Fatalf("expected lg_level to be null in search mode, got %+v", got)
	}
}

func TestSearchAndHydrate_ChildrenErrorFailsWithNoPartialList(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A"), lg(2, "B")}, 0, 500, &total),
	}}
	fc := &fakeChildren{err: fmt.Errorf("boom")}
	d := &organizationGroupsDataSource{search: fs, children: fc, pageSize: 500}

	got, err := d.searchAndHydrate(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected a children-lookup error to fail the whole hydration")
	}
	if got != nil {
		t.Fatalf("expected a nil (not partial) result on a hydration error, got %+v", got)
	}
}

// TestSearchAndHydrate_ChildrenListWithoutSelfEntryErrors pins the fail-closed
// contract for a malformed/unexpected children-lookup response: if the
// matched group's own id is not among the entries, hydration must error
// rather than silently return an unhydrated or wrong group.
func TestSearchAndHydrate_ChildrenListWithoutSelfEntryErrors(t *testing.T) {
	t.Parallel()
	total := 1
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A")}, 0, 500, &total),
	}}
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgWithParent(2, "Other", "uuid-2", 1, "uuid-1", 1), // no entry for id 1 itself
	}}
	d := &organizationGroupsDataSource{search: fs, children: fc, pageSize: 500}

	got, err := d.searchAndHydrate(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error when the children-lookup result has no self entry")
	}
	if got != nil {
		t.Fatalf("expected a nil result, got %+v", got)
	}
}

// TestSearchAndHydrate_RootGroupHasNullParent pins that a root group (whose
// self entry in the children-lookup result carries no ParentLocationGroup at
// all) hydrates to null parent_id/parent_uuid, not an error.
func TestSearchAndHydrate_RootGroupHasNullParent(t *testing.T) {
	t.Parallel()
	total := 1
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(571, "Root")}, 0, 500, &total),
	}}
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgRoot(571, "Root", "uuid-571", 0),
	}}
	d := &organizationGroupsDataSource{search: fs, children: fc, pageSize: 500}

	got, err := d.searchAndHydrate(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("searchAndHydrate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	g := got[0]
	if !g.ParentID.IsNull() || !g.ParentUUID.IsNull() {
		t.Fatalf("expected null parent_id/parent_uuid for a root group, got %+v", g)
	}
}

// --- Read (framework-level): mode dispatch, mutual exclusion, empty lists ---

func getSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	var resp datasource.SchemaResponse
	(&organizationGroupsDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp
}

// buildConfig constructs a tfsdk.Config for the data source's schema with
// every attribute defaulted to null except the overrides given.
func buildConfig(t *testing.T, s datasource.SchemaResponse, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	objType, ok := s.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range overrides {
		values[name] = v
	}
	return tfsdk.Config{Schema: s.Schema, Raw: tftypes.NewValue(objType, values)}
}

func emptyState(s datasource.SchemaResponse) tfsdk.State {
	ctx := context.Background()
	objType := s.Schema.Type().TerraformType(ctx).(tftypes.Object) //nolint:forcetypeassert // schema type is always an Object; mirrors buildConfig
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.State{Schema: s.Schema, Raw: tftypes.NewValue(objType, values)}
}

func TestRead_ChildrenModeIncludesSelf(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lg(10, "Self"), // LgLevel 0 per SDK contract, included here as self
		lg(11, "Child"),
	}}
	d := &organizationGroupsDataSource{children: fc}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"parent_id": tftypes.NewValue(tftypes.Number, 10),
	})}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var model OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.OrganizationGroups) != 2 {
		t.Fatalf("expected 2 groups (self + child), got %d", len(model.OrganizationGroups))
	}
	if model.OrganizationGroups[0].Name.ValueString() != "Self" {
		t.Fatalf("expected the first entry to be the self/parent group, got %+v", model.OrganizationGroups[0])
	}
	if fc.idSeen != 10 {
		t.Fatalf("expected GetChildLocationGroups(10), got %d", fc.idSeen)
	}
}

func TestRead_AncestorsModeReturnsUUIDsOnly(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fp := &fakeParents{items: []string{"uuid-self", "uuid-parent", "uuid-global"}}
	d := &organizationGroupsDataSource{parents: fp}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"ancestors_of_uuid": tftypes.NewValue(tftypes.String, "uuid-self"),
	})}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var model OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.OrganizationGroups) != 0 {
		t.Fatalf("expected organization_groups to be empty in ancestors mode, got %d", len(model.OrganizationGroups))
	}
	if len(model.AncestorUUIDs) != 3 {
		t.Fatalf("expected 3 ancestor uuids, got %d", len(model.AncestorUUIDs))
	}
	if model.AncestorUUIDs[0].ValueString() != "uuid-self" || model.AncestorUUIDs[2].ValueString() != "uuid-global" {
		t.Fatalf("unexpected ancestor uuid ordering: %+v", model.AncestorUUIDs)
	}
	if fp.uuidReq != "uuid-self" {
		t.Fatalf("expected GetParents(uuid-self), got %q", fp.uuidReq)
	}
}

func TestRead_EmptyResultsAreEmptyListsNotNull(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{}}
	d := &organizationGroupsDataSource{search: fs, children: &fakeChildren{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, nil)}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var model OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if model.OrganizationGroups == nil || len(model.OrganizationGroups) != 0 {
		t.Fatalf("expected an empty, non-null organization_groups list, got %#v", model.OrganizationGroups)
	}
	if model.AncestorUUIDs == nil || len(model.AncestorUUIDs) != 0 {
		t.Fatalf("expected an empty, non-null ancestor_uuids list, got %#v", model.AncestorUUIDs)
	}
}

func TestRead_HydrationErrorFailsCleanly(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	total := 1
	fs := &fakeSearch{pages: map[int]*sdk.LocationGroupSearchResultV1{
		0: searchResult([]sdk.LocationGroupV1{lg(1, "A")}, 0, 500, &total),
	}}
	fc := &fakeChildren{err: fmt.Errorf("boom")}
	d := &organizationGroupsDataSource{search: fs, children: fc}

	req := datasource.ReadRequest{Config: buildConfig(t, s, nil)}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a hydration error to fail the whole Read")
	}
}

func TestMetadataTypeName(t *testing.T) {
	t.Parallel()
	var resp datasource.MetadataResponse
	NewDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)
	if resp.TypeName != "uem_organization_groups" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

// --- ValidateConfig: mutual exclusion ---

func TestValidateConfig_TwoModesSetErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"parent_id":         tftypes.NewValue(tftypes.Number, 10),
		"ancestors_of_uuid": tftypes.NewValue(tftypes.String, "uuid-1"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when two modes are set")
	}
}

func TestValidateConfig_SearchFilterWithParentIDErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "foo"),
		"parent_id": tftypes.NewValue(tftypes.Number, 10),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when a search filter and parent_id are both set")
	}
}

func TestValidateConfig_SingleModeIsValid(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	for _, tc := range []struct {
		name      string
		overrides map[string]tftypes.Value
	}{
		{"no mode (default search, unfiltered)", nil},
		{"name filter only", map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "foo")}},
		{"id only", map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.String, "149104")}},
		{"parent_id only", map[string]tftypes.Value{"parent_id": tftypes.NewValue(tftypes.Number, 10)}},
		{"ancestors_of_uuid only", map[string]tftypes.Value{"ancestors_of_uuid": tftypes.NewValue(tftypes.String, "uuid-1")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, tc.overrides)}
			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error, got %v", resp.Diagnostics)
			}
		})
	}
}

// TestValidateConfig_UnknownValueDoesNotConflict pins the "skip validation on
// unknown values" requirement: an unknown parent_id (e.g. derived from a
// not-yet-created resource) must not be treated as "set" for the
// mutual-exclusion check, even alongside a known search filter.
func TestValidateConfig_UnknownValueDoesNotConflict(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"name":      tftypes.NewValue(tftypes.String, "foo"),
		"parent_id": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error when the conflicting attribute is unknown, got %v", resp.Diagnostics)
	}
}

// --- ValidateConfig: the new `id` exact-lookup mode ---

// TestValidateConfig_NonNumericIDErrors is the fail-on-revert pin for the
// numeric-shape validation on the String-typed `id` attribute (String, not
// Int64, per the ws1-tf onboard generator's `id = "<numeric>"` contract): a
// non-numeric value must be rejected at plan time, not merely fail later
// inside Read's ParseInt.
func TestValidateConfig_NonNumericIDErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "not-a-number"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a non-numeric id")
	}
}

func TestValidateConfig_IDModeIsValidAlone(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "149104"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for id alone, got %v", resp.Diagnostics)
	}
}

func TestValidateConfig_IDWithSearchFilterErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "149104"),
		"name": tftypes.NewValue(tftypes.String, "foo"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when id and a search filter are both set")
	}
}

func TestValidateConfig_IDWithParentIDErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "149104"),
		"parent_id": tftypes.NewValue(tftypes.Number, 138883),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when id and parent_id are both set")
	}
}

func TestValidateConfig_IDWithAncestorsOfUUIDErrors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	d := &organizationGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, "149104"),
		"ancestors_of_uuid": tftypes.NewValue(tftypes.String, "uuid-1"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when id and ancestors_of_uuid are both set")
	}
}

// --- Read: the new `id` exact-lookup mode ---

// TestRead_IDModeReturnsExactlyOneEntry_SelfNotFirstInList proves the id-mode
// fetch matches the self entry by NUMERIC ID, not by list position: the fake
// children-lookup result deliberately puts the matched group's descendant
// first and the self entry second (mirroring the live children(138883)
// response, where 138883 itself appears after some of its descendants). If
// the implementation ever regressed to "take children[0]" instead of
// findSelf's id match, this test would fail because it would return the
// descendant's uuid/parent instead of the requested group's.
func TestRead_IDModeReturnsExactlyOneEntry_SelfNotFirstInList(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgWithParent(149104, "Descendant", "uuid-149104", 138883, "uuid-self", 1),
		lgWithParent(138883, "readonly_fixtures", "uuid-self", 571, "uuid-571", 0), // self entry, deliberately not first
	}}
	d := &organizationGroupsDataSource{children: fc}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "138883"),
	})}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var model OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.OrganizationGroups) != 1 {
		t.Fatalf("expected exactly 1 organization group, got %d: %+v", len(model.OrganizationGroups), model.OrganizationGroups)
	}
	g := model.OrganizationGroups[0]
	if g.ID.ValueInt64() != 138883 || g.Name.ValueString() != "readonly_fixtures" {
		t.Fatalf("expected the SELF entry (id 138883), got %+v", g)
	}
	if g.UUID.ValueString() != "uuid-self" || g.ParentID.ValueInt64() != 571 || g.ParentUUID.ValueString() != "uuid-571" {
		t.Fatalf("id mode did not hydrate uuid/parent from the self entry: %+v", g)
	}
	if !g.LgLevel.IsNull() {
		t.Fatalf("expected lg_level to be null in id mode, got %+v", g.LgLevel)
	}
	if fc.idSeen != 138883 {
		t.Fatalf("expected GetChildLocationGroups(138883), got %d", fc.idSeen)
	}
}

// TestRead_IDMode_SelfEntryMissing_Errors pins the fail-closed contract: if
// the requested id is not among the children-lookup result's entries, the
// read must error, never silently return an empty or wrong list.
func TestRead_IDMode_SelfEntryMissing_Errors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fc := &fakeChildren{children: []sdk.LocationGroupV1{
		lgWithParent(2, "Other", "uuid-2", 1, "uuid-1", 1), // no entry for the requested id
	}}
	d := &organizationGroupsDataSource{children: fc}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "999"),
	})}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the self entry is missing from the children-lookup result")
	}
}

// TestRead_IDMode_APIError_Errors pins that any SDK error in id mode fails
// the whole read.
func TestRead_IDMode_APIError_Errors(t *testing.T) {
	t.Parallel()
	s := getSchema(t)
	fc := &fakeChildren{err: fmt.Errorf("boom")}
	d := &organizationGroupsDataSource{children: fc}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "138883"),
	})}
	resp := &datasource.ReadResponse{State: emptyState(s)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the children-lookup API call fails")
	}
}
