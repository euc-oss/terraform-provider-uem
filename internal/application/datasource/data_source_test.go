package datasource

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeAppsV2Search is a test-only fake satisfying appsV2SearchAPI.
type fakeAppsV2Search struct {
	result *sdk.ApplicationSearchV2Model
	err    error

	lastOpts *sdk.AppsV2SearchOptions
}

func (f *fakeAppsV2Search) Search(_ context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error) {
	f.lastOpts = opts
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func intPtr(i int) *int { return &i }

// --- helpers ---

func getApplicationsDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &ApplicationsDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createApplicationsDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getApplicationsDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptyApplicationsDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getApplicationsDSSchema(t)
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

func applicationsNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getApplicationsDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["applications"].(tftypes.List)
	if !ok {
		t.Fatalf("expected applications to be tftypes.List, got %T", objType.AttributeTypes["applications"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Metadata ---

func TestApplicationsDataSource_Metadata(t *testing.T) {
	t.Parallel()
	ds := &ApplicationsDataSource{}
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_applications" {
		t.Errorf("expected TypeName 'uem_applications', got '%s'", resp.TypeName)
	}
}

// --- Read ---

func TestApplicationsDataSource_Read_PopulatesFromSearch(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{
		result: &sdk.ApplicationSearchV2Model{
			Total: intPtr(2),
			Applications: []sdk.ApplicationV2Model{
				{
					ID:              intPtr(101),
					ApplicationName: "Slack",
					ApplicationType: "Public",
					BundleID:        "com.tinyspeck.chatlyio",
					// UUID is required now that the read walk (internal-ticket release blocker
					// B12) dedupes by it; live evidence shows AppsV2.Search always
					// populates it (even when id is null), so every fixture item needs
					// one for a realistic response shape.
					UUID: "bafde89c-041e-1756-082b-933aaf16cad8",
				},
				{
					ID:              intPtr(102),
					ApplicationName: "Zoom",
					ApplicationType: "Internal",
					BundleID:        "us.zoom.videomeetings",
					UUID:            "05d17100-b346-c29d-6760-a0fdedcf8623",
				},
			},
		},
	}

	ds := &ApplicationsDataSource{search: fake}
	ctx := context.Background()

	config := createApplicationsDSConfig(t, map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, nil),
		"platform":     tftypes.NewValue(tftypes.String, nil),
		"applications": tftypes.NewValue(tftypes.List{ElementType: applicationsNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyApplicationsDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var out ApplicationsDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Applications) != 2 {
		t.Fatalf("expected 2 applications, got %d", len(out.Applications))
	}

	if out.Applications[0].ID.ValueInt64() != 101 {
		t.Errorf("expected first application ID 101, got %d", out.Applications[0].ID.ValueInt64())
	}
	if out.Applications[0].Name.ValueString() != "Slack" {
		t.Errorf("expected first application name 'Slack', got %q", out.Applications[0].Name.ValueString())
	}
	if out.Applications[0].Type.ValueString() != "Public" {
		t.Errorf("expected first application type 'Public', got %q", out.Applications[0].Type.ValueString())
	}
	if out.Applications[0].BundleID.ValueString() != "com.tinyspeck.chatlyio" {
		t.Errorf("expected first application bundle id 'com.tinyspeck.chatlyio', got %q", out.Applications[0].BundleID.ValueString())
	}

	if out.Applications[1].ID.ValueInt64() != 102 {
		t.Errorf("expected second application ID 102, got %d", out.Applications[1].ID.ValueInt64())
	}
	if out.Applications[1].Name.ValueString() != "Zoom" {
		t.Errorf("expected second application name 'Zoom', got %q", out.Applications[1].Name.ValueString())
	}
}

func TestApplicationsDataSource_Read_SearchError(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{err: context.DeadlineExceeded}
	ds := &ApplicationsDataSource{search: fake}
	ctx := context.Background()

	config := createApplicationsDSConfig(t, map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, nil),
		"platform":     tftypes.NewValue(tftypes.String, nil),
		"applications": tftypes.NewValue(tftypes.List{ElementType: applicationsNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyApplicationsDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error when search fails")
	}
}

// stateApplicationsIsNull inspects the RAW state value (not the decoded Go
// struct) for the applications attribute and reports whether it is NULL, as
// opposed to a known empty list. Decoding straight into
// ApplicationsDataSourceModel would not reliably distinguish these two cases
// (both can decode to a nil/zero-length Go slice), but Terraform itself
// does: a null list attribute is what causes `terraform output` to drop the
// attribute entirely on a zero-result run, while a known empty list ([]) is
// what a clean "no applications found" exit needs. See
// internal/updates/update_deployments_data_source_test.go's
// stateUpdateDeploymentsIsNull for the pattern this mirrors.
func stateApplicationsIsNull(t *testing.T, state tfsdk.State) bool {
	t.Helper()
	var obj map[string]tftypes.Value
	if err := state.Raw.As(&obj); err != nil {
		t.Fatalf("decode state as object: %v", err)
	}
	listVal, ok := obj["applications"]
	if !ok {
		t.Fatal("state has no applications attribute at all")
	}
	return listVal.IsNull()
}

// TestApplicationsDataSource_Read_EmptyResultEmitsEmptyNotNullList proves
// the nil-vs-empty-slice fix for this data source: a zero-result search
// must leave the applications attribute as a known EMPTY list in state, not
// NULL. Against the pre-fix `var applications []ApplicationSummary`, this
// fails: an empty search result never appends anything, so applications
// stays nil and the state attribute is null instead of a known empty list.
func TestApplicationsDataSource_Read_EmptyResultEmitsEmptyNotNullList(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	ds := &ApplicationsDataSource{search: fake}
	ctx := context.Background()

	config := createApplicationsDSConfig(t, map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, nil),
		"platform":     tftypes.NewValue(tftypes.String, nil),
		"applications": tftypes.NewValue(tftypes.List{ElementType: applicationsNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyApplicationsDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}
	if stateApplicationsIsNull(t, readResp.State) {
		t.Fatal("expected applications to be a known EMPTY list on a zero-result search, got NULL")
	}

	var out ApplicationsDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}
	if len(out.Applications) != 0 {
		t.Fatalf("expected 0 applications, got %d", len(out.Applications))
	}
}

func TestApplicationsDataSource_Read_FiltersPassedToSearch(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	ds := &ApplicationsDataSource{search: fake}
	ctx := context.Background()

	config := createApplicationsDSConfig(t, map[string]tftypes.Value{
		"name":         tftypes.NewValue(tftypes.String, "MyApp"),
		"platform":     tftypes.NewValue(tftypes.String, "Apple"),
		"applications": tftypes.NewValue(tftypes.List{ElementType: applicationsNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyApplicationsDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	if fake.lastOpts == nil {
		t.Fatal("expected Search to be called with options")
	}
	if fake.lastOpts.Name == nil || *fake.lastOpts.Name != "MyApp" {
		t.Errorf("expected Name 'MyApp', got %v", fake.lastOpts.Name)
	}
	if fake.lastOpts.Platform == nil || *fake.lastOpts.Platform != "Apple" {
		t.Errorf("expected Platform 'Apple', got %v", fake.lastOpts.Platform)
	}
}
