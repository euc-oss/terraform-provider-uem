package assignment

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- nullWhenConfigNull{List,String,Bool} direct modifier tests (internal-task) ---
//
// As with the internal application/assignment resource, each modifier's
// PlanModifyX only looks at req.ConfigValue, never a sibling attribute, so a
// removal-alone plan and a removal-with-sibling-change plan are structurally
// guaranteed to produce the identical planned value for the removed
// attribute.

func TestPurchasedNullWhenConfigNullListModifier_PlanModifyList(t *testing.T) {
	t.Parallel()
	elemType := types.StringType

	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		req := planmodifier.ListRequest{
			ConfigValue: types.ListUnknown(elemType),
			StateValue:  types.ListNull(elemType),
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with non-null state plans null (removal alone)", func(t *testing.T) {
		// Removal-alone: proposed new state carries the prior value forward,
		// so PlanValue is the known prior state value, not Unknown.
		stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("sg-1")})
		if diags.HasError() {
			t.Fatalf("building list value: %v", diags)
		}
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  stateVal,
			PlanValue:   stateVal,
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on removal, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with null state plans null, never unknown (create)", func(t *testing.T) {
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  types.ListNull(elemType),
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to never be unknown on create, got %v", resp.PlanValue)
		}
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on create with no config, got %v", resp.PlanValue)
		}
	})

	t.Run("non-null config leaves plan unchanged (import roundtrip)", func(t *testing.T) {
		planVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("sg-1")})
		if diags.HasError() {
			t.Fatalf("building list value: %v", diags)
		}
		req := planmodifier.ListRequest{
			ConfigValue: planVal,
			StateValue:  planVal,
			PlanValue:   planVal,
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.Equal(planVal) {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

func TestPurchasedNullWhenConfigNullListModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan(t *testing.T) {
	t.Parallel()
	elemType := types.StringType
	stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("sg-1")})
	if diags.HasError() {
		t.Fatalf("building list value: %v", diags)
	}

	// Removal-alone: proposed new state carries the prior value forward, so
	// PlanValue is the known prior state value, not Unknown.
	removalAlone := &planmodifier.ListResponse{PlanValue: stateVal}
	nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), planmodifier.ListRequest{
		ConfigValue: types.ListNull(elemType),
		StateValue:  stateVal,
		PlanValue:   stateVal,
	}, removalAlone)

	removalWithSibling := &planmodifier.ListResponse{PlanValue: types.ListUnknown(elemType)}
	nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), planmodifier.ListRequest{
		ConfigValue: types.ListNull(elemType),
		StateValue:  stateVal,
		PlanValue:   types.ListUnknown(elemType),
	}, removalWithSibling)

	if !removalAlone.PlanValue.Equal(removalWithSibling.PlanValue) {
		t.Fatalf("expected identical planned values, got %v vs %v", removalAlone.PlanValue, removalWithSibling.PlanValue)
	}
	if !removalAlone.PlanValue.IsNull() {
		t.Fatalf("expected null planned value, got %v", removalAlone.PlanValue)
	}
}

