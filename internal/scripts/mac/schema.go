package macscript

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func (r *macscriptResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a macOS script in Workspace ONE UEM.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Script UUID assigned by UEM after creation",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_group_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Organization group UUID that owns the script",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the script",
			},
			"description": schema.StringAttribute{
				// B39: Optional+Computed with UseStateForUnknown, not
				// Optional-only. Live (as<internal-env> 26.2): creating a script with
				// description unset in config planned description as null
				// (Optional-only, non-Computed means the planned value must
				// stay exactly what config said), but UEM's readback is ""
				// (its wire format has no way to represent "unset"
				// separately from empty) -- apply then failed with
				// "Provider produced inconsistent result after apply".
				// Computed marks an omitted config Unknown in the plan
				// instead of null, so UEM's "" readback resolves it without
				// conflict. Create/Update still don't send description when
				// it's null/unknown (see setStringIfKnown in
				// internal/scripts/mac/state/state.go) -- unchanged by this
				// fix.
				Optional: true,
				Computed: true,
				MarkdownDescription: "Description of the script. Read back exactly as UEM returns it: an " +
					"unset description reads as \"\" (empty string), not null, since UEM's wire format has no way " +
					"to represent unset separately from empty.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform": schema.StringAttribute{
				Required: true,
				// B40: the example/documented value is UEM's actual wire
				// enum, "APPLE_OSX", not the V2 Profiles API's "AppleOsX"
				// (sdk.PlatformAppleOsX). Live (as<internal-env> 26.2): a script
				// created with platform = "AppleOsX" applied fine but UEM's
				// readback was "APPLE_OSX", and since platform is Required
				// (not Computed) that mismatch alone is a "Provider produced
				// inconsistent result after apply" -- Required attributes,
				// like Computed ones, must match the planned value exactly.
				// The SDK's wire model confirms scripts use the
				// SCREAMING_SNAKE_CASE family, not the Profiles API's mixed
				// case: ScriptResourceV1/CreateScriptV1.Platform is
				// documented "string enum on wire, e.g. WIN_RT"
				// (terraform-sdk-uem internal/mdm/v1/models.go), matching
				// uem_mac_sensor's fixed "APPLE_OSX" platform. There is no
				// provider-side mapping between the two spellings (faithful
				// pass-through both ways, see ToCreateScriptV1/
				// ReadAPIIntoState in internal/scripts/mac/state/state.go);
				// a config must supply UEM's own spelling.
				MarkdownDescription: "Target platform for the script (e.g. APPLE_OSX)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"script_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Script language (e.g. BASH, ZSH, PYTHON)",
			},
			"execution_context": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Execution context for the script (e.g. USER, SYSTEM)",
			},
			"platform_architecture": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Platform architecture for the script (e.g. X64, ARM64)",
			},
			"script_data": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Base64-encoded script body to execute on the device",
			},
			"timeout": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Script execution timeout in seconds. UEM returns this even for a " +
					"script created without it set explicitly (UEM's own default is 30); the provider reads " +
					"it back exactly as UEM returns it. A configuration that omits this attribute plans no " +
					"change once a value is recorded in state; set it explicitly to change it.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"script_variables": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Environment variables available to the script at runtime",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Variable name",
						},
						"value": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Variable value",
						},
					},
				},
			},
			"allowed_in_catalog": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether the script may appear in the Workspace ONE catalog",
			},
			"catalog_display": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Catalog presentation settings when allowed_in_catalog is true",
				Attributes: map[string]schema.Attribute{
					"display_name": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Name shown to end users in the catalog",
					},
					"display_desc": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Description shown to end users in the catalog",
					},
					"pre_action_text": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Text shown to the user before the script runs",
					},
					"post_action_text": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Text shown to the user after the script runs",
					},
					"action_type": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Catalog action type for the script",
					},
					"catalog_icon_url": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "URL of the icon displayed in the catalog",
					},
					"categories": schema.ListAttribute{
						Optional:    true,
						Computed:    true,
						ElementType: types.StringType,
						// Computed with UseStateForUnknown: an omitted categories takes
						// UEM's readback (e.g. []) instead of planning null, which would
						// otherwise be an inconsistent result after apply and a
						// perpetual diff.
						PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
						MarkdownDescription: "Catalog category identifiers. Optional: when omitted, the " +
							"resource takes UEM's value instead of planning null. UEM returns an empty list " +
							"for a script that is in the catalog with no categories assigned; the provider " +
							"reads that back as a known empty list, not null. A configuration that omits this " +
							"attribute plans no change once a value is recorded in state; set it explicitly to " +
							"change it.",
					},
				},
			},
			"user_interaction": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether the script requires user interaction before execution. UEM " +
					"returns this even for a script created without it set explicitly (UEM's own default is " +
					"false); the provider reads it back exactly as UEM returns it. A configuration that omits " +
					"this attribute plans no change once a value is recorded in state; set it explicitly to " +
					"change it.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
