package updates

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeUpdatesV1Search is a test-only fake satisfying updatesV1SearchAPI. It
// serves canned, paginated catalog responses and per-update-UUID deployment
// responses so the two-level enumeration can be tested without a real SDK
// client.
type fakeUpdatesV1Search struct {
	// catalogPages, keyed by 1-based page number, is what
	// GetDeviceUpdatesBySearchParameters returns for that page.
	catalogPages map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model
	catalogTotal int
	catalogErr   error

	// deploymentsByUpdate maps an update UUID to the deployments
	// GetDeploymentsByDeviceUpdate should return for it. An update UUID with
	// no entry returns nil (simulating a clean 204-empty response).
	deploymentsByUpdate map[string][]sdk.DeploymentV1Model
	deploymentErrFor    map[string]error

	// detailsByDeploymentUUID maps a deployment UUID to the detail record
	// GetDeviceUpdateDeploymentDetails should return for it. A deployment
	// UUID with NO entry in this map (and no matching detailErrFor entry)
	// gets a default non-nil detail record synthesized by
	// defaultDetailFor, so pre-existing tests that never set up detail
	// fixtures at all keep passing unaffected. Use an explicit entry
	// mapping to a nil *sdk.DeviceUpdateDeploymentV1Model (via
	// detailsByDeploymentUUID[uuid] = nil, with the key still present) to
	// simulate the 204/nil-detail case instead.
	detailsByDeploymentUUID map[string]*sdk.DeviceUpdateDeploymentV1Model
	detailErrFor            map[string]error

	catalogPagesRequested []int
	deploymentUUIDsCalled []string
	detailUUIDsCalled     []string

	// optsRequested records every options struct passed to
	// GetDeviceUpdatesBySearchParameters, so tests can assert filters like
	// Platform/UpdateName were forwarded correctly.
	optsRequested []*sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions
}

func (f *fakeUpdatesV1Search) GetDeviceUpdatesBySearchParameters(
	_ context.Context,
	opts *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions,
) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error) {
	f.optsRequested = append(f.optsRequested, opts)
	if f.catalogErr != nil {
		return nil, nil, f.catalogErr
	}
	page := 1
	if opts.Page != nil {
		page = *opts.Page
	}
	f.catalogPagesRequested = append(f.catalogPagesRequested, page)
	total := f.catalogTotal
	return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{
		Total:      &total,
		UpdateList: f.catalogPages[page],
	}, nil
}

func (f *fakeUpdatesV1Search) GetDeploymentsByDeviceUpdate(
	_ context.Context,
	updateUUID string,
	_ *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions,
) (http.Header, *[]sdk.DeploymentV1Model, error) {
	f.deploymentUUIDsCalled = append(f.deploymentUUIDsCalled, updateUUID)
	if err, ok := f.deploymentErrFor[updateUUID]; ok {
		return nil, nil, err
	}
	deps, ok := f.deploymentsByUpdate[updateUUID]
	if !ok {
		// Mirrors a real HTTP 204 (no deployments for this update): clean
		// nil, no error.
		return nil, nil, nil
	}
	return nil, &deps, nil
}

func (f *fakeUpdatesV1Search) GetDeviceUpdateDeploymentDetails(
	_ context.Context,
	uuid string,
) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	f.detailUUIDsCalled = append(f.detailUUIDsCalled, uuid)
	if err, ok := f.detailErrFor[uuid]; ok {
		return nil, nil, err
	}
	if detail, ok := f.detailsByDeploymentUUID[uuid]; ok {
		// Present but possibly nil (explicit 204/nil-detail simulation).
		return nil, detail, nil
	}
	// No fixture configured at all for this UUID: synthesize a default
	// non-nil detail record WITH a non-empty owner, so pre-existing tests
	// that never set up detail fixtures keep passing unaffected by this
	// level-3 addition -- an empty OrganizationGroupUUID is now treated as
	// an unresolved-owner skip (gate-found B1), so a synthesized owner is
	// required here, not optional.
	return nil, &sdk.DeviceUpdateDeploymentV1Model{UUID: uuid, OrganizationGroupUUID: "og-default-owner"}, nil
}

func makeCatalogPage(prefix string, n int) []sdk.DeviceUpdateDetailsDeploymentsV1Model {
	out := make([]sdk.DeviceUpdateDetailsDeploymentsV1Model, n)
	for i := range out {
		out[i] = sdk.DeviceUpdateDetailsDeploymentsV1Model{UUID: fmt.Sprintf("%s-%d", prefix, i)}
	}
	return out
}

// --- helpers ---

func getUpdateDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &updateDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createUpdateDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getUpdateDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptyUpdateDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getUpdateDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := make(map[string]tftypes.Value)
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

func updateDeploymentsNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getUpdateDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["update_deployments"].(tftypes.List)
	if !ok {
		t.Fatalf("expected update_deployments to be tftypes.List, got %T", objType.AttributeTypes["update_deployments"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

func runUpdateDSRead(t *testing.T, ds *updateDataSource, ogUUID string) (UpdateDataSourceModel, datasource.ReadResponse) {
	t.Helper()
	return runUpdateDSReadWithFilters(t, ds, ogUUID, "", "")
}

// runUpdateDSReadWithFilters is runUpdateDSRead plus the ability to set the
// optional platform/update_name filters, so tests can assert they're
// forwarded into the level-1 catalog search opts.
func runUpdateDSReadWithFilters(t *testing.T, ds *updateDataSource, ogUUID, platform, updateName string) (UpdateDataSourceModel, datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()

	platformVal := tftypes.NewValue(tftypes.String, nil)
	if platform != "" {
		platformVal = tftypes.NewValue(tftypes.String, platform)
	}
	updateNameVal := tftypes.NewValue(tftypes.String, nil)
	if updateName != "" {
		updateNameVal = tftypes.NewValue(tftypes.String, updateName)
	}

	config := createUpdateDSConfig(t, map[string]tftypes.Value{
		"organization_group_uuid": tftypes.NewValue(tftypes.String, ogUUID),
		"platform":                platformVal,
		"update_name":             updateNameVal,
		"update_deployments":      tftypes.NewValue(tftypes.List{ElementType: updateDeploymentsNestedType(t)}, nil),
	})
	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyUpdateDSState(t)}
	ds.Read(ctx, readReq, &readResp)

	var out UpdateDataSourceModel
	if !readResp.Diagnostics.HasError() {
		if diags := readResp.State.Get(ctx, &out); diags.HasError() {
			t.Fatalf("unexpected error reading state: %v", diags.Errors())
		}
	}
	return out, readResp
}

// --- Schema ---

func TestUpdateDeploymentsDataSource_Schema_RequiresOrgGroupUUID(t *testing.T) {
	t.Parallel()

	resp := getUpdateDSSchema(t)
	attr, ok := resp.Schema.Attributes["organization_group_uuid"]
	if !ok {
		t.Fatal("expected organization_group_uuid attribute to exist")
	}
	if !attr.IsRequired() {
		t.Errorf("expected organization_group_uuid to be Required, got %+v", attr)
	}
}

func TestUpdateDeploymentsDataSource_Schema_HasUUIDNotAvailableForInstall(t *testing.T) {
	t.Parallel()

	elem := updateDeploymentsNestedType(t)
	if _, ok := elem.AttributeTypes["uuid"]; !ok {
		t.Fatal("expected update_deployments entries to carry a uuid field — this is the whole point of the concept rewrite (the old catalog enumerator had no UUID field at all)")
	}
	if _, ok := elem.AttributeTypes["available_for_install"]; ok {
		t.Fatal("available_for_install is a catalog-search field from the OLD (wrong) enumerator and must not appear in the new deployments schema")
	}
}

// stateUpdateDeploymentsIsNull inspects the RAW state value (not the decoded
// Go struct) for the update_deployments attribute and reports whether it is
// NULL, as opposed to a known empty list. Decoding straight into
// UpdateDataSourceModel.UpdateDeployments would not distinguish these two
// cases (both can decode to a nil/zero-length Go slice), but Terraform
// itself does distinguish them: a null list attribute is what caused the
// gate-found `terraform output "update_deployments" not found` failure on a
// zero-deployment run, while a known empty list ([]) is what a clean
// "No update deployments found" exit needs.
func stateUpdateDeploymentsIsNull(t *testing.T, state tfsdk.State) bool {
	t.Helper()
	var obj map[string]tftypes.Value
	if err := state.Raw.As(&obj); err != nil {
		t.Fatalf("decode state as object: %v", err)
	}
	listVal, ok := obj["update_deployments"]
	if !ok {
		t.Fatal("state has no update_deployments attribute at all")
	}
	return listVal.IsNull()
}

// TestUpdateDeploymentsDataSource_Read_EmptyCatalogEmitsEmptyNotNullList
// proves the empty-catalog leg of the nil-vs-empty-slice fix: gate finding,
// live e2e against org group 138883 with zero updates in the catalog —
// `ws1-tf onboard --type update_deployment` hard-failed with
// `terraform output "update_deployments" not found` because the Go slice
// was left as a nil var, which Terraform serializes as a NULL list
// attribute and then drops from `terraform output` entirely, instead of the
// intended "No update deployments found" clean exit. Against the pre-fix
// `var deployments []DeploymentSummary`, this fails: an empty catalog never
// appends anything, so `deployments` stays nil and the state attribute is
// null, not a known empty list.
func TestUpdateDeploymentsDataSource_Read_EmptyCatalogEmitsEmptyNotNullList(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: {},
		},
		catalogTotal:        0,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if stateUpdateDeploymentsIsNull(t, resp.State) {
		t.Fatal("expected update_deployments to be a known EMPTY list on an empty catalog, got NULL — this is exactly the bug that made `terraform output` fail")
	}
	if len(out.UpdateDeployments) != 0 {
		t.Fatalf("expected 0 deployments, got %d", len(out.UpdateDeployments))
	}
}

// TestUpdateDeploymentsDataSource_Read_EveryUpdateHasNoDeploymentsEmitsEmptyNotNullList
// proves the second exit path the gate asked to be covered explicitly: a
// non-empty catalog where every single update's GetDeploymentsByDeviceUpdate
// call cleanly returns nil (204-empty), so the deployments list is built by
// zero total appends despite iterating a non-empty updateUUIDs slice. Same
// pre-fix failure mode as the empty-catalog case above: `deployments` never
// gets appended to, so it stays nil (null in state) instead of a known
// empty list.
func TestUpdateDeploymentsDataSource_Read_EveryUpdateHasNoDeploymentsEmitsEmptyNotNullList(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 5),
		},
		catalogTotal:        5,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{}, // every UUID misses -> nil/204 for all 5
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(fake.deploymentUUIDsCalled) != 5 {
		t.Fatalf("expected a deployment lookup for all 5 updates, got %d", len(fake.deploymentUUIDsCalled))
	}
	if stateUpdateDeploymentsIsNull(t, resp.State) {
		t.Fatal("expected update_deployments to be a known EMPTY list when every update has zero deployments, got NULL")
	}
	if len(out.UpdateDeployments) != 0 {
		t.Fatalf("expected 0 deployments, got %d", len(out.UpdateDeployments))
	}
}

// --- Read: multi-page catalog pagination, carrying forward the pagination
// fix, now feeding the level-2 deployment fetch. ---

