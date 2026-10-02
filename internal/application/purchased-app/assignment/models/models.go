package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NormalizeUUID trims whitespace and lowercases UUID strings.
//
// B16 decision table rows #126 (plan-level lowercase UUIDs, resource.go's
// normalizeLowercaseString(List) plan modifiers), #137 (excluded_smart_groups
// lowercase on read), and #143 (this function) all share this rule. UEM
// source: AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers routes bind UUIDs as
// Guid/{uuid:guid} and purchased search uses Guid.TryParse (canonical Q14):
// Guid parsing is case-insensitive, and there is no stable echo-case
// guarantee from the server ("Data unavailable"). Lowercasing here is
// harmless client-side normalization for stable Terraform diffs — the
// server accepts any case regardless — kept as a defensive, not a
// server-mandated, rule.
func NormalizeUUID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// ValidateAppUUID trims, lowercases, and validates an application UUID string.
func ValidateAppUUID(applicationUUID string) (string, error) {
	value := NormalizeUUID(applicationUUID)
	if value == "" {
		return "", fmt.Errorf("application_uuid must not be empty")
	}
	return value, nil
}

// FetchValidAppUUID returns a normalized, validated application UUID.
func (m PurchasedAppAssignmentRuleModel) FetchValidAppUUID() (string, error) {
	if m.ApplicationUUID.IsNull() || m.ApplicationUUID.IsUnknown() {
		return "", fmt.Errorf("application_uuid must be set and known")
	}
	return ValidateAppUUID(m.ApplicationUUID.ValueString())
}

type PurchasedAppAssignmentRuleModel struct {
	ID                  types.String `tfsdk:"id"`
	ApplicationUUID     types.String `tfsdk:"application_uuid"`
	ExcludedSmartGroups types.List   `tfsdk:"excluded_smart_groups"`
	Assignments         types.List   `tfsdk:"assignments"`
}

type PurchasedAppAssignmentModel struct {
	Priority                 types.Int64                             `tfsdk:"priority"`
	Distribution             PurchasedAppAssignmentDistributionModel `tfsdk:"distribution"`
	Restriction              *PurchasedAppAssignmentRestrictionModel `tfsdk:"restriction"`
	Tunnel                   *PurchasedAppAssignmentTunnelModel      `tfsdk:"tunnel"`
	ApplicationConfiguration types.List                              `tfsdk:"application_configuration"`
	ApplicationAttributes    types.List                              `tfsdk:"application_attributes"`
	IsDynamicTemplateSaved   types.Bool                              `tfsdk:"is_dynamic_template_saved"`
}

type PurchasedAppAssignmentDistributionModel struct {
	Name              types.String       `tfsdk:"name"`
	Description       types.String       `tfsdk:"description"`
	SmartGroups       types.List         `tfsdk:"smart_groups"`
	AppDeliveryMethod types.String       `tfsdk:"app_delivery_method"`
	EffectiveDate     types.String       `tfsdk:"effective_date"`
	VppAppDetails     VppAppDetailsModel `tfsdk:"vpp_app_details"`
}

type VppAppDetailsModel struct {
	LicenseUsage types.List `tfsdk:"license_usage"`
}

type VppLicenseUsageModel struct {
	SmartGroupUUID types.String `tfsdk:"smart_group_uuid"`
	Allocated      types.Int64  `tfsdk:"allocated"`
	Redeemed       types.Int64  `tfsdk:"redeemed"`
}

type PurchasedAppAssignmentRestrictionModel struct {
	RemoveOnUnenroll         types.Bool `tfsdk:"remove_on_unenroll"`
	PreventRemoval           types.Bool `tfsdk:"prevent_removal"`
	PreventApplicationBackup types.Bool `tfsdk:"prevent_application_backup"`
	MakeAppMdmManaged        types.Bool `tfsdk:"make_app_mdm_managed"`
	ManagedAccess            types.Bool `tfsdk:"managed_access"`
	DesiredStateManagement   types.Bool `tfsdk:"desired_state_management"`
}