func TestPurchasedNullWhenConfigNullStringModifier_PlanModifyString(t *testing.T) {
	t.Parallel()

	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		req := planmodifier.StringRequest{
			ConfigValue: types.StringUnknown(),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with non-null state plans null (removal alone)", func(t *testing.T) {
		// Removal-alone: proposed new state carries the prior value forward,
		// so PlanValue is the known prior state value, not Unknown.
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringValue("ON_DEMAND"),
			PlanValue:   types.StringValue("ON_DEMAND"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on removal, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with null state plans null, never unknown (create)", func(t *testing.T) {
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to never be unknown on create, got %v", resp.PlanValue)
		}
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on create with no config, got %v", resp.PlanValue)
		}
	})

	t.Run("non-null config leaves plan unchanged (import roundtrip)", func(t *testing.T) {
		req := planmodifier.StringRequest{
			ConfigValue: types.StringValue("SEAMLESS"),
			StateValue:  types.StringValue("SEAMLESS"),
			PlanValue:   types.StringValue("SEAMLESS"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "SEAMLESS" {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

func TestPurchasedNullWhenConfigNullBoolModifier_PlanModifyBool(t *testing.T) {
	t.Parallel()

	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolUnknown(),
			StateValue:  types.BoolNull(),
			PlanValue:   types.BoolUnknown(),
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with non-null state plans null (removal alone)", func(t *testing.T) {
		// Removal-alone: proposed new state carries the prior value forward,
		// so PlanValue is the known prior state value, not Unknown.
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolNull(),
			StateValue:  types.BoolValue(true),
			PlanValue:   types.BoolValue(true),
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on removal, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with null state plans null, never unknown (create)", func(t *testing.T) {
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolNull(),
			StateValue:  types.BoolNull(),
			PlanValue:   types.BoolUnknown(),
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to never be unknown on create, got %v", resp.PlanValue)
		}
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on create with no config, got %v", resp.PlanValue)
		}
	})

	t.Run("non-null config leaves plan unchanged (import roundtrip)", func(t *testing.T) {
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolValue(true),
			StateValue:  types.BoolValue(true),
			PlanValue:   types.BoolValue(true),
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), req, resp)
		if !resp.PlanValue.ValueBool() {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

func TestPurchasedNullWhenConfigNullBoolModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan(t *testing.T) {
	t.Parallel()

	// Removal-alone: proposed new state carries the prior value forward, so
	// PlanValue is the known prior state value, not Unknown.
	removalAlone := &planmodifier.BoolResponse{PlanValue: types.BoolValue(true)}
	nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolValue(true),
	}, removalAlone)

	removalWithSibling := &planmodifier.BoolResponse{PlanValue: types.BoolUnknown()}
	nullWhenConfigNullBoolModifier{}.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
	}, removalWithSibling)

	if !removalAlone.PlanValue.Equal(removalWithSibling.PlanValue) {
		t.Fatalf("expected identical planned values, got %v vs %v", removalAlone.PlanValue, removalWithSibling.PlanValue)
	}
	if !removalAlone.PlanValue.IsNull() {
		t.Fatalf("expected null planned value, got %v", removalAlone.PlanValue)
	}
}

// --- schema-wiring tests (internal-task item 1h) ---

func purchasedSchemaFor(t *testing.T) schema.Schema {
	t.Helper()
	r := &purchasedApplicationAssignmentResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func purchasedHasListModifierType(modifiers []planmodifier.List, target any) bool {
	want := fmt.Sprintf("%T", target)
	for _, m := range modifiers {
		if fmt.Sprintf("%T", m) == want {
			return true
		}
	}
	return false
}

func purchasedHasStringModifierType(modifiers []planmodifier.String, target any) bool {
	want := fmt.Sprintf("%T", target)
	for _, m := range modifiers {
		if fmt.Sprintf("%T", m) == want {
			return true
		}
	}
	return false
}

func purchasedHasBoolModifierType(modifiers []planmodifier.Bool, target any) bool {
	want := fmt.Sprintf("%T", target)
	for _, m := range modifiers {
		if fmt.Sprintf("%T", m) == want {
			return true
		}
	}
	return false
}

func TestPurchasedSchema_ExcludedSmartGroupsHasNullWhenConfigNullModifier(t *testing.T) {
	s := purchasedSchemaFor(t)
	attribute, ok := s.Attributes["excluded_smart_groups"].(schema.ListAttribute)
	if !ok {
		t.Fatalf("excluded_smart_groups is not a schema.ListAttribute: %T", s.Attributes["excluded_smart_groups"])
	}
	if !purchasedHasListModifierType(attribute.PlanModifiers, nullWhenConfigNullListModifier{}) {
		t.Fatalf("excluded_smart_groups PlanModifiers %v does not contain nullWhenConfigNullListModifier", attribute.PlanModifiers)
	}
}

func purchasedAssignmentsAttributes(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	s := purchasedSchemaFor(t)
	assignments, ok := s.Attributes["assignments"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("assignments is not a schema.ListNestedAttribute: %T", s.Attributes["assignments"])
	}
	return assignments.NestedObject.Attributes
}

func purchasedDistributionAttributes(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	distribution, ok := purchasedAssignmentsAttributes(t)["distribution"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("distribution is not a schema.SingleNestedAttribute: %T", purchasedAssignmentsAttributes(t)["distribution"])
	}
	return distribution.Attributes
}

func purchasedRestrictionAttributes(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	restriction, ok := purchasedAssignmentsAttributes(t)["restriction"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("restriction is not a schema.SingleNestedAttribute: %T", purchasedAssignmentsAttributes(t)["restriction"])
	}
	return restriction.Attributes
}

// TestPurchasedSchema_DistributionSmartGroupsIsComputedWithoutForcingNullOnOmit
// pins the B23 fix: nullWhenConfigNullListModifier forces the plan to null
// whenever config omits this attribute, regardless of the real (refreshed)
// state value. UEM mirrors the live license_usage groups into this field on
// every Read and silently ignores whatever this field's write side sends
// (see the ValidateConfig guard in resource_validate.go), so forcing null on
// omission planned a permanent false diff against that server-mirrored
// value -- live-confirmed 2/2 on paul-2609 (uem_purchased_application_assignment
// "Google" and "WhatsApp Messenger", both onboarded with this field
// populated from real state). It must never come back.
func TestPurchasedSchema_DistributionSmartGroupsIsComputedWithoutForcingNullOnOmit(t *testing.T) {
	attribute, ok := purchasedDistributionAttributes(t)["smart_groups"].(schema.ListAttribute)
	if !ok {
		t.Fatalf("smart_groups is not a schema.ListAttribute: %T", purchasedDistributionAttributes(t)["smart_groups"])
	}
	if !attribute.Optional || !attribute.Computed {
		t.Fatalf("smart_groups must stay Optional+Computed so a user-set value still reaches ValidateConfig's guard while an omitted one stays server-managed, got Optional=%v Computed=%v", attribute.Optional, attribute.Computed)
	}
	if purchasedHasListModifierType(attribute.PlanModifiers, nullWhenConfigNullListModifier{}) {
		t.Fatalf("smart_groups PlanModifiers %v must NOT contain nullWhenConfigNullListModifier -- that forces a false diff against the server-mirrored value on every omitted-config plan (B23)", attribute.PlanModifiers)
	}
	if !purchasedHasListModifierType(attribute.PlanModifiers, normalizeLowercaseStringListModifier{}) {
		t.Fatalf("smart_groups PlanModifiers %v lost its existing lowercase normalizer", attribute.PlanModifiers)
	}

	// Behavioral pin: an omitted config plans to the prior/refreshed state
	// value, not null -- the exact scenario that broke onboarding.
	elemType := types.StringType
	stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("83079bf7-6d73-94a3-aa94-8a2fab9fb847")})
	if diags.HasError() {
		t.Fatalf("build state list: %v", diags)
	}
	req := planmodifier.ListRequest{
		ConfigValue: types.ListNull(elemType),
		StateValue:  stateVal,
		PlanValue:   stateVal,
	}
	resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
	for _, m := range attribute.PlanModifiers {
		m.PlanModifyList(context.Background(), req, resp)
		req.PlanValue = resp.PlanValue
	}
	if resp.PlanValue.IsNull() {
		t.Fatalf("omitted config planned smart_groups to null instead of preserving the server-mirrored state value %v -- this is the exact B23 regression", stateVal)
	}
	if !resp.PlanValue.Equal(stateVal) {
		t.Fatalf("omitted config planned smart_groups to %v, want the untouched state value %v", resp.PlanValue, stateVal)
	}
}

