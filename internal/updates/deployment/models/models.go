package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UpdateDeploymentModel is the Terraform state for uem_update_deployment.
type UpdateDeploymentModel struct {
	ID                    types.String `tfsdk:"id"`
	UpdateUUID            types.String `tfsdk:"update_uuid"`
	OrganizationGroupUUID types.String `tfsdk:"organization_group_uuid"`
	Name                  types.String `tfsdk:"name"`
	DeploymentType        types.String `tfsdk:"deployment_type"`
	DeploymentStartTime   types.String `tfsdk:"deployment_start_time"`
	SmartGroupUUIDs       types.List   `tfsdk:"smart_group_uuids"`
	Notifications         types.List   `tfsdk:"notifications"`
}

// NotificationModel is a nested notification preference.
type NotificationModel struct {
	Action            types.String `tfsdk:"action"`
	Message           types.String `tfsdk:"message"`
	MessageTemplateID types.Int64  `tfsdk:"message_template_id"`
}

func validateRequiredString(fieldName, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	return trimmed, nil
}

func ValidateDeploymentUUID(id string) (string, error) {
	return validateRequiredString("id", id)
}

func (m UpdateDeploymentModel) FetchValidID() (string, error) {
	if m.ID.IsNull() || m.ID.IsUnknown() {
		return "", fmt.Errorf("id must be set and known")
	}
	return ValidateDeploymentUUID(m.ID.ValueString())
}

func (m UpdateDeploymentModel) FetchValidUpdateUUID() (string, error) {
	if m.UpdateUUID.IsNull() || m.UpdateUUID.IsUnknown() {
		return "", fmt.Errorf("update_uuid must be set and known")
	}
	return validateRequiredString("update_uuid", m.UpdateUUID.ValueString())
}

func (m UpdateDeploymentModel) FetchValidOrganizationGroupUUID() (string, error) {
	if m.OrganizationGroupUUID.IsNull() || m.OrganizationGroupUUID.IsUnknown() {
		return "", fmt.Errorf("organization_group_uuid must be set and known")
	}
	return validateRequiredString("organization_group_uuid", m.OrganizationGroupUUID.ValueString())
}
