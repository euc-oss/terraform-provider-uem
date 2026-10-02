package datasource

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeMacApplicationSearch is a test-only fake satisfying macApplicationSearchIface
// (the List-level seam the generated macApplicationDataSource depends on). Distinct
// from fakeAppsV2Search (data_source_test.go), which fakes the lower-level SDK
// appsV2SearchAPI surface for the hand-written uem_applications data source.
type fakeMacApplicationSearch struct {
	items []MacApplicationSummary
	err   error

	lastFilters macApplicationFilters
}

func (f *fakeMacApplicationSearch) List(_ context.Context, filters macApplicationFilters) ([]MacApplicationSummary, error) {
	f.lastFilters = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

// --- helpers ---

func getMacApplicationDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &macApplicationDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createMacApplicationDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getMacApplicationDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptyMacApplicationDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getMacApplicationDSSchema(t)
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

func macApplicationNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getMacApplicationDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["mac_applications"].(tftypes.List)
	if !ok {
		t.Fatalf("expected mac_applications to be tftypes.List, got %T", objType.AttributeTypes["mac_applications"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Schema (zero-filter shape) ---

// TestMacApplicationsDataSource_Schema_OnlyOrganizationGroupUuidFilter confirms the
// composition-map's single `organization_group_uuid` filter entry produces a schema with
// exactly the computed "mac_applications" list attribute plus the one OPTIONAL top-level
// filter attribute -- and that the filter is Optional, not Required, since unset callers
// must see zero behavior change.
func TestMacApplicationsDataSource_Schema_OnlyOrganizationGroupUuidFilter(t *testing.T) {
	t.Parallel()
	resp := getMacApplicationDSSchema(t)
	if len(resp.Schema.Attributes) != 2 {
		t.Fatalf("expected exactly 2 top-level attributes (mac_applications, organization_group_uuid), got %d: %v", len(resp.Schema.Attributes), resp.Schema.Attributes)
	}
	if _, ok := resp.Schema.Attributes["mac_applications"]; !ok {
		t.Fatalf("expected a mac_applications attribute, got %v", resp.Schema.Attributes)
	}
	attr, ok := resp.Schema.Attributes["organization_group_uuid"]
	if !ok {
		t.Fatalf("expected an organization_group_uuid attribute, got %v", resp.Schema.Attributes)
	}
	if attr.IsRequired() {
		t.Errorf("expected organization_group_uuid to be Optional, got Required")
	}
	if !attr.IsOptional() {
		t.Errorf("expected organization_group_uuid to be Optional")
	}
}

// --- Metadata ---

func TestMacApplicationsDataSource_Metadata(t *testing.T) {
	t.Parallel()
	ds := &macApplicationDataSource{}
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_mac_applications" {
		t.Errorf("expected TypeName 'uem_mac_applications', got '%s'", resp.TypeName)
	}
}

// --- Read (generated data source, via the List-level fake) ---

func TestGeneratedMacApplicationsDataSource_Read(t *testing.T) {
	t.Parallel()

	fake := &fakeMacApplicationSearch{
		items: []MacApplicationSummary{
			{
				Id:                    types.Int64Value(1),
				Name:                  types.StringValue("Xcode"),
				Type:                  types.StringValue("Internal"),
				BundleId:              types.StringValue("com.apple.dt.Xcode"),
				Uuid:                  types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
				OrganizationGroupUuid: types.StringValue("05d17100-b346-c29d-6760-a0fdedcf8623"),
			},
		},
	}
	ds := &macApplicationDataSource{search: fake}
	ctx := context.Background()

	config := createMacApplicationDSConfig(t, map[string]tftypes.Value{
		"mac_applications":        tftypes.NewValue(tftypes.List{ElementType: macApplicationNestedType(t)}, nil),
		"organization_group_uuid": tftypes.NewValue(tftypes.String, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyMacApplicationDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var out MacApplicationDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 mac application, got %d", len(out.Items))
	}
	if out.Items[0].Name.ValueString() != "Xcode" {
		t.Errorf("expected name 'Xcode', got %q", out.Items[0].Name.ValueString())
	}
	if out.Items[0].Type.ValueString() != "Internal" {
		t.Errorf("expected type 'Internal', got %q", out.Items[0].Type.ValueString())
	}
	if out.Items[0].BundleId.ValueString() != "com.apple.dt.Xcode" {
		t.Errorf("expected bundle_id 'com.apple.dt.Xcode', got %q", out.Items[0].BundleId.ValueString())
	}
	if out.Items[0].Id.ValueInt64() != 1 {
		t.Errorf("expected id 1, got %d", out.Items[0].Id.ValueInt64())
	}
	if out.Items[0].Uuid.ValueString() != "bafde89c-041e-1756-082b-933aaf16cad8" {
		t.Errorf("expected uuid 'bafde89c-041e-1756-082b-933aaf16cad8', got %q", out.Items[0].Uuid.ValueString())
	}
	if out.Items[0].OrganizationGroupUuid.ValueString() != "05d17100-b346-c29d-6760-a0fdedcf8623" {
		t.Errorf("expected organization_group_uuid '05d17100-b346-c29d-6760-a0fdedcf8623', got %q", out.Items[0].OrganizationGroupUuid.ValueString())
	}
}

func TestGeneratedMacApplicationsDataSource_Read_SearchError(t *testing.T) {
	t.Parallel()

	fake := &fakeMacApplicationSearch{err: context.DeadlineExceeded}
	ds := &macApplicationDataSource{search: fake}
	ctx := context.Background()

	config := createMacApplicationDSConfig(t, map[string]tftypes.Value{
		"mac_applications":        tftypes.NewValue(tftypes.List{ElementType: macApplicationNestedType(t)}, nil),
		"organization_group_uuid": tftypes.NewValue(tftypes.String, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyMacApplicationDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error when search fails")
	}
}

// --- Companion (macApplicationSearch.List) unit tests, driven against the SDK-level
// appsV2SearchAPI fake already declared in data_source_test.go (fakeAppsV2Search),
// reused here rather than redeclared. ---

// TestMacApplicationSearch_List_AppliesNoAppFamilyFilter proves the companion's List calls
// Search with neither ApplicationType nor ApplicationSource set on the SDK request options
// -- the documented, honest current behavior (see the KNOWN LIMITATION doc comment on List
// in mac_applications_data_source_hooks.go) rather than a silently-applied, unconfirmed
// guess. Per the composition-map's deliberately empty `filters` and plan Deviation D4.
func TestMacApplicationSearch_List_AppliesNoAppFamilyFilter(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	s := &macApplicationSearch{svc: fake}

	_, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastOpts == nil {
		t.Fatal("expected Search to be called with options")
	}
	if fake.lastOpts.ApplicationType != nil {
		t.Errorf("expected ApplicationType to be left unset, got %v", *fake.lastOpts.ApplicationType)
	}
	if fake.lastOpts.ApplicationSource != nil {
		t.Errorf("expected ApplicationSource to be left unset, got %v", *fake.lastOpts.ApplicationSource)
	}
}

// TestMacApplicationSearch_List_MapsResults proves the companion maps SDK results into
// MacApplicationSummary, nil-guarding the pointer ID field.
//
// Both fixture items set UUID (live-confirmed, internal-ticket release blocker B12: AppsV2.Search
// returns id=null on every item on both as<internal-env> and paul-2609, but uuid is always populated),
// since UUID is now the walk's dedupe key (see applications_pagewalk.go) -- an application
// with no uuid can no longer be paged/deduped safely and is a List error instead (see
// TestMacApplicationSearch_List_EmptyUuidIsError below), not a value silently carried through
// as an empty string the way it was before pagination existed.
func TestMacApplicationSearch_List_MapsResults(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{
		result: &sdk.ApplicationSearchV2Model{
			Total: intPtr(2),
			Applications: []sdk.ApplicationV2Model{
				{
					ID:                    intPtr(7),
					ApplicationName:       "Xcode",
					ApplicationType:       "Internal",
					BundleID:              "com.apple.dt.Xcode",
					UUID:                  "bafde89c-041e-1756-082b-933aaf16cad8",
					OrganizationGroupUUID: "05d17100-b346-c29d-6760-a0fdedcf8623",
				},
				{
					// ID deliberately nil -- must not panic, must map to a null Int64.
					ApplicationName: "Internal Tool",
					ApplicationType: "Internal",
					BundleID:        "com.example.internaltool",
					UUID:            "16fea16d-024d-1e8b-e41d-b5981759f00d",
				},
			},
		},
	}
	s := &macApplicationSearch{svc: fake}

	items, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Id.ValueInt64() != 7 {
		t.Errorf("expected first item id 7, got %d", items[0].Id.ValueInt64())
	}
	if items[0].Uuid.ValueString() != "bafde89c-041e-1756-082b-933aaf16cad8" {
		t.Errorf("expected first item uuid 'bafde89c-041e-1756-082b-933aaf16cad8', got %q", items[0].Uuid.ValueString())
	}
	if items[0].OrganizationGroupUuid.ValueString() != "05d17100-b346-c29d-6760-a0fdedcf8623" {
		t.Errorf("expected first item organization_group_uuid '05d17100-b346-c29d-6760-a0fdedcf8623', got %q", items[0].OrganizationGroupUuid.ValueString())
	}
	if items[1].Uuid.ValueString() != "16fea16d-024d-1e8b-e41d-b5981759f00d" {
		t.Errorf("expected second item uuid '16fea16d-024d-1e8b-e41d-b5981759f00d', got %q", items[1].Uuid.ValueString())
	}
	if !items[1].Id.IsNull() {
		t.Errorf("expected second item id to be null (nil SDK pointer), got %v", items[1].Id)
	}
	if items[1].Name.ValueString() != "Internal Tool" {
		t.Errorf("expected second item name 'Internal Tool', got %q", items[1].Name.ValueString())
	}
}

// TestMacApplicationSearch_List_EmptyUuidIsError proves that an application
// with no uuid -- something live evidence (internal-ticket release blocker B12)
// shows AppsV2.Search never actually sends, but a fixture could still model --
// is a List error, not a silently-mapped empty string: without a uuid the walk
// can neither dedupe it nor safely page past it.
func TestMacApplicationSearch_List_EmptyUuidIsError(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{
		result: &sdk.ApplicationSearchV2Model{
			Total: intPtr(1),
			Applications: []sdk.ApplicationV2Model{
				{ApplicationName: "No UUID App"},
			},
		},
	}
	s := &macApplicationSearch{svc: fake}

	_, err := s.List(context.Background(), macApplicationFilters{})
	if err == nil {
		t.Fatal("expected an error for an application with no uuid")
	}
}

// TestMacApplicationSearch_List_PropagatesSearchError proves the companion surfaces a
// Search error rather than swallowing it.
func TestMacApplicationSearch_List_PropagatesSearchError(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{err: context.DeadlineExceeded}
	s := &macApplicationSearch{svc: fake}

	_, err := s.List(context.Background(), macApplicationFilters{})
	if err == nil {
		t.Fatal("expected error to propagate from Search")
	}
}
