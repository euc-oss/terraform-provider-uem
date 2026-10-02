package deployment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (r *updateDeploymentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             1,
		MarkdownDescription: "Manages a device update deployment in Workspace ONE UEM (MDM Updates API V1).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Deployment UUID assigned by UEM after creation",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"update_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Device update UUID this deployment belongs to",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization_group_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Organization group UUID where the deployment is created",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Deployment name",
			},
			"deployment_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Deployment type. Allowed values: DOWNLOAD_AND_INSTALL, DOWNLOAD_ONLY, INSTALL_ONLY",
			},
			"deployment_start_time": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Deployment start time (UEM datetime, e.g. 2026-07-28T16:00:00.000Z)",
			},
			"smart_group_uuids": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "Smart group UUIDs that receive this update deployment. Read back exactly as UEM returns it (B16 (b)-row #196 removal): a server-omitted list reads as null, not [].",
			},
			"notifications": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Optional notification preferences for the deployment (max 2). Read back exactly as UEM returns it (B16 (b)-row #196 removal): a server-omitted list reads as null, not [].",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Notification trigger action: DOWNLOAD_SUCCESS or INSTALL_SUCCESS, sent to and read from UEM exactly as configured (B16 (b)-row #194 removal: no uppercasing).",
						},
						"message": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Push notification message (max 4000 characters)",
						},
						"message_template_id": schema.Int64Attribute{
							Optional:            true,
							MarkdownDescription: "Email message template ID (mutually exclusive with message)",
						},
					},
				},
			},
		},
	}
}
