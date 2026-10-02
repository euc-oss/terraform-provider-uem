package macscript

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// configWithExecutionContext builds a config whose attributes are all null
// except execution_context, which is set to v.
func configWithExecutionContext(t *testing.T, v tftypes.Value) tfsdk.Config {
	t.Helper()
	s := resourceSchema(t).Schema
	objType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	values["execution_context"] = v
	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, values)}
}

// uix: UEM answers a create without execution_context with HTTP 500, so a
// null execution_context is rejected at plan time on that attribute path.
func TestValidateConfig_ExecutionContext(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   tftypes.Value
		wantErr bool
	}{
		{"null -> error", tftypes.NewValue(tftypes.String, nil), true},
		{"SYSTEM -> ok", tftypes.NewValue(tftypes.String, "SYSTEM"), false},
		{"USER -> ok", tftypes.NewValue(tftypes.String, "USER"), false},
		{"unknown -> skipped", tftypes.NewValue(tftypes.String, tftypes.UnknownValue), false},
		{"empty string -> left to UEM", tftypes.NewValue(tftypes.String, ""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &resource.ValidateConfigResponse{}
			(&macscriptResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: configWithExecutionContext(t, tc.value)}, resp)

			if !tc.wantErr {
				if resp.Diagnostics.HasError() {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if resp.Diagnostics.ErrorsCount() != 1 {
				t.Fatalf("want exactly 1 error, got %v", resp.Diagnostics)
			}
			d, ok := resp.Diagnostics.Errors()[0].(interface{ Path() path.Path })
			if !ok || !d.Path().Equal(path.Root("execution_context")) {
				t.Fatalf("error is not on path execution_context: %v", resp.Diagnostics.Errors()[0])
			}
		})
	}
}
