package state

import (
	"context"
	"strings"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	client "github.com/euc-oss/terraform-sdk-uem/client"
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

	// excluded_smart_groups
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

	// excluded_smart_groups
	sgElems := make([]attr.Value, len(api.ExcludedSmartGroups))
	for i, s := range api.ExcludedSmartGroups {
		sgElems[i] = types.StringValue(strings.ToLower(s))
	}
	sgList, d := types.ListValue(types.StringType, sgElems)
	diags.Append(d...)
	data.ExcludedSmartGroups = sgList

	// assignments
	tfAssigns := make([]tf.AppAssignmentModel, len(api.Assignments))
	for i, a := range api.Assignments {
		tfAssigns[i] = assignmentFromAPI(a)
	}
	assignList, d := types.ListValueFrom(ctx, assignmentElemType, tfAssigns)
	diags.Append(d...)
	data.Assignments = assignList

	return diags
}

// to api.
func assignmentToAPI(ctx context.Context, a tf.AppAssignmentModel) (sdk.AppAssignmentV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	priority := int(a.Priority.ValueInt64())
	dist, d := distributionToAPI(ctx, a.Distribution)
	diags.Append(d...)
	out := sdk.AppAssignmentV2Model{
		Priority:     &priority,
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

	// smart_groups
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

func assignmentFromAPI(a sdk.AppAssignmentV2Model) tf.AppAssignmentModel {
	var priority int64
	if a.Priority != nil {
		priority = int64(*a.Priority)
	}
	return tf.AppAssignmentModel{
		Priority:     types.Int64Value(priority),
		Distribution: distributionFromAPI(a.Distribution),
		Restriction:  restrictionFromAPI(a.Restriction),
	}
}

// from api.
func distributionFromAPI(d sdk.AppAssignmentDistributionV2Model) tf.AppAssignmentDistributionModel {
	tf := tf.AppAssignmentDistributionModel{
		Name:        types.StringValue(d.Name),
		Description: types.StringValue(d.Description),
		SmartGroups: stringSliceToList(d.SmartGroups),
	}

	// app_delivery_method: normalize to canonical uppercase string.
	if d.AppDeliveryMethod != "" {
		tf.AppDeliveryMethod = types.StringValue(strings.ToUpper(d.AppDeliveryMethod))
	} else {
		tf.AppDeliveryMethod = types.StringNull()
	}

	// effective_date: time.Time → RFC3339 string
	if !d.EffectiveDate.IsZero() {
		tf.EffectiveDate = types.StringValue(d.EffectiveDate.Format(time.RFC3339))
	} else {
		tf.EffectiveDate = types.StringNull()
	}

	return tf
}

func restrictionFromAPI(r *sdk.AppAssignmentRestrictionV1ModelV2) *tf.AppAssignmentRestrictionModel {
	if r == nil {
		return nil
	}
	tf := &tf.AppAssignmentRestrictionModel{}
	if r.RemoveOnUnenroll != nil {
		tf.RemoveOnUnenroll = types.BoolValue(*r.RemoveOnUnenroll)
	} else {
		tf.RemoveOnUnenroll = types.BoolNull()
	}
	return tf
}

// stringSliceToList converts a []string to a types.List of StringType.
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