type PurchasedAppAssignmentTunnelModel struct {
	PerAppVpnProfileUUID      types.String `tfsdk:"per_app_vpn_profile_uuid"`
	AfwPerAppVpnProfileUUID   types.String `tfsdk:"afw_per_app_vpn_profile_uuid"`
	AmapiPerAppVpnProfileUUID types.String `tfsdk:"amapi_per_app_vpn_profile_uuid"`
}

type AppConfigurationEntryModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
	Type  types.String `tfsdk:"type"`
}

// DistributionSmartGroupsUnsupportedSummary and
// DistributionSmartGroupsUnsupportedDetail describe the error for a non-empty
// distribution.smart_groups. UEM silently drops smart_groups on VPP
// assignments, so a smart group is only assigned through license_usage.
const (
	DistributionSmartGroupsUnsupportedSummary = "Unsupported distribution.smart_groups"
	DistributionSmartGroupsUnsupportedDetail  = "distribution.smart_groups is not supported for VPP purchased-app assignments: " +
		"the UEM API silently ignores it, so the listed groups would never be assigned. " +
		"To assign a smart group, use vpp_app_details.license_usage instead " +
		"(one entry per group with smart_group_uuid and allocated), and leave distribution.smart_groups unset."
)

// DistributionEffectiveDateUnsupportedSummary and
// DistributionEffectiveDateUnsupportedDetail describe the error for a
// non-null distribution.effective_date on a VPP purchased-app assignment.
// UEM ignores effective_date for VPP assignments, so setting it would be
// silently discarded by the server.
const (
	DistributionEffectiveDateUnsupportedSummary = "Unsupported distribution.effective_date"
	DistributionEffectiveDateUnsupportedDetail  = "distribution.effective_date is not applicable to VPP assignments; UEM ignores it. " +
		"Leave distribution.effective_date unset."
)

const missingLicenseUsageMessage = "each assignment must set distribution.vpp_app_details.license_usage"

// HasDistributionSmartGroups reports whether the assignment sets a non-empty
// distribution.smart_groups.
func (a PurchasedAppAssignmentModel) HasDistributionSmartGroups(ctx context.Context) bool {
	return hasNonEmptyStringList(ctx, a.Distribution.SmartGroups)
}

// ValidateTargeting ensures each assignment targets through VPP license usage
// and does not set distribution.smart_groups.
func (a PurchasedAppAssignmentModel) ValidateTargeting(ctx context.Context) error {
	if a.HasDistributionSmartGroups(ctx) {
		return fmt.Errorf("%s", DistributionSmartGroupsUnsupportedDetail)
	}
	if a.Distribution.VppAppDetails.LicenseUsage.IsNull() || a.Distribution.VppAppDetails.LicenseUsage.IsUnknown() {
		return fmt.Errorf("%s", missingLicenseUsageMessage)
	}
	var usage []VppLicenseUsageModel
	diags := a.Distribution.VppAppDetails.LicenseUsage.ElementsAs(ctx, &usage, false)
	if diags.HasError() {
		return fmt.Errorf("invalid license_usage: %v", diags)
	}
	if len(usage) == 0 {
		return fmt.Errorf("%s", missingLicenseUsageMessage)
	}
	return nil
}

func hasNonEmptyStringList(ctx context.Context, list types.List) bool {
	if list.IsNull() || list.IsUnknown() {
		return false
	}
	var elements []types.String
	diags := list.ElementsAs(ctx, &elements, false)
	if diags.HasError() {
		return false
	}
	return len(elements) > 0
}

// ValidateAssignmentsTargeting validates targeting for all assignments in a rule.
func ValidateAssignmentsTargeting(ctx context.Context, assignments types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	if assignments.IsNull() || assignments.IsUnknown() {
		return diags
	}
	var assigns []PurchasedAppAssignmentModel
	diags.Append(assignments.ElementsAs(ctx, &assigns, false)...)
	if diags.HasError() {
		return diags
	}
	for i, a := range assigns {
		if err := a.ValidateTargeting(ctx); err != nil {
			diags.AddError(
				fmt.Sprintf("Invalid assignments[%d] targeting", i),
				err.Error(),
			)
		}
	}
	return diags
}
