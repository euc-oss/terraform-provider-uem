package profile

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeProfilesV2Search is a test-only fake satisfying profilesV2SearchAPI.
type fakeProfilesV2Search struct {
	result *sdk.ProfileSearchResultV2Entity
	err    error

	lastOpts *sdk.ProfilesV2SearchProfilesOptions
}

func (f *fakeProfilesV2Search) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	f.lastOpts = opts
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func intPtr(i int) *int { return &i }

// --- helpers ---

func getProfilesDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &ProfilesDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createProfilesDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getProfilesDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptyProfilesDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getProfilesDSSchema(t)
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

func profilesNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getProfilesDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["profiles"].(tftypes.List)
	if !ok {
		t.Fatalf("expected profiles to be tftypes.List, got %T", objType.AttributeTypes["profiles"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Metadata ---

func TestProfilesDataSource_Metadata(t *testing.T) {
	t.Parallel()
	ds := &ProfilesDataSource{}
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_profiles" {
		t.Errorf("expected TypeName 'uem_profiles', got '%s'", resp.TypeName)
	}
}

// --- Read ---

func TestProfilesDataSource_Read_PopulatesFromSearch(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{
		result: &sdk.ProfileSearchResultV2Entity{
			TotalResults: intPtr(2),
			ProfileList: []sdk.ProfileDetailsV2Entity{
				{
					ProfileID:             intPtr(101),
					ProfileName:           "Corporate WiFi",
					Platform:              "Android",
					OrganizationGroupUUID: "uuid-1",
				},
				{
					ProfileID:             intPtr(102),
					ProfileName:           "Passcode Policy",
					Platform:              "AppleIos",
					OrganizationGroupUUID: "uuid-2",
				},
			},
		},
	}

	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var out ProfilesDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(out.Profiles))
	}

	if out.Profiles[0].ProfileID.ValueInt64() != 101 {
		t.Errorf("expected first profile ID 101, got %d", out.Profiles[0].ProfileID.ValueInt64())
	}
	if out.Profiles[0].Name.ValueString() != "Corporate WiFi" {
		t.Errorf("expected first profile name 'Corporate WiFi', got %q", out.Profiles[0].Name.ValueString())
	}
	if out.Profiles[0].Platform.ValueString() != "Android" {
		t.Errorf("expected first profile platform 'Android', got %q", out.Profiles[0].Platform.ValueString())
	}
	if out.Profiles[0].OrganizationGroupUUID.ValueString() != "uuid-1" {
		t.Errorf("expected first profile org group uuid 'uuid-1', got %q", out.Profiles[0].OrganizationGroupUUID.ValueString())
	}

	if out.Profiles[1].ProfileID.ValueInt64() != 102 {
		t.Errorf("expected second profile ID 102, got %d", out.Profiles[1].ProfileID.ValueInt64())
	}
	if out.Profiles[1].Name.ValueString() != "Passcode Policy" {
		t.Errorf("expected second profile name 'Passcode Policy', got %q", out.Profiles[1].Name.ValueString())
	}
}

// TestProfilesDataSource_Read_NormalizesShortFormPlatform reproduces
// internal-ticket: the /api/mdm/profiles/search v2 endpoint returns platform in
// SHORT-FORM (e.g. "Apple", "WinRT"), but ImportState validates against the
// LONG-FORM SupportedImportPlatforms vocab (internal/profile/platform/platform.go).
// The data source must normalize short-form API values to long-form so a
// value emitted here can be fed straight into `terraform import` without
// tripping IsValidImportPlatform.
func TestProfilesDataSource_Read_NormalizesShortFormPlatform(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{
		result: &sdk.ProfileSearchResultV2Entity{
			TotalResults: intPtr(2),
			ProfileList: []sdk.ProfileDetailsV2Entity{
				{
					ProfileID:             intPtr(201),
					ProfileName:           "iOS Restrictions",
					Platform:              "Apple", // short-form, per profiles_search_v2.json fixture
					OrganizationGroupUUID: "uuid-3",
				},
				{
					ProfileID:             intPtr(202),
					ProfileName:           "WinRT Wifi",
					Platform:              "WinRT", // short-form, per profiles_search_v2.json fixture
					OrganizationGroupUUID: "uuid-4",
				},
			},
		},
	}

	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var out ProfilesDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(out.Profiles))
	}

	if got := out.Profiles[0].Platform.ValueString(); got != "Apple iOS" {
		t.Errorf("expected short-form 'Apple' to be normalized to long-form 'Apple iOS', got %q", got)
	}
	if got := out.Profiles[1].Platform.ValueString(); got != "Windows 10" {
		t.Errorf("expected short-form 'WinRT' to be normalized to long-form 'Windows 10', got %q", got)
	}
}

