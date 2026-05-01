package assignment

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem"
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

// normalizeLowercaseStringListModifier canonicalizes configured string lists
// to lowercase so case-only differences do not produce perpetual diffs.
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
			},

			"excluded_smart_groups": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Computed:    true,
				//PlanModifiers: []planmodifier.List{
				//	normalizeLowercaseStringList(),
				//},
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
								},
								"effective_date": schema.StringAttribute{
									// Required: true,
									Optional: true,
									Computed: true,
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
