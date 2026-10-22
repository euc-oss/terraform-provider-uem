package macsensor

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithModifyPlan = &macsensorResource{}

// descriptionClearMessage is the plan-time error detail for a config that
// would clear an existing sensor description.
const descriptionClearMessage = "UEM cannot clear a sensor description: an update that omits the description, " +
	"or sets it to \"\", keeps the current one. Set description to a new non-empty value, or keep the current one."

// replaceTriggerAttributes are the attributes whose change forces replacement
// (they carry stringplanmodifier.RequiresReplace in the schema;
// TestReplaceTriggerAttributesMatchSchema keeps the two in step). The
// framework passes resource ModifyPlan an empty RequiresReplace, so a
// replacement is detected by comparing these planned values with state.
var replaceTriggerAttributes = []string{"organization_group_uuid", "name", "response_data_type"}

// ModifyPlan fails the plan when an in-place update would clear an existing
// description: prior state has a non-empty description and the config sets it
// to null or "". A sensor PUT never clears Description (live-verified:
// omitted, "" and null all keep the current value), so such an apply would end
// in "Provider produced inconsistent result after apply". Create, destroy,
// unknown config values and a replacement (the new sensor is simply created
// without a description) are left alone.
//
// Live-confirmed 2026-09-24 on as<internal-env> 26.2: create with a description, then
// remove it — plan errors and GET keeps the value; an explicit "" also
// errors; a new (non-empty) value applies with a clean replan. Evidence:
// internal-design-doc
func (r *macsensorResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var stateDescription, configDescription types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("description"), &stateDescription)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("description"), &configDescription)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !descriptionWouldClear(stateDescription, configDescription) {
		return
	}

	for _, name := range replaceTriggerAttributes {
		var planned, prior types.String
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(name), &planned)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(name), &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if planned.IsUnknown() || !planned.Equal(prior) {
			return
		}
	}

	resp.Diagnostics.AddAttributeError(path.Root("description"), "Sensor description cannot be cleared", descriptionClearMessage)
}

// descriptionWouldClear reports whether the config removes or empties a
// non-empty prior description. Unknown config values never count.
func descriptionWouldClear(state, config types.String) bool {
	if state.IsNull() || state.IsUnknown() || state.ValueString() == "" {
		return false
	}
	if config.IsUnknown() {
		return false
	}
	return config.IsNull() || config.ValueString() == ""
}