func TestProfilesDataSource_Read_SearchError(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{err: context.DeadlineExceeded}
	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error when search fails")
	}
}

func TestProfilesDataSource_Read_FiltersPassedToSearch(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{result: &sdk.ProfileSearchResultV2Entity{TotalResults: intPtr(0)}}
	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, "MyProfile"),
		"organization_group_id": tftypes.NewValue(tftypes.String, "42"),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	if fake.lastOpts == nil {
		t.Fatal("expected SearchProfiles to be called with options")
	}
	if fake.lastOpts.SearchText == nil || *fake.lastOpts.SearchText != "MyProfile" {
		t.Errorf("expected SearchText 'MyProfile', got %v", fake.lastOpts.SearchText)
	}
	if fake.lastOpts.OrganizationGroupID == nil || *fake.lastOpts.OrganizationGroupID != 42 {
		t.Errorf("expected OrganizationGroupID 42, got %v", fake.lastOpts.OrganizationGroupID)
	}
}

// stateProfilesIsNull inspects the RAW state value (not the decoded Go
// struct) for the profiles attribute and reports whether it is NULL, as
// opposed to a known empty list. Decoding straight into
// ProfilesDataSourceModel would not reliably distinguish these two cases
// (both can decode to a nil/zero-length Go slice), but Terraform itself
// does: a null list attribute is what causes `terraform output` to drop the
// attribute entirely on a zero-result run, while a known empty list ([]) is
// what a clean "no profiles found" exit needs. See
// internal/updates/update_deployments_data_source_test.go's
// stateUpdateDeploymentsIsNull for the pattern this mirrors.
func stateProfilesIsNull(t *testing.T, state tfsdk.State) bool {
	t.Helper()
	var obj map[string]tftypes.Value
	if err := state.Raw.As(&obj); err != nil {
		t.Fatalf("decode state as object: %v", err)
	}
	listVal, ok := obj["profiles"]
	if !ok {
		t.Fatal("state has no profiles attribute at all")
	}
	return listVal.IsNull()
}

// TestProfilesDataSource_Read_EmptyResultEmitsEmptyNotNullList proves the
// nil-vs-empty-slice fix for this data source: a zero-result search must
// leave the profiles attribute as a known EMPTY list in state, not NULL.
// Against the pre-fix `var profiles []ProfileSummary`, this fails: an empty
// search result never appends anything, so profiles stays nil and the state
// attribute is null instead of a known empty list.
func TestProfilesDataSource_Read_EmptyResultEmitsEmptyNotNullList(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{result: &sdk.ProfileSearchResultV2Entity{TotalResults: intPtr(0)}}
	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}
	if stateProfilesIsNull(t, readResp.State) {
		t.Fatal("expected profiles to be a known EMPTY list on a zero-result search, got NULL")
	}

	var out ProfilesDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}
	if len(out.Profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(out.Profiles))
	}
}

