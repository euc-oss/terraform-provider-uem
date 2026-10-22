package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func validateRequiredString(fieldName, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	return trimmed, nil
}

// MacScriptResourceModel represents the main script resource for Terraform.
type MacScriptResourceModel struct {
	ID                    types.String             `tfsdk:"id"`
	OrganizationGroupUUID types.String             `tfsdk:"organization_group_uuid"`
	Name                  types.String             `tfsdk:"name"`
	Description           types.String             `tfsdk:"description"`
	Platform              types.String             `tfsdk:"platform"`
	ScriptType            types.String             `tfsdk:"script_type"`
	ExecutionContext      types.String             `tfsdk:"execution_context"`
	PlatformArchitecture  types.String             `tfsdk:"platform_architecture"`
	ScriptData            types.String             `tfsdk:"script_data"`
	Timeout               types.Int64              `tfsdk:"timeout"`
	ScriptVariables       []MacScriptVariableModel `tfsdk:"script_variables"`
	AllowedInCatalog      types.Bool               `tfsdk:"allowed_in_catalog"`
	CatalogDisplay        *MacCatalogDisplayModel  `tfsdk:"catalog_display"`
	UserInteraction       types.Bool               `tfsdk:"user_interaction"`
}

// MacScriptVariableModel represents a script variable with name and value.
type MacScriptVariableModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// MacCatalogDisplayModel represents the catalog display configuration for a script.
type MacCatalogDisplayModel struct {
	DisplayName    types.String `tfsdk:"display_name"`
	DisplayDesc    types.String `tfsdk:"display_desc"`
	PreActionText  types.String `tfsdk:"pre_action_text"`
	PostActionText types.String `tfsdk:"post_action_text"`
	ActionType     types.String `tfsdk:"action_type"`
	CatalogIconURL types.String `tfsdk:"catalog_icon_url"`
	// UseDefaultIcon types.Bool     `tfsdk:"use_default_icon"`
	Categories types.List `tfsdk:"categories"`
}

func (m MacScriptResourceModel) FetchValidOrganizationGroupUUID() (string, error) {
	if m.OrganizationGroupUUID.IsNull() || m.OrganizationGroupUUID.IsUnknown() {
		return "", fmt.Errorf("organization_group_uuid must be set and known")
	}
	return validateRequiredString("organization_group_uuid", m.OrganizationGroupUUID.ValueString())
}

// ValidateScriptUUID trims and validates a script UUID string.
func ValidateScriptUUID(scriptUUID string) (string, error) {
	return validateRequiredString("id", scriptUUID)
}

func (m MacScriptResourceModel) FetchValidScriptUUID() (string, error) {
	if m.ID.IsNull() || m.ID.IsUnknown() {
		return "", fmt.Errorf("id must be set and known")
	}
	return ValidateScriptUUID(m.ID.ValueString())
}
