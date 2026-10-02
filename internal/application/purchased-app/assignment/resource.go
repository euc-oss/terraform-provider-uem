package assignment

import (
	"context"
	"strings"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &purchasedApplicationAssignmentResource{}
var _ resource.ResourceWithImportState = &purchasedApplicationAssignmentResource{}

func NewResource() resource.Resource {
	return &purchasedApplicationAssignmentResource{}
}

func (r *purchasedApplicationAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_purchased_application_assignment"
}

type normalizeLowercaseStringModifier struct{}

func (m normalizeLowercaseStringModifier) Description(_ context.Context) string {
	return "Normalize configured string values to lowercase UUID form."
}

func (m normalizeLowercaseStringModifier) MarkdownDescription(_ context.Context) string {
	return "Normalize configured string values to lowercase UUID form."
}

func (m normalizeLowercaseStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	normalized := tf.NormalizeUUID(req.ConfigValue.ValueString())
	if normalized != req.ConfigValue.ValueString() {
		resp.PlanValue = types.StringValue(normalized)
	}
}

func normalizeLowercaseString() planmodifier.String {
	return normalizeLowercaseStringModifier{}
}

type normalizeLowercaseStringListModifier struct{}

func (m normalizeLowercaseStringListModifier) Description(_ context.Context) string {
	return "Normalize configured string list values to lowercase."
}

func (m normalizeLowercaseStringListModifier) MarkdownDescription(_ context.Context) string {
	return "Normalize configured string list values to lowercase."
}

func (m normalizeLowercaseStringListModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var elements []types.String
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &elements, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	changed := false
	normalized := make([]attr.Value, len(elements))
	for i, element := range elements {
		if element.IsNull() || element.IsUnknown() {
			normalized[i] = element
			continue
		}
		value := strings.ToLower(strings.TrimSpace(element.ValueString()))
		if value != element.ValueString() {
			changed = true
		}
		normalized[i] = types.StringValue(value)
	}
	if !changed {
		return
	}
	listValue, diags := types.ListValue(types.StringType, normalized)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.PlanValue = listValue
}

func normalizeLowercaseStringList() planmodifier.List {
	return normalizeLowercaseStringListModifier{}
}

// nullWhenConfigNullListModifier plans an Optional+Computed List attribute as
// an explicit typed null whenever the configuration omits it — whether or
// not prior state held a value. The Update path here is a full-replace PUT,
// and live research proved UEM resets any omitted field to a server default
// on each PUT, so a null plan is what actually clears the field on apply
// (mirrors internal/profile/resource.go's nullWhenConfigNull* modifiers for
// internal-task). Unknown config is left untouched so the plan can stay unknown.
// Live-confirmed 2026-09-23 on as<internal-env> 26.2: UEM's assignment-rules PUT is a
// full replace that never keeps an omitted field (delivery method resets to
// ON_DEMAND, flags to false, excluded groups to []). Evidence:
// internal-design-doc
type nullWhenConfigNullListModifier struct{}

func (m nullWhenConfigNullListModifier) Description(_ context.Context) string {
	return "Plans this attribute as null whenever it is omitted from configuration, so removing it from HCL clears it on apply instead of preserving the prior value."
}

func (m nullWhenConfigNullListModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullWhenConfigNullListModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.ConfigValue.IsNull() {
		resp.PlanValue = types.ListNull(req.PlanValue.ElementType(ctx))
	}
}

func nullWhenConfigNullList() planmodifier.List {
	return nullWhenConfigNullListModifier{}
}

// nullWhenConfigNullStringModifier is the String-typed equivalent of
// nullWhenConfigNullListModifier — see its doc comment for the removal
// semantics (internal-task).
type nullWhenConfigNullStringModifier struct{}

func (m nullWhenConfigNullStringModifier) Description(_ context.Context) string {
	return "Plans this attribute as null whenever it is omitted from configuration, so removing it from HCL clears it on apply instead of preserving the prior value."
}

func (m nullWhenConfigNullStringModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullWhenConfigNullStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() {
		resp.PlanValue = types.StringNull()
	}
}

func nullWhenConfigNullString() planmodifier.String {
	return nullWhenConfigNullStringModifier{}
}

// nullWhenConfigNullBoolModifier is the Bool-typed equivalent of
// nullWhenConfigNullListModifier — see its doc comment for the removal
// semantics (internal-task). Used for the restriction flags that have no
// schema Default (prevent_application_backup, make_app_mdm_managed,
// desired_state_management) and for is_dynamic_template_saved. Note:
// remove_on_unenroll/prevent_removal keep their booldefault.StaticBool(false)
// unchanged and do NOT get this modifier. Nor does managed_access — it needs
// the more selective managedAccessPlanModifier below (internal-task-65i) rather
// than either a plain Default or an unconditional null-on-omit.
type nullWhenConfigNullBoolModifier struct{}

func (m nullWhenConfigNullBoolModifier) Description(_ context.Context) string {
	return "Plans this attribute as null whenever it is omitted from configuration, so removing it from HCL clears it on apply instead of preserving the prior value."
}

func (m nullWhenConfigNullBoolModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullWhenConfigNullBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.ConfigValue.IsNull() {
		resp.PlanValue = types.BoolNull()
	}
}

func nullWhenConfigNullBool() planmodifier.Bool {
	return nullWhenConfigNullBoolModifier{}
}

// managedAccessPlanModifier replaces managed_access's former
// booldefault.StaticBool(false) (internal-task-65i, live-found perpetual diff).
// Live-confirmed 2026-09-24 on as<internal-env> 26.2: plugin-framework runs Defaults
// before the plan comparison, so the prior Default(false) produced spurious
// unknowns; this modifier reproduces the default without that regression.
// Evidence: internal-design-doc
//
// Framework v1.19.0's internal/fwserver/server_planresourcechange.go runs, in
// order: TransformDefaults (L168) -> compare plan to prior state (L200) ->
// MarkComputedNilsAsUnknown (L252) -> SchemaModifyPlan attribute modifiers
// (L293) -> the resource's own ResourceWithModifyPlan.ModifyPlan (L313,
// modifyPlanForIOS in resource_modify_plan.go). Terraform core's
// proposed-new-state algorithm carries a null-in-config Optional+Computed
// attribute's prior STATE value into the plan, so an iOS assignment with
// managed_access=true in state and config leaving it unset starts the
// pipeline with a proposed value of true. But TransformDefaults
// (internal/fwschemadata/data_default.go) ignores what is already in the
// plan: whenever CONFIG is null at a path with a schema Default, it
// unconditionally overwrites the plan value with that default -- stomping
// the carried-forward true back down to false, before the L200 comparison
// ever runs. That makes plan != prior state, and MarkComputedNilsAsUnknown
// then marks EVERY OTHER Computed, config-null attribute in the whole
// resource unknown too (application_attributes, application_configuration,
// distribution.effective_date, license_usage[].redeemed, ...) -- that pass
// runs schema-wide once any diff exists, not just at managed_access's own
// path. The resource's own modifyPlanForIOS does set managed_access back to
// true, but only at L313, well after all of those siblings were already
// marked unknown, so the plan never converges: every subsequent
// `terraform plan` repeats the same "(known after apply)" noise forever.
//
// The fix is to stop using a schema Default (so TransformDefaults never
// touches this attribute) and reproduce its effect here, at the
// SchemaModifyPlan stage, with one exception: when CONFIG truly leaves
// managed_access unset AND config also has make_app_mdm_managed or
// prevent_application_backup known true, modifyPlanForIOS is going to force
// managed_access on regardless (live-verified iOS behaviour, internal-task) -- so
// this modifier leaves the plan value alone in that case (it already holds
// the carried-forward prior true), keeping plan == prior state and avoiding
// the unknown cascade entirely. In every other null-config case this
// reproduces booldefault.StaticBool(false) exactly: a create (no prior
// state), or an update where neither sibling flag is configured true, plans
// false, and modifyPlanForIOS/modifyPlanForMacOS still run unchanged
// afterwards to enforce the platform-specific rules.
type managedAccessPlanModifier struct{}

func (m managedAccessPlanModifier) Description(_ context.Context) string {
	return "Defaults managed_access to false when configuration omits it, except when the prior state already has it forced on for an iOS VPP app whose configuration still requests make_app_mdm_managed or prevent_application_backup -- kept as-is there so the plan does not spuriously mark every other computed attribute unknown (see doc comment)."
}

func (m managedAccessPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m managedAccessPlanModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.ConfigValue.IsNull() {
		// Explicit config value (true or false), or config still unknown:
		// nothing to do.
		return
	}

	if isKnownTrue(req.StateValue) {
		var makeAppMdmManaged, preventApplicationBackup types.Bool
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, req.Path.ParentPath().AtName("make_app_mdm_managed"), &makeAppMdmManaged)...)
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, req.Path.ParentPath().AtName("prevent_application_backup"), &preventApplicationBackup)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if isKnownTrue(makeAppMdmManaged) || isKnownTrue(preventApplicationBackup) {
			// Leave resp.PlanValue as-is: it already carries the prior true
			// forward, and modifyPlanForIOS is about to require it anyway.
			return
		}
	}

	resp.PlanValue = types.BoolValue(false)
}

