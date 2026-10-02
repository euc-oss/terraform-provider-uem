package assignment

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
)

// refreshOrgGroupUUID copies the parent script's organization group into
// data. If the script GET fails or returns no organization group (for
// example because the script is also gone), the prior value is kept and only
// a warning is logged: the attribute is Computed with UseStateForUnknown, so
// keeping it never causes a diff, and Create/Read must not fail over it. An
// unknown prior value (a fresh Create) becomes null so state stays known.
// Import needs no special case: the Read that follows fills it here.
func (r *scriptAssignmentResource) refreshOrgGroupUUID(
	ctx context.Context,
	scriptUUID string,
	data *tf.ScriptAssignmentRuleModel,
) {
	if data.OrganizationGroupUUID.IsUnknown() {
		data.OrganizationGroupUUID = types.StringNull()
	}
	svc, err := r.scriptService()
	if err != nil {
		tflog.Warn(ctx, "cannot refresh organization_group_uuid; keeping prior value", map[string]any{
			"script_uuid": scriptUUID,
			"error":       err.Error(),
		})
		return
	}
	_, script, err := svc.GetScriptAsync(ctx, scriptUUID)
	if err != nil || script == nil || strings.TrimSpace(script.OrganizationGroupUUID) == "" {
		tflog.Warn(ctx, "parent script lookup gave no organization_group_uuid; keeping prior value", map[string]any{
			"script_uuid": scriptUUID,
			"error":       fmt.Sprint(err),
		})
		return
	}
	data.OrganizationGroupUUID = types.StringValue(script.OrganizationGroupUUID)
}