// (B13: the base is FIXED at 1, never probed -- see WalkTwiceVerifiedFromBase
// -- and page 3's 73 items overshoot the requested catalogPageSize=50 so it
// isn't recognized as a short/final page -- one more, confirming empty page
// 4 is fetched. A multi-page result is then walked a second, fully
// independent time. Total calls: (pages 1-3 + confirming page 4) x 2 = 8.)
func TestUpdateDeploymentsDataSource_Read_WalksAllCatalogPagesBeforeFetchingDeployments(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("p1", 50),
			2: makeCatalogPage("p2", 50),
			3: makeCatalogPage("p3", 73),
		},
		catalogTotal: 173,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"p1-0": {{UUID: "dep-1", Name: "Pilot"}},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(fake.catalogPagesRequested) != 8 {
		t.Fatalf("expected 8 catalog page requests (see doc comment), got %d: %v", len(fake.catalogPagesRequested), fake.catalogPagesRequested)
	}
	if len(fake.deploymentUUIDsCalled) != 173 {
		t.Fatalf("expected a GetDeploymentsByDeviceUpdate call for all 173 catalog entries (page 2/3 must not be silently dropped), got %d", len(fake.deploymentUUIDsCalled))
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected 1 flattened deployment (only p1-0 has one), got %d", len(out.UpdateDeployments))
	}
	if out.UpdateDeployments[0].UUID.ValueString() != "dep-1" {
		t.Errorf("expected deployment uuid 'dep-1', got %q", out.UpdateDeployments[0].UUID.ValueString())
	}
}

// TestUpdateDeploymentsDataSource_Read_SinglePage is the non-tautological
// carry-forward of the pagination-fix test against the new file: everything
// fits on page 1. Exactly 1 catalog page request is made (B13: the base is
// FIXED at 1, never probed -- see WalkTwiceVerifiedFromBase -- and a
// single-page result is never walked twice).
func TestUpdateDeploymentsDataSource_Read_SinglePage(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 10),
		},
		catalogTotal:        10,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{},
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(fake.catalogPagesRequested) != 1 {
		t.Fatalf("expected exactly 1 catalog page request, got %d: %v", len(fake.catalogPagesRequested), fake.catalogPagesRequested)
	}
	if len(fake.deploymentUUIDsCalled) != 10 {
		t.Fatalf("expected a deployment lookup for all 10 updates, got %d", len(fake.deploymentUUIDsCalled))
	}
}

// TestUpdateDeploymentsDataSource_Read_NonEmptyPageWithNilTotalErrors (B13):
// pagewalk.WalkTwiceVerified requires a reported Total on every non-empty
// page -- the old loop's "fall back to a short/empty-page heuristic when
// Total is nil" path is gone, replaced by the stricter, shared pagewalk
// contract (the same one B12's uem_scripts/uem_applications/
// uem_mac_applications walks already enforce). A real, non-empty catalog
// page with no Total at all is now a hard error, not silently tolerated.
// Fail-on-revert: reintroducing the old nil-Total fallback would make this
// test see a successful walk instead of the error below.
func TestUpdateDeploymentsDataSource_Read_NonEmptyPageWithNilTotalErrors(t *testing.T) {
	t.Parallel()

	// The shared fakeUpdatesV1Search always sets Total to &f.catalogTotal
	// (never nil), so to exercise a genuinely nil Total we use a small local
	// fake instead.
	nilTotalFake := &nilTotalCatalogFake{
		pages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("p1", 50),
		},
	}
	ds := &updateDataSource{search: nilTotalFake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a non-empty catalog page with no reported total")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "no reported total") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 'no reported total' error, got: %v", resp.Diagnostics.Errors())
	}
	if len(nilTotalFake.deploymentUUIDsCalled) != 0 {
		t.Fatalf("expected zero GetDeploymentsByDeviceUpdate calls when the catalog walk itself errors, got %d", len(nilTotalFake.deploymentUUIDsCalled))
	}
}

