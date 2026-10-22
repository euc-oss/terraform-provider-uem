package macscript

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// nonNullPriorState is a prior state whose Raw value is merely non-null --
// PlanModifyInt64/PlanModifyBool's "Do nothing if there is no state (resource
// is being created)" check only looks at req.State.Raw.IsNull(), so this
// need not match any real schema/type to stand in for "an existing resource,
// not a fresh create" in these direct modifier-level tests.
func nonNullPriorState() tfsdk.State {
	return tfsdk.State{Raw: tftypes.NewValue(tftypes.Bool, true)}
}

// B34: timeout and user_interaction must be Optional+Computed (not
// Optional-only) so a configuration that omits either plans no diff once a
// value is recorded in state -- see the plan-modifier tests below for the
// mechanism this schema shape enables.
func TestTimeoutSchema_OptionalComputedUseStateForUnknown(t *testing.T) {
	attr, ok := resourceSchema(t).Schema.Attributes["timeout"]
	if !ok {
		t.Fatal("schema has no timeout attribute")
	}
	if !attr.IsOptional() {
		t.Error("timeout must be Optional")
	}
	if !attr.IsComputed() {
		t.Error("timeout must be Computed (B34: needed so an omitted config plans no diff against a known state value)")
	}
	if attr.IsRequired() {
		t.Error("timeout must not be Required")
	}
}

func TestUserInteractionSchema_OptionalComputedUseStateForUnknown(t *testing.T) {
	attr, ok := resourceSchema(t).Schema.Attributes["user_interaction"]
	if !ok {
		t.Fatal("schema has no user_interaction attribute")
	}
	if !attr.IsOptional() {
		t.Error("user_interaction must be Optional")
	}
	if !attr.IsComputed() {
		t.Error("user_interaction must be Computed (B34: needed so an omitted config plans no diff against a known state value)")
	}
	if attr.IsRequired() {
		t.Error("user_interaction must not be Required")
	}
}

// TestTimeoutPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange exercises
// the stock int64planmodifier.UseStateForUnknown() directly, the same way the
// framework core invokes it: a Computed attribute whose config is null is
// first marked Unknown in the plan (terraform-plugin-framework's
// server_planresourcechange.go does this for every Computed attribute,
// independent of any modifier); UseStateForUnknown then copies the known
// prior state (30) into the plan instead of leaving it Unknown, so a
// configuration that omits timeout against a state of 30 plans no change.
func TestTimeoutPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange(t *testing.T) {
	req := planmodifier.Int64Request{
		State:       nonNullPriorState(),
		ConfigValue: types.Int64Null(),
		StateValue:  types.Int64Value(30),
		PlanValue:   types.Int64Unknown(), // what the framework core sets before modifiers run
	}
	resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
	int64planmodifier.UseStateForUnknown().PlanModifyInt64(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.Int64Value(30)) {
		t.Fatalf("planned timeout = %v, want 30 (no diff against known state)", resp.PlanValue)
	}
}

// A set config value is not affected by UseStateForUnknown at all (the
// framework core never marks a configured attribute Unknown in the first
// place), so it is simply sent through as planned/configured.
func TestTimeoutPlanModifier_SetConfigValueIsSent(t *testing.T) {
	req := planmodifier.Int64Request{
		ConfigValue: types.Int64Value(45),
		StateValue:  types.Int64Value(30),
		PlanValue:   types.Int64Value(45),
	}
	resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
	int64planmodifier.UseStateForUnknown().PlanModifyInt64(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.Int64Value(45)) {
		t.Fatalf("planned timeout = %v, want 45 (explicit config value)", resp.PlanValue)
	}
}

func TestUserInteractionPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange(t *testing.T) {
	req := planmodifier.BoolRequest{
		State:       nonNullPriorState(),
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(false),
		PlanValue:   types.BoolUnknown(),
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	boolplanmodifier.UseStateForUnknown().PlanModifyBool(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.BoolValue(false)) {
		t.Fatalf("planned user_interaction = %v, want false (no diff against known state)", resp.PlanValue)
	}
}

func TestUserInteractionPlanModifier_SetConfigValueIsSent(t *testing.T) {
	req := planmodifier.BoolRequest{
		ConfigValue: types.BoolValue(true),
		StateValue:  types.BoolValue(false),
		PlanValue:   types.BoolValue(true),
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	boolplanmodifier.UseStateForUnknown().PlanModifyBool(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.BoolValue(true)) {
		t.Fatalf("planned user_interaction = %v, want true (explicit config value)", resp.PlanValue)
	}
}
