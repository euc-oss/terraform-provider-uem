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

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
)

// assignmentElemType is the complete attr.Type for a single Tf.AppAssignmentModel.
// It must stay in sync with the struct's tfsdk tags.
var assignmentElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"priority": types.Int64Type,
		"distribution": types.ObjectType{AttrTypes: map[string]attr.Type{
			"name":                types.StringType,
			"description":         types.StringType,
			"smart_groups":        types.ListType{ElemType: types.StringType},
			"app_delivery_method": types.StringType,
			"effective_date":      types.StringType,
		}},
		"restriction": types.ObjectType{AttrTypes: map[string]attr.Type{
			"remove_on_unenroll": types.BoolType,
		}},
	},
}

// AppAssignmentRuleToAPI converts the Terraform state model into the SDK
// request body for UpdateAssignmentRuleAsync.
// ApplicationUUID is a URL path parameter only – callers pass it separately.
func AppAssignmentRuleToAPI(ctx context.Context, m *tf.AppAssignmentRuleModel) (*sdk.AppAssignmentRuleV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := &sdk.AppAssignmentRuleV2Model{}

	// excluded_smart_groups: lowercase UUID normalization (b16 decision
	// table row #147, KEEP — see the app_delivery_method/smart_groups
	// citation below in distributionToAPI for the shared Q14 rationale).
	if !m.ExcludedSmartGroups.IsNull() && !m.ExcludedSmartGroups.IsUnknown() {
		var list []types.String
		diags.Append(m.ExcludedSmartGroups.ElementsAs(ctx, &list, false)...)

		if diags.HasError() {
			return nil, diags
		}

		for _, v := range list {
			if !v.IsNull() && !v.IsUnknown() {
				api.ExcludedSmartGroups = append(api.ExcludedSmartGroups, strings.ToLower(v.ValueString()))
			}
		}
	}

	// assignments
	if !m.Assignments.IsNull() && !m.Assignments.IsUnknown() {
		var assigns []tf.AppAssignmentModel
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

// EmptyAppAssignmentRuleToAPI builds an explicit empty assignment-rule body
// used for clear/reset operations.
//
// Note: the SDK AppAssignmentRuleV2Model does not expose
// remediation_parameters, so that property cannot be set here.
func EmptyAppAssignmentRuleToAPI() *sdk.AppAssignmentRuleV2Model {
	return &sdk.AppAssignmentRuleV2Model{
		Assignments:                    []sdk.AppAssignmentV2Model{},
		ExcludedSmartGroups:            []string{},
		ApplicationMsiDeploymentParams: &sdk.MsiDeploymentOptionsV1ModelV2{},
	}
}

func SetMinimalState(data *tf.AppAssignmentRuleModel, applicationUUID string) {
	data.ApplicationUUID = types.StringValue(applicationUUID)
	data.ID = types.StringValue(applicationUUID)
}

// ReadAPIIntoState maps a live GetAssignmentRuleAsync response back into the terraform state model.
// ApplicationUUID and ID are left untouched so callers can set them from the plan/import context.
func ReadAPIIntoState(ctx context.Context, data *tf.AppAssignmentRuleModel, api *sdk.AppAssignmentRuleV2Model) diag.Diagnostics {
	var diags diag.Diagnostics

	// excluded_smart_groups. A null prior means the attribute was never (or
	// no longer) configured; the server default when omitted is [], so that
	// case keeps the null instead of manufacturing an explicit empty list.
	// A non-null prior, or a non-empty server value, is always taken as-is.
	// Live-confirmed 2026-09-23 on as<internal-env> 26.2: excluded groups reset to []
	// on a full-replace PUT that omits the field; effective_date and
	// excluded-groups removal both clear on apply. Evidence:
	// internal-design-doc
	priorExcludedSmartGroupsNull := data.ExcludedSmartGroups.IsNull()
	sgElems := make([]attr.Value, len(api.ExcludedSmartGroups))
	// Lowercase UUID normalization on read (B16 decision table row #147,
	// KEEP). UEM source: routes bind UUIDs as Guid/{uuid:guid} (canonical
	// Q14): Guid parsing is case-insensitive with no stable echo-case
	// guarantee — harmless client-side normalization, not server-mandated.
	for i, s := range api.ExcludedSmartGroups {
		sgElems[i] = types.StringValue(strings.ToLower(s))
	}
	if priorExcludedSmartGroupsNull && len(sgElems) == 0 {
		data.ExcludedSmartGroups = types.ListNull(types.StringType)
	} else {
		sgList, d := types.ListValue(types.StringType, sgElems)
		diags.Append(d...)
		data.ExcludedSmartGroups = sgList
	}

	// assignments. Pair each API assignment with the prior plan/state
	// assignment at the same index (if any) so Read can tell apart "this
	// entry was never configured" (no prior object at this position — a
	// fresh import, or a newly appended element) from "this entry's
	// effective_date/app_delivery_method was explicitly removed" (a prior
	// object exists but the field is null). See distributionFromAPI and
	// effectiveDateFromAPI for how the two cases are told apart (internal-task,
	// rule D). This index-based pairing mirrors the same pattern already
	// used by the purchased-app assignment resource's descriptionFromAPI.
	var prior []tf.AppAssignmentModel
	if !data.Assignments.IsNull() && !data.Assignments.IsUnknown() {
		diags.Append(data.Assignments.ElementsAs(ctx, &prior, false)...)
		if diags.HasError() {
			return diags
		}
	}

	tfAssigns := make([]tf.AppAssignmentModel, len(api.Assignments))
	for i, a := range api.Assignments {
		var priorAssign *tf.AppAssignmentModel
		if i < len(prior) {
			priorAssign = &prior[i]
		}
		tfAssigns[i] = assignmentFromAPI(a, priorAssign)
	}
	assignList, d := types.ListValueFrom(ctx, assignmentElemType, tfAssigns)
	diags.Append(d...)
	data.Assignments = assignList

	return diags
}

// to api.
func assignmentToAPI(ctx context.Context, a tf.AppAssignmentModel) (sdk.AppAssignmentV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	dist, d := distributionToAPI(ctx, a.Distribution)
	diags.Append(d...)
	out := sdk.AppAssignmentV2Model{
		Priority:     int(a.Priority.ValueInt64()),
		Distribution: dist,
		Restriction:  restrictionToAPI(a.Restriction),
	}
	return out, diags
}

func distributionToAPI(ctx context.Context, d tf.AppAssignmentDistributionModel) (sdk.AppAssignmentDistributionV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	api := sdk.AppAssignmentDistributionV2Model{
		Name:        d.Name.ValueString(),
		Description: d.Description.ValueString(),
	}

	// smart_groups: lowercase UUID normalization (B16 decision table row
	// #147, KEEP). UEM source: routes bind UUIDs as Guid/{uuid:guid} and
	// purchased search uses Guid.TryParse (canonical Q14): Guid parsing is
	// case-insensitive and there is no stable echo-case guarantee from the
	// server — harmless client-side normalization for stable diffs, not a
	// server-mandated rule.
	if !d.SmartGroups.IsNull() && !d.SmartGroups.IsUnknown() {
		var sg []types.String
		diags.Append(d.SmartGroups.ElementsAs(ctx, &sg, false)...)
		for _, s := range sg {
			if !s.IsNull() && !s.IsUnknown() {
				api.SmartGroups = append(api.SmartGroups, strings.ToLower(s.ValueString()))
			}
		}
	}

	// app_delivery_method: normalize to canonical uppercase string.
	//
	// Load-bearing (B16 decision table row #148, KEEP+cite; same pattern as
	// row #132 in the purchased-app package). UEM source:
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:69-71
	// (canonical Q15): app_delivery_method is
	// [JsonConverter(typeof(StringEnumConverter))] with Newtonsoft's
	// case-sensitive default — a lowercase configured value would fail
	// server-side enum matching, so sending the exact-case wire string
	// (`AUTO`/`ON_DEMAND`) here is required, not merely cosmetic.
	if !d.AppDeliveryMethod.IsNull() && !d.AppDeliveryMethod.IsUnknown() {
		api.AppDeliveryMethod = strings.ToUpper(d.AppDeliveryMethod.ValueString())
	}

	// effective_date: RFC3339 string → time.Time
	if !d.EffectiveDate.IsNull() && !d.EffectiveDate.IsUnknown() && d.EffectiveDate.ValueString() != "" {
		if t, err := time.Parse(time.RFC3339, d.EffectiveDate.ValueString()); err == nil {
			api.EffectiveDate = client.NewUEMTime(t)
		}
	}

	return api, diags
}

func restrictionToAPI(r *tf.AppAssignmentRestrictionModel) *sdk.AppAssignmentRestrictionV1ModelV2 {
	if r == nil {
		return nil
	}
	api := &sdk.AppAssignmentRestrictionV1ModelV2{}
	if !r.RemoveOnUnenroll.IsNull() && !r.RemoveOnUnenroll.IsUnknown() {
		v := r.RemoveOnUnenroll.ValueBool()
		api.RemoveOnUnenroll = &v
	}
	return api
}

func assignmentFromAPI(a sdk.AppAssignmentV2Model, prior *tf.AppAssignmentModel) tf.AppAssignmentModel {
	return tf.AppAssignmentModel{
		Priority:     types.Int64Value(int64(a.Priority)),
		Distribution: distributionFromAPI(a.Distribution, priorDistribution(prior)),
		Restriction:  restrictionFromAPI(a.Restriction, priorRestriction(prior)),
	}
}

func priorDistribution(prior *tf.AppAssignmentModel) *tf.AppAssignmentDistributionModel {
	if prior == nil {
		return nil
	}
	return &prior.Distribution
}

// from api.
func distributionFromAPI(d sdk.AppAssignmentDistributionV2Model, prior *tf.AppAssignmentDistributionModel) tf.AppAssignmentDistributionModel {
	tf := tf.AppAssignmentDistributionModel{
		Name:        types.StringValue(d.Name),
		Description: types.StringValue(d.Description),
		SmartGroups: stringSliceToList(d.SmartGroups),
	}
	// description is Optional (not Computed): UEM echoes "" for an unset
	// description, so a null prior (never configured, or import) stays null
	// instead of producing a perpetual "" -> null diff.
	//
	// B16 decision table row #150 (KEEP+cite; same pattern as row #135 in the
	// purchased-app package). UEM source:
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:42-46,
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model.Mappers/AppAssignmentV1ModelMapper.cs:33
	// (canonical Q16): a stored null Description serializes with
	// NullValueHandling.Ignore — omitted on the wire (surfaced here as
	// d.Description == ""), not sent as "". A real explicit "" would echo
	// back as "". This only collapses "" to null when the prior was ALSO
	// null, so an explicit "" is preserved.
	if d.Description == "" && (prior == nil || prior.Description.IsNull()) {
		tf.Description = types.StringNull()
	}

	// app_delivery_method: UEM defaults an omitted value to "ON_DEMAND" on
	// every full-replace PUT. A null prior (never configured) collapses
	// that default back to null; a non-null prior, or a non-default
	// server value, is always taken as-is. Live-confirmed 2026-09-23 on
	// as<internal-env> 26.2: delivery-method removal gives an identical plan alone and
	// with a sibling edit; GET shows ON_DEMAND; the replan is clean.
	// Evidence:
	// internal-design-doc
	//
	// The uppercasing itself (B16 decision table row #152, KEEP+cite; same
	// pattern as row #131 in the purchased-app package). UEM source:
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/AppDeliveryMethod.cs:32-37,
	// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:69-71
	// (canonical Q15): the wire enum names are already `AUTO`/`ON_DEMAND`
	// (caps), and Newtonsoft's StringEnumConverter default is case-sensitive
	// — so this uppercasing is redundant in practice (the server always
	// emits the canonical caps already) but harmless, and defensive against
	// a hypothetical lowercase echo.
	var priorAppDeliveryMethod types.String
	if prior != nil {
		priorAppDeliveryMethod = prior.AppDeliveryMethod
	}
	if d.AppDeliveryMethod == "" {
		tf.AppDeliveryMethod = types.StringNull()
	} else {
		upper := strings.ToUpper(d.AppDeliveryMethod)
		if priorAppDeliveryMethod.IsNull() && upper == "ON_DEMAND" {
			tf.AppDeliveryMethod = types.StringNull()
		} else {
			tf.AppDeliveryMethod = types.StringValue(upper)
		}
	}

	tf.EffectiveDate = effectiveDateFromAPI(d.EffectiveDate, prior)

	return tf
}

// effectiveDateFromAPI implements internal-task rule (D) for the internal
// (non-VPP) application assignment resource's distribution.effective_date.
// UEM stamps a fresh "now" onto any full-replace PUT that omits
// effective_date, so unlike a fixed default value, the raw server value
// alone cannot tell apart "the user never set a date" from "the user set
// today's date". Two cases are told apart by whether a prior plan/state
// assignment object exists at this position:
//
//   - No prior object at this position (prior == nil): a fresh import, or a
//     newly appended assignment with no prior element to compare against.
//     The server's date is captured as-is so an import pins the real date.
//   - A prior object exists at this position: this is a create/update
//     readback (prior is the plan just applied) or a removal readback
//     (prior is the last-known state with the field already cleared). If
//     the prior's effective_date was null, keep it null even though the
//     server stamped "now" — the plan is source of truth here, not the
//     server's side-effect timestamp. A non-null prior always surfaces the
//     server's value as-is, so real drift on an explicitly configured date
//     is visible.
//
// Live-confirmed 2026-09-23 on as<internal-env> 26.2: "internal effective_date NOW" on
// an omitted PUT field; an import captures the server date, and
// effective_date removal clears on apply. Evidence:
// internal-design-doc
func effectiveDateFromAPI(apiDate client.UEMTime, prior *tf.AppAssignmentDistributionModel) types.String {
	if prior == nil {
		if apiDate.IsZero() {
			return types.StringNull()
		}
		return types.StringValue(apiDate.Format(time.RFC3339))
	}
	if prior.EffectiveDate.IsNull() {
		return types.StringNull()
	}
	if apiDate.IsZero() {
		return types.StringNull()
	}
	return types.StringValue(apiDate.Format(time.RFC3339))
}

func priorRestriction(prior *tf.AppAssignmentModel) *tf.AppAssignmentRestrictionModel {
	if prior == nil {
		return nil
	}
	return prior.Restriction
}

// restrictionFromAPI maps the assignment restriction. UEM always echoes a
// restriction block (remove_on_unenroll false when it was not sent), while
// the block and its flag are Optional-only. So an all-default server block
// with no configured prior block maps to nil, and an unset flag inside a
// configured block stays null when the server returns the false default;
// otherwise creating an assignment without a restriction block fails with
// "inconsistent result after apply". Non-default values are always taken
// as-is so drift surfaces.
//
// B16 decision table row #154 (KEEP+cite — direct, clean match). UEM
// source: AirWatch API/AW.Mam.Api/AW.Mam.Api.Model.Mappers/AppAssignmentV1ModelMapper.cs:45,
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Helpers/AppsControllerHelper.cs:4371-4374
// (canonical Q22): RemoveOnUnenroll defaults false at create when omitted;
// Android is forced true; every other internal (non-Android, including
// macOS) platform keeps false. GET echoes the stored bool via the same
// mapper as-is — exactly what this function models.
func restrictionFromAPI(r *sdk.AppAssignmentRestrictionV1ModelV2, prior *tf.AppAssignmentRestrictionModel) *tf.AppAssignmentRestrictionModel {
	if r == nil {
		return nil
	}
	isDefault := r.RemoveOnUnenroll == nil || !*r.RemoveOnUnenroll
	if prior == nil && isDefault {
		return nil
	}
	out := &tf.AppAssignmentRestrictionModel{}
	switch {
	case r.RemoveOnUnenroll == nil:
		out.RemoveOnUnenroll = types.BoolNull()
	case !*r.RemoveOnUnenroll && prior != nil && prior.RemoveOnUnenroll.IsNull():
		out.RemoveOnUnenroll = types.BoolNull()
	default:
		out.RemoveOnUnenroll = types.BoolValue(*r.RemoveOnUnenroll)
	}
	return out
}

// stringSliceToList converts a []string to a types.List of StringType,
// lowercase-normalizing smart-group UUIDs on read (B16 decision table row
// #147, KEEP). UEM source: routes bind UUIDs as
// Guid/{uuid:guid} (canonical Q14): Guid parsing is case-insensitive with no
// stable echo-case guarantee from the server — harmless client-side
// normalization for stable diffs, not a server-mandated rule.
func stringSliceToList(ss []string) types.List {
	if len(ss) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(strings.ToLower(s))
	}
	return types.ListValueMust(types.StringType, elems)
}
