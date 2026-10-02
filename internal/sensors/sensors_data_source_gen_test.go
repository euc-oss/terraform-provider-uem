package sensors

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeSensorSearch is a test-only fake satisfying sensorSearchIface.
type fakeSensorSearch struct {
	items []SensorSummary
	err   error

	lastFilters sensorFilters
}

func (f *fakeSensorSearch) List(_ context.Context, filters sensorFilters) ([]SensorSummary, error) {
	f.lastFilters = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

// --- helpers ---

func getSensorDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &sensorDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createSensorDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getSensorDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptySensorDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getSensorDSSchema(t)
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

func sensorNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getSensorDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["sensors"].(tftypes.List)
	if !ok {
		t.Fatalf("expected sensors to be tftypes.List, got %T", objType.AttributeTypes["sensors"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Schema ---

func TestGeneratedSensorsDataSource_Schema_RequiresOrgGroupUuid(t *testing.T) {
	t.Parallel()

	resp := getSensorDSSchema(t)
	attr, ok := resp.Schema.Attributes["organization_group_uuid"]
	if !ok {
		t.Fatal("expected organization_group_uuid attribute in schema")
	}
	if !attr.IsRequired() {
		t.Errorf("expected organization_group_uuid to be Required, got IsRequired()=%v", attr.IsRequired())
	}
}

// TestGeneratedSensorsDataSource_Schema_OrganizationGroupUuidIsComputed proves the nested
// per-item organization_group_uuid field added to the sensors composition-map entry is an
// output field (Computed), not a filter (Required/Optional) -- mirroring the assertion
// style of TestGeneratedSensorsDataSource_Schema_RequiresOrgGroupUuid above, but for the
// nested "sensors" list's element schema rather than the top-level filter attribute.
func TestGeneratedSensorsDataSource_Schema_OrganizationGroupUuidIsComputed(t *testing.T) {
	t.Parallel()

	resp := getSensorDSSchema(t)
	sensorsAttr, ok := resp.Schema.Attributes["sensors"]
	if !ok {
		t.Fatal("expected sensors attribute in schema")
	}
	listAttr, ok := sensorsAttr.(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("expected sensors to be schema.ListNestedAttribute, got %T", sensorsAttr)
	}
	orgGroupAttr, ok := listAttr.NestedObject.Attributes["organization_group_uuid"]
	if !ok {
		t.Fatal("expected organization_group_uuid attribute in nested sensors object")
	}
	if !orgGroupAttr.IsComputed() {
		t.Errorf("expected organization_group_uuid to be Computed, got IsComputed()=%v", orgGroupAttr.IsComputed())
	}
	if orgGroupAttr.IsRequired() || orgGroupAttr.IsOptional() {
		t.Errorf("expected organization_group_uuid to be output-only (not Required/Optional), got IsRequired()=%v IsOptional()=%v", orgGroupAttr.IsRequired(), orgGroupAttr.IsOptional())
	}
}

// --- Read ---

func TestGeneratedSensorsDataSource_Read(t *testing.T) {
	t.Parallel()

	fake := &fakeSensorSearch{
		items: []SensorSummary{
			{
				Id:   types.Int64Value(42),
				Name: types.StringValue("Battery Health"),
			},
		},
	}
	ds := &sensorDataSource{search: fake}
	ctx := context.Background()

	config := createSensorDSConfig(t, map[string]tftypes.Value{
		"name":                    tftypes.NewValue(tftypes.String, nil),
		"organization_group_uuid": tftypes.NewValue(tftypes.String, "og-uuid-123"),
		"sensors":                 tftypes.NewValue(tftypes.List{ElementType: sensorNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptySensorDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var out SensorDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 sensor, got %d", len(out.Items))
	}
	if out.Items[0].Name.ValueString() != "Battery Health" {
		t.Errorf("expected name 'Battery Health', got %q", out.Items[0].Name.ValueString())
	}
	if out.Items[0].Id.ValueInt64() != 42 {
		t.Errorf("expected id 42, got %d", out.Items[0].Id.ValueInt64())
	}

	// Assert the fake seam received the correct OG-UUID from the filter.
	if fake.lastFilters.OrganizationGroupUuid.ValueString() != "og-uuid-123" {
		t.Errorf("expected filter organization_group_uuid 'og-uuid-123', got %q", fake.lastFilters.OrganizationGroupUuid.ValueString())
	}
}

// --- sensorSearch.List (direct companion coverage) ---
//
// The tests above only exercise the generated wrapper via fakeSensorSearch, never the real
// sensorSearch companion in sensors_data_source_hooks.go. These tests call sensorSearch.List
// directly so the empty/null organization_group_uuid guard is actually load-bearing: if the
// guard were removed, List would proceed to call s.svc.GetDeviceSensorsAsync on a nil
// *sdk.DeviceSensorsV2Service (svc left nil deliberately below), which panics rather than
// merely returning wrong data.

func TestSensorSearchList_EmptyOrgGroupUUID_ReturnsError(t *testing.T) {
	s := &sensorSearch{} // svc left nil deliberately -- if the guard is removed, this would panic on a nil-pointer method call, not just return wrong data
	_, err := s.List(context.Background(), sensorFilters{
		OrganizationGroupUuid: types.StringValue(""),
	})
	if err == nil {
		t.Fatal("expected error for empty organization_group_uuid, got nil")
	}
}

func TestSensorSearchList_NullOrgGroupUUID_ReturnsError(t *testing.T) {
	s := &sensorSearch{}
	_, err := s.List(context.Background(), sensorFilters{
		OrganizationGroupUuid: types.StringNull(),
	})
	if err == nil {
		t.Fatal("expected error for null organization_group_uuid, got nil")
	}
}