// TestUpdateDeploymentsDataSource_Read_ServerCappedPageSizeStillReachesTotal
// proves the short-page-overrides-Total gate finding: a server is free to
// cap the effective page size below the requested PageSize (here: PageSize
// requested is 50, but the server always returns pages of 20), so a
// short/non-full page must NOT be treated as end-of-results while a Total is
// present and not yet reached. Against the pre-fix ordering (short-page
// checked before Total), this fails: the very first page, 20 items, is
// already short compared to the requested pageSize=50, so the loop stops
// immediately with only 20 of 120 enumerated.
//
// B13: the base is FIXED at 1, never probed (WalkTwiceVerifiedFromBase) --
// the fake's page-0 guard is defensive documentation of the real endpoint's
// shape, not exercised by this call path. The exact expected page count
// (accounting for the mandatory second, independent walk of a multi-page
// result) is asserted below.
func TestUpdateDeploymentsDataSource_Read_ServerCappedPageSizeStillReachesTotal(t *testing.T) {
	t.Parallel()

	fake := &serverCappedPageSizeFake{effectivePageSize: 20, total: 120}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(fake.deploymentUUIDsCalled) != 120 {
		t.Fatalf("expected all 120 catalog entries enumerated despite the server capping every page at 20 (well under the requested pageSize=50), got %d — Total must win over a short-page heuristic when Total is present", len(fake.deploymentUUIDsCalled))
	}
	if len(fake.pagesRequested) != 12 {
		t.Fatalf("expected 12 pages requested (pages 1-6, x2 independent walks), got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// serverCappedPageSizeFake simulates a server that ignores the requested
// PageSize and always returns effectivePageSize items per page (always
// SHORT relative to the 50 requested by Read), while still reporting an
// accurate Total. Used to prove Total is honored over a short-page
// heuristic when Total is present.
type serverCappedPageSizeFake struct {
	effectivePageSize int
	total             int

	pagesRequested        []int
	deploymentUUIDsCalled []string
}

func (f *serverCappedPageSizeFake) GetDeviceUpdatesBySearchParameters(
	_ context.Context,
	opts *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions,
) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error) {
	page := 1
	if opts.Page != nil {
		page = *opts.Page
	}
	f.pagesRequested = append(f.pagesRequested, page)

	// Page 0 is out of range for this endpoint (B13: "Base 1 is correct") --
	// this fake's own start-index math below assumes 1-based paging and
	// would otherwise happily return real-looking data for page 0 too,
	// defeating pagewalk.WalkTwiceVerified's base probe.
	if page < 1 {
		return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{}, nil
	}

	start := (page - 1) * f.effectivePageSize
	n := f.effectivePageSize
	if start+n > f.total {
		n = f.total - start
	}
	if n < 0 {
		n = 0
	}
	total := f.total
	return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{
		Total:      &total,
		UpdateList: makeCatalogPage(fmt.Sprintf("page%d", page), n),
	}, nil
}

func (f *serverCappedPageSizeFake) GetDeploymentsByDeviceUpdate(
	_ context.Context,
	updateUUID string,
	_ *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions,
) (http.Header, *[]sdk.DeploymentV1Model, error) {
	f.deploymentUUIDsCalled = append(f.deploymentUUIDsCalled, updateUUID)
	return nil, nil, nil
}

// GetDeviceUpdateDeploymentDetails is never reached by this fake in
// practice (GetDeploymentsByDeviceUpdate above always returns a nil deps
// slice, so the level-3 loop body never runs), but is required to satisfy
// updatesV1SearchAPI.
func (f *serverCappedPageSizeFake) GetDeviceUpdateDeploymentDetails(
	_ context.Context,
	uuid string,
) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	return nil, &sdk.DeviceUpdateDeploymentV1Model{UUID: uuid}, nil
}

// TestUpdateDeploymentsDataSource_Read_CatalogPaginationHardCapErrors proves
// the maxCatalogPages termination guard: a fake that always returns a FULL
// (non-short) page with a nil Total, forever, must cause Read to return a
// diagnostic ERROR once the hard cap is hit — not loop forever, and not
// silently return a truncated partial result set as if it were complete.
//
// Asserts directly on the RAW state (not the decoded UpdateDataSourceModel)
// and on the level-2 fake's call count: runUpdateDSRead only decodes state
// into the returned struct when there's no error, so a prior version of
// this test that only checked len(out.UpdateDeployments) was VACUOUS — `out`
// is always the zero value on any error path, so that assertion would still
// pass even if the loop broke (instead of returning) and the caller went on
// to write partial state, or if level-2 deployment lookups ran anyway before
// the cap fired. Checking resp.State.Raw is unchanged from its initial empty
// value, AND that GetDeploymentsByDeviceUpdate was never called, closes both
// gaps — confirmed by reverting the guard to `break` instead of `return` in
// a scratch copy: with that mutation the level-1 loop still stops, but level
// 2 then runs against whatever partial updateUUIDs it collected, so
// deploymentUUIDsCalled becomes non-zero and this test correctly fails.
func TestUpdateDeploymentsDataSource_Read_CatalogPaginationHardCapErrors(t *testing.T) {
	t.Parallel()

	fake := &infiniteFullPageCatalogFake{pageSize: 50}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic error once the hard page cap is exceeded, got none")
	}
	if !stateRawIsEmptyInitial(t, resp.State) {
		t.Fatal("expected state to remain unchanged (null/empty) on hard-cap error, got a populated state — that would mean partial data was written despite the error")
	}
	if fake.deploymentCallCount != 0 {
		t.Fatalf("expected zero GetDeploymentsByDeviceUpdate calls once the hard cap fires (level 2 must never run against a partial/capped level-1 result), got %d", fake.deploymentCallCount)
	}
	if fake.callCount > maxCatalogPages+1 {
		t.Fatalf("expected the loop to stop at or just past maxCatalogPages (%d), got %d calls — it kept looping past the cap", maxCatalogPages, fake.callCount)
	}
}

// stateRawIsEmptyInitial reports whether the state's update_deployments
// attribute is still null, i.e. Read never got far enough to call
// resp.State.Set with any data — the expected shape when Read returns early
// on a diagnostic error.
func stateRawIsEmptyInitial(t *testing.T, state tfsdk.State) bool {
	t.Helper()
	return stateUpdateDeploymentsIsNull(t, state)
}

// TestUpdateDeploymentsDataSource_Read_PageIgnoringServerFailsClosedAtCap
// proves the deduped-count-vs-Total gate finding: the Total stop condition
// (len(updateUUIDs) >= *res.Total) compares the DEDUPED accumulator, not raw
// items received. A server that ignores the requested Page param and always
// returns the SAME 50 items, with an accurate Total=120 it never actually
// reaches (since the deduped count is stuck at 50), must therefore run out
// to maxCatalogPages and return the pagination-limit error — not loop
// forever, and not silently return a stuck/incomplete 50-item result as if
// it were the full 120. This is the intended fail-closed behavior (the
// pre-dedup code would have silently returned the same 50 rows repeated,
// which is worse than a loud error).
//
// Fail-on-revert: this test does NOT catch the dedup logic being removed
// entirely (that's TestUpdateDeploymentsDataSource_Read_DedupsUpdateUUIDsAcrossPages's
// job) — it specifically catches a mutation that compares Total against the
// RAW (non-deduped) count instead of the deduped one, or that drops the
// maxCatalogPages cap. Confirmed against both mutations in a scratch copy:
// comparing against a raw counter incremented once per item (250 after 5
// pages of 50) would reach Total=120 on page 3 and Read would succeed with
// no error — this test's `!resp.Diagnostics.HasError()` check catches that;
// dropping the cap would spin the fake's callCount unboundedly — the
// callCount bound below catches that.
func TestUpdateDeploymentsDataSource_Read_PageIgnoringServerFailsClosedAtCap(t *testing.T) {
	t.Parallel()

	fake := &pageIgnoringCatalogFake{pageSize: 50, total: 120}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic error once the hard page cap is exceeded (a Page-ignoring server with an unreachable deduped Total must fail closed), got none")
	}
	if !stateRawIsEmptyInitial(t, resp.State) {
		t.Fatal("expected state to remain unchanged (null/empty) on the cap error, got a populated state")
	}
	if fake.deploymentCallCount != 0 {
		t.Fatalf("expected zero GetDeploymentsByDeviceUpdate calls once the cap fires, got %d", fake.deploymentCallCount)
	}
	if fake.callCount > maxCatalogPages+1 {
		t.Fatalf("expected the loop to stop at or just past maxCatalogPages (%d), got %d calls", maxCatalogPages, fake.callCount)
	}
}

