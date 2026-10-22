package macapplication

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// appVersionRequiresReplace replaces on a genuine version change but not when
// state and plan are numerically equivalent (tf.AppVersionsEquivalent — e.g.
// import recorded the API's "1.0.0.0" or "1.1.0.0" and the configuration says
// "1.0.0" or "1.01"); that case becomes
// a one-time in-place Update that persists the configured value with no API
// call. A null or unknown value on either side keeps the previous
// behavior: any change replaces.
func appVersionRequiresReplace() planmodifier.String {
	const desc = "Changing app_version forces replacement, except between numerically equivalent versions " +
		"(\"1.0.0\" vs \"1.0.0.0\", \"1.01\" vs \"1.1.0.0\"), which is updated in place without an API call."
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
				resp.RequiresReplace = true
				return
			}
			resp.RequiresReplace = !tf.AppVersionsEquivalent(req.StateValue.ValueString(), req.PlanValue.ValueString())
		},
		desc, desc,
	)
}
