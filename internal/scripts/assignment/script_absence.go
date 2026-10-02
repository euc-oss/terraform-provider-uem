package assignment

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
	"github.com/euc-oss/terraform-provider-uem/internal/scripts/scriptlookup"
)

// stateRmHint is appended to an ambiguous 500/1000 error when state has no
// organization_group_uuid to confirm the script is gone with.
const stateRmHint = "The parent script may have been deleted outside Terraform, but this state has no " +
	"organization_group_uuid (it predates that attribute and has not been refreshed since), so absence " +
	"cannot be confirmed. If the script is gone, remove the assignment from state with " +
	"`terraform state rm <addr>` (for example `terraform state rm uem_script_assignment.example`)."

// scriptListPageSize is the page size used when paging the OG-scoped script
// list. It is a var only so tests can exercise multi-page paging cheaply.
var scriptListPageSize = scriptlookup.DefaultPageSize

// scriptListMaxPages is the last-resort cap on pages walked; see
// scriptlookup.DefaultMaxPages. It is a var only so tests can shrink it.
var scriptListMaxPages = scriptlookup.DefaultMaxPages

// scriptAbsentFromOrgGroup reports whether scriptUUID is confirmed absent
// from the organization group's script list. It delegates to
// scriptlookup.AbsentFromOrgGroup, which documents the termination rule;
// only (true, nil) means absent.
func scriptAbsentFromOrgGroup(
	ctx context.Context,
	svc scriptServiceAPI,
	orgGroupUUID string,
	scriptUUID string,
) (bool, error) {
	return scriptlookup.AbsentFromOrgGroup(ctx, svc, orgGroupUUID, scriptUUID, scriptListPageSize, scriptListMaxPages)
}

// knownOrgGroupUUID returns the trimmed organization_group_uuid from state,
// or ok=false when it is null, unknown or blank.
func knownOrgGroupUUID(data *tf.ScriptAssignmentRuleModel) (string, bool) {
	if data.OrganizationGroupUUID.IsNull() || data.OrganizationGroupUUID.IsUnknown() {
		return "", false
	}
	v := strings.TrimSpace(data.OrganizationGroupUUID.ValueString())
	return v, v != ""
}

// resolveAmbiguousGone handles an ambiguous 500/1000 from Read or Delete.
// It returns gone=true only when state holds a known organization group and
// the confirming list lookup succeeded without finding the script. Otherwise
// the caller keeps its original error; hint is the extra recovery text to
// append (non-empty only when no organization group is known).
func (r *scriptAssignmentResource) resolveAmbiguousGone(
	ctx context.Context,
	data *tf.ScriptAssignmentRuleModel,
	scriptUUID string,
) (gone bool, hint string) {
	orgGroupUUID, ok := knownOrgGroupUUID(data)
	if !ok {
		return false, stateRmHint
	}
	svc, err := r.scriptService()
	if err != nil {
		return false, ""
	}
	absent, err := scriptAbsentFromOrgGroup(ctx, svc, orgGroupUUID, scriptUUID)
	tflog.Debug(ctx, "confirming lookup for script assignment's script after 500/1000", map[string]any{
		"script_uuid":             scriptUUID,
		"organization_group_uuid": orgGroupUUID,
		"absent":                  absent,
		"lookup_error":            fmt.Sprint(err),
	})
	return err == nil && absent, ""
}

// appendHint appends hint to detail on a new paragraph when hint is set.
func appendHint(detail, hint string) string {
	if hint == "" {
		return detail
	}
	return detail + "\n\n" + hint
}