// pageIgnoringCatalogFake simulates a server that ignores the requested Page
// param entirely and always returns the exact SAME pageSize items (same
// UUIDs every call), while still reporting an accurate Total larger than
// that one page's unique-item count — so the deduped accumulator can never
// reach Total no matter how many pages are walked.
type pageIgnoringCatalogFake struct {
	pageSize  int
	total     int
	callCount int

	deploymentCallCount int
}

func (f *pageIgnoringCatalogFake) GetDeviceUpdatesBySearchParameters(
	_ context.Context,
	_ *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions,
) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error) {
	f.callCount++
	total := f.total
	return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{
		Total:      &total,
		UpdateList: makeCatalogPage("stuck", f.pageSize),
	}, nil
}

func (f *pageIgnoringCatalogFake) GetDeploymentsByDeviceUpdate(
	_ context.Context,
	_ string,
	_ *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions,
) (http.Header, *[]sdk.DeploymentV1Model, error) {
	f.deploymentCallCount++
	return nil, nil, nil
}

// GetDeviceUpdateDeploymentDetails is never reached by this fake (see
// serverCappedPageSizeFake's equivalent method for why), but is required to
// satisfy updatesV1SearchAPI.
func (f *pageIgnoringCatalogFake) GetDeviceUpdateDeploymentDetails(
	_ context.Context,
	uuid string,
) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	return nil, &sdk.DeviceUpdateDeploymentV1Model{UUID: uuid}, nil
}

// nilTotalCatalogFake is a minimal fake satisfying updatesV1SearchAPI whose
// GetDeviceUpdatesBySearchParameters ALWAYS returns a genuinely nil Total
// (unlike fakeUpdatesV1Search, which always populates Total from
// catalogTotal), so nil-Total pagination behavior can be tested in isolation.
type nilTotalCatalogFake struct {
	pages                 map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model
	pagesRequested        []int
	deploymentUUIDsCalled []string
}

func (f *nilTotalCatalogFake) GetDeviceUpdatesBySearchParameters(
	_ context.Context,
	opts *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions,
) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error) {
	page := 1
	if opts.Page != nil {
		page = *opts.Page
	}
	f.pagesRequested = append(f.pagesRequested, page)
	return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{
		Total:      nil,
		UpdateList: f.pages[page],
	}, nil
}

func (f *nilTotalCatalogFake) GetDeploymentsByDeviceUpdate(
	_ context.Context,
	updateUUID string,
	_ *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions,
) (http.Header, *[]sdk.DeploymentV1Model, error) {
	f.deploymentUUIDsCalled = append(f.deploymentUUIDsCalled, updateUUID)
	return nil, nil, nil
}

// GetDeviceUpdateDeploymentDetails is never reached by this fake (see
// serverCappedPageSizeFake's equivalent method for why), but is required to
// satisfy updatesV1SearchAPI.
func (f *nilTotalCatalogFake) GetDeviceUpdateDeploymentDetails(
	_ context.Context,
	uuid string,
) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	return nil, &sdk.DeviceUpdateDeploymentV1Model{UUID: uuid}, nil
}

// infiniteFullPageCatalogFake always returns a full (non-short) page of
// pageSize items with a nil Total, on every page, forever — simulating a
// pathological/misbehaving API response that never signals end-of-results.
// Used to prove the maxCatalogPages hard-cap guard actually terminates the
// loop with a diagnostic error instead of looping unboundedly.
type infiniteFullPageCatalogFake struct {
	pageSize            int
	callCount           int
	deploymentCallCount int
}

func (f *infiniteFullPageCatalogFake) GetDeviceUpdatesBySearchParameters(
	_ context.Context,
	opts *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions,
) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error) {
	f.callCount++
	page := 1
	if opts.Page != nil {
		page = *opts.Page
	}
	return nil, &sdk.DeviceUpdatePagedSearchResultsV1Model{
		Total:      nil,
		UpdateList: makeCatalogPage(fmt.Sprintf("page%d", page), f.pageSize),
	}, nil
}

func (f *infiniteFullPageCatalogFake) GetDeploymentsByDeviceUpdate(
	_ context.Context,
	_ string,
	_ *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions,
) (http.Header, *[]sdk.DeploymentV1Model, error) {
	f.deploymentCallCount++
	return nil, nil, nil
}

// GetDeviceUpdateDeploymentDetails is never reached by this fake (see
// serverCappedPageSizeFake's equivalent method for why), but is required to
// satisfy updatesV1SearchAPI.
func (f *infiniteFullPageCatalogFake) GetDeviceUpdateDeploymentDetails(
	_ context.Context,
	uuid string,
) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	return nil, &sdk.DeviceUpdateDeploymentV1Model{UUID: uuid}, nil
}

// --- Read: an update with zero deployments (204-empty) is a clean skip, not
// an error. ---

func TestUpdateDeploymentsDataSource_Read_EmptyDeploymentsForAnUpdateIsCleanSkip(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 2),
		},
		catalogTotal: 2,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}},
			// u-1 deliberately absent — simulates HTTP 204 (nil, no error).
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected a clean skip for the zero-deployment update, got error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected exactly 1 deployment (u-1 contributes none), got %d", len(out.UpdateDeployments))
	}
}

