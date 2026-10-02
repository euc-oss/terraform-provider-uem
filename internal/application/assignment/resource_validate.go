package assignment

import (
	"context"

	"github.com/euc-oss/terraform-provider-uem/internal/application/assignmentpriority"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithValidateConfig = &applicationAssignmentResource{}

// ValidateConfig rejects an assignments[].priority list whose priorities are
// not exactly {0, ..., n-1} (assignmentpriority.KindGap / KindDuplicate), or
// that is a valid set but not listed in ascending priority order
// (assignmentpriority.KindOutOfOrder) — see the assignmentpriority package
// doc comment for why UEM requires both. The config is walked as generic
// objects so an unknown assignments list (for example one built from a
// not-yet-known variable) is skipped rather than failing the decode.
//
// Possible later improvement: this resource's state.go pairs the UEM
// readback with the configured assignments by list index rather than by
// priority; re-pairing by priority on Read would allow lifting the
// out-of-order check (KindOutOfOrder) here.
func (r *applicationAssignmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var assignments types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("assignments"), &assignments)...)
	if resp.Diagnostics.HasError() || assignments.IsNull() || assignments.IsUnknown() {
		return
	}

	priorities, ok := assignmentpriority.CollectPriorities(assignments)
	if !ok {
		return
	}

	// Live-confirmed 2026-09-24 on as<internal-env> 26.2: UEM's 400 was live-probed for
	// both the {0..n-1} gap/duplicate case ([0,2], [0,0] rejected) and the
	// ascending-order case (UEM itself accepts [1,0]; the order check here is
	// a provider artifact of pairing Read's readback by list index). Evidence:
	// internal-design-doc

	for _, finding := range assignmentpriority.Check(priorities) {
		summary, detail := assignmentpriority.Diagnostic(finding, len(priorities))
		resp.Diagnostics.AddAttributeError(
			path.Root("assignments").AtListIndex(finding.Index).AtName("priority"),
			summary,
			detail,
		)
	}
}
