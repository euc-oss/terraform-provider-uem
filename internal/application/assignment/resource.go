package assignment

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure resource types satisfy framework interfaces.
var _ resource.Resource = &applicationAssignmentResource{}
var _ resource.ResourceWithImportState = &applicationAssignmentResource{}

func NewResource() resource.Resource {
	return &applicationAssignmentResource{}
}

func NewApplicationAssignmentResource() resource.Resource {
	return NewResource()
}

// applicationAssignmentResource defines the resource implementation.
type applicationAssignmentResource struct {
	client                  *sdk.Client
	newAppAssignmentService func(*sdk.Client) appAssignmentServiceAPI

	// appAssignmentSvc is the lazily initialized Layer 2 service used by CRUD paths.
	appAssignmentSvc     appAssignmentServiceAPI
	appAssignmentSvcOnce sync.Once
	appAssignmentSvcErr  error
}

// appAssignmentService returns the Layer 2 AppAssignmentService, constructing it lazily on
// first use via the configured factory. Subsequent calls return the cached
// service — or the cached error, if the first attempt failed.
func (r *applicationAssignmentResource) appAssignmentService(ctx context.Context) (appAssignmentServiceAPI, error) {
	_ = ctx
	r.appAssignmentSvcOnce.Do(func() {
		if r.client == nil {
			r.appAssignmentSvcErr = fmt.Errorf("application assignment resource is not configured: SDK client is nil")
			return
		}
		factory := r.newAppAssignmentService
		if factory == nil {
			factory = defaultAppAssignmentServiceFactory
		}
		r.appAssignmentSvc = factory(r.client)
	})
	return r.appAssignmentSvc, r.appAssignmentSvcErr
}

// Note: useStateForNullModifier, useStateForNullListModifier, and useStateForNullStringModifier
// have been removed as they are not currently used in the schema.
// If needed in future, they can be restored from git history.

// nullWhenConfigNullListModifier plans an Optional+Computed List attribute as
// an explicit typed null whenever the configuration omits it — whether or
// not prior state held a value. This is what makes removing the attribute
// from HCL (e.g. excluded_smart_groups) plan as NULL instead of carrying the
// prior value forward: the Update path is a full-replace PUT, and UEM resets
// any omitted field to its server default, so a null plan is what actually
// clears the field on apply (mirrors internal/profile/resource.go's
// nullWhenConfigNull* modifiers for internal-task). Unknown config (e.g. fed by
// a not-yet-known variable) is left untouched so the plan can stay unknown.
// Live-confirmed 2026-09-23 on as<internal-env> 26.2: UEM's assignment-rules PUT is a
// full replace that never keeps an omitted field (excluded groups reset to
// [], delivery method to ON_DEMAND, effective_date to NOW); internal
// delivery-method and effective_date/excluded-groups removal were each
// separately confirmed to clear on apply. Evidence:
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
// semantics (internal-task). Used for distribution.app_delivery_method and
// distribution.effective_date, called by the framework once per assignments
// list element since those are nested attributes.
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

// normalizeLowercaseStringListModifier canonicalizes configured string lists
// to lowercase so case-only differences do not produce perpetual diffs.
//
// B16 decision table row #146 (KEEP). UEM source: routes bind UUIDs as
// Guid/{uuid:guid} (canonical Q14): Guid parsing is case-insensitive with no
// stable echo-case guarantee from the server — harmless client-side
// normalization for stable diffs, not a server-mandated rule.
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

		value := strings.ToLower(element.ValueString())
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

func (r *applicationAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_assignment"
}

func (r *applicationAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             1,
		MarkdownDescription: "Manages a Workspace ONE UEM Application Assignment",

		Attributes: map[string]schema.Attribute{

			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"application_uuid": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "UUID of the application this assignment rule applies to. Changing this " +
					"targets a different application's assignment rule (the UEM assignment API is scoped per " +
					"application UUID) rather than updating the current one in place, so it forces replacement — " +
					"otherwise `id` (which mirrors application_uuid) would change across what Terraform planned " +
					"as an in-place update, which the framework rejects as an inconsistent provider result.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"excluded_smart_groups": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Computed:    true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
			},

			"assignments": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{

						"priority": schema.Int64Attribute{
							Required: true,
						},
						"distribution": schema.SingleNestedAttribute{
							Required: true,
							Attributes: map[string]schema.Attribute{
								"name":        schema.StringAttribute{Required: true},
								"description": schema.StringAttribute{Optional: true},
								"smart_groups": schema.ListAttribute{
									Required: true,
									// Optional:    true,
									ElementType: types.StringType,
									// Computed:    true,
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
									MarkdownDescription: "RFC3339 effective date. When unset at create, UEM stamps the current date; an import captures the date UEM has. Set it explicitly to pin it. Removing it from the configuration lets UEM re-stamp it at the next apply.",
									// Required: true,
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.String{
										nullWhenConfigNullString(),
									},
								},
							},
						},

						"restriction": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"remove_on_unenroll": schema.BoolAttribute{Optional: true},
								//			"prevent_application_backup": schema.BoolAttribute{Optional: true},
								//			"make_app_mdm_managed":       schema.BoolAttribute{Optional: true},
								//			"managed_access":             schema.BoolAttribute{Optional: true},
							},
						},

						//	"tunnel": schema.SingleNestedAttribute{
						//		Optional: true,
						//		Attributes: map[string]schema.Attribute{
						//			"per_app_vpn_profile_uuid":     schema.StringAttribute{Optional: true},
						//			"afw_per_app_vpn_profile_uuid": schema.StringAttribute{Optional: true},
						//		},
						//	},

						//	"application_configuration": schema.ListNestedAttribute{
						//		Optional: true,
						//		NestedObject: schema.NestedAttributeObject{
						//			Attributes: map[string]schema.Attribute{
						//				"key":   schema.StringAttribute{Required: true},
						//				"value": schema.StringAttribute{Required: true},
						//				"type":  schema.StringAttribute{Required: true},
						//			},
						//		},
						//	},
					},
				},
			},

			//"application_msi_deployment_params": schema.SingleNestedAttribute{
			//	Optional: true,
			//	Attributes: map[string]schema.Attribute{
			//		"install_device_restart":      schema.StringAttribute{Optional: true},
			//		"uninstall_device_restart":    schema.StringAttribute{Optional: true},
			//		"installer_success_exit_code": schema.StringAttribute{Optional: true},
			//	},
			//},

			//"remediation_assignment_parameters": schema.SingleNestedAttribute{
			//	Optional: true,
			//	Attributes: map[string]schema.Attribute{
			//		"remediation_id":         schema.StringAttribute{Optional: true},
			//		"vulnerability_id":       schema.StringAttribute{Optional: true},
			//		"product_id":             schema.StringAttribute{Optional: true},
			//		"remediation_action":     schema.Int64Attribute{Optional: true},
			//		"vulnerability_provider": schema.Int64Attribute{Optional: true},
			//	},
			//},
		},
	}
}