func TestPurchasedSchema_DistributionAppDeliveryMethodHasNullWhenConfigNullModifier(t *testing.T) {
	attribute, ok := purchasedDistributionAttributes(t)["app_delivery_method"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("app_delivery_method is not a schema.StringAttribute: %T", purchasedDistributionAttributes(t)["app_delivery_method"])
	}
	if !purchasedHasStringModifierType(attribute.PlanModifiers, nullWhenConfigNullStringModifier{}) {
		t.Fatalf("app_delivery_method PlanModifiers %v does not contain nullWhenConfigNullStringModifier", attribute.PlanModifiers)
	}
}

// TestPurchasedSchema_DistributionEffectiveDateHasNoRemovalModifier pins that
// effective_date deliberately does NOT get the removal modifier (internal-task
// item 4): it is unusable on VPP assignments (the plan-time validator
// rejects setting it), so there is nothing to remove.
func TestPurchasedSchema_DistributionEffectiveDateHasNoRemovalModifier(t *testing.T) {
	attribute, ok := purchasedDistributionAttributes(t)["effective_date"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("effective_date is not a schema.StringAttribute: %T", purchasedDistributionAttributes(t)["effective_date"])
	}
	if purchasedHasStringModifierType(attribute.PlanModifiers, nullWhenConfigNullStringModifier{}) {
		t.Fatalf("effective_date PlanModifiers %v unexpectedly contains nullWhenConfigNullStringModifier", attribute.PlanModifiers)
	}
}

