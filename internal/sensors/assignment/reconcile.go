package assignment

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/models"
	assignmentState "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/state"
)

func reconcileAssignments(
	ctx context.Context,
	svc sensorAssignmentServiceAPI,
	sensorUUID string,
	desired []tf.SensorAssignmentEntryModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	_, current, err := svc.GetDeviceSensorAssignmentsAsync(ctx, sensorUUID)
	if err != nil {
		if isNotFoundAPIError(err) {
			current = &[]sdk.DeviceSensorAssignmentResponseV1ModelV2{}
		} else {
			diags.AddError("Client Error", fmt.Sprintf("Unable to read sensor assignments, got error: %s", err))
			return diags
		}
	}

	currentItems := []sdk.DeviceSensorAssignmentResponseV1ModelV2{}
	if current != nil {
		currentItems = *current
	}

	matches := matchDesiredToCurrent(desired, currentItems)
	keepUUIDs := make(map[string]struct{}, len(matches))
	for _, uuid := range matches {
		keepUUIDs[strings.ToLower(uuid)] = struct{}{}
	}

	currentByUUID := indexCurrentByUUID(currentItems)
	for _, item := range currentItems {
		uuid := strings.ToLower(strings.TrimSpace(item.UUID))
		if uuid == "" {
			continue
		}
		if _, ok := keepUUIDs[uuid]; ok {
			continue
		}
		if _, err := svc.DeleteDeviceSensorAssignmentAsync(ctx, item.UUID); err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to delete sensor assignment %s, got error: %s", item.UUID, err))
			return diags
		}
	}

	for _, entry := range desired {
		key := assignmentState.AssignmentKey(entry)
		apiReq, mapDiags := assignmentState.AssignmentsToAPI(ctx, entry)
		diags.Append(mapDiags...)
		if diags.HasError() {
			return diags
		}

		if uuid, ok := matches[key]; ok {
			currentItem, found := currentByUUID[strings.ToLower(uuid)]
			if found && assignmentEquals(entry, currentItem) {
				continue
			}
			if _, _, err := svc.UpdateDeviceSensorAssignmentAsync(ctx, uuid, apiReq); err != nil {
				diags.AddError("Client Error", fmt.Sprintf("Unable to update sensor assignment %s, got error: %s", uuid, err))
				return diags
			}
			continue
		}

		headers, body, err := svc.AddDeviceSensorAssignmentAsync(ctx, sensorUUID, apiReq)
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to create sensor assignment %q, got error: %s", entry.Name.ValueString(), err))
			return diags
		}
		newUUID, err := parseAssignmentUUID(headers, body)
		if err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to parse assignment UUID for %q: %s", entry.Name.ValueString(), err))
			return diags
		}
		matches[key] = newUUID
	}

	rankings, rankDiags := assignmentState.RankingsFromAssignments(withResolvedUUIDs(desired, matches))
	diags.Append(rankDiags...)
	if diags.HasError() {
		return diags
	}
	if len(rankings) > 0 {
		opts := &sdk.DeviceSensorsV2BulkUpdateDeviceSensorAssignmentRankingsAsyncOptions{
			Action: "update-ranking",
		}
		if _, err := svc.BulkUpdateDeviceSensorAssignmentRankingsAsync(ctx, sensorUUID, &rankings, opts); err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to update sensor assignment rankings, got error: %s", err))
		}
	}

	return diags
}

func deleteAllAssignments(ctx context.Context, svc sensorAssignmentServiceAPI, sensorUUID string) diag.Diagnostics {
	var diags diag.Diagnostics

	_, current, err := svc.GetDeviceSensorAssignmentsAsync(ctx, sensorUUID)
	if err != nil {
		if isNotFoundAPIError(err) {
			return diags
		}
		diags.AddError("Client Error", fmt.Sprintf("Unable to read sensor assignments, got error: %s", err))
		return diags
	}
	if current == nil {
		return diags
	}

	for _, item := range *current {
		uuid := strings.TrimSpace(item.UUID)
		if uuid == "" {
			continue
		}
		if _, err := svc.DeleteDeviceSensorAssignmentAsync(ctx, uuid); err != nil {
			diags.AddError("Client Error", fmt.Sprintf("Unable to delete sensor assignment %s, got error: %s", uuid, err))
			return diags
		}
	}

	return diags
}

