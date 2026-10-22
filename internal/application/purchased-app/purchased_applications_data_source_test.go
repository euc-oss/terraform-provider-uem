package purchasedapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeTenant is an httptest-backed stand-in for the two UEM endpoints this
// data source calls. Its search endpoint reproduces the LIVE-VERIFIED paging
// contract of GET /api/mam/apps/purchased/search (see listAll's doc comment):
// 0-based pages, Total = the number of items on the returned page, and HTTP
// 204 No Content past the last page.
type fakeTenant struct {
	mu sync.Mutex

	apps []map[string]any // the full, ordered result set

	// echoPageSize, when > 0, is what the server reports (and uses) as the
	// page size regardless of the requested one -- simulates a server-side
	// cap below the requested size.
	echoPageSize int
	// ignorePage makes the server always serve page 0 (a misbehaving server).
	ignorePage bool
	// searchStatus, when non-zero, is returned for every search request.
	searchStatus int

	// assignmentsByUUID is the number of assignments each app's
	// assignment-rules GET returns; an app with no entry gets a 204.
	assignmentsByUUID map[string]int
	ruleStatusFor     map[string]int

	pagesRequested []int
	pageSizes      []int
	ogUUIDs        []string
	rulesRequested []string
}

func (f *fakeTenant) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/mam/apps/purchased/search":
			q := r.URL.Query()
			page, _ := strconv.Atoi(q.Get("page"))
			size, _ := strconv.Atoi(q.Get("pagesize"))
			f.pagesRequested = append(f.pagesRequested, page)
			f.pageSizes = append(f.pageSizes, size)
			f.ogUUIDs = append(f.ogUUIDs, q.Get("organizationgroupuuid"))
			if f.searchStatus != 0 {
				w.WriteHeader(f.searchStatus)
				return
			}
			if f.echoPageSize > 0 {
				size = f.echoPageSize
			}
			if f.ignorePage {
				page = 0
			}
			start := page * size
			if size <= 0 || start >= len(f.apps) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			end := start + size
			if end > len(f.apps) {
				end = len(f.apps)
			}
			items := f.apps[start:end]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Application": items,
				"Page":        page,
				"PageSize":    size,
				"Total":       len(items), // per-page count, as observed live
			})
		case strings.HasPrefix(r.URL.Path, "/api/mam/apps/") && strings.HasSuffix(r.URL.Path, "/assignment-rules"):
			uuid := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/mam/apps/"), "/assignment-rules")
			f.rulesRequested = append(f.rulesRequested, uuid)
			if st, ok := f.ruleStatusFor[uuid]; ok {
				w.WriteHeader(st)
				return
			}
			n, ok := f.assignmentsByUUID[uuid]
			if !ok {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			assignments := make([]map[string]any, n)
			for i := range assignments {
				assignments[i] = map[string]any{"priority": i}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"assignments": assignments})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// appUUID is the UUID makeApps gives app i. Tests reference app UUIDs only
// through it, never as literals: the public sync rewrites UUID literals in
// .go files, which would split them from the ones built here.
func appUUID(i int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
}

func makeApps(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = map[string]any{
			"Id":                    map[string]any{"Value": 35700 + i},
			"Uuid":                  appUUID(i),
			"ApplicationName":       fmt.Sprintf("App %d", i),
			"BundleId":              fmt.Sprintf("com.example.app%d", i),
			"Platform":              2,
			"LocationGroupId":       138883,
			"OrganizationGroupUuid": "og-owner",
		}
	}
	return out
}