// --- Read: flatten across multiple updates, each with multiple
// deployments. ---

func TestUpdateDeploymentsDataSource_Read_FlattensAcrossMultipleUpdates(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 3),
		},
		catalogTotal: 3,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}, {UUID: "dep-2", Name: "Broad"}},
			"u-1": {{UUID: "dep-3", Name: "Canary"}},
			// u-2 has zero deployments.
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 3 {
		t.Fatalf("expected 3 flattened deployments across 2 contributing updates, got %d", len(out.UpdateDeployments))
	}
	gotUUIDs := map[string]bool{}
	for _, d := range out.UpdateDeployments {
		gotUUIDs[d.UUID.ValueString()] = true
	}
	for _, want := range []string{"dep-1", "dep-2", "dep-3"} {
		if !gotUUIDs[want] {
			t.Errorf("expected flattened output to contain deployment %q, got %+v", want, out.UpdateDeployments)
		}
	}
}

// --- Read: deployment fields map correctly (uuid/name/deployment_type/
// ranking/smart_group_count). ---

func TestUpdateDeploymentsDataSource_Read_MapsDeploymentFields(t *testing.T) {
	t.Parallel()

	ranking := 1
	smartGroupCount := 4
	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{
				UUID:            "dep-1",
				Name:            "Pilot",
				DeploymentType:  "AUTO",
				Ranking:         &ranking,
				SmartGroupCount: &smartGroupCount,
			}},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.UpdateDeployments))
	}
	got := out.UpdateDeployments[0]
	if got.Name.ValueString() != "Pilot" {
		t.Errorf("expected name 'Pilot', got %q", got.Name.ValueString())
	}
	if got.DeploymentType.ValueString() != "AUTO" {
		t.Errorf("expected deployment_type 'AUTO', got %q", got.DeploymentType.ValueString())
	}
	if got.Ranking.ValueInt64() != 1 {
		t.Errorf("expected ranking 1, got %d", got.Ranking.ValueInt64())
	}
	if got.SmartGroupCount.ValueInt64() != 4 {
		t.Errorf("expected smart_group_count 4, got %d", got.SmartGroupCount.ValueInt64())
	}
}

// --- Empty org-group UUID must error cleanly before calling the SDK. ---

func TestUpdateDeploymentsDataSource_Read_EmptyOrgGroupUUID_Errors(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for empty organization_group_uuid")
	}
	if len(fake.catalogPagesRequested) != 0 {
		t.Fatal("expected the SDK to never be called for an empty organization_group_uuid")
	}
}

// --- Deployment-lookup error propagates as a diagnostic. ---

func TestUpdateDeploymentsDataSource_Read_DeploymentLookupErrorPropagates(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentErrFor: map[string]error{
			"u-0": fmt.Errorf("boom"),
		},
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the deployment lookup error to surface as a diagnostic")
	}
}

// --- Catalog-call error propagates as a diagnostic (level-1 error path). ---

func TestUpdateDeploymentsDataSource_Read_CatalogSearchErrorPropagates(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogErr: fmt.Errorf("catalog boom"),
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the catalog search error to surface as a diagnostic")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "catalog boom") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the underlying catalog error message to be surfaced in a diagnostic, got: %v", resp.Diagnostics.Errors())
	}
	if len(fake.deploymentUUIDsCalled) != 0 {
		t.Fatalf("expected zero GetDeploymentsByDeviceUpdate calls when the catalog search itself fails, got %d", len(fake.deploymentUUIDsCalled))
	}
}

// --- platform/update_name filters are forwarded into the level-1 catalog
// search opts. ---

func TestUpdateDeploymentsDataSource_Read_ForwardsPlatformAndUpdateNameFilters(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: {},
		},
		catalogTotal: 0,
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSReadWithFilters(t, ds, "og-uuid-1", "AppleOSX", "Security Update")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(fake.optsRequested) == 0 {
		t.Fatal("expected at least one catalog search call, got none")
	}
	opts := fake.optsRequested[0]
	if opts.Platform == nil || *opts.Platform != "AppleOSX" {
		t.Fatalf("expected opts.Platform to be forwarded as 'AppleOSX', got %v", opts.Platform)
	}
	if opts.UpdateName == nil || *opts.UpdateName != "Security Update" {
		t.Fatalf("expected opts.UpdateName to be forwarded as 'Security Update', got %v", opts.UpdateName)
	}
}

// --- deployment_start_time mapping: set vs. unset. ---

// TestUpdateDeploymentsDataSource_Read_MapsSetDeploymentStartTime proves a
// non-zero DeploymentStartTime decodes to the canonical UEMTime.String()
// representation ("2026-03-15T10:30:00.000Z" for the UTC instant used here),
// not an empty string or a Go zero-time string.
func TestUpdateDeploymentsDataSource_Read_MapsSetDeploymentStartTime(t *testing.T) {
	t.Parallel()

	startTime := client.NewUEMTime(time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC))
	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot", DeploymentStartTime: startTime}},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.UpdateDeployments))
	}
	got := out.UpdateDeployments[0].DeploymentStartTime
	if got.IsNull() {
		t.Fatal("expected deployment_start_time to be set, got null")
	}
	wantStr := startTime.String()
	if got.ValueString() != wantStr {
		t.Errorf("expected deployment_start_time %q, got %q", wantStr, got.ValueString())
	}
}

// TestUpdateDeploymentsDataSource_Read_UnsetDeploymentStartTimeIsNull proves
// a zero-value (unset) DeploymentStartTime decodes to types.StringNull(),
// not an empty string or a zero-time string.
func TestUpdateDeploymentsDataSource_Read_UnsetDeploymentStartTimeIsNull(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}}, // DeploymentStartTime left zero
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.UpdateDeployments))
	}
	got := out.UpdateDeployments[0].DeploymentStartTime
	if !got.IsNull() {
		t.Fatalf("expected deployment_start_time to be null for an unset DeploymentStartTime, got %q", got.ValueString())
	}
}

