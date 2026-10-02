package datasource

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- Pagination (internal-ticket, release blocker B12): AppsV2.Search paginates via
// Page/PageSize + Applications/Total, no cursor, shared by uem_applications and
// uem_mac_applications via walkApplications (applications_pagewalk.go). The heavy-lifting
// walk/probe logic itself is exhaustively covered generically in internal/pagewalk; these
// tests pin the wiring in both data sources onto it: dedupe key (lower-cased application
// UUID -- live-confirmed as the only reliably-populated identifier on both as<internal-env> and
// paul-2609, since AppsV2.Search returns id=null on every item there), filter propagation,
// and the fail-closed guards surfacing as errors. ---

func appPage(names []string, total int) *sdk.ApplicationSearchV2Model {
	apps := make([]sdk.ApplicationV2Model, len(names))
	for i, n := range names {
		apps[i] = sdk.ApplicationV2Model{ApplicationName: n, UUID: n, ApplicationType: "Internal"}
	}
	return &sdk.ApplicationSearchV2Model{Applications: apps, Total: intPtr(total)}
}

func appNamesFor(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return out
}

// pagedAppsFake is a test-only fake satisfying appsV2SearchAPI whose response
// depends on the requested page (keyed by the ACTUAL page index the fixture's
// base uses -- 0 or 1). A page with no entry returns a nil result -- the
// live-verified HTTP-204 out-of-range shape on both as<internal-env> and paul-2609 --
// never an error.
type pagedAppsFake struct {
	pages          map[int]*sdk.ApplicationSearchV2Model
	err            error
	pagesRequested []int
	lastOpts       *sdk.AppsV2SearchOptions
}

func (f *pagedAppsFake) Search(_ context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error) {
	f.lastOpts = opts
	if f.err != nil {
		return nil, nil, f.err
	}
	if opts.Page == nil {
		return nil, nil, fmt.Errorf("test fake: Search called without a Page option set -- the fix must always set one explicitly")
	}
	p := *opts.Page
	f.pagesRequested = append(f.pagesRequested, p)
	res, ok := f.pages[p]
	if !ok {
		return nil, nil, nil
	}
	return nil, res, nil
}

func readApplications(t *testing.T, ds *ApplicationsDataSource) (*datasource.ReadResponse, ApplicationsDataSourceModel) {
	t.Helper()
	ctx := context.Background()
	config := createApplicationsDSConfig(t, map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, nil),
		"platform":     tftypes.NewValue(tftypes.String, nil),
		"applications": tftypes.NewValue(tftypes.List{ElementType: applicationsNestedType(t)}, nil),
	})
	readReq := datasource.ReadRequest{Config: config}
	readResp := &datasource.ReadResponse{State: emptyApplicationsDSState(t)}
	ds.Read(ctx, readReq, readResp)
	var out ApplicationsDataSourceModel
	if !readResp.Diagnostics.HasError() {
		if diags := readResp.State.Get(ctx, &out); diags.HasError() {
			t.Fatalf("unexpected error reading state: %v", diags.Errors())
		}
	}
	return readResp, out
}

func TestApplicationsDataSource_Read_ZeroBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = applicationSearchPageSize + 200
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: appPage(appNamesFor("p0", applicationSearchPageSize), total),
		1: appPage(appNamesFor("p1", 200), total), // short: 200 < pageSize
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, out := readApplications(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Applications) != total {
		t.Fatalf("expected %d applications, got %d", total, len(out.Applications))
	}
	if fake.pagesRequested[0] != 0 {
		t.Fatalf("expected the probe to request page 0 first, got: %v", fake.pagesRequested)
	}
}

func TestApplicationsDataSource_Read_OneBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = applicationSearchPageSize + 50
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		// no entry for page 0: forces the fallback probe to page 1.
		1: appPage(appNamesFor("p1", applicationSearchPageSize), total),
		2: appPage(appNamesFor("p2", 50), total), // short
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, out := readApplications(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Applications) != total {
		t.Fatalf("expected %d applications, got %d", total, len(out.Applications))
	}
	if len(fake.pagesRequested) < 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected the probe to try page 0 then page 1, got: %v", fake.pagesRequested)
	}
}

// TestApplicationsDataSource_Read_ZeroBasedPageOneReturns204 proves the live
// out-of-range shape (HTTP 204, a nil result) observed on both as<internal-env> and
// paul-2609: a page-size-aligned result's confirming page must be treated as
// end-of-results, not an error.
func TestApplicationsDataSource_Read_ZeroBasedPageOneReturns204(t *testing.T) {
	t.Parallel()

	const total = applicationSearchPageSize
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: appPage(appNamesFor("p0", applicationSearchPageSize), total),
		// no entry for page 1: Search returns (nil, nil, nil) -- the HTTP 204 shape.
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, out := readApplications(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Applications) != total {
		t.Fatalf("expected %d applications, got %d", total, len(out.Applications))
	}
	want := []int{0, 1, 0, 1}
	if len(fake.pagesRequested) != len(want) {
		t.Fatalf("expected page 0 then a confirming page 1, walked twice (%v), got: %v", want, fake.pagesRequested)
	}
}

