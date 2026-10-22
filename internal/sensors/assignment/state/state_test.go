package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/models"
)

// TestAssignmentsToAPI_passesThroughSmartGroupsAndEvents proves B16
// (b)-rows #182/#184 removal: smart_group_uuids and event_triggers are sent
// to UEM exactly as configured, no lower-casing or uppercasing. The
// emptiness check on smart_group_uuids remains (it's a validation, not a
// normalization).
func TestAssignmentsToAPI_passesThroughSmartGroupsAndEvents(t *testing.T) {
	ctx := context.Background()
	entry := tf.SensorAssignmentEntryModel{
		Name: types.StringValue("All Devices"),
		SmartGroupUUIDs: types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("SG-UUID-UPPER"),
		}),
		TriggerType: types.StringValue("schedule"),
		EventTriggers: types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("Login"),
		}),
	}

	api, diags := AssignmentsToAPI(ctx, entry)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(api.SmartGroupUUIDs) != 1 || api.SmartGroupUUIDs[0] != "SG-UUID-UPPER" {
		t.Fatalf("expected smart_group_uuids to pass through unchanged, got: %v", api.SmartGroupUUIDs)
	}
	if api.TriggerType != "schedule" {
		t.Fatalf("unexpected trigger type: %q", api.TriggerType)
	}
	if len(api.EventTriggers) != 1 || api.EventTriggers[0] != "Login" {
		t.Fatalf("expected event_triggers to pass through unchanged, got: %v", api.EventTriggers)
	}
}

// TestAssignmentsToAPI_rejectsEmptySmartGroupUUID proves the emptiness
// validation on smart_group_uuids survives the B16 (b)-row #184 removal
// (only the lower-casing/trimming normalization was removed).
func TestAssignmentsToAPI_rejectsEmptySmartGroupUUID(t *testing.T) {
	ctx := context.Background()
	entry := tf.SensorAssignmentEntryModel{
		Name: types.StringValue("All Devices"),
		SmartGroupUUIDs: types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("   "),
		}),
	}

	_, diags := AssignmentsToAPI(ctx, entry)
	if !diags.HasError() {
		t.Fatal("expected an error for a blank smart_group_uuids entry")
	}
}

// TestReadAPIIntoState_mapsAssignments proves B16 (b)-rows #183/#184
// removal: TriggerType, EventTriggers and SmartGroupUUIDs are stored exactly
// as the server returned them, no uppercasing or lower-casing.
func TestReadAPIIntoState_mapsAssignments(t *testing.T) {
	ctx := context.Background()
	ranking := 1
	api := []sdk.DeviceSensorAssignmentResponseV1ModelV2{
		{
			UUID:          "assign-1",
			Name:          "All Devices",
			Ranking:       &ranking,
			TriggerType:   "Schedule",
			EventTriggers: []string{"Login"},
			AssignedSmartGroups: []sdk.DeviceSensorAssignedSmartGroupV1ModelV2{
				{SmartGroupUUID: "SG-1"},
			},
		},
	}

	data := &tf.SensorAssignmentModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var assigns []tf.SensorAssignmentEntryModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}
	if assigns[0].AssignmentUUID.ValueString() != "assign-1" {
		t.Fatalf("unexpected assignment_uuid: %q", assigns[0].AssignmentUUID.ValueString())
	}
	if assigns[0].TriggerType.ValueString() != "Schedule" {
		t.Fatalf("expected TriggerType to pass through unchanged, got %q", assigns[0].TriggerType.ValueString())
	}
	eventTrigger, ok := assigns[0].EventTriggers.Elements()[0].(types.String)
	if !ok {
		t.Fatalf("expected event_trigger element to be types.String, got %T", assigns[0].EventTriggers.Elements()[0])
	}
	if eventTrigger.ValueString() != "Login" {
		t.Fatalf("expected event_trigger to pass through unchanged, got %q", eventTrigger.ValueString())
	}
	smartGroupUUID, ok := assigns[0].SmartGroupUUIDs.Elements()[0].(types.String)
	if !ok {
		t.Fatalf("expected smart_group_uuid element to be types.String, got %T", assigns[0].SmartGroupUUIDs.Elements()[0])
	}
	if smartGroupUUID.ValueString() != "SG-1" {
		t.Fatalf("expected smart_group_uuid to pass through unchanged, got %q", smartGroupUUID.ValueString())
	}
}

func TestReadAPIIntoState_emptyAssignments(t *testing.T) {
	ctx := context.Background()
	data := &tf.SensorAssignmentModel{}
	diags := ReadAPIIntoState(ctx, data, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var assigns []tf.SensorAssignmentEntryModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 0 {
		t.Fatalf("expected 0 assignments, got %d", len(assigns))
	}
}