// --- Level-3 per-deployment detail lookup: owning organization group. ---

// TestUpdateDeploymentsDataSource_Read_DeploymentDetailErrorFailsClosed proves
// condition (a) from the plan: a detail-lookup error for one deployment must
// fail the WHOLE Read closed -- a diagnostic error, no partial state written,
// and processing must not continue past the failing deployment. Confirmed
// against a scratch-copy mutation that logs-and-continues instead of failing
// closed (drop the `return` after AddError and instead `continue`): with that
// mutation this test's HasError() check still passes, but the
// stateUpdateDeploymentsIsNull check fails because Read goes on to write a
// (partial) non-null update_deployments list to state, and the
// detailUUIDsCalled-after-error assertion below fails because the second
// deployment's detail lookup still runs.
func TestUpdateDeploymentsDataSource_Read_DeploymentDetailErrorFailsClosed(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			// Two deployments for the single update, so we can prove the
			// SECOND deployment's detail lookup never runs once the first
			// one's detail lookup fails.
			"u-0": {{UUID: "dep-1", Name: "Pilot"}, {UUID: "dep-2", Name: "Broad"}},
		},
		detailErrFor: map[string]error{
			"dep-1": fmt.Errorf("detail boom"),
		},
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the deployment detail lookup error to surface as a diagnostic")
	}
	if !stateUpdateDeploymentsIsNull(t, resp.State) {
		t.Fatal("expected state to remain unchanged (null) on a detail-lookup error, got a populated state — that would mean partial data was written despite the error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "detail boom") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the underlying detail error message to be surfaced in a diagnostic, got: %v", resp.Diagnostics.Errors())
	}
	for _, uuid := range fake.detailUUIDsCalled {
		if uuid == "dep-2" {
			t.Fatal("expected processing to stop immediately after dep-1's detail error — dep-2's detail lookup must never run, but it did")
		}
	}
}

// TestUpdateDeploymentsDataSource_Read_EmptyOwnerDetailIsWarnAndSkip proves
// condition (b) against what the pinned SDK ACTUALLY returns for a 204: a
// NON-NIL *sdk.DeviceUpdateDeploymentV1Model whose fields are all at their
// zero value (handleResponse skips decoding an empty body but still returns
// &response, never nil) — so OrganizationGroupUUID == "" is the real-world
// signal, not a nil pointer. Gate-found (BLOCKING B1): the original version
// of this test used a nil fixture, which a nil-only check in the production
// code would pass while leaving the true 204 case completely uncovered — an
// empty-owner deployment would flow through as "owned by ()" and
// --include-inherited would wrongly import an unknown-owner item. Deployment dep-1 must
// be SKIPPED from the output with a WARNING diagnostic (not an error), while
// dep-2 (a successful detail lookup) still appears. Confirmed against a
// scratch-copy mutation that drops the AddWarning call (silently `continue`s
// with no diagnostic recorded): with that mutation the
// len(resp.Diagnostics.Warnings()) assertion below fails (0 warnings instead
// of 1). Also confirmed against a scratch-copy mutation reverting the
// `detail.OrganizationGroupUUID == ""` check back to nil-only: with that
// mutation dep-1 wrongly survives with an empty owner.
func TestUpdateDeploymentsDataSource_Read_EmptyOwnerDetailIsWarnAndSkip(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}, {UUID: "dep-2", Name: "Broad"}},
		},
		detailsByDeploymentUUID: map[string]*sdk.DeviceUpdateDeploymentV1Model{
			// dep-1's detail is a real 204 simulation: non-nil, every field zero.
			"dep-1": {},
			"dep-2": {UUID: "dep-2", OrganizationGroupUUID: "og-owner-2"},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a 204/empty-owner detail response, got: %v", resp.Diagnostics.Errors())
	}
	if len(resp.Diagnostics.Warnings()) == 0 {
		t.Fatal("expected a warning diagnostic recording that dep-1 was skipped due to an empty-owner detail response, got none")
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected exactly 1 deployment (dep-1 skipped, dep-2 kept), got %d: %+v", len(out.UpdateDeployments), out.UpdateDeployments)
	}
	if out.UpdateDeployments[0].UUID.ValueString() != "dep-2" {
		t.Fatalf("expected the surviving deployment to be dep-2, got %q", out.UpdateDeployments[0].UUID.ValueString())
	}
}

// TestUpdateDeploymentsDataSource_Read_TrueNilDeploymentDetailIsWarnAndSkip
// keeps the true-nil-pointer case covered too, even though the pinned SDK
// never actually produces one (see the comment above) — the production
// check is `detail == nil || detail.OrganizationGroupUUID == ""`, and a
// short-circuited `||` means a future SDK bump that DID start returning nil
// must still be provably handled.
func TestUpdateDeploymentsDataSource_Read_TrueNilDeploymentDetailIsWarnAndSkip(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}, {UUID: "dep-2", Name: "Broad"}},
		},
		detailsByDeploymentUUID: map[string]*sdk.DeviceUpdateDeploymentV1Model{
			"dep-1": nil,
			"dep-2": {UUID: "dep-2", OrganizationGroupUUID: "og-owner-2"},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a nil detail response, got: %v", resp.Diagnostics.Errors())
	}
	if len(resp.Diagnostics.Warnings()) == 0 {
		t.Fatal("expected a warning diagnostic recording that dep-1 was skipped due to a nil detail response, got none")
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected exactly 1 deployment (dep-1 skipped, dep-2 kept), got %d: %+v", len(out.UpdateDeployments), out.UpdateDeployments)
	}
	if out.UpdateDeployments[0].UUID.ValueString() != "dep-2" {
		t.Fatalf("expected the surviving deployment to be dep-2, got %q", out.UpdateDeployments[0].UUID.ValueString())
	}
}