func managedAccessDefault() planmodifier.Bool {
	return managedAccessPlanModifier{}
}

func appConfigurationSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"key":   schema.StringAttribute{Required: true},
		"value": schema.StringAttribute{Required: true},
		"type":  schema.StringAttribute{Required: true},
	}
}

func restrictionSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		// UEM echoes every restriction flag, returning false for any the
		// request omitted. Defaulting these to false keeps a partially
		// configured restriction block consistent with the applied result.
		// Live-confirmed 2026-09-23 on as<internal-env> 26.2: a null restriction prior
		// plus an all-default server object stays null, and the sub-flags are
		// Optional+Computed with Default false so a partial config is
		// consistent. Evidence:
		// internal-design-doc
		"remove_on_unenroll": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		"prevent_removal":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
		"prevent_application_backup": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				nullWhenConfigNullBool(),
			},
		},
		"make_app_mdm_managed": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				nullWhenConfigNullBool(),
			},
		},
		"managed_access": schema.BoolAttribute{
			MarkdownDescription: "Whether UEM manages this app's access restrictions. Defaults to false when left " +
				"unset. On iOS, UEM forces this on (and rejects an explicit false) whenever make_app_mdm_managed or " +
				"prevent_application_backup is true — see the plan-time error this resource raises for that case.",
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				managedAccessDefault(),
			},
		},
		"desired_state_management": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				nullWhenConfigNullBool(),
			},
		},
	}
}

func tunnelSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"per_app_vpn_profile_uuid": schema.StringAttribute{
			Optional: true,
			PlanModifiers: []planmodifier.String{
				normalizeLowercaseString(),
			},
		},
		"afw_per_app_vpn_profile_uuid": schema.StringAttribute{
			Optional: true,
			PlanModifiers: []planmodifier.String{
				normalizeLowercaseString(),
			},
		},
		"amapi_per_app_vpn_profile_uuid": schema.StringAttribute{
			Optional: true,
			PlanModifiers: []planmodifier.String{
				normalizeLowercaseString(),
			},
		},
	}
}

func (r *purchasedApplicationAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             1,
		MarkdownDescription: "Manages assignment rules for a purchased (VPP) Apple application by UUID using the MAM Apps API v2.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"application_uuid": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					normalizeLowercaseString(),
				},
			},
			"excluded_smart_groups": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
					normalizeLowercaseStringList(),
				},
			},
			"assignments": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"priority": schema.Int64Attribute{Required: true},
						"distribution": schema.SingleNestedAttribute{
							Required: true,
							Attributes: map[string]schema.Attribute{
								"name":        schema.StringAttribute{Required: true},
								"description": schema.StringAttribute{Optional: true},
								"smart_groups": schema.ListAttribute{
									MarkdownDescription: "Must be left unset or empty: UEM ignores smart groups here for VPP apps, so a non-empty list is rejected at plan time. Assign groups with `vpp_app_details.license_usage`. Read still reports any groups the server returns.",
									Optional:            true,
									Computed:            true,
									ElementType:         types.StringType,
									// B23: no nullWhenConfigNullList() here, unlike its
									// siblings above. That modifier forces the PLANNED
									// value to null whenever config omits the attribute —
									// correct for fields UEM actually resets to a default
									// on an omitted PUT, but wrong here: UEM silently
									// ignores writes to this field entirely (see the
									// ValidateConfig guard below) and always mirrors the
									// live license_usage groups back on Read regardless of
									// what was sent. Forcing null on omit would plan a
									// diff against that server-computed value on every
									// single run — live-confirmed 2/2 on paul-2609
									// (uem_purchased_application_assignment "Google" and
									// "WhatsApp Messenger", both onboarded with this field
									// populated from real state). Leaving only the
									// normalizer keeps this attribute genuinely
									// read-only/computed: an omitted config plans to the
									// prior/refreshed state value instead of null.
									PlanModifiers: []planmodifier.List{
										normalizeLowercaseStringList(),
									},
								},
								"app_delivery_method": schema.StringAttribute{
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.String{
										nullWhenConfigNullString(),
									},
								},
								"effective_date": schema.StringAttribute{
									MarkdownDescription: "Not applicable to purchased (VPP) apps: UEM ignores it, so setting it is rejected at plan time.",
									Optional:            true,
									Computed:            true,
								},
								"vpp_app_details": schema.SingleNestedAttribute{
									Optional: true,
									Attributes: map[string]schema.Attribute{
										"license_usage": schema.ListNestedAttribute{
											MarkdownDescription: "Smart groups that receive the app, with the number of VPP licenses allocated to each. This is the only way to assign a smart group to a purchased (VPP) app.",
											Optional:            true,
											NestedObject: schema.NestedAttributeObject{
												Attributes: map[string]schema.Attribute{
													"smart_group_uuid": schema.StringAttribute{
														Required: true,
														PlanModifiers: []planmodifier.String{
															normalizeLowercaseString(),
														},
													},
													"allocated": schema.Int64Attribute{Required: true},
													// redeemed deliberately has no
													// int64planmodifier.UseStateForUnknown(): this
													// attribute is index-paired within
													// license_usage, so a reorder of the list (or a
													// redemption happening between plan and apply)
													// could make the carried-forward state value
													// wrong for the index it lands on after the PUT
													// went through — producing "inconsistent result
													// after apply" instead of a merely noisy "(known
													// after apply)" diff. The noise this leaves
													// behind is tracked by internal-task.
													"redeemed": schema.Int64Attribute{
														Computed: true,
													},
												},
											},
										},
									},
								},
							},
						},
						"restriction": schema.SingleNestedAttribute{
							Optional:   true,
							Attributes: restrictionSchemaAttributes(),
						},
						"tunnel": schema.SingleNestedAttribute{
							Optional:   true,
							Attributes: tunnelSchemaAttributes(),
						},
						"application_configuration": schema.ListNestedAttribute{
							Optional: true,
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: appConfigurationSchemaAttributes(),
							},
						},
						"application_attributes": schema.ListNestedAttribute{
							Optional: true,
							Computed: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: appConfigurationSchemaAttributes(),
							},
						},
						"is_dynamic_template_saved": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							PlanModifiers: []planmodifier.Bool{
								nullWhenConfigNullBool(),
							},
						},
					},
				},
			},
		},
	}
}
