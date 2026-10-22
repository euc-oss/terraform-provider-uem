package assignment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (r *scriptAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             1,
		MarkdownDescription: "Manages script assignments for a Workspace ONE UEM script.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_group_uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the organization group that owns the parent script. Read from UEM on every successful refresh and used to confirm the script is gone when UEM answers an ambiguous HTTP 500 (errorCode 1000). State written before this attribute existed gets it on the next successful refresh.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"script_uuid": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"assignments": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"assignment_uuid": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"priority": schema.Int64Attribute{
							Required: true,
						},
						"deployment_mode": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Deployment mode, sent to and read from UEM exactly as configured (B16 (b)-row #167/#168 removal: no uppercasing).",
						},
						"show_in_catalog": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"memberships": schema.ListNestedAttribute{
							Required: true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"smart_group_uuid": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "Smart group UUID, sent to and read from UEM exactly as configured (B16 (b)-row #169 removal: no lower-casing).",
									},
									"smart_group_name": schema.StringAttribute{
										Optional: true,
										Computed: true,
									},
								},
							},
						},
						"script_deployment": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"trigger_type": schema.StringAttribute{
									Optional:            true,
									Computed:            true,
									MarkdownDescription: "Trigger type, sent to and read from UEM exactly as configured (B16 (b)-row #167/#168 removal: no uppercasing).",
								},
								"trigger_events": schema.ListAttribute{
									Optional:            true,
									ElementType:         types.StringType,
									Computed:            true,
									MarkdownDescription: "Trigger event names, sent to and read from UEM exactly as configured (B16 (b)-row #166 removal: no uppercasing).",
								},
								"trigger_schedule": schema.StringAttribute{
									Optional:            true,
									Computed:            true,
									MarkdownDescription: "Trigger schedule, sent to and read from UEM exactly as configured (B16 (b)-row #167/#168 removal: no uppercasing).",
								},
							},
						},
					},
				},
			},
		},
	}
}