// newTestDataSource wires a purchasedApplicationsDataSource to real SDK
// services pointed at an httptest server running f.
func newTestDataSource(t *testing.T, f *fakeTenant, pageSize int) *purchasedApplicationsDataSource {
	t.Helper()
	server := httptest.NewServer(f.handler(t))
	t.Cleanup(server.Close)
	c, err := client.NewClient(&client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return &purchasedApplicationsDataSource{
		search:   sdk.NewPurchasedAppsV1Service(c),
		rules:    sdk.NewAppsV2Service(c),
		pageSize: pageSize,
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

// --- pagination (listAll) ---

func TestListAll_SingleShortPageStopsImmediately(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(2)}
	d := newTestDataSource(t, f, 3)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(got))
	}
	if !equalInts(f.pagesRequested, []int{0}) {
		t.Fatalf("expected exactly one request for page 0 (pages are 0-based; a short page ends the walk), got %v", f.pagesRequested)
	}
	if f.ogUUIDs[0] != "og-uuid" || f.pageSizes[0] != 3 {
		t.Fatalf("expected organizationgroupuuid=og-uuid pagesize=3, got og=%q size=%d", f.ogUUIDs[0], f.pageSizes[0])
	}
}

func TestListAll_FullPagesThenShortFinalPage(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(5)}
	d := newTestDataSource(t, f, 2)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected all 5 apps across pages, got %d", len(got))
	}
	if !equalInts(f.pagesRequested, []int{0, 1, 2}) {
		t.Fatalf("expected pages 0,1,2 then stop on the short page 2, got %v", f.pagesRequested)
	}
	for i, a := range got {
		if want := fmt.Sprintf("00000000-0000-0000-0000-%012d", i); a.UUID != want {
			t.Fatalf("app %d: uuid %q, want %q (page order must be preserved)", i, a.UUID, want)
		}
	}
}

// TestListAll_TotalIsPerPageCountNotGrandTotal pins the live-verified
// meaning of Total: the number of items on THAT page. Every page here
// reports Total=2 (== its own length) even though 4 apps exist, so a loop
// that stopped once it had collected Total items would stop after page 0
// and silently drop page 1. The item count is an exact multiple of the page
// size, so the walk must also cope with the final 204 No Content page.
func TestListAll_TotalIsPerPageCountNotGrandTotal(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(4)}
	d := newTestDataSource(t, f, 2)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected all 4 apps (Total=2 per page must not be read as a grand total), got %d", len(got))
	}
	if !equalInts(f.pagesRequested, []int{0, 1, 2}) {
		t.Fatalf("expected pages 0,1 (full) then page 2 (204 empty) ending the walk, got %v", f.pagesRequested)
	}
}

func TestListAll_EmptyFirstPage(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{}
	d := newTestDataSource(t, f, 2)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no apps, got %d", len(got))
	}
	if !equalInts(f.pagesRequested, []int{0}) {
		t.Fatalf("expected a single page-0 request answered 204, got %v", f.pagesRequested)
	}
}

// TestListAll_UsesServerEchoedPageSize: the server caps the page size at 2
// even though 3 was requested. A page of 2 is FULL by the server's own
// echoed PageSize, so it must not be mistaken for the short last page.
func TestListAll_UsesServerEchoedPageSize(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(5), echoPageSize: 2}
	d := newTestDataSource(t, f, 3)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected all 5 apps despite the server-side page-size cap, got %d", len(got))
	}
	if !equalInts(f.pagesRequested, []int{0, 1, 2}) {
		t.Fatalf("expected pages 0,1,2, got %v", f.pagesRequested)
	}
}

func TestListAll_ServerIgnoringPageErrorsInsteadOfLooping(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(4), ignorePage: true}
	d := newTestDataSource(t, f, 2)

	_, err := d.listAll(context.Background(), "og-uuid")
	if err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("expected a did-not-advance error, got %v", err)
	}
	if !equalInts(f.pagesRequested, []int{0, 1}) {
		t.Fatalf("expected the walk to give up after page 1 repeated page 0, got %v", f.pagesRequested)
	}
}