// --- Pagination (internal-ticket, release blocker B1): SearchProfiles paginates via
// Page/PageSize + ProfileList/TotalResults, no cursor, and the page-index
// BASE differs by UEM version (0-indexed on a 26.9 tenant, 1-indexed on a
// 26.2 tenant -- see data_source.go's probeProfilePageBase doc comment).
// Mirrors internal/organizationgroup and internal/smartgroup's hardened
// search-walk test shape. ---

// profilePage is one page's canned response for pagedProfilesFake: a
// ProfileList and an optional TotalResults (nil unless explicitly set),
// matching the live shape where only a page that actually holds items
// reports a trustworthy total.
type profilePage struct {
	list  []sdk.ProfileDetailsV2Entity
	total *int
}

func page(list []sdk.ProfileDetailsV2Entity, total int) profilePage {
	return profilePage{list: list, total: &total}
}

// pagedProfilesFake is a test-only fake satisfying profilesV2SearchAPI whose
// SearchProfiles response depends on the requested page (keyed by the
// ACTUAL page index the fixture's base uses -- 0 or 1), so multi-page and
// page-base-probing behavior can be tested in isolation from the
// single-shot fakeProfilesV2Search above. A page with no entry in pages
// returns an empty body with no TotalResults -- the live-verified
// out-of-range/HTTP-204 shape -- never an error.
type pagedProfilesFake struct {
	pages map[int]profilePage
	err   error

	pagesRequested []int
}

func (f *pagedProfilesFake) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	if opts.Page == nil {
		return nil, nil, fmt.Errorf("test fake: SearchProfiles called without a Page option set -- the fix must always set one explicitly")
	}
	p := *opts.Page
	f.pagesRequested = append(f.pagesRequested, p)

	resp, ok := f.pages[p]
	if !ok {
		return nil, &sdk.ProfileSearchResultV2Entity{}, nil
	}
	result := &sdk.ProfileSearchResultV2Entity{ProfileList: resp.list}
	if resp.total != nil {
		result.TotalResults = resp.total
	}
	return nil, result, nil
}

func makeProfilePage(prefix string, ids []int) []sdk.ProfileDetailsV2Entity {
	out := make([]sdk.ProfileDetailsV2Entity, len(ids))
	for i, id := range ids {
		out[i] = sdk.ProfileDetailsV2Entity{
			ProfileID:             &id,
			ProfileName:           fmt.Sprintf("%s-%d", prefix, id),
			Platform:              "Apple",
			OrganizationGroupUUID: "uuid-1",
		}
	}
	return out
}

func idRange(start, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = start + i
	}
	return out
}

func readProfiles(t *testing.T, ds *ProfilesDataSource) (*datasource.ReadResponse, ProfilesDataSourceModel) {
	t.Helper()
	ctx := context.Background()
	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})
	readReq := datasource.ReadRequest{Config: config}
	readResp := &datasource.ReadResponse{State: emptyProfilesDSState(t)}
	ds.Read(ctx, readReq, readResp)
	var out ProfilesDataSourceModel
	if !readResp.Diagnostics.HasError() {
		if diags := readResp.State.Get(ctx, &out); diags.HasError() {
			t.Fatalf("unexpected error reading state: %v", diags.Errors())
		}
	}
	return readResp, out
}

// TestProfilesDataSource_Read_ZeroBasedMultiPage proves a 3-page, 0-INDEXED
// result (live-confirmed shape on tenant paul-2609, UEM 26.9) accumulates
// correctly across all 3 pages, probing page 0 first and never touching page
// 1 as a probe (page 0 already has items). Against an implementation
// hard-coded to start at page 1, this either errors out (page 3 -- the real
// end -- is never requested, so the walk runs out of promised profiles) or
// silently returns fewer than TotalResults; either way the assertions below
// fail.
func TestProfilesDataSource_Read_ZeroBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = 1200
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: page(makeProfilePage("p0", idRange(1, 500)), total),
			1: page(makeProfilePage("p1", idRange(501, 500)), total),
			2: page(makeProfilePage("p2", idRange(1001, 200)), total), // short: 200 < pageSize(500)
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, out := readProfiles(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Profiles) != total {
		t.Fatalf("expected %d accumulated profiles, got %d", total, len(out.Profiles))
	}
	if fake.pagesRequested[0] != 0 {
		t.Fatalf("expected the probe to request page 0 first, got: %v", fake.pagesRequested)
	}
}

