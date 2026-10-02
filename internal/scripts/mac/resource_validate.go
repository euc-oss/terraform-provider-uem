package macscript

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithValidateConfig = &macscriptResource{}

// ValidateConfig rejects a config that leaves execution_context unset. The
// attribute stays Optional in the schema, but UEM answers a create without it
// with HTTP 500 (errorCode 1000) and creates nothing (live, as<internal-env>), so the
// failure is surfaced at plan time instead. An unknown value (for example one
// taken from another resource) is skipped; an empty string is left to UEM.
func (r *macscriptResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var executionContext types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("execution_context"), &executionContext)...)
	if resp.Diagnostics.HasError() || executionContext.IsUnknown() || !executionContext.IsNull() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("execution_context"),
		"Missing execution_context",
		"execution_context is required by UEM: creating a macOS script without it fails with HTTP 500. Set it to USER or SYSTEM.",
	)
}
