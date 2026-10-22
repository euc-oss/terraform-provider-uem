package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
)

func TestEmptyScriptAssignmentRuleToAPI(t *testing.T) {
	api := EmptyScriptAssignmentRuleToAPI()
	if api == nil {
		t.Fatal("expected non-nil API payload")
	}
	if api.Assignments == nil {
		t.Fatal("expected assignments to be an explicit empty slice")
	}
	if len(api.Assignments) != 0 {
		t.Fatalf("expected 0 assignments, got %d", len(api.Assignments))
	}
}

func TestScriptAssignmentRuleToAPI_MapsCoreFields(t *testing.T) {
	ctx := context.Background()

	memberships, mDiags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"smart_group_uuid": types.StringType,
			"smart_group_name": types.StringType,
		},
	}, []models.MembershipModel{
		{
			SmartGroupUUID: types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
			SmartGroupName: types.StringValue("utk (utk)"),
		},
	})
	if mDiags.HasError() {
		t.Fatalf("failed to build memberships list: %v", mDiags)
	}

	triggerEvents, teDiags := types.ListValueFrom(ctx, types.StringType, []string{
		"RUN_IMMEDIATELY", "LOGIN", "LOGOUT", "STARTUP", "NETWORK_CHANGE",
	})
	if teDiags.HasError() {
		t.Fatalf("failed to build trigger events list: %v", teDiags)
	}

	assignment := models.ScriptAssignmentModel{
		AssignmentUUID: types.StringValue("12b9377c-be7e-5c94-e8a7-0d9d23929523"),
		Name:           types.StringValue("test"),
		Priority:       types.Int64Value(1),
		DeploymentMode: types.StringValue("AUTO"),
		ShowInCatalog:  types.BoolValue(true),
		Memberships:    memberships,
		ScriptDeployment: models.ScriptDeploymentModel{
			TriggerType:     types.StringValue("SCHEDULE_AND_EVENT"),
			TriggerEvents:   triggerEvents,
			TriggerSchedule: types.StringValue("FOUR_HOURS"),
		},
	}

	assignments, aDiags := types.ListValueFrom(ctx, assignmentElemType, []models.ScriptAssignmentModel{assignment})
	if aDiags.HasError() {
		t.Fatalf("failed to build assignments list: %v", aDiags)
	}

	model := &models.ScriptAssignmentRuleModel{
		ScriptUUID:  types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
		Assignments: assignments,
	}

	api, diags := ScriptAssignmentRuleToAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api == nil {
		t.Fatal("expected non-nil api model")
	}
	if len(api.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %d", len(api.Assignments))
	}

	got := api.Assignments[0]
	if got.ScripUUID != "bafde89c-041e-1756-082b-933aaf16cad8" {
		t.Fatalf("unexpected script uuid: %q", got.ScripUUID)
	}
	if got.Name != "test" {
		t.Fatalf("unexpected name: %q", got.Name)
	}
	if got.DeploymentMode != "AUTO" {
		t.Fatalf("unexpected deployment mode: %q", got.DeploymentMode)
	}
	if got.ScriptDeployment == nil {
		t.Fatal("expected script_deployment to be set")
	}
	if got.ScriptDeployment.TriggerType != "SCHEDULE_AND_EVENT" {
		t.Fatalf("unexpected trigger type: %q", got.ScriptDeployment.TriggerType)
	}
	if got.ScriptDeployment.TriggerSchedule != "FOUR_HOURS" {
		t.Fatalf("unexpected trigger schedule: %q", got.ScriptDeployment.TriggerSchedule)
	}
	if len(got.ScriptDeployment.TriggerEvents) != 5 {
		t.Fatalf("expected 5 trigger events, got %d", len(got.ScriptDeployment.TriggerEvents))
	}
}

// TestScriptAssignmentRuleToAPI_PassesThroughCaseAndUUID proves B16 (b)-rows
// #166/#167/#169 removal: DeploymentMode, TriggerType, TriggerSchedule,
// TriggerEvents and SmartGroupUUID are sent to UEM exactly as configured --
// mixed case is preserved and smart_group_uuid is not lower-cased.
func TestScriptAssignmentRuleToAPI_PassesThroughCaseAndUUID(t *testing.T) {
	ctx := context.Background()

	memberships, mDiags := types.ListValueFrom(ctx, types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"smart_group_uuid": types.StringType,
			"smart_group_name": types.StringType,
		},
	}, []models.MembershipModel{
		{
			SmartGroupUUID: types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
			SmartGroupName: types.StringValue("utk (utk)"),
		},
	})
	if mDiags.HasError() {
		t.Fatalf("failed to build memberships list: %v", mDiags)
	}

	triggerEvents, teDiags := types.ListValueFrom(ctx, types.StringType, []string{"Run_Immediately", "login"})
	if teDiags.HasError() {
		t.Fatalf("failed to build trigger events list: %v", teDiags)
	}

	assignment := models.ScriptAssignmentModel{
		Name:           types.StringValue("test"),
		Priority:       types.Int64Value(1),
		DeploymentMode: types.StringValue("Auto"),
		Memberships:    memberships,
		ScriptDeployment: models.ScriptDeploymentModel{
			TriggerType:     types.StringValue("Schedule_And_Event"),
			TriggerEvents:   triggerEvents,
			TriggerSchedule: types.StringValue("Four_Hours"),
		},
	}

	assignments, aDiags := types.ListValueFrom(ctx, assignmentElemType, []models.ScriptAssignmentModel{assignment})
	if aDiags.HasError() {
		t.Fatalf("failed to build assignments list: %v", aDiags)
	}

	model := &models.ScriptAssignmentRuleModel{
		ScriptUUID:  types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
		Assignments: assignments,
	}

	api, diags := ScriptAssignmentRuleToAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	got := api.Assignments[0]
	if got.DeploymentMode != "Auto" {
		t.Errorf("expected DeploymentMode to pass through as %q, got %q", "Auto", got.DeploymentMode)
	}
	if got.Memberships[0].SmartGroupUUID != "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7" {
		t.Errorf("expected SmartGroupUUID to pass through unchanged, got %q", got.Memberships[0].SmartGroupUUID)
	}
	if got.ScriptDeployment.TriggerType != "Schedule_And_Event" {
		t.Errorf("expected TriggerType to pass through as %q, got %q", "Schedule_And_Event", got.ScriptDeployment.TriggerType)
	}
	if got.ScriptDeployment.TriggerSchedule != "Four_Hours" {
		t.Errorf("expected TriggerSchedule to pass through as %q, got %q", "Four_Hours", got.ScriptDeployment.TriggerSchedule)
	}
	want := []string{"Run_Immediately", "login"}
	if len(got.ScriptDeployment.TriggerEvents) != len(want) {
		t.Fatalf("expected %d trigger events, got %d", len(want), len(got.ScriptDeployment.TriggerEvents))
	}
	for i, w := range want {
		if got.ScriptDeployment.TriggerEvents[i] != w {
			t.Errorf("expected TriggerEvents[%d] = %q, got %q", i, w, got.ScriptDeployment.TriggerEvents[i])
		}
	}
}