// TestProfilesDataSource_Read_OneBasedMultiPage proves the SAME 3-page result
// walks correctly when 1-INDEXED (live-confirmed shape on tenant as<internal-env>, UEM
// 26.2): page 0 is probed first, comes back empty (no entry in the fixture),
// so page 1 is probed next and detected as the base.
func TestProfilesDataSource_Read_OneBasedMultiPage(t *testing.T) {
	t.Parallel()

	const total = 1200
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			// Deliberately no entry for page 0: probing it must come back
			// empty, forcing the probe to fall through to page 1.
			1: page(makeProfilePage("p1", idRange(1, 500)), total),
			2: page(makeProfilePage("p2", idRange(501, 500)), total),
			3: page(makeProfilePage("p3", idRange(1001, 200)), total), // short
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, out := readProfiles(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Profiles) != total {
		t.Fatalf("expected %d accumulated profiles, got %d", total, len(out.Profiles))
	}
	if len(fake.pagesRequested) < 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected the probe to try page 0 then page 1, got: %v", fake.pagesRequested)
	}
}

// TestProfilesDataSource_Read_ZeroBasedPageOneReturns204 proves the exact
// live shape behind release blocker B1's diagnosis: a 0-based tenant whose profile
// count is pageSize-aligned (500 profiles at pageSize 500) returns a FULL
// page 0 that reaches TotalResults exactly, and page 1 -- the confirming page
// the hardened walk always fetches after a full page reaches total -- comes
// back as an HTTP-204-shaped empty body (no ProfileList, no TotalResults),
// live-confirmed on tenant paul-2609. This must be treated as the
// end-of-results confirmation, not an error and not "zero results" (page 0
// already proved profiles exist).
func TestProfilesDataSource_Read_ZeroBasedPageOneReturns204(t *testing.T) {
	t.Parallel()

	const total = 500
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: page(makeProfilePage("p0", idRange(1, 500)), total),
			// Deliberately no entry for page 1: the confirming fetch comes
			// back as an empty 204-shaped body.
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, out := readProfiles(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Profiles) != total {
		t.Fatalf("expected %d profiles, got %d", total, len(out.Profiles))
	}
	// pages=2 (page 0 + the confirming page 1) is a multi-page result, so
	// listAllProfiles walks it a SECOND, fully independent time (this fake
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

// TestProfilesDataSource_Read_EmptyIsZeroResults proves that when BOTH the
// page-0 and page-1 probes come back empty with no TotalResults anywhere,
// the search is confirmed to have zero results -- not an error -- matching
// the gate's re-ruling that the API never sends a non-zero/absent-total
// empty body for a real result.
func TestProfilesDataSource_Read_EmptyIsZeroResults(t *testing.T) {
	t.Parallel()

	fake := &pagedProfilesFake{pages: map[int]profilePage{}}
	ds := &ProfilesDataSource{search: fake}
	resp, out := readProfiles(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected NO error for a confirmed-empty search, got: %v", resp.Diagnostics.Errors())
	}
	if len(out.Profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(out.Profiles))
	}
	if len(fake.pagesRequested) != 2 || fake.pagesRequested[0] != 0 || fake.pagesRequested[1] != 1 {
		t.Fatalf("expected both page 0 and page 1 to be probed before confirming zero, got: %v", fake.pagesRequested)
	}
}

// TestProfilesDataSource_Read_NonZeroTotalNoItemsIsError proves the other
// half of the zero-results ruling: a non-zero TotalResults reported on an
// otherwise-empty probe, with no items on EITHER page 0 or page 1, is a
// silent-partial-result condition and must error rather than being treated
// as zero.
func TestProfilesDataSource_Read_NonZeroTotalNoItemsIsError(t *testing.T) {
	t.Parallel()

	five := 5
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: {list: nil, total: &five},
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: TotalResults=5 reported but no profiles on page 0 or page 1")
	}
}

