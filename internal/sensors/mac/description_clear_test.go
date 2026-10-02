package macsensor

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// sensorValues is a known, complete sensor object used to build state, plan
// and config for the ModifyPlan tests.
func sensorValues() map[string]types.String {
	return map[string]types.String{
		"id":                      types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
		"organization_group_uuid": types.StringValue("05d17100-b346-c29d-6760-a0fdedcf8623"),
		"name":                    types.StringValue("tf_sensor"),
		"description":             types.StringValue("x"),
		"language":                types.StringValue("BASH"),
		"execution_context":       types.StringValue("SYSTEM"),
		"execution_architecture":  types.StringValue("EITHER64OR32BIT"),
		"response_data_type":      types.StringValue("STRING"),
		"code":                    types.StringValue("echo hi"),
	}
}

func setSensorValues(t *testing.T, st *tfsdk.State, vals map[string]types.String) {
	t.Helper()
	ctx := context.Background()
	for name, v := range vals {
		if diags := st.SetAttribute(ctx, path.Root(name), v); diags.HasError() {
			t.Fatalf("set %s: %v", name, diags)
		}
	}
}

func runSensorModifyPlan(t *testing.T, prior, config map[string]types.String) *resource.ModifyPlanResponse {
	t.Helper()
	state := emptyResourceState(t)
	setSensorValues(t, &state, prior)
	planState := emptyResourceState(t)
	setSensorValues(t, &planState, config)

	req := resource.ModifyPlanRequest{
		State:  state,
		Plan:   tfsdk.Plan(planState),
		Config: tfsdk.Config(planState),
	}
	resp := &resource.ModifyPlanResponse{Plan: req.Plan}
	(&macsensorResource{}).ModifyPlan(context.Background(), req, resp)
	return resp
}

func withValue(vals map[string]types.String, name string, v types.String) map[string]types.String {
	out := make(map[string]types.String, len(vals))
	for k, x := range vals {
		out[k] = x
	}
	out[name] = v
	return out
}

func TestModifyPlan_DescriptionClear(t *testing.T) {
	t.Parallel()
	base := sensorValues()
	cases := []struct {
		name      string
		prior     map[string]types.String
		config    map[string]types.String
		wantError bool
	}{
		{"state x, config null", base, withValue(base, "description", types.StringNull()), true},
		{"state x, config empty", base, withValue(base, "description", types.StringValue("")), true},
		{"state x, config y", base, withValue(base, "description", types.StringValue("y")), false},
		{"state x, config unknown", base, withValue(base, "description", types.StringUnknown()), false},
		{"state null, config null", withValue(base, "description", types.StringNull()), withValue(base, "description", types.StringNull()), false},
		{"state empty, config null", withValue(base, "description", types.StringValue("")), withValue(base, "description", types.StringNull()), false},
		{"rename (replace) + removal", base, withValue(withValue(base, "description", types.StringNull()), "name", types.StringValue("tf_sensor_two")), false},
		{"OG change (replace) + removal", base, withValue(withValue(base, "description", types.StringNull()), "organization_group_uuid", types.StringValue("16fea16d-024d-1e8b-e41d-b5981759f00d")), false},
		{"response type change (replace) + removal", base, withValue(withValue(base, "description", types.StringNull()), "response_data_type", types.StringValue("INTEGER")), false},
		{"unknown name (possible replace) + removal", base, withValue(withValue(base, "description", types.StringNull()), "name", types.StringUnknown()), false},
		{"in-place change + removal", base, withValue(withValue(base, "description", types.StringNull()), "code", types.StringValue("echo bye")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := runSensorModifyPlan(t, tc.prior, tc.config)
			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("HasError = %v, want %v (diags: %v)", got, tc.wantError, resp.Diagnostics)
			}
			if tc.wantError {
				d := resp.Diagnostics.Errors()[0]
				if d.Detail() != descriptionClearMessage {
					t.Fatalf("unexpected detail: %s", d.Detail())
				}
			}
		})
	}
}

func TestModifyPlan_CreateAndDestroyNoError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	known := emptyResourceState(t)
	setSensorValues(t, &known, sensorValues()) // description "x"
	nullRaw := tftypes.NewValue(known.Raw.Type(), nil)
	r := &macsensorResource{}

	// Create: no prior object, config without a description.
	createCfg := emptyResourceState(t)
	setSensorValues(t, &createCfg, withValue(sensorValues(), "description", types.StringNull()))
	create := &resource.ModifyPlanResponse{Plan: tfsdk.Plan(createCfg)}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State:  tfsdk.State{Schema: known.Schema, Raw: nullRaw},
		Plan:   tfsdk.Plan(createCfg),
		Config: tfsdk.Config(createCfg),
	}, create)
	if create.Diagnostics.HasError() {
		t.Fatalf("create: unexpected error: %v", create.Diagnostics)
	}

	// Destroy: prior description "x", null plan and config.
	destroy := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: known.Schema, Raw: nullRaw}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		State:  known,
		Plan:   tfsdk.Plan{Schema: known.Schema, Raw: nullRaw},
		Config: tfsdk.Config{Schema: known.Schema, Raw: nullRaw},
	}, destroy)
	if len(destroy.Diagnostics) != 0 {
		t.Fatalf("destroy: expected no diagnostics, got: %v", destroy.Diagnostics)
	}
}

// TestReplaceTriggerAttributesMatchSchema keeps replaceTriggerAttributes in
// step with the attributes that carry RequiresReplace in the schema.
func TestReplaceTriggerAttributesMatchSchema(t *testing.T) {
	t.Parallel()
	resp := &resource.SchemaResponse{}
	(&macsensorResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)

	var fromSchema []string
	for name, a := range resp.Schema.Attributes {
		sa, ok := a.(schema.StringAttribute)
		if !ok {
			continue
		}
		for _, m := range sa.PlanModifiers {
			if strings.Contains(m.Description(context.Background()), "destroy and recreate") {
				fromSchema = append(fromSchema, name)
			}
		}
	}
	want := append([]string(nil), replaceTriggerAttributes...)
	sort.Strings(fromSchema)
	sort.Strings(want)
	if strings.Join(fromSchema, ",") != strings.Join(want, ",") {
		t.Fatalf("RequiresReplace attributes in schema = %v, replaceTriggerAttributes = %v", fromSchema, want)
	}
}