// TestReadAPIIntoState_PassesThroughCaseAndUUID proves B16 (b)-row #168
// removal: DeploymentMode, TriggerType, TriggerSchedule, TriggerEvents and
// SmartGroupUUID are stored exactly as the server returned them, no
// uppercasing or lower-casing.
func TestReadAPIIntoState_PassesThroughCaseAndUUID(t *testing.T) {
	ctx := context.Background()

	priority := 1
	api := &sdk.ScriptAssignmentsSearchResultV1{
		SearchResults: []sdk.ScriptAssignmentResourceV1{
			{
				AssignmentUUID: "16fea16d-024d-1e8b-e41d-b5981759f00d",
				Name:           "Sample assignment",
				Priority:       &priority,
				DeploymentMode: "Auto",
				AssignedSmartGroups: []sdk.SmartGroupDataV1{
					{SmartGroupUUID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4", SmartGroupName: "All Devices"},
				},
				TriggerType:     "Schedule_And_Event",
				EventTriggers:   []string{"Login", "network_change"},
				ScheduleTrigger: "Four_Hours",
			},
		},
	}

	data := &models.ScriptAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var assigns []models.ScriptAssignmentModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if assigns[0].DeploymentMode.ValueString() != "Auto" {
		t.Errorf("expected DeploymentMode = %q, got %q", "Auto", assigns[0].DeploymentMode.ValueString())
	}
	if assigns[0].ScriptDeployment.TriggerType.ValueString() != "Schedule_And_Event" {
		t.Errorf("expected TriggerType = %q, got %q", "Schedule_And_Event", assigns[0].ScriptDeployment.TriggerType.ValueString())
	}
	if assigns[0].ScriptDeployment.TriggerSchedule.ValueString() != "Four_Hours" {
		t.Errorf("expected TriggerSchedule = %q, got %q", "Four_Hours", assigns[0].ScriptDeployment.TriggerSchedule.ValueString())
	}

	var memberships []models.MembershipModel
	diags = assigns[0].Memberships.ElementsAs(ctx, &memberships, false)
	if diags.HasError() {
		t.Fatalf("failed to decode memberships: %v", diags)
	}
	if memberships[0].SmartGroupUUID.ValueString() != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Errorf("expected SmartGroupUUID to pass through unchanged, got %q", memberships[0].SmartGroupUUID.ValueString())
	}

	var events []types.String
	diags = assigns[0].ScriptDeployment.TriggerEvents.ElementsAs(ctx, &events, false)
	if diags.HasError() {
		t.Fatalf("failed to decode trigger events: %v", diags)
	}
	if events[0].ValueString() != "Login" || events[1].ValueString() != "network_change" {
		t.Errorf("expected trigger events to pass through unchanged, got %v", events)
	}
}

func TestReadAPIIntoState_MapsAssignments(t *testing.T) {
	ctx := context.Background()

	show := true
	priority := 1
	api := &sdk.ScriptAssignmentsSearchResultV1{
		SearchResults: []sdk.ScriptAssignmentResourceV1{
			{
				AssignmentUUID: "16fea16d-024d-1e8b-e41d-b5981759f00d",
				Name:           "Sample assignment 1",
				Priority:       &priority,
				ShowInCatalog:  &show,
				DeploymentMode: "AUTO",
				AssignedSmartGroups: []sdk.SmartGroupDataV1{
					{
						SmartGroupUUID: "eec458d3-722f-678a-52b9-22398b02009e",
						SmartGroupName: "All Devices",
					},
				},
				TriggerType:     "NONE",
				EventTriggers:   []string{"LOGIN"},
				ScheduleTrigger: "UNKNOWN",
			},
		},
	}

	data := &models.ScriptAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var assigns []models.ScriptAssignmentModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}
	if assigns[0].AssignmentUUID.ValueString() != "16fea16d-024d-1e8b-e41d-b5981759f00d" {
		t.Fatalf("unexpected assignment_uuid: %q", assigns[0].AssignmentUUID.ValueString())
	}
	if assigns[0].ScriptDeployment.TriggerType.ValueString() != "NONE" {
		t.Fatalf("unexpected trigger_type: %q", assigns[0].ScriptDeployment.TriggerType.ValueString())
	}
}