func matchDesiredToCurrent(
	desired []tf.SensorAssignmentEntryModel,
	current []sdk.DeviceSensorAssignmentResponseV1ModelV2,
) map[string]string {
	matches := make(map[string]string, len(desired))
	currentByUUID := indexCurrentByUUID(current)
	currentByName := indexCurrentByName(current)

	for _, entry := range desired {
		key := assignmentState.AssignmentKey(entry)
		if strings.HasPrefix(key, "uuid:") {
			uuid := strings.TrimPrefix(key, "uuid:")
			if item, ok := currentByUUID[uuid]; ok {
				matches[key] = item.UUID
			}
			continue
		}

		nameKey := strings.TrimPrefix(key, "name:")
		if item, ok := currentByName[nameKey]; ok {
			matches[key] = item.UUID
		}
	}

	return matches
}

func withResolvedUUIDs(
	desired []tf.SensorAssignmentEntryModel,
	matches map[string]string,
) []tf.SensorAssignmentEntryModel {
	out := make([]tf.SensorAssignmentEntryModel, len(desired))
	copy(out, desired)
	for i := range out {
		key := assignmentState.AssignmentKey(out[i])
		if uuid, ok := matches[key]; ok {
			out[i].AssignmentUUID = types.StringValue(uuid)
		}
	}
	return out
}

func indexCurrentByUUID(current []sdk.DeviceSensorAssignmentResponseV1ModelV2) map[string]sdk.DeviceSensorAssignmentResponseV1ModelV2 {
	out := make(map[string]sdk.DeviceSensorAssignmentResponseV1ModelV2, len(current))
	for _, item := range current {
		uuid := strings.ToLower(strings.TrimSpace(item.UUID))
		if uuid == "" {
			continue
		}
		out[uuid] = item
	}
	return out
}

func indexCurrentByName(current []sdk.DeviceSensorAssignmentResponseV1ModelV2) map[string]sdk.DeviceSensorAssignmentResponseV1ModelV2 {
	out := make(map[string]sdk.DeviceSensorAssignmentResponseV1ModelV2, len(current))
	for _, item := range current {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if name == "" {
			continue
		}
		out[name] = item
	}
	return out
}

// assignmentEquals compares the plan entry against the current API item
// exactly (B16 (b)-row #188 removal: no case- or order-insensitive equality
// that would skip a PUT UEM should actually receive).
func assignmentEquals(entry tf.SensorAssignmentEntryModel, current sdk.DeviceSensorAssignmentResponseV1ModelV2) bool {
	if entry.Name.ValueString() != current.Name {
		return false
	}
	if entry.TriggerType.ValueString() != current.TriggerType {
		return false
	}
	if !stringSlicesEqual(eventTriggersFromEntry(entry), current.EventTriggers) {
		return false
	}
	return stringSlicesEqual(smartGroupUUIDsFromEntry(entry), smartGroupUUIDsFromAPI(current))
}

func eventTriggersFromEntry(entry tf.SensorAssignmentEntryModel) []string {
	if entry.EventTriggers.IsNull() || entry.EventTriggers.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(entry.EventTriggers.Elements()))
	for _, elem := range entry.EventTriggers.Elements() {
		if s, ok := elem.(interface{ ValueString() string }); ok {
			out = append(out, s.ValueString())
		}
	}
	return out
}

func smartGroupUUIDsFromEntry(entry tf.SensorAssignmentEntryModel) []string {
	if entry.SmartGroupUUIDs.IsNull() || entry.SmartGroupUUIDs.IsUnknown() {
		return nil
	}
	out := make([]string, 0, len(entry.SmartGroupUUIDs.Elements()))
	for _, elem := range entry.SmartGroupUUIDs.Elements() {
		if s, ok := elem.(interface{ ValueString() string }); ok {
			out = append(out, s.ValueString())
		}
	}
	return out
}

func smartGroupUUIDsFromAPI(current sdk.DeviceSensorAssignmentResponseV1ModelV2) []string {
	out := make([]string, 0, len(current.AssignedSmartGroups))
	for _, group := range current.AssignedSmartGroups {
		out = append(out, group.SmartGroupUUID)
	}
	return out
}

// stringSlicesEqual compares two slices exactly, position by position (B16
// (b)-row #188 removal: order and case both matter now, matching what a PUT
// would actually change).
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
