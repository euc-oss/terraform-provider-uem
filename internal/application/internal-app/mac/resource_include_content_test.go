package macapplication

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// TestMacApplicationIncludeContentTogglesRequireReplace guards the gap the
// generic no-op Update test above cannot see: include_content is the one
// mutable attribute that reaches Update in practice (every other mutable
// attribute is already RequiresReplace). Only Create populates
// dmg_content_base64/plist_content_base64, and those Computed fields use
// UseStateForUnknown, so a no-op Update toggling include_content would
// silently leave stale/null content instead of actually (de)populating it.
// The include_content attribute must therefore be RequiresReplace itself so a toggle goes
// through Create (which does populate correctly) rather than Update.
func TestMacApplicationIncludeContentTogglesRequireReplace(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attr, ok := schemaResp.Schema.Attributes["include_content"].(rschema.BoolAttribute)
	if !ok {
		t.Fatal("include_content is not a BoolAttribute")
	}
	if len(attr.PlanModifiers) == 0 {
		t.Fatal("expected include_content to carry a RequiresReplace plan modifier, found none")
	}

	ctx := context.Background()

	stateData := tf.MacApplicationResourceModel{
		ID:             typesInt32(42),
		UUID:           typesString("9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		OrgGroupID:     typesInt32(123),
		DMGFilePath:    typesString("/tmp/app.dmg"),
		PlistFilePath:  typesString("/tmp/app.plist"),
		AppVersion:     tf.NewAppVersionValue("1.0.0"),
		DMGFileSHA256:  typesString("deadbeef"),
		IncludeContent: types.BoolValue(false),
	}
	planData := stateData
	planData.IncludeContent = types.BoolValue(true)

	state := emptyMacApplicationState()
	stateData = withTypedRecordNulls(stateData)
	if diags := state.Set(ctx, &stateData); diags.HasError() {
		t.Fatalf("unexpected error building state: %v", diags)
	}
	plan := newMacApplicationPlan(t, planData)

	req := planmodifier.BoolRequest{
		State:      state,
		Plan:       plan,
		Config:     tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
		StateValue: types.BoolValue(false),
		PlanValue:  types.BoolValue(true),
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}

	attr.PlanModifiers[0].PlanModifyBool(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}
	if !resp.RequiresReplace {
		t.Fatal("expected toggling include_content false->true to require replace, but it did not")
	}
}

// TestMacApplicationIncludeContentUnchanged_NoReplace is the control case:
// an unchanged include_content value must NOT force a replace.
func TestMacApplicationIncludeContentUnchanged_NoReplace(t *testing.T) {
	var schemaResp resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attr, ok := schemaResp.Schema.Attributes["include_content"].(rschema.BoolAttribute)
	if !ok {
		t.Fatal("include_content is not a BoolAttribute")
	}

	ctx := context.Background()

	data := tf.MacApplicationResourceModel{
		ID:             typesInt32(42),
		UUID:           typesString("9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		OrgGroupID:     typesInt32(123),
		DMGFilePath:    typesString("/tmp/app.dmg"),
		PlistFilePath:  typesString("/tmp/app.plist"),
		AppVersion:     tf.NewAppVersionValue("1.0.0"),
		DMGFileSHA256:  typesString("deadbeef"),
		IncludeContent: types.BoolValue(true),
	}

	state := emptyMacApplicationState()
	data = withTypedRecordNulls(data)
	if diags := state.Set(ctx, &data); diags.HasError() {
		t.Fatalf("unexpected error building state: %v", diags)
	}
	plan := newMacApplicationPlan(t, data)

	req := planmodifier.BoolRequest{
		State:      state,
		Plan:       plan,
		Config:     tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
		StateValue: types.BoolValue(true),
		PlanValue:  types.BoolValue(true),
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}

	attr.PlanModifiers[0].PlanModifyBool(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}
	if resp.RequiresReplace {
		t.Fatal("expected an unchanged include_content value not to require replace")
	}
}
