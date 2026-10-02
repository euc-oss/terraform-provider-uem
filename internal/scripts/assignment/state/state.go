package state

import (
	"context"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
)

var assignmentElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"assignment_uuid": types.StringType,
		"name":            types.StringType,
		"priority":        types.Int64Type,
		"deployment_mode": types.StringType,
		"show_in_catalog": types.BoolType,
		"memberships": types.ListType{ElemType: types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"smart_group_uuid": types.StringType,
				"smart_group_name": types.StringType,
			},
		}},
		"script_deployment": types.ObjectType{AttrTypes: map[string]attr.Type{
			"trigger_type":     types.StringType,
			"trigger_events":   types.ListType{ElemType: types.StringType},
			"trigger_schedule": types.StringType,
		}},
	},
}

// ScriptAssignmentRuleToAPI converts the Terraform state model into the SDK
// body for BulkUpdateScriptAssignmentsAsync request.
func ScriptAssignmentRuleToAPI(ctx context.Context, m *tf.ScriptAssignmentRuleModel) (*sdk.BulkUpdateScriptAssignmentV1, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := &sdk.BulkUpdateScriptAssignmentV1{}

	scriptUUID, err := m.FetchValidScriptUUID()
	if err != nil {
		diags.AddError("Invalid script_uuid", err.Error())
		return nil, diags
	}

	if !m.Assignments.IsNull() && !m.Assignments.IsUnknown() {
		var assigns []tf.ScriptAssignmentModel
		diags.Append(m.Assignments.ElementsAs(ctx, &assigns, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, a := range assigns {
			apiA, d := assignmentToAPI(ctx, scriptUUID, a)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			api.Assignments = append(api.Assignments, apiA)
		}
	}

	return api, diags
}

// EmptyScriptAssignmentRuleToAPI builds an explicit empty assignment body used for delete.
func EmptyScriptAssignmentRuleToAPI() *sdk.BulkUpdateScriptAssignmentV1 {
	return &sdk.BulkUpdateScriptAssignmentV1{
		Assignments: []sdk.BaseScriptAssignmentV1{},
	}
}

func SetMinimalState(data *tf.ScriptAssignmentRuleModel, scriptUUID string) {
	data.ScriptUUID = types.StringValue(scriptUUID)
	data.ID = types.StringValue(scriptUUID)
}

// ReadAPIIntoState maps a live GetScriptAssignmentsAsync response back into the terraform state model.
func ReadAPIIntoState(ctx context.Context, data *tf.ScriptAssignmentRuleModel, api *sdk.ScriptAssignmentsSearchResultV1) diag.Diagnostics {
	var diags diag.Diagnostics
	if api == nil {
		return diags
	}

	tfAssigns := make([]tf.ScriptAssignmentModel, len(api.SearchResults))
	for i, a := range api.SearchResults {
		tfAssigns[i] = assignmentFromAPI(a)
	}
	assignList, d := types.ListValueFrom(ctx, assignmentElemType, tfAssigns)
	diags.Append(d...)
	data.Assignments = assignList

	return diags
}

func assignmentToAPI(ctx context.Context, scriptUUID string, a tf.ScriptAssignmentModel) (sdk.BaseScriptAssignmentV1, diag.Diagnostics) {
	var diags diag.Diagnostics
	priority := int(a.Priority.ValueInt64())

	out := sdk.BaseScriptAssignmentV1{
		ScripUUID: scriptUUID,
		Name:      a.Name.ValueString(),
		// B16 (b)-row #167 removal: send the plan value exactly as configured,
		// no uppercasing.
		DeploymentMode: a.DeploymentMode.ValueString(),
		Priority:       &priority,
	}

	if !a.AssignmentUUID.IsNull() && !a.AssignmentUUID.IsUnknown() {
		out.AssignmentUUID = strings.TrimSpace(a.AssignmentUUID.ValueString())
	}

	if !a.ShowInCatalog.IsNull() && !a.ShowInCatalog.IsUnknown() {
		show := a.ShowInCatalog.ValueBool()
		out.ShowInCatalog = &show
	}

	if !a.Memberships.IsNull() && !a.Memberships.IsUnknown() {
		var memberships []tf.MembershipModel
		diags.Append(a.Memberships.ElementsAs(ctx, &memberships, false)...)
		if diags.HasError() {
			return out, diags
		}
		for _, m := range memberships {
			out.Memberships = append(out.Memberships, sdk.SmartGroupDataV1{
				// B16 (b)-row #169 removal: send the plan value exactly as
				// configured, no lower-casing.
				SmartGroupUUID: m.SmartGroupUUID.ValueString(),
				SmartGroupName: m.SmartGroupName.ValueString(),
			})
		}
	}

	out.ScriptDeployment = scriptDeploymentToAPI(ctx, a.ScriptDeployment)

	return out, diags
}

// scriptDeploymentToAPI maps the plan's ScriptDeploymentModel onto the SDK's
// ScriptDeploymentV1, sending each configured value exactly as the caller set
// it (B16 (b)-rows #166/#167 removal: no uppercasing of trigger_type,
// trigger_events, or trigger_schedule).
func scriptDeploymentToAPI(ctx context.Context, d tf.ScriptDeploymentModel) *sdk.ScriptDeploymentV1 {
	api := &sdk.ScriptDeploymentV1{}

	if !d.TriggerType.IsNull() && !d.TriggerType.IsUnknown() {
		api.TriggerType = d.TriggerType.ValueString()
	}
	if !d.TriggerSchedule.IsNull() && !d.TriggerSchedule.IsUnknown() {
		api.TriggerSchedule = d.TriggerSchedule.ValueString()
	}
	if !d.TriggerEvents.IsNull() && !d.TriggerEvents.IsUnknown() {
		var events []types.String
		diags := d.TriggerEvents.ElementsAs(ctx, &events, false)
		if diags.HasError() {
			return api
		}
		for _, event := range events {
			if !event.IsNull() && !event.IsUnknown() {
				api.TriggerEvents = append(api.TriggerEvents, event.ValueString())
			}
		}
	}

	if api.TriggerType == "" && api.TriggerSchedule == "" && len(api.TriggerEvents) == 0 {
		return nil
	}
	return api
}

func assignmentFromAPI(a sdk.ScriptAssignmentResourceV1) tf.ScriptAssignmentModel {
	var priority int64
	if a.Priority != nil {
		priority = int64(*a.Priority)
	}

	// B16 (b)-row #168 removal: store DeploymentMode/TriggerType/TriggerSchedule
	// exactly as the server returned them, no uppercasing.
	out := tf.ScriptAssignmentModel{
		AssignmentUUID: types.StringValue(a.AssignmentUUID),
		Name:           types.StringValue(a.Name),
		Priority:       types.Int64Value(priority),
		DeploymentMode: types.StringValue(a.DeploymentMode),
		Memberships:    membershipsFromAPI(a.AssignedSmartGroups),
		ScriptDeployment: tf.ScriptDeploymentModel{
			TriggerType:     types.StringValue(a.TriggerType),
			TriggerEvents:   stringSliceToList(a.EventTriggers),
			TriggerSchedule: types.StringValue(a.ScheduleTrigger),
		},
	}

	if a.ShowInCatalog != nil {
		out.ShowInCatalog = types.BoolValue(*a.ShowInCatalog)
	} else {
		out.ShowInCatalog = types.BoolNull()
	}

	return out
}

func membershipsFromAPI(groups []sdk.SmartGroupDataV1) types.List {
	if len(groups) == 0 {
		return types.ListValueMust(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"smart_group_uuid": types.StringType,
				"smart_group_name": types.StringType,
			},
		}, []attr.Value{})
	}

	elems := make([]attr.Value, len(groups))
	for i, g := range groups {
		obj, _ := types.ObjectValue(map[string]attr.Type{
			"smart_group_uuid": types.StringType,
			"smart_group_name": types.StringType,
		}, map[string]attr.Value{
			// B16 (b)-row #169 removal: store the server value as-is, no
			// lower-casing.
			"smart_group_uuid": types.StringValue(g.SmartGroupUUID),
			"smart_group_name": types.StringValue(g.SmartGroupName),
		})
		elems[i] = obj
	}

	return types.ListValueMust(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"smart_group_uuid": types.StringType,
			"smart_group_name": types.StringType,
		},
	}, elems)
}

// stringSliceToList stores each server value as-is (B16 (b)-row #168
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
