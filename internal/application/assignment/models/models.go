package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type AppAssignmentRuleModel struct {
	ID                  types.String `tfsdk:"id"`
	ApplicationUUID     types.String `tfsdk:"application_uuid"`
	ExcludedSmartGroups types.List   `tfsdk:"excluded_smart_groups"`
	Assignments         types.List   `tfsdk:"assignments"`
	// MsiDeploymentParams   *AppMsiDeploymentOptionsModel            `tfsdk:"application_msi_deployment_params"`
	// RemediationParameters *RemediationAssignmentParametersModel `tfsdk:"remediation_assignment_parameters"`
}

// ValidateAppUUID trims and validates an application UUID string.
func ValidateAppUUID(applicationUUID string) (string, error) {
	value := strings.TrimSpace(applicationUUID)
	if value == "" {
		return "", fmt.Errorf("application_uuid must not be empty")
	}
	return value, nil
}

// FetchValidAppUUID returns a trimmed, validated application UUID.
func (m AppAssignmentRuleModel) FetchValidAppUUID() (string, error) {
	if m.ApplicationUUID.IsNull() || m.ApplicationUUID.IsUnknown() {
		return "", fmt.Errorf("application_uuid must be set and known")
	}

	return ValidateAppUUID(m.ApplicationUUID.ValueString())
}

type AppAssignmentModel struct {
	Priority types.Int64 `tfsdk:"priority"`
	// IsAppleEducationAssignment types.Bool                       `tfsdk:"is_apple_education_assignment"`
	Distribution AppAssignmentDistributionModel `tfsdk:"distribution"`
	Restriction  *AppAssignmentRestrictionModel `tfsdk:"restriction"`
	// Tunnel                     *AppAssignmentTunnelModel      `tfsdk:"tunnel"`
	//ApplicationConfiguration   types.List                       `tfsdk:"application_configuration"`
}

type AppAssignmentDistributionModel struct {
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	SmartGroups       types.List   `tfsdk:"smart_groups"`
	AppDeliveryMethod types.String `tfsdk:"app_delivery_method"`
	EffectiveDate     types.String `tfsdk:"effective_date"`
	// 1 is missing cant find it.
}

type AppAssignmentRestrictionModel struct {
	RemoveOnUnenroll types.Bool `tfsdk:"remove_on_unenroll"`
	// PreventApplicationBackup types.Bool `tfsdk:"prevent_application_backup"`
	// MakeAppMdmManaged        types.Bool `tfsdk:"make_app_mdm_managed"`
	// ManagedAccess            types.Bool `tfsdk:"managed_access"`
}

//type AppAssignmentTunnelModel struct {
//	PerAppVpnProfileUUID    types.String `tfsdk:"per_app_vpn_profile_uuid"`
//	AfwPerAppVpnProfileUUID types.String `tfsdk:"afw_per_app_vpn_profile_uuid"`
//}

//type AppConfigurationModel struct {
//	Key   types.String `tfsdk:"key"`
//	Value types.String `tfsdk:"value"`
//	Type  types.String `tfsdk:"type"`
//}

//type AppMsiDeploymentOptionsModel struct {
//	InstallDeviceRestart   types.String `tfsdk:"install_device_restart"`
//	UninstallDeviceRestart types.String `tfsdk:"uninstall_device_restart"`
//	InstallerSuccessCode   types.String `tfsdk:"installer_success_exit_code"`
//}

//type RemediationAssignmentParametersModel struct {
//	RemediationID         types.String `tfsdk:"remediation_id"`
//	VulnerabilityID       types.String `tfsdk:"vulnerability_id"`
//	ProductID             types.String `tfsdk:"product_id"`
//	RemediationAction     types.Int64  `tfsdk:"remediation_action"`
//	VulnerabilityProvider types.Int64  `tfsdk:"vulnerability_provider"`
//}
