package assignment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// This file covers internal-task's "unknown values break plan" finding:
// ModifyPlan used to decode plan/config assignments into typed structs via
// ElementsAs, which fails outright (an error diagnostic, not a skip) the
// moment any nested object or list element is UNKNOWN — e.g. a
// vpp_app_details or restriction block fed from a not-yet-created resource.
// The fix walks the list generically instead, so an unknown anywhere in the
// checked (restriction-only) path causes the platform check to be skipped
// (no lookup, no plan change, no diagnostics) rather than failing the whole
// plan; an unknown value OUTSIDE that path (e.g. vpp_app_details) must not
// affect the check at all. TestModifyPlan_UnknownRestrictionObject_Skipped,
// TestModifyPlan_UnknownVppAppDetailsObject_Skipped and
// TestModifyPlan_UnknownAssignmentElement_Skipped were confirmed (by running
// them against the pre-fix ElementsAs-based ModifyPlan) to fail with an "An
// unexpected error was encountered trying to build a value" error
// diagnostic before the fix landed.

// baseKnownAssignment returns one assignment with a platform-sensitive
// restriction flag (prevent_removal) planned true, so that, absent the
// unknown-skip behaviour under test, ModifyPlan would attempt a platform
// lookup and/or fail to decode.
func baseKnownAssignment(st modifyPlanSchemaTypes) tf.PurchasedAppAssignmentModel {
	return st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
		PreventRemoval: types.BoolValue(true),
	}, types.BoolNull())
}

// configFromMutatedPlan builds a tfsdk.Config carrying the same raw value as
// planT, mirroring modifyPlanSchemaTypes.config's own "derive config's raw
// from a plan" approach (tfsdk.Config has no SetAttribute of its own, so
// mutations are applied to a Plan and then copied across).
func configFromMutatedPlan(st modifyPlanSchemaTypes, planT tfsdk.Plan) tfsdk.Config {
	return tfsdk.Config{Schema: st.schemaResp.Schema, Raw: planT.Raw}
}

func TestModifyPlan_UnknownRestrictionObject_Skipped(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{baseKnownAssignment(st)}
	planT := st.plan(t, assignments)

	restrictionType, ok := st.assignmentElem.AttrTypes["restriction"].(types.ObjectType)
	if !ok {
		t.Fatal("restriction is not an object type")
	}
	unknownRestriction := types.ObjectUnknown(restrictionType.AttrTypes)

	restrictionPath := path.Root("assignments").AtListIndex(0).AtName("restriction")
	if diags := planT.SetAttribute(ctx, restrictionPath, unknownRestriction); diags.HasError() {
		t.Fatalf("set plan restriction unknown: %v", diags)
	}
	configT := configFromMutatedPlan(st, planT)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics when restriction is unknown, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls when restriction is unknown, got %d", fake.calls)
	}
}

func TestModifyPlan_UnknownVppAppDetailsObject_Skipped(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{baseKnownAssignment(st)}
	planT := st.plan(t, assignments)

	distType, ok := st.assignmentElem.AttrTypes["distribution"].(types.ObjectType)
	if !ok {
		t.Fatal("distribution is not an object type")
	}
	vppType, ok := distType.AttrTypes["vpp_app_details"].(types.ObjectType)
	if !ok {
		t.Fatal("vpp_app_details is not an object type")
	}
	unknownVppAppDetails := types.ObjectUnknown(vppType.AttrTypes)

	vppPath := path.Root("assignments").AtListIndex(0).AtName("distribution").AtName("vpp_app_details")
	if diags := planT.SetAttribute(ctx, vppPath, unknownVppAppDetails); diags.HasError() {
		t.Fatalf("set plan vpp_app_details unknown: %v", diags)
	}
	configT := configFromMutatedPlan(st, planT)

	resp := runModifyPlan(t, r, planT, configT)
	// restriction.prevent_removal is known-true and the platform is macOS
	// (fake), so the normal macOS-ignored error is expected here:
	// vpp_app_details being unknown is unrelated to the restriction walk and
	// must not suppress it (nor, before the fix, should it have blocked the
	// whole decode).
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 || errs[0].Summary() != macOSIgnoredSummary {
		t.Fatalf("expected exactly 1 macOS-ignored error (vpp_app_details unknown must not block the restriction check), got: %v", resp.Diagnostics)
	}
	if fake.calls != 1 {
		t.Fatalf("expected exactly 1 lookup call (vpp_app_details unknown must not block the restriction check), got %d", fake.calls)
	}
}

func TestModifyPlan_UnknownAssignmentElement_Skipped(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{baseKnownAssignment(st)}
	planT := st.plan(t, assignments)

	unknownElement := types.ObjectUnknown(st.assignmentElem.AttrTypes)
	elementPath := path.Root("assignments").AtListIndex(0)
	if diags := planT.SetAttribute(ctx, elementPath, unknownElement); diags.HasError() {
		t.Fatalf("set plan element unknown: %v", diags)
	}
	configT := configFromMutatedPlan(st, planT)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics when a whole assignment element is unknown, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls when a whole assignment element is unknown, got %d", fake.calls)
	}
}

func TestModifyPlan_UnknownAssignmentsList_Skipped(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{baseKnownAssignment(st)}
	planT := st.plan(t, assignments)

	unknownList := types.ListUnknown(st.assignmentElem)
	listPath := path.Root("assignments")
	if diags := planT.SetAttribute(ctx, listPath, unknownList); diags.HasError() {
		t.Fatalf("set plan assignments unknown: %v", diags)
	}
	configT := configFromMutatedPlan(st, planT)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics when the whole assignments list is unknown, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls when the whole assignments list is unknown, got %d", fake.calls)
	}
}

// TestModifyPlan_UnknownRestrictionWithKnownFlagElsewhere_Skipped pins that
// one unknown restriction skips the whole platform check, even when another
// assignment plans a platform-sensitive flag true: treating the unknown block
// as absent would still look up and reject assignments[1] on macOS.
func TestModifyPlan_UnknownRestrictionWithKnownFlagElsewhere_Skipped(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	planT := st.plan(t, []tf.PurchasedAppAssignmentModel{baseKnownAssignment(st), baseKnownAssignment(st)})
	restrictionType, ok := st.assignmentElem.AttrTypes["restriction"].(types.ObjectType)
	if !ok {
		t.Fatal("restriction is not an object type")
	}
	restrictionPath := path.Root("assignments").AtListIndex(0).AtName("restriction")
	if diags := planT.SetAttribute(ctx, restrictionPath, types.ObjectUnknown(restrictionType.AttrTypes)); diags.HasError() {
		t.Fatalf("set plan restriction unknown: %v", diags)
	}

	resp := runModifyPlan(t, r, planT, configFromMutatedPlan(st, planT))
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics when any restriction is unknown, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls when any restriction is unknown, got %d", fake.calls)
	}
}
