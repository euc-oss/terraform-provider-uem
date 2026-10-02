package assignment

import (
	"context"
	"strings"
	"testing"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// emptyAssignmentState builds a zero-valued state for applicationAssignmentResource,
// used as a template for constructing test configs via tfsdk.Plan.
func emptyAssignmentState(t *testing.T) tfsdk.State {
	t.Helper()

	r := &applicationAssignmentResource{}
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

// assignmentValidateSpec describes one assignment in a validation test
// config. A nil priority leaves priority unknown (rather than unset, since
// priority is Required and cannot be left null).
type assignmentValidateSpec struct {
	priority        int64
	unknownPriority bool
}

// assignmentValidationConfig builds a resource config with one assignment
// per spec, each targeting a single smart group (required by the schema).
func assignmentValidationConfig(t *testing.T, specs ...assignmentValidateSpec) tfsdk.Config {
	t.Helper()
	ctx := context.Background()

	empty := emptyAssignmentState(t)
	plan := tfsdk.Plan{Schema: empty.Schema, Raw: empty.Raw.Copy()}

	listType, ok := plan.Schema.GetAttributes()["assignments"].GetType().(types.ListType)
	if !ok {
		t.Fatal("assignments is not a list type")
	}
	elemType, ok := listType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}

	models := make([]tf.AppAssignmentModel, 0, len(specs))
	for _, spec := range specs {
		models = append(models, tf.AppAssignmentModel{
			Priority: types.Int64Value(spec.priority),
			Distribution: tf.AppAssignmentDistributionModel{
				Name:              types.StringValue("APP"),
				Description:       types.StringNull(),
				SmartGroups:       types.ListValueMust(types.StringType, []attr.Value{types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7")}),
				AppDeliveryMethod: types.StringNull(),
				EffectiveDate:     types.StringNull(),
			},
			Restriction: nil,
		})
	}
	assignments, diags := types.ListValueFrom(ctx, elemType, models)
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}

	model := tf.AppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue("596b30c4-5fd8-f8a4-2f40-553c312b9f1a"),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         assignments,
	}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}
	config := tfsdk.Config(plan)

	// Make the requested priorities unknown after Set, since
	// types.Int64Value cannot express "unknown" through the model struct.
	hasUnknown := false
	for _, spec := range specs {
		if spec.unknownPriority {
			hasUnknown = true
			break
		}
	}
	if !hasUnknown {
		return config
	}

	objType, ok := config.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("config type is not an object")
	}
	rootValues := map[string]tftypes.Value{}
	if err := config.Raw.As(&rootValues); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	assignmentsListType, ok := objType.AttributeTypes["assignments"].(tftypes.List)
	if !ok {
		t.Fatal("assignments attribute is not a list type")
	}
	var elements []tftypes.Value
	if err := rootValues["assignments"].As(&elements); err != nil {
		t.Fatalf("decode assignments: %v", err)
	}
	elemObjType, ok := assignmentsListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	for i, spec := range specs {
		if !spec.unknownPriority {
			continue
		}
		elemValues := map[string]tftypes.Value{}
		if err := elements[i].As(&elemValues); err != nil {
			t.Fatalf("decode assignment[%d]: %v", i, err)
		}
		elemValues["priority"] = tftypes.NewValue(elemObjType.AttributeTypes["priority"], tftypes.UnknownValue)
		elements[i] = tftypes.NewValue(elemObjType, elemValues)
	}
	rootValues["assignments"] = tftypes.NewValue(assignmentsListType, elements)
	config.Raw = tftypes.NewValue(objType, rootValues)
	return config
}

func runAssignmentValidateConfig(t *testing.T, config tfsdk.Config) *resource.ValidateConfigResponse {
	t.Helper()
	r := &applicationAssignmentResource{}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: config}, resp)
	return resp
}

// TestAssignmentValidateConfigValidPrioritiesNoDiags proves a config whose
// priorities are already 0..n-1 in order produces no diagnostics.
func TestAssignmentValidateConfigValidPrioritiesNoDiags(t *testing.T) {
	config := assignmentValidationConfig(t,
		assignmentValidateSpec{priority: 0},
		assignmentValidateSpec{priority: 1},
		assignmentValidateSpec{priority: 2},
	)
	resp := runAssignmentValidateConfig(t, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

// TestAssignmentValidateConfigGapReportsAtOffendingIndex proves a priority
// gap ([0, 2]) is reported as a (a)-rule error at the offending index.
func TestAssignmentValidateConfigGapReportsAtOffendingIndex(t *testing.T) {
	config := assignmentValidationConfig(t,
		assignmentValidateSpec{priority: 0},
		assignmentValidateSpec{priority: 2},
	)
	resp := runAssignmentValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[1].priority" {
		t.Fatalf("error path = %q, want assignments[1].priority", got)
	}
	if !strings.Contains(errs[0].Detail(), "UEM requires the priorities") {
		t.Fatalf("error detail %q does not describe the (a) rule", errs[0].Detail())
	}
}

// TestAssignmentValidateConfigOutOfOrderReportsAtFirstMismatch proves a
// valid-but-out-of-order priority set ([1, 0]) is reported as a (b)-rule
// error at the first index where priority != position.
func TestAssignmentValidateConfigOutOfOrderReportsAtFirstMismatch(t *testing.T) {
	config := assignmentValidationConfig(t,
		assignmentValidateSpec{priority: 1},
		assignmentValidateSpec{priority: 0},
	)
	resp := runAssignmentValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[0].priority" {
		t.Fatalf("error path = %q, want assignments[0].priority", got)
	}
	if !strings.Contains(errs[0].Detail(), "listed at position") {
		t.Fatalf("error detail %q does not describe the (b) rule", errs[0].Detail())
	}
}

// TestAssignmentValidateConfigUnknownPrioritySkipsCheck proves an unknown
// priority (fed by a not-yet-known variable) suppresses the whole priority
// check rather than failing.
func TestAssignmentValidateConfigUnknownPrioritySkipsCheck(t *testing.T) {
	config := assignmentValidationConfig(t,
		assignmentValidateSpec{priority: 0, unknownPriority: true},
		assignmentValidateSpec{priority: 2},
	)
	resp := runAssignmentValidateConfig(t, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}