// TestProfilesDataSource_Read_TotalMismatchIsError proves that a
// TotalResults which changes between two non-empty pages of the SAME walk is
// an error, never silently trusted from whichever page reported it last.
func TestProfilesDataSource_Read_TotalMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: page(makeProfilePage("p0", idRange(1, 5)), 10),
			1: page(makeProfilePage("p1", idRange(6, 5)), 20), // Total changed from 10 to 20
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: TotalResults changed between page 0 (10) and page 1 (20)")
	}
}

// twiceWalkMismatchFake simulates a profile list that changes between the
// first and second (confirming) independent walk of a multi-page result: a
// new walk always begins by probing page 0, so this fake tracks how many
// times page 0 has been requested and serves a DIFFERENT (self-consistent)
// 3-page dataset on the second walk than the first, proving listAllProfiles
// catches the disagreement rather than trusting either walk.
type twiceWalkMismatchFake struct {
	walk           int
	pagesRequested []int
}

func (f *twiceWalkMismatchFake) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	if opts.Page == nil {
		return nil, nil, fmt.Errorf("test fake: SearchProfiles called without a Page option set")
	}
	p := *opts.Page
	if p == 0 {
		f.walk++
	}
	f.pagesRequested = append(f.pagesRequested, p)

	// walk 1: total 1200 (500/500/200). walk 2 (and beyond, defensively):
	// total 1150 (500/500/150) -- both self-consistent, but disagreeing with
	// each other.
	var total, n int
	switch {
	case f.walk <= 1:
		total = 1200
		switch p {
		case 0, 1:
			n = 500
		case 2:
			n = 200
		}
	default:
		total = 1150
		switch p {
		case 0, 1:
			n = 500
		case 2:
			n = 150
		}
	}
	if p > 2 {
		return nil, &sdk.ProfileSearchResultV2Entity{}, nil
	}
	ids := idRange(p*10000+1, n)
	return nil, &sdk.ProfileSearchResultV2Entity{
		ProfileList:  makeProfilePage(fmt.Sprintf("w%d-p%d", f.walk, p), ids),
		TotalResults: &total,
	}, nil
}

// TestProfilesDataSource_Read_TwiceWalkMismatchIsError proves the twice-walk
// confirmation: a multi-page (>1 page) result is walked a second, fully
// independent time, and a disagreement between the two walks (here: a
// different TotalResults and a different-sized final page) is an error, not
// a silently-trusted first answer.
func TestProfilesDataSource_Read_TwiceWalkMismatchIsError(t *testing.T) {
	t.Parallel()

	fake := &twiceWalkMismatchFake{}
	ds := &ProfilesDataSource{search: fake}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: the second independent walk disagreed with the first")
	}
	if fake.walk < 2 {
		t.Fatalf("expected a second walk to have been attempted (multi-page result), got %d walk(s)", fake.walk)
	}
}

// infinitePagedProfilesFake always returns a FULL (non-short) page with a
// nil TotalResults, forever, for any page requested -- simulating a
// pathological/misbehaving API response that never signals end-of-results.
// Used to prove the maxProfilePages hard-cap guard actually terminates the
// loop with a diagnostic error instead of looping unboundedly. Page 0 is
// given the same shape so the base-probe itself doesn't short-circuit before
// the cap logic is exercised.
type infinitePagedProfilesFake struct {
	pagesRequested []int
}

func (f *infinitePagedProfilesFake) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	page := 0
	if opts.Page != nil {
		page = *opts.Page
	}
	f.pagesRequested = append(f.pagesRequested, page)

	ids := make([]int, profileSearchPageSize)
	base := page * profileSearchPageSize
	for i := range ids {
		ids[i] = base + i + 1
	}
	return nil, &sdk.ProfileSearchResultV2Entity{
		ProfileList: makeProfilePage(fmt.Sprintf("page%d", page), ids),
	}, nil
}