// TestUpdateDeploymentsDataSource_Read_MapsOrganizationGroupUuidFromDetail
// proves the field-mapping requirement: a deployment whose detail lookup
// returns a known OrganizationGroupUUID must have that value decode onto
// DeploymentSummary.OrganizationGroupUuid in the final state.
func TestUpdateDeploymentsDataSource_Read_MapsOrganizationGroupUuidFromDetail(t *testing.T) {
	t.Parallel()

	const wantOGUUID = "eec458d3-722f-678a-52b9-22398b02009e"

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: makeCatalogPage("u", 1),
		},
		catalogTotal: 1,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-0": {{UUID: "dep-1", Name: "Pilot"}},
		},
		detailsByDeploymentUUID: map[string]*sdk.DeviceUpdateDeploymentV1Model{
			"dep-1": {UUID: "dep-1", OrganizationGroupUUID: wantOGUUID},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.UpdateDeployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.UpdateDeployments))
	}
	if out.UpdateDeployments[0].OrganizationGroupUuid.ValueString() != wantOGUUID {
		t.Errorf("expected organization_group_uuid %q, got %q", wantOGUUID, out.UpdateDeployments[0].OrganizationGroupUuid.ValueString())
	}
}

// --- Dedup catalog update UUIDs across pages. ---

// TestUpdateDeploymentsDataSource_Read_DedupsUpdateUUIDsAcrossPages proves
// the gate-found dedup fix: if the same update UUID comes back on more than
// one catalog page (server ignoring Page, or the catalog shifting between
// page fetches), GetDeploymentsByDeviceUpdate must only be called once for
// it, and its deployments must appear exactly once in the final flattened
// list — not duplicated. Against the pre-fix code (`updateUUIDs` appended to
// directly with no dedup), this fails: u-2 appears on both pages, so it's
// looked up twice and its single deployment shows up twice in the output.
func TestUpdateDeploymentsDataSource_Read_DedupsUpdateUUIDsAcrossPages(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: {{UUID: "u-1"}, {UUID: "u-2"}},
			2: {{UUID: "u-2"}, {UUID: "u-3"}},
		},
		// B13: pagewalk.WalkTwiceVerified's Total is the TRUE distinct
		// count (3: u-1, u-2, u-3), not the raw/non-deduped item count
		// across pages (4) the old hand-rolled loop tolerated.
		catalogTotal: 3,
		deploymentsByUpdate: map[string][]sdk.DeploymentV1Model{
			"u-1": {{UUID: "dep-1", Name: "One"}},
			"u-2": {{UUID: "dep-2", Name: "Two"}},
			"u-3": {{UUID: "dep-3", Name: "Three"}},
		},
	}
	ds := &updateDataSource{search: fake}

	out, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}

	u2Calls := 0
	for _, uuid := range fake.deploymentUUIDsCalled {
		if uuid == "u-2" {
			u2Calls++
		}
	}
	if u2Calls != 1 {
		t.Fatalf("expected GetDeploymentsByDeviceUpdate to be called exactly once for u-2 (it appears on both pages), got %d calls: %v", u2Calls, fake.deploymentUUIDsCalled)
	}

	u2DeploymentCount := 0
	for _, d := range out.UpdateDeployments {
		if d.UUID.ValueString() == "dep-2" {
			u2DeploymentCount++
		}
	}
	if u2DeploymentCount != 1 {
		t.Fatalf("expected dep-2 (u-2's deployment) to appear exactly once in the flattened result, got %d: %+v", u2DeploymentCount, out.UpdateDeployments)
	}

	if len(out.UpdateDeployments) != 3 {
		t.Fatalf("expected 3 flattened deployments (one per distinct update: u-1, u-2, u-3), got %d: %+v", len(out.UpdateDeployments), out.UpdateDeployments)
	}
}

// --- Strict total check (B13). ---

// TestUpdateDeploymentsDataSource_Read_MoreDistinctItemsThanTotalErrors pins
// the "strict total check" B13 explicitly asked for: pagewalk.WalkTwiceVerified
// errors the instant a page's DISTINCT item count exceeds the server's own
// reported Total, rather than trusting Total only as a "when to stop"
// threshold (the old hand-rolled loop only ever compared
// len(updateUUIDs) >= Total to decide when to STOP; it never treated "more
// unique items than Total claims" as an anomaly worth reporting on its own).
// Fail-on-revert: replacing pagewalk.WalkTwiceVerified with any loop that
// only checks for reaching Total (never exceeding it) would make this test
// see a successful walk instead of the error below.
func TestUpdateDeploymentsDataSource_Read_MoreDistinctItemsThanTotalErrors(t *testing.T) {
	t.Parallel()

	fake := &fakeUpdatesV1Search{
		catalogPages: map[int][]sdk.DeviceUpdateDetailsDeploymentsV1Model{
			1: {{UUID: "u-1"}, {UUID: "u-2"}, {UUID: "u-3"}},
		},
		catalogTotal: 2, // claims fewer distinct items than page 1 actually has
	}
	ds := &updateDataSource{search: fake}

	_, resp := runUpdateDSRead(t, ds, "og-uuid-1")
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when distinct items exceed the reported total")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "distinct items") && strings.Contains(d.Detail(), "total is 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a 'distinct items ... total is 2' error, got: %v", resp.Diagnostics.Errors())
	}
	if len(fake.deploymentUUIDsCalled) != 0 {
		t.Fatalf("expected zero GetDeploymentsByDeviceUpdate calls when the catalog walk itself errors, got %d", len(fake.deploymentUUIDsCalled))
	}
}
