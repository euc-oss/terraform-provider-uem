package assignment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (r *sensorAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             2,
		MarkdownDescription: "Manages device sensor assignments for a Workspace ONE UEM sensor (MDM API V2).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"sensor_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Device sensor UUID to manage assignments for",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"assignments": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Assignment groups for the sensor, including smart groups and triggers",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"assignment_uuid": schema.StringAttribute{
							Optional:            true,
							Computed:            true,
							MarkdownDescription: "Assignment UUID assigned by UEM",
						},
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Assignment group name",
						},
						"ranking": schema.Int64Attribute{
							Required:            true,
							MarkdownDescription: "Assignment priority rank (1 is highest)",
						},
						"smart_group_uuids": schema.ListAttribute{
							Required:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Smart group UUIDs assigned to this assignment group, sent to and read from UEM exactly as configured (B16 (b)-row #184 removal: no lower-casing).",
						},
						"trigger_type": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Trigger type (e.g. SCHEDULE, EVENT, SCHEDULEANDEVENT), sent to and read from UEM exactly as configured (B16 (b)-row #183 removal: no uppercasing).",
						},
						"event_triggers": schema.ListAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Event trigger values when trigger_type includes events (e.g. LOGIN, STARTUP), sent to and read from UEM exactly as configured (B16 (b)-row #181/#182/#183 removal: no uppercasing).",
						},
					},
				},
			},
		},
	}
}
