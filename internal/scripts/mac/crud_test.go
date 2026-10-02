package macscript

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func emptyResourceState(t *testing.T) tfsdk.State {
	t.Helper()

	r := &macscriptResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}

	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

func TestImportState(t *testing.T) {
	ctx := context.Background()

	t.Run("valid script UUID", func(t *testing.T) {
		r := &macscriptResource{}
		req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
		resp := &resource.ImportStateResponse{State: emptyResourceState(t)}

		r.ImportState(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}

		var id types.String
		resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("id"), &id)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read id from state: %v", resp.Diagnostics)
		}
		if id.ValueString() != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
			t.Fatalf("id mismatch: got %q", id.ValueString())
		}
	})

	t.Run("empty import ID", func(t *testing.T) {
		r := &macscriptResource{}
		req := resource.ImportStateRequest{ID: ""}
		resp := &resource.ImportStateResponse{State: emptyResourceState(t)}

		r.ImportState(ctx, req, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected diagnostics error for empty import ID")
		}
	})
}
