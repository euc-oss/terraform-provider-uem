package state

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// --- Update body (ToAPI) omission tests (internal-task item 5a/5b) ---

func TestRestrictionToAPI_AllNullFlags_ReturnsNil(t *testing.T) {
	got := restrictionToAPI(&tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll:         types.BoolNull(),
		PreventRemoval:           types.BoolNull(),
		PreventApplicationBackup: types.BoolNull(),
		MakeAppMdmManaged:        types.BoolNull(),
		ManagedAccess:            types.BoolNull(),
		DesiredStateManagement:   types.BoolNull(),
	})
	if got != nil {
		t.Fatalf("expected a nil restriction body when every flag is null, got %#v", got)
	}
}

func TestRestrictionToAPI_PreventApplicationBackupNull_Omitted(t *testing.T) {
	got := restrictionToAPI(&tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll:         types.BoolValue(true),
		PreventRemoval:           types.BoolNull(),
		PreventApplicationBackup: types.BoolNull(),
		MakeAppMdmManaged:        types.BoolNull(),
		ManagedAccess:            types.BoolNull(),
		DesiredStateManagement:   types.BoolNull(),
	})
	if got == nil {
		t.Fatal("expected a non-nil restriction body")
	}
	if got.PreventApplicationBackup != nil {
		t.Fatalf("expected prevent_application_backup to be omitted, got %v", *got.PreventApplicationBackup)
	}
}

func TestDistributionToAPI_NullAppDeliveryMethod_OmitsField(t *testing.T) {
	api, diags := distributionToAPI(t.Context(), tf.PurchasedAppAssignmentDistributionModel{
		Name:              types.StringValue("VPP"),
		SmartGroups:       types.ListNull(types.StringType),
		AppDeliveryMethod: types.StringNull(),
		EffectiveDate:     types.StringNull(),
		VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: types.ListNull(vppLicenseUsageElemType)},
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.AppDeliveryMethod != "" {
		t.Fatalf("expected app_delivery_method to be omitted, got %q", api.AppDeliveryMethod)
	}
}

// TestRestrictionAndDistributionToAPI_RemovalAloneVsWithSiblingChange_IdenticalOmission
// proves removing prevent_application_backup/app_delivery_method alone and
// removing them alongside a sibling field change (description, and
// remove_on_unenroll) produce the identical omitted shape.
func TestRestrictionAndDistributionToAPI_RemovalAloneVsWithSiblingChange_IdenticalOmission(t *testing.T) {
	ctx := t.Context()

	buildDistribution := func(description string) tf.PurchasedAppAssignmentDistributionModel {
		return tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       types.StringValue(description),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: types.ListNull(vppLicenseUsageElemType)},
		}
	}
	removalAlone, diags := distributionToAPI(ctx, buildDistribution("original"))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	removalWithSibling, diags := distributionToAPI(ctx, buildDistribution("changed"))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if removalAlone.AppDeliveryMethod != removalWithSibling.AppDeliveryMethod || removalAlone.AppDeliveryMethod != "" {
		t.Fatalf("expected identical app_delivery_method omission, got %q vs %q", removalAlone.AppDeliveryMethod, removalWithSibling.AppDeliveryMethod)
	}

	buildRestriction := func(removeOnUnenroll bool) *tf.PurchasedAppAssignmentRestrictionModel {
		return &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(removeOnUnenroll),
			PreventRemoval:           types.BoolNull(),
			PreventApplicationBackup: types.BoolNull(),
			MakeAppMdmManaged:        types.BoolNull(),
			ManagedAccess:            types.BoolNull(),
			DesiredStateManagement:   types.BoolNull(),
		}
	}
	restrictionAlone := restrictionToAPI(buildRestriction(false))
	restrictionWithSibling := restrictionToAPI(buildRestriction(true))
	if restrictionAlone.PreventApplicationBackup != nil || restrictionWithSibling.PreventApplicationBackup != nil {
		t.Fatalf("expected identical prevent_application_backup omission, got %#v vs %#v", restrictionAlone.PreventApplicationBackup, restrictionWithSibling.PreventApplicationBackup)
	}
}
