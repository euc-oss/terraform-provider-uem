package state

import (
	"context"
	"strings"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	client "github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

var vppLicenseUsageElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"smart_group_uuid": types.StringType,
		"allocated":        types.Int64Type,
		"redeemed":         types.Int64Type,
	},
}

var appConfigurationElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"key":   types.StringType,
		"value": types.StringType,
		"type":  types.StringType,
	},
}

var assignmentElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"priority": types.Int64Type,
		"distribution": types.ObjectType{AttrTypes: map[string]attr.Type{
			"name":                types.StringType,
			"description":         types.StringType,
			"smart_groups":        types.ListType{ElemType: types.StringType},
			"app_delivery_method": types.StringType,
			"effective_date":      types.StringType,
			"vpp_app_details": types.ObjectType{AttrTypes: map[string]attr.Type{
				"license_usage": types.ListType{ElemType: vppLicenseUsageElemType},
			}},
		}},
		"restriction": types.ObjectType{AttrTypes: map[string]attr.Type{
			"remove_on_unenroll":         types.BoolType,
			"prevent_removal":            types.BoolType,
			"prevent_application_backup": types.BoolType,
			"make_app_mdm_managed":       types.BoolType,
			"managed_access":             types.BoolType,
			"desired_state_management":   types.BoolType,
		}},
		"tunnel": types.ObjectType{AttrTypes: map[string]attr.Type{
			"per_app_vpn_profile_uuid":       types.StringType,
			"afw_per_app_vpn_profile_uuid":   types.StringType,
			"amapi_per_app_vpn_profile_uuid": types.StringType,
		}},
		"application_configuration": types.ListType{ElemType: appConfigurationElemType},
		"application_attributes":    types.ListType{ElemType: appConfigurationElemType},
		"is_dynamic_template_saved": types.BoolType,
	},
}