// TestListAll_MaxPagesCapErrorsNeverTruncates proves the maxSearchPages
// guard is a hard stop that ERRORS once hit, rather than silently returning
// whatever was collected so far. The fake server here never naturally
// terminates the walk (every page is full of genuinely new items -- there
// are exactly maxSearchPages one-item pages, none short or empty), so the
// only way the loop can end is via the maxSearchPages cap.
func TestListAll_MaxPagesCapErrorsNeverTruncates(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{apps: makeApps(maxSearchPages)}
	d := newTestDataSource(t, f, 1)

	got, err := d.listAll(context.Background(), "og-uuid")
	if err == nil {
		t.Fatalf("expected an error once the walk hit maxSearchPages, got %d apps with no error", len(got))
	}
	if !strings.Contains(err.Error(), "maxSearchPages") {
		t.Fatalf("expected the maxSearchPages cap error, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected a nil (not truncated/partial) result once the cap errors, got %d apps", len(got))
	}
	if len(f.pagesRequested) != maxSearchPages {
		t.Fatalf("expected exactly %d page requests (0..%d) before the cap aborted the walk, got %d", maxSearchPages, maxSearchPages-1, len(f.pagesRequested))
	}
}

// TestListAll_PartialOverlapPageDedupsAndContinues covers a page that is
// PARTIALLY overlapping with previously-seen applications: some of its
// items were already collected on an earlier page, and some are genuinely
// new. This must NOT trip the "full page, zero new items" did-not-advance
// guard (that guard should only fire when a page contributes NOTHING new),
// and the already-seen item must appear exactly once in the final result.
// This is distinct from TestListAll_ServerIgnoringPageErrorsInsteadOfLooping,
// which covers a FULLY redundant page (zero new items).
func TestListAll_PartialOverlapPageDedupsAndContinues(t *testing.T) {
	t.Parallel()
	apps := makeApps(4) // A=0, B=1, C=2, D=3
	uuidOf := func(i int) string {
		t.Helper()
		v, ok := apps[i]["Uuid"].(string)
		if !ok {
			t.Fatalf("apps[%d][\"Uuid\"] is not a string: %#v", i, apps[i]["Uuid"])
		}
		return v
	}
	pages := [][]map[string]any{
		{apps[0], apps[1]}, // page 0: A, B -- both new
		{apps[1], apps[2]}, // page 1: B (already seen), C (new) -- partial overlap, full page
		{apps[3]},          // page 2: D -- short page, ends the walk
	}
	var pagesRequested []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/mam/apps/purchased/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pagesRequested = append(pagesRequested, page)
		if page >= len(pages) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		items := pages[page]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Application": items,
			"Page":        page,
			"PageSize":    2,
			"Total":       len(items),
		})
	}))
	t.Cleanup(server.Close)
	c, err := client.NewClient(&client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	d := &purchasedApplicationsDataSource{
		search:   sdk.NewPurchasedAppsV1Service(c),
		pageSize: 2,
	}

	got, err := d.listAll(context.Background(), "og-uuid")
	if err != nil {
		t.Fatalf("listAll: %v", err)
	}
	if !equalInts(pagesRequested, []int{0, 1, 2}) {
		t.Fatalf("expected pages 0,1,2 (page 2 short, ending the walk), got %v", pagesRequested)
	}

	// A, B, C, D -- exactly once each; B (the partially-overlapping item)
	// must not be duplicated even though it was served on two pages.
	if len(got) != 4 {
		t.Fatalf("expected 4 deduplicated apps, got %d: %+v", len(got), got)
	}
	seen := map[string]int{}
	for _, a := range got {
		seen[strings.ToLower(a.UUID)]++
	}
	bUUID := strings.ToLower(uuidOf(1))
	if seen[bUUID] != 1 {
		t.Fatalf("expected the partially-overlapping app to appear exactly once, appeared %d times", seen[bUUID])
	}
	wantOrder := []string{uuidOf(0), uuidOf(1), uuidOf(2), uuidOf(3)}
	for i, want := range wantOrder {
		if got[i].UUID != want {
			t.Fatalf("app %d: uuid %q, want %q (first-seen order must be preserved)", i, got[i].UUID, want)
		}
	}
}