func TestApplicationsDataSource_Read_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{}}
	ds := &ApplicationsDataSource{search: fake}
	resp, out := readApplications(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a confirmed-empty search, got: %v", resp.Diagnostics.Errors())
	}
	if len(out.Applications) != 0 {
		t.Fatalf("expected 0 applications, got %d", len(out.Applications))
	}
	if len(fake.pagesRequested) != 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected both page 0 and page 1 to be probed before confirming zero, got: %v", fake.pagesRequested)
	}
}

func TestApplicationsDataSource_Read_TotalMismatchIsError(t *testing.T) {
	t.Parallel()

	full := appPage(appNamesFor("p0", applicationSearchPageSize), applicationSearchPageSize)
	page1 := appPage(appNamesFor("p1", 1), 999) // total changed
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: full,
		1: page1,
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, _ := readApplications(t, ds)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error: total changed between page 0 (%d) and page 1 (999)", applicationSearchPageSize)
	}
}

func TestApplicationsDataSource_Read_NoProgressIsError(t *testing.T) {
	t.Parallel()

	names := appNamesFor("p0", applicationSearchPageSize)
	full := appPage(names, applicationSearchPageSize*2)
	repeated := appPage(names, applicationSearchPageSize*2) // same uuids as page 0
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: full,
		1: repeated,
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, _ := readApplications(t, ds)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: page 1 repeated page 0's applications and made no progress toward total")
	}
	if len(fake.pagesRequested) != 2 {
		t.Fatalf("expected the no-progress guard to stop the walk after exactly 2 requests, got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// twiceWalkMismatchAppsFake simulates an application list that changes
// between the first and second (confirming) independent walk of a multi-page
// result.
type twiceWalkMismatchAppsFake struct {
	walk           int
	pagesRequested []int
}

func (f *twiceWalkMismatchAppsFake) Search(_ context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error) {
	p := *opts.Page
	if p == 0 {
		f.walk++
	}
	f.pagesRequested = append(f.pagesRequested, p)

	var total, n int
	switch {
	case f.walk <= 1:
		total = applicationSearchPageSize + 200
		switch p {
		case 0:
			n = applicationSearchPageSize
		case 1:
			n = 200
		}
	default:
		total = applicationSearchPageSize + 150
		switch p {
		case 0:
			n = applicationSearchPageSize
		case 1:
			n = 150
		}
	}
	if p > 1 {
		return nil, nil, nil
	}
	names := appNamesFor(fmt.Sprintf("w%d-p%d", f.walk, p), n)
	return nil, appPage(names, total), nil
}

func TestApplicationsDataSource_Read_TwiceWalkMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &twiceWalkMismatchAppsFake{}
	ds := &ApplicationsDataSource{search: fake}
	resp, _ := readApplications(t, ds)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: the second independent walk disagreed with the first")
	}
	if fake.walk < 2 {
		t.Fatalf("expected a second walk to have been attempted (multi-page result), got %d walk(s)", fake.walk)
	}
}

// infiniteAppsFake serves fresh, full 0-indexed pages that all report the
// same total (lastPage+1 full pages) and a nil result past lastPage, so a
// walk is internally consistent and only the page cap can stop it before it
// completes.
type infiniteAppsFake struct {
	lastPage       int
	pagesRequested []int
}

func (f *infiniteAppsFake) Search(_ context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error) {
	p := *opts.Page
	f.pagesRequested = append(f.pagesRequested, p)
	if p > f.lastPage {
		return nil, nil, nil
	}
	total := (f.lastPage + 1) * applicationSearchPageSize
	return nil, appPage(appNamesFor(fmt.Sprintf("cap-%d", p), applicationSearchPageSize), total), nil
}

// TestApplicationsDataSource_Read_MaxPageCapStopsConsistentWalk proves the
// injected page cap stops an otherwise-successful, internally-consistent
// multi-page walk with an error rather than looping past it.
func TestApplicationsDataSource_Read_MaxPageCapStopsConsistentWalk(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &infiniteAppsFake{lastPage: 19}
	ds := &ApplicationsDataSource{search: fake, maxPages: testMaxPages}
	resp, _ := readApplications(t, ds)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the injected maxPages cap to stop a consistent 20-page walk with an error, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+1 {
		t.Fatalf("expected at most %d page requests before the cap fired, got %d: %v", testMaxPages+1, len(fake.pagesRequested), fake.pagesRequested)
	}
}

