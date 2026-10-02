package scripts

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fakeScriptSearch is a test-only fake satisfying scriptSearchIface.
type fakeScriptSearch struct {
	items []ScriptSummary
	err   error

	lastFilters scriptFilters
}

func (f *fakeScriptSearch) List(_ context.Context, filters scriptFilters) ([]ScriptSummary, error) {
	f.lastFilters = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

// --- helpers ---

func getScriptDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &scriptDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

func createScriptDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getScriptDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func emptyScriptDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getScriptDSSchema(t)
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

func scriptNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getScriptDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["scripts"].(tftypes.List)
	if !ok {
		t.Fatalf("expected scripts to be tftypes.List, got %T", objType.AttributeTypes["scripts"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Read ---

func TestGeneratedScriptsDataSource_Read(t *testing.T) {
	t.Parallel()

	fake := &fakeScriptSearch{
		items: []ScriptSummary{
			{
				Name:                  types.StringValue("Reboot Device"),
				OrganizationGroupUuid: types.StringValue("og-uuid-1"),
				Platform:              types.StringValue("WINDOWS"),
				ScriptType:            types.StringValue("Shell"),
				ScriptUuid:            types.StringValue("script-uuid-1"),
				Version:               types.StringValue("1"),
			},
		},
	}
	ds := &scriptDataSource{search: fake}
	ctx := context.Background()

	config := createScriptDSConfig(t, map[string]tftypes.Value{
		"name":                    tftypes.NewValue(tftypes.String, nil),
		"organization_group_uuid": tftypes.NewValue(tftypes.String, "og-uuid-1"),
		"scripts":                 tftypes.NewValue(tftypes.List{ElementType: scriptNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyScriptDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	// The seam must have received the org-group UUID exactly as configured.
	if fake.lastFilters.OrganizationGroupUuid.ValueString() != "og-uuid-1" {
		t.Errorf("expected List to receive organization_group_uuid %q, got %q", "og-uuid-1", fake.lastFilters.OrganizationGroupUuid.ValueString())
	}

	var out ScriptDataSourceModel
	if diags := readResp.State.Get(ctx, &out); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags.Errors())
	}

	if len(out.Items) != 1 {
		t.Fatalf("expected 1 script, got %d", len(out.Items))
	}
	if out.Items[0].Name.ValueString() != "Reboot Device" {
		t.Errorf("expected name 'Reboot Device', got %q", out.Items[0].Name.ValueString())
	}
	if out.Items[0].Platform.ValueString() != "WINDOWS" {
		t.Errorf("expected platform 'WINDOWS', got %q", out.Items[0].Platform.ValueString())
	}
	if out.Items[0].ScriptType.ValueString() != "Shell" {
		t.Errorf("expected script_type 'Shell', got %q", out.Items[0].ScriptType.ValueString())
	}
	if out.Items[0].ScriptUuid.ValueString() != "script-uuid-1" {
		t.Errorf("expected script_uuid 'script-uuid-1', got %q", out.Items[0].ScriptUuid.ValueString())
	}
	if out.Items[0].OrganizationGroupUuid.ValueString() != "og-uuid-1" {
		t.Errorf("expected org group uuid 'og-uuid-1', got %q", out.Items[0].OrganizationGroupUuid.ValueString())
	}
	if out.Items[0].Version.ValueString() != "1" {
		t.Errorf("expected version '1', got %q", out.Items[0].Version.ValueString())
	}
}

// TestGeneratedScriptsDataSource_Read_EmptyOrgGroupUUID exercises the real (non-fake)
// companion's defensive handling of an empty organization_group_uuid. The schema marks
// this attribute Required, so Terraform itself would normally reject a null value before
// Read is ever invoked in production -- but an empty string ("") still satisfies
// "Required" (non-null) at the framework level, so the companion must defend against it
// itself rather than passing a blank OG-UUID to the SDK. This must surface as a clean
// diagnostic error, never a panic.
func TestGeneratedScriptsDataSource_Read_EmptyOrgGroupUUID(t *testing.T) {
	t.Parallel()

	ds := &scriptDataSource{search: &scriptSearch{}} // real companion, nil svc: must error before touching svc
	ctx := context.Background()

	config := createScriptDSConfig(t, map[string]tftypes.Value{
		"name":                    tftypes.NewValue(tftypes.String, nil),
		"organization_group_uuid": tftypes.NewValue(tftypes.String, ""),
		"scripts":                 tftypes.NewValue(tftypes.List{ElementType: scriptNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyScriptDSState(t)}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Read panicked on empty organization_group_uuid: %v", r)
		}
	}()

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic error for empty organization_group_uuid, got none")
	}
}

// TestScriptSearchList_EmptyOrgGroupUUID_ReturnsError unit-tests the companion's List
// directly: an empty organization_group_uuid must produce a plain error (not a panic),
// and must do so without ever dereferencing the (nil, in this test) SDK service.
func TestScriptSearchList_EmptyOrgGroupUUID_ReturnsError(t *testing.T) {
	t.Parallel()

	s := &scriptSearch{} // svc left nil on purpose: a real SDK call would panic
	_, err := s.List(context.Background(), scriptFilters{OrganizationGroupUuid: types.StringValue("")})
	if err == nil {
		t.Fatal("expected error for empty organization_group_uuid, got nil")
	}
}
