package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// priorRule builds a prior plan/state model holding a single assignment with
// the given description and restriction. A nil restriction is a null object.
func priorRule(t *testing.T, description types.String, restriction *tf.PurchasedAppAssignmentRestrictionModel) *tf.PurchasedAppAssignmentRuleModel {
	t.Helper()
	ctx := context.Background()
	a := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       description,
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: types.ListNull(vppLicenseUsageElemType)},
		},
		Restriction:              restriction,
		ApplicationConfiguration: types.ListNull(appConfigurationElemType),
		ApplicationAttributes:    types.ListNull(appConfigurationElemType),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
	list, diags := types.ListValueFrom(ctx, assignmentElemType, []tf.PurchasedAppAssignmentModel{a})
	if diags.HasError() {
		t.Fatalf("failed to build prior assignments: %v", diags)
	}
	return &tf.PurchasedAppAssignmentRuleModel{
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         list,
	}
}

// apiRule builds a GET response with one assignment carrying the given
// description and restriction.
func apiRule(description string, restriction *sdk.AppAssignmentRestrictionV1ModelV2) *sdk.AppAssignmentRuleV2Model {
	return &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Priority: 0,
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name:        "VPP",
				Description: description,
			},
			Restriction: restriction,
		}},
	}
}

// allFalseRestriction is the server's default restriction shape when nothing
// was configured.
func allFalseRestriction() *sdk.AppAssignmentRestrictionV1ModelV2 {
	f := false
	return &sdk.AppAssignmentRestrictionV1ModelV2{
		RemoveOnUnenroll:         &f,
		PreventRemoval:           &f,
		PreventApplicationBackup: &f,
		MakeAppMdmManaged:        &f,
		ManagedAccess:            &f,
		DesiredStateManagement:   &f,
	}
}

func readSingleAssignment(t *testing.T, data *tf.PurchasedAppAssignmentRuleModel, api *sdk.AppAssignmentRuleV2Model) tf.PurchasedAppAssignmentModel {
	t.Helper()
	ctx := context.Background()
	if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}
	return assigns[0]
}

func TestReadAPIIntoState_Restriction_PriorNullServerAllFalse_KeepsNull(t *testing.T) {
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), nil), apiRule("", allFalseRestriction()))
	if got.Restriction != nil {
		t.Fatalf("expected null restriction when prior was null and server sent all-default, got %#v", got.Restriction)
	}
}

func TestReadAPIIntoState_Restriction_NoPriorServerAllFalse_KeepsNull(t *testing.T) {
	// Import path: no prior assignments at all.
	got := readSingleAssignment(t, &tf.PurchasedAppAssignmentRuleModel{}, apiRule("", allFalseRestriction()))
	if got.Restriction != nil {
		t.Fatalf("expected null restriction on import of all-default server shape, got %#v", got.Restriction)
	}
}

func TestReadAPIIntoState_Restriction_PriorNullServerHasTrue_SurfacesDrift(t *testing.T) {
	api := allFalseRestriction()
	tr := true
	api.PreventRemoval = &tr
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), nil), apiRule("", api))
	if got.Restriction == nil {
		t.Fatal("expected restriction object for non-default server state, got null")
	}
	if !got.Restriction.PreventRemoval.ValueBool() {
		t.Fatalf("expected prevent_removal=true, got %v", got.Restriction.PreventRemoval)
	}
	if got.Restriction.RemoveOnUnenroll.IsNull() || got.Restriction.RemoveOnUnenroll.ValueBool() {
		t.Fatalf("expected remove_on_unenroll=false, got %v", got.Restriction.RemoveOnUnenroll)
	}
}

func TestReadAPIIntoState_Restriction_PriorNonNullServerAllFalse_TakesServer(t *testing.T) {
	prior := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll:         types.BoolValue(true),
		PreventRemoval:           types.BoolValue(true),
		PreventApplicationBackup: types.BoolNull(),
		MakeAppMdmManaged:        types.BoolNull(),
		ManagedAccess:            types.BoolValue(true),
		DesiredStateManagement:   types.BoolNull(),
	}
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), prior), apiRule("", allFalseRestriction()))
	if got.Restriction == nil {
		t.Fatal("expected restriction object when prior was configured, got null")
	}
	if got.Restriction.RemoveOnUnenroll.ValueBool() || got.Restriction.PreventRemoval.ValueBool() || got.Restriction.ManagedAccess.ValueBool() {
		t.Fatalf("expected server's all-false values to surface as drift, got %#v", got.Restriction)
	}
}
