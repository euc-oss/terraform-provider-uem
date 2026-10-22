package macsensor

import (
	"context"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/models"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func (r *macsensorResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             3,
		MarkdownDescription: "Manages a macOS device sensor in Workspace ONE UEM (MDM API V2).",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Device sensor UUID assigned by UEM after creation",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_group_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Organization group UUID that owns the sensor",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Sensor name. Must start with a lowercase letter and use only lowercase letters, digits, and underscores (2-64 characters).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						tf.SensorNamePattern,
						"name must start with a lowercase letter, use only lowercase letters, digits, and underscores, and be 2-64 characters",
					),
				},
			},
			"description": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Description of the device sensor. UEM cannot clear a description once set: " +
					"removing it or setting it to \"\" on an existing sensor is rejected at plan time. " +
					"Change it to a new non-empty value, or keep the current one. Read back exactly as UEM " +
					"returns it: an unset description reads as \"\" (empty string), not null, since UEM's wire " +
					"format has no way to represent unset separately from empty.",
			},
			"language": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Script language for the sensor (e.g. BASH, ZSH)",
			},
			"execution_context": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Execution context for the sensor script (e.g. SYSTEM, USER)",
			},
			"execution_architecture": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Execution architecture under which the sensor script runs on the device. " +
					"Defaults to \"EITHER64OR32BIT\". Only \"EITHER64OR32BIT\" is currently supported for " +
					"APPLE_OSX sensors: the UEM API accepts \"64BIT\" and \"32BIT\" as documented values in the " +
					"SDK's type definition, but live-testing confirms both are rejected with an HTTP 400 for this " +
					"platform, so this provider restricts the value to the one live-confirmed-valid option.",
				Default: stringdefault.StaticString(tf.ExecutionArchitectureEitherOr),
				Validators: []validator.String{
					stringvalidator.OneOf(tf.ExecutionArchitectureEitherOr),
				},
			},
			"response_data_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Expected response data type from the sensor script (e.g. STRING, INTEGER)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"code": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Base64-encoded sensor script executed on the device",
			},
			"is_read_only": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the sensor is read-only for the current organization group",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
