package macscript

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// B39: description must be Optional+Computed (not Optional-only), the same
// shape B34 gave timeout/user_interaction, so a configuration that omits it
// plans Unknown rather than a fixed null. Live (as<internal-env> 26.2): a script
// created with description unset planned null (Optional-only) but UEM's
// readback is "" (never null), and apply failed with "Provider produced
// inconsistent result after apply".
//
// Revert check: dropping Computed (and the UseStateForUnknown modifier)
// makes this test fail.
func TestDescriptionSchema_OptionalComputedUseStateForUnknown(t *testing.T) {
	attr, ok := resourceSchema(t).Schema.Attributes["description"]
	if !ok {
		t.Fatal("schema has no description attribute")
	}
	if !attr.IsOptional() {
		t.Error("description must be Optional")
	}
	if !attr.IsComputed() {
		t.Error("description must be Computed (B39: needed so an omitted config plans Unknown, not a fixed null, against UEM's \"\" readback)")
	}
	if attr.IsRequired() {
		t.Error("description must not be Required")
	}
}

// TestDescriptionPlanModifier_OmittedConfigResolvesToUEMValue exercises the
// stock stringplanmodifier.UseStateForUnknown() directly, the same way B34's
// timeout/user_interaction tests do: a Computed attribute whose config is
// null is first marked Unknown in the plan by the framework core; on a
// CREATE (no prior state) UseStateForUnknown leaves it Unknown, which may
// then resolve to anything -- including UEM's "" -- without an "inconsistent
// result after apply".
func TestDescriptionPlanModifier_OmittedConfigResolvesToUEMValue(t *testing.T) {
	req := planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		PlanValue:   types.StringUnknown(), // what the framework core sets before modifiers run
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	stringplanmodifier.UseStateForUnknown().PlanModifyString(context.Background(), req, resp)
	if !resp.PlanValue.IsUnknown() {
		t.Fatalf("planned description = %v, want still Unknown (no prior state to carry forward on create)", resp.PlanValue)
	}
}

// TestDescriptionPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange
// mirrors schema_b34_test.go's timeout/user_interaction case: against an
// existing resource's known prior state, UseStateForUnknown copies that
// state forward instead of leaving the plan Unknown, so a config that omits
// description plans no change once a value (including UEM's own "") is
// recorded in state.
func TestDescriptionPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange(t *testing.T) {
	req := planmodifier.StringRequest{
		State:       nonNullPriorState(),
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue(""),
		PlanValue:   types.StringUnknown(),
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	stringplanmodifier.UseStateForUnknown().PlanModifyString(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.StringValue("")) {
		t.Fatalf("planned description = %v, want \"\" (no diff against known state)", resp.PlanValue)
	}
}

// A set config value is not affected by UseStateForUnknown at all (the
// framework core never marks a configured attribute Unknown in the first
// place), so it is simply sent through as planned/configured.
func TestDescriptionPlanModifier_SetConfigValueIsSent(t *testing.T) {
	req := planmodifier.StringRequest{
		ConfigValue: types.StringValue("a description"),
		StateValue:  types.StringValue(""),
		PlanValue:   types.StringValue("a description"),
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	stringplanmodifier.UseStateForUnknown().PlanModifyString(context.Background(), req, resp)
	if !resp.PlanValue.Equal(types.StringValue("a description")) {
		t.Fatalf("planned description = %v, want %q (explicit config value)", resp.PlanValue, "a description")
	}
}

// B40: platform's documented example value is UEM's real wire enum
// ("APPLE_OSX"), not the V2 Profiles API's "AppleOsX" (sdk.PlatformAppleOsX).
// Live (as<internal-env> 26.2): a script created with platform = "AppleOsX" applied,
// but UEM's readback was "APPLE_OSX"; platform is Required (not Computed),
// so that mismatch alone is a "Provider produced inconsistent result after
// apply" -- Required attributes must match the planned value exactly, the
// same as Computed ones must resolve without conflict. Unlike description
// (B39), platform does not become Optional/Computed here: it stays Required
// with RequiresReplace, since nothing about this bug was about an omitted
// config -- it was the practitioner-supplied value not matching what UEM
// echoes back.
//
// Revert check: reverting the MarkdownDescription text back to "AppleOsX"
// makes this test fail.
func TestPlatformSchema_DocumentsUEMWireValue(t *testing.T) {
	attr, ok := resourceSchema(t).Schema.Attributes["platform"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("platform is not a StringAttribute: %T", resourceSchema(t).Schema.Attributes["platform"])
	}
	if !attr.IsRequired() {
		t.Error("platform must remain Required (B40 did not change this)")
	}
	if attr.IsComputed() {
		t.Error("platform must remain non-Computed (B40 did not change this)")
	}
	if strings.Contains(attr.MarkdownDescription, "AppleOsX") {
		t.Errorf("platform MarkdownDescription still names the Profiles-API spelling AppleOsX: %q", attr.MarkdownDescription)
	}
	if !strings.Contains(attr.MarkdownDescription, "APPLE_OSX") {
		t.Errorf("platform MarkdownDescription must name UEM's actual wire value APPLE_OSX: %q", attr.MarkdownDescription)
	}
}