// TestProfilesDataSource_Read_MaxPageCapFailsLoudly proves the hard max-page
// cap must trigger and fail loudly with a clear diagnostic once exceeded,
// rather than looping forever or silently returning a truncated result set.
// Against an implementation with no cap (or a cap that silently `break`s
// instead of erroring), this test's HasError() check fails, or the loop
// would run indefinitely.
//
// This injects a tiny maxPages (via ProfilesDataSource.maxPages) instead of
// relying on the production maxProfilePages=100 default: against
// infinitePagedProfilesFake, which never signals end-of-results, a mutation
// that weakens or removes the cap check makes this test loop far longer
// before Go's own test timeout kicks in. With maxPages=3 the cap fires (or
// fails to) within a handful of iterations, so this test resolves in well
// under a second either way instead of silently absorbing 100+ fake-page
// round trips (or hanging) before the mutant is caught.
func TestProfilesDataSource_Read_MaxPageCapFailsLoudly(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &infinitePagedProfilesFake{}
	ds := &ProfilesDataSource{search: fake, maxPages: testMaxPages}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic error once the injected maxPages cap is exceeded, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+2 {
		t.Fatalf("expected the loop to stop at or just past the injected cap (%d), got %d requests -- it kept looping past the cap", testMaxPages, len(fake.pagesRequested))
	}
	if !stateProfilesIsNull(t, resp.State) {
		t.Fatal("expected state to remain unchanged (null) on the cap error, got a populated state -- that would mean partial data was written despite the error")
	}
}

// TestProfilesDataSource_Read_NoProgressIsError proves the no-progress guard:
// a full page that adds no new (unique) profile id -- the server ignoring
// the page parameter, or repeating a page -- must error rather than loop
// forever or silently stop short.
func TestProfilesDataSource_Read_NoProgressIsError(t *testing.T) {
	t.Parallel()

	const total = 1000
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: page(makeProfilePage("p0", idRange(1, 500)), total),
			1: page(makeProfilePage("p0", idRange(1, 500)), total), // same ids as page 0: no progress
		},
	}
	ds := &ProfilesDataSource{search: fake, maxPages: 5}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error: page 1 repeated page 0's ids and made no progress toward TotalResults")
	}
	// The no-progress guard must fire IMMEDIATELY on page 1 (exactly 2
	// requests: the page-0 probe, then the repeated page 1) rather than
	// continuing on to page 2+ (which has no fixture entry and would
	// eventually trip the unrelated "empty page before total reached"
	// guard instead) -- this is what actually distinguishes the
	// no-progress check from the other termination guards.
	if len(fake.pagesRequested) != 2 {
		t.Fatalf("expected the no-progress guard to stop the walk after exactly 2 requests (page 0, then the repeated page 1), got %d: %v", len(fake.pagesRequested), fake.pagesRequested)
	}
}

// TestProfilesDataSource_Read_DedupsProfileIDsAcrossPages proves the
// dedup-by-profile-id requirement: if the same profile ID comes back on
// more than one page (server overlap/ignoring Page), it must only appear
// once in the final flattened list.
func TestProfilesDataSource_Read_DedupsProfileIDsAcrossPages(t *testing.T) {
	t.Parallel()

	total := 3
	fake := &pagedProfilesFake{
		pages: map[int]profilePage{
			0: page(makeProfilePage("p0", []int{1, 2}), total),
			1: page(makeProfilePage("p1", []int{2, 3}), total), // 2 overlaps with page 0
		},
	}
	ds := &ProfilesDataSource{search: fake}
	resp, out := readProfiles(t, ds)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	seen := map[int64]int{}
	for _, p := range out.Profiles {
		seen[p.ProfileID.ValueInt64()]++
	}
	if seen[2] != 1 {
		t.Fatalf("expected profile ID 2 (overlapping across pages 0 and 1) to appear exactly once, got %d: %+v", seen[2], out.Profiles)
	}
	if len(out.Profiles) != 3 {
		t.Fatalf("expected 3 deduped profiles (IDs 1, 2, 3), got %d: %+v", len(out.Profiles), out.Profiles)
	}
}

