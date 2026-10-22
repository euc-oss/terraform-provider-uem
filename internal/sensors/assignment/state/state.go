package state

import (
	"context"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/models"
)

var assignmentElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"assignment_uuid":   types.StringType,
		"name":              types.StringType,
		"ranking":           types.Int64Type,
		"smart_group_uuids": types.ListType{ElemType: types.StringType},
		"trigger_type":      types.StringType,
		"event_triggers":    types.ListType{ElemType: types.StringType},
	},
}

func SetMinimalState(data *tf.SensorAssignmentModel, sensorUUID string) {
	data.SensorUUID = types.StringValue(sensorUUID)
	data.ID = types.StringValue(sensorUUID)
}

func AssignmentsToAPI(ctx context.Context, entry tf.SensorAssignmentEntryModel) (*sdk.DeviceSensorAssignmentRequestV1ModelV2, diag.Diagnostics) {
	var diags diag.Diagnostics

	api := &sdk.DeviceSensorAssignmentRequestV1ModelV2{
		Name: entry.Name.ValueString(),
	}

	setStringIfKnown(&api.TriggerType, entry.TriggerType)

	if !entry.SmartGroupUUIDs.IsNull() && !entry.SmartGroupUUIDs.IsUnknown() {
		var uuids []types.String
		diags.Append(entry.SmartGroupUUIDs.ElementsAs(ctx, &uuids, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, uuid := range uuids {
			// B16 (b)-row #184 removal: send the plan value exactly as
			// configured, no trimming or lower-casing. Only the emptiness
			// check remains -- it's a validation, not a normalization.
			value := uuid.ValueString()
			if strings.TrimSpace(value) == "" {
				diags.AddError("Invalid smart_group_uuids", "smart_group_uuids must not contain empty values")
				return nil, diags
			}
			api.SmartGroupUUIDs = append(api.SmartGroupUUIDs, value)
		}
	}

	if !entry.EventTriggers.IsNull() && !entry.EventTriggers.IsUnknown() {
		var events []types.String
		diags.Append(entry.EventTriggers.ElementsAs(ctx, &events, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, event := range events {
			if event.IsNull() || event.IsUnknown() {
				continue
			}
			// B16 (b)-row #182 removal: send the plan value exactly as
			// configured, no uppercasing.
			api.EventTriggers = append(api.EventTriggers, event.ValueString())
		}
	}

	return api, diags
}

func ReadAPIIntoState(ctx context.Context, data *tf.SensorAssignmentModel, api []sdk.DeviceSensorAssignmentResponseV1ModelV2) diag.Diagnostics {
	var diags diag.Diagnostics

	tfAssigns := make([]tf.SensorAssignmentEntryModel, len(api))
	for i, a := range api {
		tfAssigns[i] = assignmentFromAPI(a)
	}

	assignList, d := types.ListValueFrom(ctx, assignmentElemType, tfAssigns)
	diags.Append(d...)
	data.Assignments = assignList

	return diags
}

func assignmentFromAPI(a sdk.DeviceSensorAssignmentResponseV1ModelV2) tf.SensorAssignmentEntryModel {
	var ranking int64
	if a.Ranking != nil {
		ranking = int64(*a.Ranking)
	}

	// B16 (b)-row #183 removal: store TriggerType exactly as the server
	// returned it, no uppercasing.
	out := tf.SensorAssignmentEntryModel{
		AssignmentUUID:  types.StringValue(a.UUID),
		Name:            types.StringValue(a.Name),
		Ranking:         types.Int64Value(ranking),
		TriggerType:     types.StringValue(a.TriggerType),
		EventTriggers:   stringSliceToList(a.EventTriggers),
		SmartGroupUUIDs: smartGroupUUIDsFromAPI(a),
	}

	return out
}

// smartGroupUUIDsFromAPI stores each server value as-is (B16 (b)-row #184
// removal: no lower-casing of smart_group_uuids on read).
func smartGroupUUIDsFromAPI(a sdk.DeviceSensorAssignmentResponseV1ModelV2) types.List {
	uuids := make([]string, 0, len(a.AssignedSmartGroups))
	for _, g := range a.AssignedSmartGroups {
		uuids = append(uuids, g.SmartGroupUUID)
	}
	if len(uuids) == 0 && len(a.AssignedSmartGroups) == 0 {
		// Fall back to smart group count only being metadata; keep empty list explicit.
		return types.ListValueMust(types.StringType, []attr.Value{})
	}

	elems := make([]attr.Value, len(uuids))
	for i, uuid := range uuids {
		elems[i] = types.StringValue(uuid)
	}
	return types.ListValueMust(types.StringType, elems)
}

// stringSliceToList stores each server value as-is (B16 (b)-row #183
// removal: no uppercasing of event_triggers on read).
func stringSliceToList(ss []string) types.List {
	if len(ss) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(s)
	}
	return types.ListValueMust(types.StringType, elems)
}

func DesiredAssignments(ctx context.Context, m *tf.SensorAssignmentModel) ([]tf.SensorAssignmentEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil || m.Assignments.IsNull() || m.Assignments.IsUnknown() {
		return nil, diags
	}

	var assigns []tf.SensorAssignmentEntryModel
	diags.Append(m.Assignments.ElementsAs(ctx, &assigns, false)...)
	return assigns, diags
}

func RankingsFromAssignments(assigns []tf.SensorAssignmentEntryModel) ([]sdk.DeviceSensorAssignmentRankingV1ModelV2, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]sdk.DeviceSensorAssignmentRankingV1ModelV2, 0, len(assigns))
	for _, a := range assigns {
		uuid := strings.TrimSpace(a.AssignmentUUID.ValueString())
		if uuid == "" {
			continue
		}
		ranking := int(a.Ranking.ValueInt64())
		out = append(out, sdk.DeviceSensorAssignmentRankingV1ModelV2{
			UUID:    uuid,
			Ranking: &ranking,
		})
	}
	if len(out) == 0 {
		return nil, diags
	}
	return out, diags
}

func setStringIfKnown(dst *string, s types.String) {
	if s.IsNull() || s.IsUnknown() {
		return
	}
	*dst = s.ValueString()
}

func AssignmentKey(entry tf.SensorAssignmentEntryModel) string {
	if !entry.AssignmentUUID.IsNull() && !entry.AssignmentUUID.IsUnknown() {
		uuid := strings.TrimSpace(entry.AssignmentUUID.ValueString())
		if uuid != "" {
			return "uuid:" + strings.ToLower(uuid)
		}
	}
	return "name:" + strings.ToLower(strings.TrimSpace(entry.Name.ValueString()))
}

// internal-ticket removed ValidateDesiredAssignments (B16 decision table row
// #187, REMOVE per faithful doctrine). Canonical Q26: no uniqueness check
// in AssignmentGroup_Save; no server rule found. No server rule rejects a duplicate assignment name or
// natural key per sensor. Duplicate desired entries are now passed straight
// through to reconcileAssignments like any other entry, matching UEM's own
// behavior instead of a provider-only guard with no source. This is a
// deliberate UX tradeoff (duplicates are no longer rejected client-side and
// will be sent to the server as separate assignments) rather than a bug
// fix — recorded here in case a future owner wants to re-add it as an
// explicit, clearly-labeled provider-only safety net rather than an implied
// UEM mirror.