// TestPurchasedSchema_RedeemedHasNoUseStateForUnknownModifier pins that
// redeemed deliberately does NOT carry
// int64planmodifier.UseStateForUnknown() (internal-task item 2, ruling): it is
// index-paired within license_usage, so a reorder or a redemption happening
// between plan and apply could carry forward the wrong value for the index
// it lands on after the PUT went through, producing "inconsistent result
// after apply". The safe choice is the "(known after apply)" noise, tracked
// by internal-task.
func TestPurchasedSchema_RedeemedHasNoUseStateForUnknownModifier(t *testing.T) {
	distribution := purchasedDistributionAttributes(t)
	vppAppDetails, ok := distribution["vpp_app_details"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("vpp_app_details is not a schema.SingleNestedAttribute: %T", distribution["vpp_app_details"])
	}
	licenseUsage, ok := vppAppDetails.Attributes["license_usage"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("license_usage is not a schema.ListNestedAttribute: %T", vppAppDetails.Attributes["license_usage"])
	}
	redeemed, ok := licenseUsage.NestedObject.Attributes["redeemed"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("redeemed is not a schema.Int64Attribute: %T", licenseUsage.NestedObject.Attributes["redeemed"])
	}
	for _, m := range redeemed.PlanModifiers {
		if fmt.Sprintf("%T", m) == "int64planmodifier.useStateForUnknownModifier" {
			t.Fatalf("redeemed PlanModifiers %v unexpectedly contains UseStateForUnknown", redeemed.PlanModifiers)
		}
	}
}

func TestPurchasedSchema_RestrictionFieldsWiring(t *testing.T) {
	restriction := purchasedRestrictionAttributes(t)

	for _, name := range []string{"prevent_application_backup", "make_app_mdm_managed", "desired_state_management"} {
		attribute, ok := restriction[name].(schema.BoolAttribute)
		if !ok {
			t.Fatalf("%s is not a schema.BoolAttribute: %T", name, restriction[name])
		}
		if attribute.Default != nil {
			t.Fatalf("%s unexpectedly has a schema Default; item 2/4 require no Default here", name)
		}
		if !purchasedHasBoolModifierType(attribute.PlanModifiers, nullWhenConfigNullBoolModifier{}) {
			t.Fatalf("%s PlanModifiers %v does not contain nullWhenConfigNullBoolModifier", name, attribute.PlanModifiers)
		}
	}

	for _, name := range []string{"remove_on_unenroll", "prevent_removal"} {
		attribute, ok := restriction[name].(schema.BoolAttribute)
		if !ok {
			t.Fatalf("%s is not a schema.BoolAttribute: %T", name, restriction[name])
		}
		if attribute.Default == nil {
			t.Fatalf("%s lost its booldefault.StaticBool(false); it must stay unchanged", name)
		}
		if purchasedHasBoolModifierType(attribute.PlanModifiers, nullWhenConfigNullBoolModifier{}) {
			t.Fatalf("%s unexpectedly got nullWhenConfigNullBoolModifier; it must keep its Default(false) unchanged", name)
		}
	}

	// managed_access deliberately lost its booldefault.StaticBool(false)
	// (internal-task-65i, live-found perpetual diff): a schema Default
	// unconditionally overwrites the plan value whenever config is null,
	// even when Terraform core's proposed-new-state already carried the
	// prior true forward for an iOS assignment about to have managed_access
	// forced back on by modifyPlanForIOS. That stomp made plan != prior
	// state and forced every other computed attribute in the resource
	// unknown too. See managedAccessPlanModifier's doc comment in
	// resource.go for the full pipeline-order explanation.
	managedAccess, ok := restriction["managed_access"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("managed_access is not a schema.BoolAttribute: %T", restriction["managed_access"])
	}
	if managedAccess.Default != nil {
		t.Fatalf("managed_access unexpectedly has a schema Default; it must use managedAccessPlanModifier instead (internal-task-65i)")
	}
	if purchasedHasBoolModifierType(managedAccess.PlanModifiers, nullWhenConfigNullBoolModifier{}) {
		t.Fatalf("managed_access unexpectedly got nullWhenConfigNullBoolModifier; it must use managedAccessPlanModifier instead")
	}
	if !purchasedHasBoolModifierType(managedAccess.PlanModifiers, managedAccessPlanModifier{}) {
		t.Fatalf("managed_access PlanModifiers %v does not contain managedAccessPlanModifier", managedAccess.PlanModifiers)
	}
}

func TestPurchasedSchema_IsDynamicTemplateSavedHasNullWhenConfigNullModifier(t *testing.T) {
	attribute, ok := purchasedAssignmentsAttributes(t)["is_dynamic_template_saved"].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("is_dynamic_template_saved is not a schema.BoolAttribute: %T", purchasedAssignmentsAttributes(t)["is_dynamic_template_saved"])
	}
	if !purchasedHasBoolModifierType(attribute.PlanModifiers, nullWhenConfigNullBoolModifier{}) {
		t.Fatalf("is_dynamic_template_saved PlanModifiers %v does not contain nullWhenConfigNullBoolModifier", attribute.PlanModifiers)
	}
}