func TestProfilesDataSource_Read_InvalidOrganizationGroupID_ReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	fake := &fakeProfilesV2Search{result: &sdk.ProfileSearchResultV2Entity{TotalResults: intPtr(0)}}
	ds := &ProfilesDataSource{search: fake}
	ctx := context.Background()

	config := createProfilesDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, "not-a-number"),
		"profiles":              tftypes.NewValue(tftypes.List{ElementType: profilesNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyProfilesDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic error for a non-numeric organization_group_id")
	}
	if fake.lastOpts != nil {
		t.Fatal("expected SearchProfiles NOT to be called when organization_group_id is invalid")
	}
}

// boundedPagedProfilesFake serves fresh, full 0-indexed pages that all report
// the same TotalResults (lastPage+1 full pages) and an empty body past
// lastPage, so a walk is internally consistent and only the page cap can
// stop it before it completes.
type boundedPagedProfilesFake struct {
	lastPage       int
	pagesRequested []int
}

func (f *boundedPagedProfilesFake) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	p := *opts.Page
	f.pagesRequested = append(f.pagesRequested, p)
	if p > f.lastPage {
		return nil, &sdk.ProfileSearchResultV2Entity{}, nil
	}
	total := (f.lastPage + 1) * profileSearchPageSize
	return nil, &sdk.ProfileSearchResultV2Entity{
		ProfileList:  makeProfilePage("cap", idRange(p*profileSearchPageSize+1, profileSearchPageSize)),
		TotalResults: &total,
	}, nil
}

// TestProfilesDataSource_Read_MaxPageCapStopsConsistentWalk (gate B1): with a
// consistent TotalResults the walk would otherwise succeed after 20 pages, so
// removing the cap makes this read succeed and the test fail.
func TestProfilesDataSource_Read_MaxPageCapStopsConsistentWalk(t *testing.T) {
	t.Parallel()

	const testMaxPages = 3
	fake := &boundedPagedProfilesFake{lastPage: 19}
	ds := &ProfilesDataSource{search: fake, maxPages: testMaxPages}
	resp, _ := readProfiles(t, ds)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the injected maxPages cap to stop a consistent 20-page walk with an error, got none")
	}
	if len(fake.pagesRequested) > testMaxPages+1 {
		t.Fatalf("expected at most %d page requests before the cap fired, got %d: %v", testMaxPages+1, len(fake.pagesRequested), fake.pagesRequested)
	}
}

// nilResultProfilesFake models an HTTP 204 surfaced by the SDK as a nil
// result entity (rather than an empty one) on every page except page 1.
type nilResultProfilesFake struct{}

func (nilResultProfilesFake) SearchProfiles(_ context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error) {
	if *opts.Page != 1 {
		return nil, nil, nil
	}
	total := 2
	return nil, &sdk.ProfileSearchResultV2Entity{ProfileList: makeProfilePage("n", []int{7, 8}), TotalResults: &total}, nil
}

// TestProfilesDataSource_Read_NilResultIs204Shape (gate B1): a 1-indexed
// tenant (live as<internal-env>, UEM 26.2: page 0 is HTTP 204) whose 204 arrives as a
// nil entity is still detected as base 1 and read in full.
func TestProfilesDataSource_Read_NilResultIs204Shape(t *testing.T) {
	t.Parallel()

	resp, out := readProfiles(t, &ProfilesDataSource{search: nilResultProfilesFake{}})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics.Errors())
	}
	if len(out.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(out.Profiles))
	}
}
