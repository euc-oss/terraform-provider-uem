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

// --- nullWhenConfigNull{List,String} direct modifier tests (internal-task) ---
//
// These mirror internal/profile/resource.go's nullWhenConfigNull* tests: the
// modifier's PlanModifyX only ever looks at req.ConfigValue, never at any
// sibling attribute, so a removal-alone plan and a removal-with-sibling-
// change plan are structurally guaranteed to produce the identical planned
// value for the removed attribute — there is no code path by which a
// sibling's value could reach this modifier's decision.

func TestNullWhenConfigNullListModifier_PlanModifyList(t *testing.T) {
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
		// Removal-alone: Terraform core's proposed new state carries the
		// prior value forward when nothing else in the plan disturbs it, so
		// the modifier receives PlanValue = StateValue (known), not Unknown.
		stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("from-state")})
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
		planVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("configured")})
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

// TestNullWhenConfigNullListModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan
// exercises the (a)/(b) removal-alone vs removal-with-sibling-change
// requirement directly: two independent PlanModifyList invocations — one
// modeling "only this attribute changed", another modeling "a sibling also
// changed" (which this modifier's request type cannot even represent, since
// planmodifier.ListRequest carries no sibling attributes) — must produce
// bit-identical planned values.
func TestNullWhenConfigNullListModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan(t *testing.T) {
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

func TestNullWhenConfigNullStringModifier_PlanModifyString(t *testing.T) {
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
			ConfigValue: types.StringValue("2026-04-23T00:00:00Z"),
			StateValue:  types.StringValue("2026-04-23T00:00:00Z"),
			PlanValue:   types.StringValue("2026-04-23T00:00:00Z"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "2026-04-23T00:00:00Z" {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

// TestNullWhenConfigNullStringModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan
// is the String-typed equivalent covering distribution.app_delivery_method
// and distribution.effective_date: removal alone and removal alongside a
// sibling field change (e.g. distribution.description) must plan the
// removed attribute identically, since this modifier is a pure function of
// its own ConfigValue.
func TestNullWhenConfigNullStringModifier_RemovalAloneVsWithSiblingChange_IdenticalPlan(t *testing.T) {
	t.Parallel()

	// Removal-alone: proposed new state carries the prior value forward, so
	// PlanValue is the known prior state value, not Unknown.
	removalAlone := &planmodifier.StringResponse{PlanValue: types.StringValue("2026-04-23T00:00:00Z")}
	nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue("2026-04-23T00:00:00Z"),
		PlanValue:   types.StringValue("2026-04-23T00:00:00Z"),
	}, removalAlone)

	removalWithSibling := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
	nullWhenConfigNullStringModifier{}.PlanModifyString(context.Background(), planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue("2026-04-23T00:00:00Z"),
		PlanValue:   types.StringUnknown(),
	}, removalWithSibling)

	if !removalAlone.PlanValue.Equal(removalWithSibling.PlanValue) {
		t.Fatalf("expected identical planned values, got %v vs %v", removalAlone.PlanValue, removalWithSibling.PlanValue)
	}
	if !removalAlone.PlanValue.IsNull() {
		t.Fatalf("expected null planned value (not unknown), got %v", removalAlone.PlanValue)
	}
}

// --- schema-wiring tests: pin that each attribute from the Q4 verdict list
// carries the null-when-config-null modifier (internal-task item 1h). ---

func schemaFor(t *testing.T) schema.Schema {
	t.Helper()
	r := &applicationAssignmentResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func hasModifierType(modifiers []planmodifier.String, target any) bool {
	want := fmt.Sprintf("%T", target)
	for _, m := range modifiers {
		if fmt.Sprintf("%T", m) == want {
			return true
		}
	}
	return false
}

func hasListModifierType(modifiers []planmodifier.List, target any) bool {
	want := fmt.Sprintf("%T", target)
	for _, m := range modifiers {
		if fmt.Sprintf("%T", m) == want {
			return true
		}
	}
	return false
}

func TestSchema_ExcludedSmartGroupsHasNullWhenConfigNullModifier(t *testing.T) {
	s := schemaFor(t)
	attribute, ok := s.Attributes["excluded_smart_groups"].(schema.ListAttribute)
	if !ok {
		t.Fatalf("excluded_smart_groups is not a schema.ListAttribute: %T", s.Attributes["excluded_smart_groups"])
	}
	if !hasListModifierType(attribute.PlanModifiers, nullWhenConfigNullListModifier{}) {
		t.Fatalf("excluded_smart_groups PlanModifiers %v does not contain nullWhenConfigNullListModifier", attribute.PlanModifiers)
	}
}

func assignmentsDistributionAttributes(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	s := schemaFor(t)
	assignments, ok := s.Attributes["assignments"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("assignments is not a schema.ListNestedAttribute: %T", s.Attributes["assignments"])
	}
	distribution, ok := assignments.NestedObject.Attributes["distribution"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("distribution is not a schema.SingleNestedAttribute: %T", assignments.NestedObject.Attributes["distribution"])
	}
	return distribution.Attributes
}

func TestSchema_DistributionAppDeliveryMethodHasNullWhenConfigNullModifier(t *testing.T) {
	attrs := assignmentsDistributionAttributes(t)
	attribute, ok := attrs["app_delivery_method"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("app_delivery_method is not a schema.StringAttribute: %T", attrs["app_delivery_method"])
	}
	if !hasModifierType(attribute.PlanModifiers, nullWhenConfigNullStringModifier{}) {
		t.Fatalf("app_delivery_method PlanModifiers %v does not contain nullWhenConfigNullStringModifier", attribute.PlanModifiers)
	}
}

func TestSchema_DistributionEffectiveDateHasNullWhenConfigNullModifier(t *testing.T) {
	attrs := assignmentsDistributionAttributes(t)
	attribute, ok := attrs["effective_date"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("effective_date is not a schema.StringAttribute: %T", attrs["effective_date"])
	}
	if !hasModifierType(attribute.PlanModifiers, nullWhenConfigNullStringModifier{}) {
		t.Fatalf("effective_date PlanModifiers %v does not contain nullWhenConfigNullStringModifier", attribute.PlanModifiers)
	}
}
