package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
)

func TestEmptyAppAssignmentRuleToAPI(t *testing.T) {
	api := EmptyAppAssignmentRuleToAPI()
	if api == nil {
		t.Fatal("expected non-nil API payload")
	}
	if api.Assignments == nil {
		t.Fatal("expected assignments to be an explicit empty slice")
	}
	if len(api.Assignments) != 0 {
		t.Fatalf("expected 0 assignments, got %d", len(api.Assignments))
	}
	if api.ExcludedSmartGroups == nil {
		t.Fatal("expected excluded smart groups to be an explicit empty slice")
	}
	if len(api.ExcludedSmartGroups) != 0 {
		t.Fatalf("expected 0 excluded smart groups, got %d", len(api.ExcludedSmartGroups))
	}
	if api.ApplicationMsiDeploymentParams == nil {
		t.Fatal("expected application_msi_deployment_params to be an explicit empty object")
	}
}

func TestAppAssignmentRuleToAPI_MapsCoreFields(t *testing.T) {
	ctx := context.Background()

	excluded, exDiags := types.ListValueFrom(ctx, types.StringType, []string{"sg-excluded-1"})
	if exDiags.HasError() {
		t.Fatalf("failed to build excluded list: %v", exDiags)
	}
	smartGroups, sgDiags := types.ListValueFrom(ctx, types.StringType, []string{"sg-1", "sg-2"})
	if sgDiags.HasError() {
		t.Fatalf("failed to build smart groups list: %v", sgDiags)
	}

	assignment := models.AppAssignmentModel{
		Priority: types.Int64Value(7),
		Distribution: models.AppAssignmentDistributionModel{
			Name:              types.StringValue("Prod Rollout"),
			Description:       types.StringValue("Main rollout assignment"),
			SmartGroups:       smartGroups,
			AppDeliveryMethod: types.StringValue("AUTO"),
			EffectiveDate:     types.StringValue("2026-04-23T00:00:00Z"),
		},
		Restriction: &models.AppAssignmentRestrictionModel{
			RemoveOnUnenroll: types.BoolValue(true),
		},
	}

	assignments, aDiags := types.ListValueFrom(ctx, assignmentElemType, []models.AppAssignmentModel{assignment})
	if aDiags.HasError() {
		t.Fatalf("failed to build assignments list: %v", aDiags)
	}

	model := &models.AppAssignmentRuleModel{
		ExcludedSmartGroups: excluded,
		Assignments:         assignments,
	}

	api, diags := AppAssignmentRuleToAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api == nil {
		t.Fatal("expected non-nil api model")
	}
	if len(api.ExcludedSmartGroups) != 1 || api.ExcludedSmartGroups[0] != "sg-excluded-1" {
		t.Fatalf("unexpected excluded smart groups: %#v", api.ExcludedSmartGroups)
	}
	if len(api.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %d", len(api.Assignments))
	}

	got := api.Assignments[0]
	if got.Priority == nil || *got.Priority != 7 {
		t.Fatalf("unexpected priority: %#v", got.Priority)
	}
	if got.Distribution.Name != "Prod Rollout" {
		t.Fatalf("unexpected distribution name: %q", got.Distribution.Name)
	}
	if got.Distribution.AppDeliveryMethod != "AUTO" {
		t.Fatalf("unexpected app delivery method value: %#v", got.Distribution.AppDeliveryMethod)
	}
	if got.Restriction == nil || got.Restriction.RemoveOnUnenroll == nil || !*got.Restriction.RemoveOnUnenroll {
		t.Fatalf("restriction.remove_on_unenroll not mapped: %#v", got.Restriction)
	}
}

func TestReadAPIIntoState_AssignmentDefaultsAndNormalization(t *testing.T) {
	ctx := context.Background()

	api := &sdk.AppAssignmentRuleV2Model{
		ExcludedSmartGroups: []string{"SG-EXCLUDED"},
		Assignments: []sdk.AppAssignmentV2Model{
			{
				// Priority intentionally nil: mapper should default to zero.
				Distribution: sdk.AppAssignmentDistributionV2Model{
					Name:              "Dev Rollout",
					Description:       "case normalization",
					SmartGroups:       []string{"SG-A", "sg-b"},
					AppDeliveryMethod: "auto",
				},
			},
		},
	}

	data := &models.AppAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var excluded []types.String
	diags = data.ExcludedSmartGroups.ElementsAs(ctx, &excluded, false)
	if diags.HasError() {
		t.Fatalf("failed to decode excluded_smart_groups: %v", diags)
	}
	if len(excluded) != 1 || excluded[0].ValueString() != "sg-excluded" {
		t.Fatalf("unexpected excluded_smart_groups: %#v", excluded)
	}

	var assigns []models.AppAssignmentModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}

	if assigns[0].Priority.ValueInt64() != 0 {
		t.Fatalf("expected default priority 0, got %d", assigns[0].Priority.ValueInt64())
	}
	if assigns[0].Distribution.AppDeliveryMethod.ValueString() != "AUTO" {
		t.Fatalf("expected app_delivery_method AUTO, got %q", assigns[0].Distribution.AppDeliveryMethod.ValueString())
	}

	var smartGroups []types.String
	diags = assigns[0].Distribution.SmartGroups.ElementsAs(ctx, &smartGroups, false)
	if diags.HasError() {
		t.Fatalf("failed to decode smart_groups: %v", diags)
	}
	if len(smartGroups) != 2 || smartGroups[0].ValueString() != "sg-a" || smartGroups[1].ValueString() != "sg-b" {
		t.Fatalf("unexpected smart_groups normalization: %#v", smartGroups)
	}
}