func TestApplicationsDataSource_Read_DedupsAcrossPages(t *testing.T) {
	t.Parallel()

	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: appPage([]string{"a1", "a2"}, 3),
		1: appPage([]string{"a2", "a3"}, 3), // a2 overlaps with page 0
	}}
	ds := &ApplicationsDataSource{search: fake}
	resp, out := readApplications(t, ds)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	seen := map[string]int{}
	for _, a := range out.Applications {
		seen[a.Name.ValueString()]++
	}
	if seen["a2"] != 1 {
		t.Fatalf("expected application a2 (overlapping across pages 0 and 1) to appear exactly once, got %d: %+v", seen["a2"], out.Applications)
	}
	if len(out.Applications) != 3 {
		t.Fatalf("expected 3 deduped applications, got %d: %+v", len(out.Applications), out.Applications)
	}
}

// --- macApplicationSearch.List pagination wiring (same walkApplications helper) ---

func TestMacApplicationSearch_List_ZeroBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = applicationSearchPageSize + 200
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: appPage(appNamesFor("p0", applicationSearchPageSize), total),
		1: appPage(appNamesFor("p1", 200), total),
	}}
	s := &macApplicationSearch{svc: fake}
	items, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != total {
		t.Fatalf("expected %d items, got %d", total, len(items))
	}
	if fake.pagesRequested[0] != 0 {
		t.Fatalf("expected the probe to request page 0 first, got: %v", fake.pagesRequested)
	}
}

func TestMacApplicationSearch_List_OneBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = applicationSearchPageSize + 50
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		1: appPage(appNamesFor("p1", applicationSearchPageSize), total),
		2: appPage(appNamesFor("p2", 50), total),
	}}
	s := &macApplicationSearch{svc: fake}
	items, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != total {
		t.Fatalf("expected %d items, got %d", total, len(items))
	}
	if len(fake.pagesRequested) < 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected the probe to try page 0 then page 1, got: %v", fake.pagesRequested)
	}
}

func TestMacApplicationSearch_List_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{}}
	s := &macApplicationSearch{svc: fake}
	items, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("expected no error for a confirmed-empty search, got: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestMacApplicationSearch_List_TotalMismatchIsError(t *testing.T) {
	t.Parallel()

	full := appPage(appNamesFor("p0", applicationSearchPageSize), applicationSearchPageSize)
	page1 := appPage(appNamesFor("p1", 1), 999)
	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{0: full, 1: page1}}
	s := &macApplicationSearch{svc: fake}
	_, err := s.List(context.Background(), macApplicationFilters{})
	if err == nil {
		t.Fatal("expected an error: total changed between pages")
	}
}

func TestMacApplicationSearch_List_MaxPageCapFailsLoudly(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &infiniteAppsFake{lastPage: 19}
	s := &macApplicationSearch{svc: fake, maxPages: testMaxPages}
	_, err := s.List(context.Background(), macApplicationFilters{})
	if err == nil {
		t.Fatal("expected an error once the injected maxPages cap is exceeded, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+1 {
		t.Fatalf("expected at most %d page requests before the cap fired, got %d: %v", testMaxPages+1, len(fake.pagesRequested), fake.pagesRequested)
	}
}

func TestMacApplicationSearch_List_TwiceWalkMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &twiceWalkMismatchAppsFake{}
	s := &macApplicationSearch{svc: fake}
	_, err := s.List(context.Background(), macApplicationFilters{})
	if err == nil {
		t.Fatal("expected an error: the second independent walk disagreed with the first")
	}
	if fake.walk < 2 {
		t.Fatalf("expected a second walk to have been attempted (multi-page result), got %d walk(s)", fake.walk)
	}
}

// TestMacApplicationSearch_List_OrgGroupFilterPropagatedToEveryPage proves the
// organization_group_uuid filter (an existing, pre-B12 filter) is carried onto
// every page's request options unchanged, not just the first.
func TestMacApplicationSearch_List_OrgGroupFilterPropagatedToEveryPage(t *testing.T) {
	t.Parallel()

	fake := &pagedAppsFake{pages: map[int]*sdk.ApplicationSearchV2Model{
		0: appPage(appNamesFor("p0", applicationSearchPageSize), applicationSearchPageSize+1),
		1: appPage(appNamesFor("p1", 1), applicationSearchPageSize+1),
	}}
	var lastOGs []*string
	wrap := &ogCapturingFake{inner: fake, captured: &lastOGs}
	s := &macApplicationSearch{svc: wrap}

	const wantUUID = "fc1bf13c-f814-d160-6b79-0bd6696dd98b"
	_, err := s.List(context.Background(), macApplicationFilters{OrganizationGroupUuid: types.StringValue(wantUUID)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lastOGs) != 4 { // 2 pages, walked twice
		t.Fatalf("expected 4 calls (page 0 + confirming page 1, walked twice), got %d", len(lastOGs))
	}
	for i, og := range lastOGs {
		if og == nil || *og != wantUUID {
			t.Fatalf("expected the org group filter to reach every page's opts, call %d got %v", i, og)
		}
	}
}

type ogCapturingFake struct {
	inner    *pagedAppsFake
	captured *[]*string
}

func (f *ogCapturingFake) Search(ctx context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error) {
	*f.captured = append(*f.captured, opts.OrganizationGroupUUID)
	return f.inner.Search(ctx, opts)
}
