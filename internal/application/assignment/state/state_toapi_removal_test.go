package state

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"testing"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
)

// --- Update body (ToAPI) omission tests (internal-task item 5a/5b) ---
//
// distributionToAPI already treats a null AppDeliveryMethod/EffectiveDate as
// omitted from the request body — these tests pin that, and prove removal
// alone and removal alongside a sibling field change produce the identical
// omission.

func TestDistributionToAPI_NullAppDeliveryMethod_OmitsField(t *testing.T) {
	api, diags := distributionToAPI(t.Context(), tf.AppAssignmentDistributionModel{
		Name:              types.StringValue("n"),
		SmartGroups:       types.ListNull(types.StringType),
		AppDeliveryMethod: types.StringNull(),
		EffectiveDate:     types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.AppDeliveryMethod != "" {
		t.Fatalf("expected app_delivery_method to be omitted, got %q", api.AppDeliveryMethod)
	}
}

func TestDistributionToAPI_NullEffectiveDate_OmitsField(t *testing.T) {
	api, diags := distributionToAPI(t.Context(), tf.AppAssignmentDistributionModel{
		Name:              types.StringValue("n"),
		SmartGroups:       types.ListNull(types.StringType),
		AppDeliveryMethod: types.StringNull(),
		EffectiveDate:     types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !api.EffectiveDate.IsZero() {
		t.Fatalf("expected effective_date to be omitted (zero), got %v", api.EffectiveDate)
	}
}

// TestDistributionToAPI_RemovalAloneVsWithSiblingChange_IdenticalOmission
// builds two distribution models — one where only app_delivery_method and
// effective_date are removed, another where they are removed AND a sibling
// (description) also changed — and asserts both produce the identical
// (omitted) shape for the removed fields.
func TestDistributionToAPI_RemovalAloneVsWithSiblingChange_IdenticalOmission(t *testing.T) {
	ctx := t.Context()

	removalAlone, diags := distributionToAPI(ctx, tf.AppAssignmentDistributionModel{
		Name:              types.StringValue("n"),
		Description:       types.StringValue("original"),
		SmartGroups:       types.ListNull(types.StringType),
		AppDeliveryMethod: types.StringNull(),
		EffectiveDate:     types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	removalWithSibling, diags := distributionToAPI(ctx, tf.AppAssignmentDistributionModel{
		Name:              types.StringValue("n"),
		Description:       types.StringValue("changed"),
		SmartGroups:       types.ListNull(types.StringType),
		AppDeliveryMethod: types.StringNull(),
		EffectiveDate:     types.StringNull(),
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if removalAlone.AppDeliveryMethod != removalWithSibling.AppDeliveryMethod {
		t.Fatalf("expected identical app_delivery_method omission, got %q vs %q", removalAlone.AppDeliveryMethod, removalWithSibling.AppDeliveryMethod)
	}
	if removalAlone.EffectiveDate.IsZero() != removalWithSibling.EffectiveDate.IsZero() {
		t.Fatalf("expected identical effective_date omission, got %v vs %v", removalAlone.EffectiveDate, removalWithSibling.EffectiveDate)
	}
	if removalAlone.AppDeliveryMethod != "" || !removalAlone.EffectiveDate.IsZero() {
		t.Fatalf("expected both fields omitted, got %#v", removalAlone)
	}
}