// PurchasedAppAssignmentRuleToAPI converts Terraform state into the SDK request body.
func PurchasedAppAssignmentRuleToAPI(ctx context.Context, m *tf.PurchasedAppAssignmentRuleModel) (*sdk.AppAssignmentRuleV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := &sdk.AppAssignmentRuleV2Model{}

	if !m.ExcludedSmartGroups.IsNull() && !m.ExcludedSmartGroups.IsUnknown() {
		var list []types.String
		diags.Append(m.ExcludedSmartGroups.ElementsAs(ctx, &list, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, v := range list {
			if !v.IsNull() && !v.IsUnknown() {
				api.ExcludedSmartGroups = append(api.ExcludedSmartGroups, tf.NormalizeUUID(v.ValueString()))
			}
		}
	}

	if !m.Assignments.IsNull() && !m.Assignments.IsUnknown() {
		var assigns []tf.PurchasedAppAssignmentModel
		diags.Append(m.Assignments.ElementsAs(ctx, &assigns, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for _, a := range assigns {
			apiA, d := assignmentToAPI(ctx, a)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			api.Assignments = append(api.Assignments, apiA)
		}
	}

	return api, diags
}

// EmptyPurchasedAppAssignmentRuleToAPI builds an explicit empty assignment-rule body.
func EmptyPurchasedAppAssignmentRuleToAPI() *sdk.AppAssignmentRuleV2Model {
	return &sdk.AppAssignmentRuleV2Model{
		Assignments:         []sdk.AppAssignmentV2Model{},
		ExcludedSmartGroups: []string{},
	}
}

func SetMinimalState(data *tf.PurchasedAppAssignmentRuleModel, applicationUUID string) {
	normalized := tf.NormalizeUUID(applicationUUID)
	data.ApplicationUUID = types.StringValue(normalized)
	data.ID = types.StringValue(normalized)
}

// ReadAPIIntoState maps a live API response back into the Terraform state model.
func ReadAPIIntoState(ctx context.Context, data *tf.PurchasedAppAssignmentRuleModel, api *sdk.AppAssignmentRuleV2Model) diag.Diagnostics {
	var diags diag.Diagnostics

	priorExcludedSmartGroupsNull := data.ExcludedSmartGroups.IsNull()

	sgElems := make([]attr.Value, len(api.ExcludedSmartGroups))
	for i, s := range api.ExcludedSmartGroups {
		sgElems[i] = types.StringValue(tf.NormalizeUUID(s))
	}
	if priorExcludedSmartGroupsNull && len(sgElems) == 0 {
		// Server default when omitted is []. A null prior means the
		// attribute was never (or no longer) configured, so keep it null
		// instead of manufacturing an explicit empty list. Live-confirmed
		// 2026-09-23 on as<internal-env> 26.2: "excluded groups []" is UEM's reset on a
		// full-replace PUT that omits the field. Evidence:
		// internal-design-doc
		data.ExcludedSmartGroups = types.ListNull(types.StringType)
	} else {
		sgList, d := types.ListValue(types.StringType, sgElems)
		diags.Append(d...)
		data.ExcludedSmartGroups = sgList
	}

	var prior []tf.PurchasedAppAssignmentModel
	if !data.Assignments.IsNull() && !data.Assignments.IsUnknown() {
		diags.Append(data.Assignments.ElementsAs(ctx, &prior, false)...)
		if diags.HasError() {
			return diags
		}
	}

	tfAssigns := make([]tf.PurchasedAppAssignmentModel, len(api.Assignments))
	for i, a := range api.Assignments {
		var priorAssign *tf.PurchasedAppAssignmentModel
		if i < len(prior) {
			priorAssign = &prior[i]
		}
		tfAssigns[i] = assignmentFromAPI(ctx, a, priorAssign)
	}
	assignList, d := types.ListValueFrom(ctx, assignmentElemType, tfAssigns)
	diags.Append(d...)
	data.Assignments = assignList

	return diags
}

func assignmentToAPI(ctx context.Context, a tf.PurchasedAppAssignmentModel) (sdk.AppAssignmentV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	dist, d := distributionToAPI(ctx, a.Distribution)
	diags.Append(d...)
	appConfig, d := appConfigurationListToAPI(ctx, a.ApplicationConfiguration)
	diags.Append(d...)
	appAttrs, d := appConfigurationListToAPI(ctx, a.ApplicationAttributes)
	diags.Append(d...)

	out := sdk.AppAssignmentV2Model{
		Priority:                 int(a.Priority.ValueInt64()),
		Distribution:             dist,
		Restriction:              restrictionToAPI(a.Restriction),
		Tunnel:                   tunnelToAPI(a.Tunnel),
		ApplicationConfiguration: ensureAppConfigSlice(appConfig),
		ApplicationAttributes:    ensureAppConfigSlice(appAttrs),
	}
	if !a.IsDynamicTemplateSaved.IsNull() && !a.IsDynamicTemplateSaved.IsUnknown() {
		v := a.IsDynamicTemplateSaved.ValueBool()
		out.IsDynamicTemplateSaved = &v
	}
	return out, diags
}

func distributionToAPI(ctx context.Context, d tf.PurchasedAppAssignmentDistributionModel) (sdk.AppAssignmentDistributionV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := sdk.AppAssignmentDistributionV2Model{
		Name:        d.Name.ValueString(),
		Description: d.Description.ValueString(),
	}

	if !d.SmartGroups.IsNull() && !d.SmartGroups.IsUnknown() {
		var sg []types.String
		diags.Append(d.SmartGroups.ElementsAs(ctx, &sg, false)...)
		for _, s := range sg {
			if !s.IsNull() && !s.IsUnknown() {
				api.SmartGroups = append(api.SmartGroups, tf.NormalizeUUID(s.ValueString()))
			}
		}
	}

	// Load-bearing (B16 decision table row #132, KEEP+cite). UEM source:
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:69-71
	// (canonical Q15): app_delivery_method is [JsonConverter(typeof(StringEnumConverter))]
	// with Newtonsoft's case-sensitive default — a lowercase configured value
	// would fail server-side enum matching, so sending the exact-case wire
	// string (`AUTO`/`ON_DEMAND`) here is required, not merely cosmetic.
	if !d.AppDeliveryMethod.IsNull() && !d.AppDeliveryMethod.IsUnknown() {
		api.AppDeliveryMethod = strings.ToUpper(d.AppDeliveryMethod.ValueString())
	}

	if !d.EffectiveDate.IsNull() && !d.EffectiveDate.IsUnknown() && d.EffectiveDate.ValueString() != "" {
		if t, err := time.Parse(time.RFC3339, d.EffectiveDate.ValueString()); err == nil {
			api.EffectiveDate = client.NewUEMTime(t)
		}
	}

	vpp, vppDiags := vppAppDetailsToAPI(ctx, d.VppAppDetails)
	diags.Append(vppDiags...)
	if vpp != nil {
		api.VppAppDetails = vpp
	}

	return api, diags
}

func vppAppDetailsToAPI(ctx context.Context, v tf.VppAppDetailsModel) (*sdk.AppAssignmentVppV1ModelV2, diag.Diagnostics) {
	var diags diag.Diagnostics
	if v.LicenseUsage.IsNull() || v.LicenseUsage.IsUnknown() {
		return nil, diags
	}
	var usage []tf.VppLicenseUsageModel
	diags.Append(v.LicenseUsage.ElementsAs(ctx, &usage, false)...)
	if diags.HasError() || len(usage) == 0 {
		return nil, diags
	}
	api := &sdk.AppAssignmentVppV1ModelV2{}
	for _, u := range usage {
		entry := sdk.AssignmentLicenseUsageV1ModelV2{
			SmartGroupUUID: tf.NormalizeUUID(u.SmartGroupUUID.ValueString()),
		}
		if !u.Allocated.IsNull() && !u.Allocated.IsUnknown() {
			allocated := int(u.Allocated.ValueInt64())
			entry.Allocated = &allocated
		}
		api.LicenseUsage = append(api.LicenseUsage, entry)
	}
	return api, diags
}

func restrictionToAPI(r *tf.PurchasedAppAssignmentRestrictionModel) *sdk.AppAssignmentRestrictionV1ModelV2 {
	if r == nil {
		return nil
	}
	api := &sdk.AppAssignmentRestrictionV1ModelV2{}
	api.RemoveOnUnenroll = optionalBoolToAPI(r.RemoveOnUnenroll)
	api.PreventRemoval = optionalBoolToAPI(r.PreventRemoval)
	api.PreventApplicationBackup = optionalBoolToAPI(r.PreventApplicationBackup)
	api.MakeAppMdmManaged = optionalBoolToAPI(r.MakeAppMdmManaged)
	api.ManagedAccess = optionalBoolToAPI(r.ManagedAccess)
	api.DesiredStateManagement = optionalBoolToAPI(r.DesiredStateManagement)
	if api.RemoveOnUnenroll == nil && api.PreventRemoval == nil && api.PreventApplicationBackup == nil &&
		api.MakeAppMdmManaged == nil && api.ManagedAccess == nil && api.DesiredStateManagement == nil {
		return nil
	}
	return api
}

func tunnelToAPI(t *tf.PurchasedAppAssignmentTunnelModel) *sdk.AppAssignmentTunnelV1ModelV2 {
	if t == nil {
		return nil
	}
	api := &sdk.AppAssignmentTunnelV1ModelV2{
		PerAppVpnProfileUUID:      tf.NormalizeUUID(optionalStringToAPI(t.PerAppVpnProfileUUID)),
		AfwPerAppVpnProfileUUID:   tf.NormalizeUUID(optionalStringToAPI(t.AfwPerAppVpnProfileUUID)),
		AmapiPerAppVpnProfileUUID: tf.NormalizeUUID(optionalStringToAPI(t.AmapiPerAppVpnProfileUUID)),
	}
	if api.PerAppVpnProfileUUID == "" && api.AfwPerAppVpnProfileUUID == "" && api.AmapiPerAppVpnProfileUUID == "" {
		return nil
	}
	return api
}

func appConfigurationListToAPI(ctx context.Context, list types.List) ([]sdk.AppConfigurationV1ModelV2, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return []sdk.AppConfigurationV1ModelV2{}, diags
	}
	var entries []tf.AppConfigurationEntryModel
	diags.Append(list.ElementsAs(ctx, &entries, false)...)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]sdk.AppConfigurationV1ModelV2, 0, len(entries))
	for _, e := range entries {
		apiType, err := appConfigTypeToAPIString(e.Type.ValueString())
		if err != nil {
			diags.AddError("Invalid application configuration type", err.Error())
			return nil, diags
		}
		out = append(out, sdk.AppConfigurationV1ModelV2{
			Key:   e.Key.ValueString(),
			Value: e.Value.ValueString(),
			Type:  apiType,
		})
	}
	return out, diags
}

// ensureAppConfigSlice returns a non-nil empty slice when the API field is unset.
// UEM returns application_configuration/application_attributes as [] not null.
func ensureAppConfigSlice(items []sdk.AppConfigurationV1ModelV2) []sdk.AppConfigurationV1ModelV2 {
	if items == nil {
		return []sdk.AppConfigurationV1ModelV2{}
	}
	return items
}

func assignmentFromAPI(ctx context.Context, a sdk.AppAssignmentV2Model, prior *tf.PurchasedAppAssignmentModel) tf.PurchasedAppAssignmentModel {
	var priorIsDynamicTemplateSaved types.Bool
	if prior != nil {
		priorIsDynamicTemplateSaved = prior.IsDynamicTemplateSaved
	}
	out := tf.PurchasedAppAssignmentModel{
		Priority:               types.Int64Value(int64(a.Priority)),
		Distribution:           distributionFromAPI(ctx, a.Distribution, priorDistribution(prior)),
		Restriction:            restrictionFromAPI(a.Restriction, priorRestriction(prior)),
		Tunnel:                 tunnelFromAPI(a.Tunnel),
		IsDynamicTemplateSaved: nullIfDefaultFalseBool(a.IsDynamicTemplateSaved, priorIsDynamicTemplateSaved),
	}
	out.ApplicationConfiguration = appConfigurationListFromAPI(ctx, a.ApplicationConfiguration)
	out.ApplicationAttributes = appConfigurationListFromAPI(ctx, a.ApplicationAttributes)
	return out
}

func priorRestriction(prior *tf.PurchasedAppAssignmentModel) *tf.PurchasedAppAssignmentRestrictionModel {
	if prior == nil {
		return nil
	}
	return prior.Restriction
}

func priorDistribution(prior *tf.PurchasedAppAssignmentModel) *tf.PurchasedAppAssignmentDistributionModel {
	if prior == nil {
		return nil
	}
	return &prior.Distribution
}

func distributionFromAPI(ctx context.Context, d sdk.AppAssignmentDistributionV2Model, prior *tf.PurchasedAppAssignmentDistributionModel) tf.PurchasedAppAssignmentDistributionModel {
	tfDist := tf.PurchasedAppAssignmentDistributionModel{
		Name:          types.StringValue(d.Name),
		Description:   descriptionFromAPI(d.Description, prior),
		SmartGroups:   smartGroupsFromAPI(d.SmartGroups, prior),
		VppAppDetails: vppAppDetailsFromAPI(ctx, d.VppAppDetails),
	}
	var priorAppDeliveryMethod types.String
	if prior != nil {
		priorAppDeliveryMethod = prior.AppDeliveryMethod
	}
	tfDist.AppDeliveryMethod = appDeliveryMethodFromAPI(d.AppDeliveryMethod, priorAppDeliveryMethod)
	// effective_date always reads back null: UEM ignores this field for VPP
	// apps (internal-task item 4), so any value it happens to echo (including a
	// stale or default non-zero date from an import) must never surface, or
	// the plan-time validator would reject a value the provider itself
	// produced on read.
	tfDist.EffectiveDate = types.StringNull()
	return tfDist
}

// smartGroupsFromAPI keeps a null prior distribution.smart_groups null when
// UEM echoes [] (it ignores the field for purchased apps and always returns
// an empty list). Without this, the null plan from the removal modifier
// disagrees with the [] readback ("inconsistent result after apply").
// Live-confirmed 2026-09-23 on as<internal-env> 26.2 (a u3a2 regression caught before
// the gate). Evidence:
// internal-design-doc
func smartGroupsFromAPI(apiVal []string, prior *tf.PurchasedAppAssignmentDistributionModel) types.List {
	if len(apiVal) == 0 && (prior == nil || prior.SmartGroups.IsNull()) {
		return types.ListNull(types.StringType)
	}
	return stringSliceToList(apiVal)
}

// descriptionFromAPI keeps a null prior description null when UEM echoes "".
// Once a description is configured, including an explicit "", the server
// value is taken as-is.
//
// B16 decision table row #135 (KEEP+cite). UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:42-46,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model.Mappers/AppAssignmentV1ModelMapper.cs:33
// (canonical Q16): a stored null Description serializes with
// NullValueHandling.Ignore — the field is OMITTED (which this provider's SDK
// layer surfaces as apiVal == ""), not sent as "". A real explicit ""
// would echo back as "". This logic already matches: it only collapses ""
// to null when the prior was ALSO null, so an explicit "" is preserved.
func descriptionFromAPI(apiVal string, prior *tf.PurchasedAppAssignmentDistributionModel) types.String {
	if apiVal == "" && (prior == nil || prior.Description.IsNull()) {
		return types.StringNull()
	}
	return types.StringValue(apiVal)
}

// vppAppDetailsFromAPI maps the server's VPP license usage into state.
//
// B16 decision table row #141 (CORRECT). UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Helpers/AppsControllerHelper.cs:3108-3119,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/AppAssignmentVppV1Model.cs:29-30
// (canonical Q19): license_usage is populated only when
// AssignmentLicenseUsage?.Count > 0; otherwise vpp_app_details/
// AssignmentLicenseUsage is NOT SET on the distribution model at all —
// omitted (null), never an empty array. This previously manufactured an
// explicit [] whenever the API had no entries; it now reads back null to
// match UEM's own shape.
func vppAppDetailsFromAPI(ctx context.Context, api *sdk.AppAssignmentVppV1ModelV2) tf.VppAppDetailsModel {
	if api == nil || len(api.LicenseUsage) == 0 {
		return tf.VppAppDetailsModel{
			LicenseUsage: types.ListNull(vppLicenseUsageElemType),
		}
	}
	usage := make([]tf.VppLicenseUsageModel, len(api.LicenseUsage))
	for i, u := range api.LicenseUsage {
		usage[i] = tf.VppLicenseUsageModel{
			SmartGroupUUID: types.StringValue(tf.NormalizeUUID(u.SmartGroupUUID)),
			Allocated:      int64FromAPIInt(u.Allocated),
			Redeemed:       int64FromAPIInt(u.Redeemed),
		}
	}
	list, diags := types.ListValueFrom(ctx, vppLicenseUsageElemType, usage)
	if diags.HasError() {
		return tf.VppAppDetailsModel{
			LicenseUsage: types.ListNull(vppLicenseUsageElemType),
		}
	}
	return tf.VppAppDetailsModel{LicenseUsage: list}
}

// restrictionFromAPI maps the server restriction into state. UEM always echoes
// a restriction object, sending all six flags false when none were
// configured. When the prior plan/state restriction is null and the server
// shape is all-default, the null is kept so an unconfigured block does not
// turn into a manufactured object. Any non-default flag, or a non-null prior,
// maps the server values as-is so real drift is surfaced. Live-confirmed
// 2026-09-23 on as<internal-env> 26.2: "a null prior plus an all-default server object
// stays null". Evidence:
// internal-design-doc
func restrictionFromAPI(r *sdk.AppAssignmentRestrictionV1ModelV2, prior *tf.PurchasedAppAssignmentRestrictionModel) *tf.PurchasedAppAssignmentRestrictionModel {
	if r == nil {
		return nil
	}
	if prior == nil && isDefaultRestriction(r) {
		return nil
	}
	var priorRemove, priorPreventRemoval, priorPreventBackup, priorMdm, priorManaged, priorDesired types.Bool
	if prior != nil {
		priorRemove = prior.RemoveOnUnenroll
		priorPreventRemoval = prior.PreventRemoval
		priorPreventBackup = prior.PreventApplicationBackup
		priorMdm = prior.MakeAppMdmManaged
		priorManaged = prior.ManagedAccess
		priorDesired = prior.DesiredStateManagement
	}
	return &tf.PurchasedAppAssignmentRestrictionModel{
		// remove_on_unenroll/prevent_removal keep their booldefault.StaticBool(false)
		// schema default and their existing KeepStateBool mapping unchanged.
		// managed_access also keeps the KeepStateBool mapping unchanged (UEM
		// always echoes a concrete true/false for it, so this mapping never
		// actually depended on the schema Default); its plan-time defaulting
		// moved to managedAccessPlanModifier in resource.go (internal-task-65i).
		//
		// B16 decision table row #139 (KEEP+cite). UEM source:
		// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/AppAssignmentRestrictionV1Model.cs:35-74
		// (canonical Q17): these three fields are non-nullable bool on the
		// internal-app assignment model (default false, GET always echoes a
		// concrete value), but the separate VPP path's
		// VppDeploymentParametersV2Model declares RemoveOnUnenroll/
		// PreventRemoval as bool? (ManagedAccess isn't in that separate model).
		// Q17's nullability is type-level, not proof the VPP path ever actually
		// sends null for these — the in-code comment above already notes
		// live evidence (mh5-65i) that ManagedAccess always echoes a concrete
		// true/false in practice. KeepStateBool is kept here as
		// belt-and-braces defense against a null this VPP path could send,
		// not because it's strictly required by observed behavior.
		RemoveOnUnenroll: KeepStateBool(boolFromAPI(r.RemoveOnUnenroll), priorRemove),
		PreventRemoval:   KeepStateBool(boolFromAPI(r.PreventRemoval), priorPreventRemoval),
		ManagedAccess:    KeepStateBool(boolFromAPI(r.ManagedAccess), priorManaged),
		// prevent_application_backup/make_app_mdm_managed/desired_state_management
		// have no schema Default: collapse the server's false default back to
		// null when the prior was null, so removing them from config doesn't
		// manufacture an explicit false.
		PreventApplicationBackup: nullIfDefaultFalseBool(r.PreventApplicationBackup, priorPreventBackup),
		MakeAppMdmManaged:        nullIfDefaultFalseBool(r.MakeAppMdmManaged, priorMdm),
		DesiredStateManagement:   nullIfDefaultFalseBool(r.DesiredStateManagement, priorDesired),
	}
}

// isDefaultRestriction reports whether every restriction flag is false or
// absent, the server's zero-value shape.
func isDefaultRestriction(r *sdk.AppAssignmentRestrictionV1ModelV2) bool {
	for _, v := range []*bool{
		r.RemoveOnUnenroll,
		r.PreventRemoval,
		r.PreventApplicationBackup,
		r.MakeAppMdmManaged,
		r.ManagedAccess,
		r.DesiredStateManagement,
	} {
		if v != nil && *v {
			return false
		}
	}
	return true
}

// tunnelFromAPI maps the server's per-app-VPN tunnel UUIDs into state.
//
// B16 decision table row #140 (CORRECT, keep-prior part). UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Helpers/AppsControllerHelper.cs:3570-3572,3806-3810
// (canonical Q18): on assignment update, an omitted tunnel UUID is coerced
// to Guid.Empty, and vpnProfileId == 0 means the new ADP entity gets no
// VpnProfileId at all — the full assignment-replace flow CLEARS the prior
// per-app VPN for that assignment, it does not merge/preserve it. This
// function previously called KeepStateString per UUID, which kept the
// PRIOR state value whenever the API came back empty — silently masking
// exactly that clear as if nothing had changed. An empty/unresolved UUID
// from the API must now map straight to null (cleared), never falling back
// to the prior value. Each UUID is independently lowercase-normalized
// (canonical Q14: Guid parsing is case-insensitive, no stable echo-case
// guarantee) via uuidStringFromAPI/tf.NormalizeUUID, unaffected by this fix.
func tunnelFromAPI(t *sdk.AppAssignmentTunnelV1ModelV2) *tf.PurchasedAppAssignmentTunnelModel {
	if t == nil {
		return nil
	}
	out := &tf.PurchasedAppAssignmentTunnelModel{
		PerAppVpnProfileUUID:      uuidStringFromAPI(t.PerAppVpnProfileUUID),
		AfwPerAppVpnProfileUUID:   uuidStringFromAPI(t.AfwPerAppVpnProfileUUID),
		AmapiPerAppVpnProfileUUID: uuidStringFromAPI(t.AmapiPerAppVpnProfileUUID),
	}
	if out.PerAppVpnProfileUUID.IsNull() && out.AfwPerAppVpnProfileUUID.IsNull() && out.AmapiPerAppVpnProfileUUID.IsNull() {
		return nil
	}
	return out
}

func uuidStringFromAPI(value string) types.String {
	normalized := tf.NormalizeUUID(value)
	if normalized == "" {
		return types.StringNull()
	}
	return types.StringValue(normalized)
}

func appConfigurationListFromAPI(ctx context.Context, api []sdk.AppConfigurationV1ModelV2) types.List {
	if len(api) == 0 {
		return types.ListValueMust(appConfigurationElemType, []attr.Value{})
	}
	entries := make([]tf.AppConfigurationEntryModel, len(api))
	for i, e := range api {
		entries[i] = tf.AppConfigurationEntryModel{
			Key:   types.StringValue(e.Key),
			Value: types.StringValue(e.Value),
			Type:  appConfigTypeFromAPIString(e.Type),
		}
	}
	list, diags := types.ListValueFrom(ctx, appConfigurationElemType, entries)
	if diags.HasError() {
		return types.ListValueMust(appConfigurationElemType, []attr.Value{})
	}
	return list
}

func stringSliceToList(ss []string) types.List {
	if len(ss) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(tf.NormalizeUUID(s))
	}
	return types.ListValueMust(types.StringType, elems)
}