func TestListAll_SearchErrorPropagates(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{searchStatus: http.StatusBadRequest}
	d := newTestDataSource(t, f, 2)

	if _, err := d.listAll(context.Background(), "og-uuid"); err == nil {
		t.Fatal("expected the search error to propagate")
	}
}

// --- Read (framework-level) ---

func getSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	var resp datasource.SchemaResponse
	(&purchasedApplicationsDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp
}

func runRead(t *testing.T, d *purchasedApplicationsDataSource, ogUUID string) (PurchasedApplicationsDataSourceModel, datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	s := getSchema(t).Schema
	objType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	cfgValues := map[string]tftypes.Value{}
	stateValues := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		cfgValues[name] = tftypes.NewValue(at, nil)
		stateValues[name] = tftypes.NewValue(at, nil)
	}
	cfgValues["organization_group_uuid"] = tftypes.NewValue(tftypes.String, ogUUID)

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, cfgValues)}}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(objType, stateValues)}}
	d.Read(ctx, req, &resp)

	var model PurchasedApplicationsDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &model)...)
	}
	return model, resp
}

func TestRead_MapsFieldsAndAssignmentCount(t *testing.T) {
	t.Parallel()
	apps := makeApps(3)
	apps[2]["Platform"] = 10
	f := &fakeTenant{
		apps: apps,
		assignmentsByUUID: map[string]int{
			appUUID(0): 2,
			appUUID(1): 0,
			// app 2 has no entry: its assignment-rules GET answers 204.
		},
	}
	d := newTestDataSource(t, f, 2)

	model, resp := runRead(t, d, "og-uuid")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	got := model.PurchasedApplications
	if len(got) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(got))
	}
	first := got[0]
	if first.UUID.ValueString() != appUUID(0) ||
		first.Name.ValueString() != "App 0" ||
		first.BundleID.ValueString() != "com.example.app0" ||
		first.ID.ValueInt64() != 35700 ||
		first.Platform.ValueInt64() != 2 ||
		first.LocationGroupID.ValueInt64() != 138883 ||
		first.OrganizationGroupUuid.ValueString() != "og-owner" {
		t.Fatalf("unexpected field mapping: %+v", first)
	}
	if got[2].Platform.ValueInt64() != 10 {
		t.Fatalf("expected platform 10 for app 2, got %v", got[2].Platform)
	}
	for i, want := range []int64{2, 0, 0} {
		if got[i].AssignmentCount.ValueInt64() != want {
			t.Fatalf("app %d: assignment_count %v, want %d", i, got[i].AssignmentCount, want)
		}
	}
	if len(f.rulesRequested) != 3 {
		t.Fatalf("expected one assignment-rules read per app, got %v", f.rulesRequested)
	}
}

func TestRead_EmptyResultIsEmptyListNotNull(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{}
	d := newTestDataSource(t, f, 2)

	model, resp := runRead(t, d, "og-uuid")
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if model.PurchasedApplications == nil || len(model.PurchasedApplications) != 0 {
		t.Fatalf("expected an empty, non-null list, got %#v", model.PurchasedApplications)
	}
}

func TestRead_AssignmentRuleErrorFailsClosed(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{
		apps:          makeApps(2),
		ruleStatusFor: map[string]int{appUUID(1): http.StatusBadRequest},
	}
	d := newTestDataSource(t, f, 2)

	_, resp := runRead(t, d, "og-uuid")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an assignment-rule read error to fail the whole Read")
	}
}

func TestRead_EmptyOrgGroupUUIDRejected(t *testing.T) {
	t.Parallel()
	f := &fakeTenant{}
	d := newTestDataSource(t, f, 2)

	_, resp := runRead(t, d, "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty organization_group_uuid")
	}
	if len(f.pagesRequested) != 0 {
		t.Fatalf("expected no search request, got %v", f.pagesRequested)
	}
}

func TestMetadataTypeName(t *testing.T) {
	t.Parallel()
	var resp datasource.MetadataResponse
	NewPurchasedApplicationsDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)
	if resp.TypeName != "uem_purchased_applications" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}
