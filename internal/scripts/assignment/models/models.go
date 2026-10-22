package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ScriptAssignmentRuleModel is the Terraform model for uem_script_assignment.
//
// OrganizationGroupUUID is Computed-only: it is copied from the parent
// script on every successful Create/Read/Update and is only used to scope
// the confirming script-list lookup after an ambiguous HTTP 500/1000. State
// written before the attribute existed holds null until the next successful
// refresh; it is never filled retroactively.
type ScriptAssignmentRuleModel struct {
	ID                    types.String `tfsdk:"id"`
	ScriptUUID            types.String `tfsdk:"script_uuid"`
	OrganizationGroupUUID types.String `tfsdk:"organization_group_uuid"`
	Assignments           types.List   `tfsdk:"assignments"`
}

// ValidateScriptUUID trims and validates a script UUID string.
func ValidateScriptUUID(scriptUUID string) (string, error) {
	value := strings.TrimSpace(scriptUUID)
	if value == "" {
		return "", fmt.Errorf("script_uuid must not be empty")
	}
	return value, nil
}

// FetchValidScriptUUID returns a trimmed, validated script UUID.
func (m ScriptAssignmentRuleModel) FetchValidScriptUUID() (string, error) {
	if m.ScriptUUID.IsNull() || m.ScriptUUID.IsUnknown() {
		return "", fmt.Errorf("script_uuid must be set and known")
	}
	return ValidateScriptUUID(m.ScriptUUID.ValueString())
}

type ScriptAssignmentModel struct {
	AssignmentUUID   types.String          `tfsdk:"assignment_uuid"`
	Name             types.String          `tfsdk:"name"`
	Priority         types.Int64           `tfsdk:"priority"`
	DeploymentMode   types.String          `tfsdk:"deployment_mode"`
	ShowInCatalog    types.Bool            `tfsdk:"show_in_catalog"`
	Memberships      types.List            `tfsdk:"memberships"`
	ScriptDeployment ScriptDeploymentModel `tfsdk:"script_deployment"`
}

type MembershipModel struct {
	SmartGroupUUID types.String `tfsdk:"smart_group_uuid"`
	SmartGroupName types.String `tfsdk:"smart_group_name"`
}

type ScriptDeploymentModel struct {
	TriggerType     types.String `tfsdk:"trigger_type"`
	TriggerEvents   types.List   `tfsdk:"trigger_events"`
	TriggerSchedule types.String `tfsdk:"trigger_schedule"`
}
