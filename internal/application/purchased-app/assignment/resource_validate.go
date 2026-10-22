package assignment

import (
	"context"
	"fmt"

	"github.com/euc-oss/terraform-provider-uem/internal/application/assignmentpriority"
	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithValidateConfig = &purchasedApplicationAssignmentResource{}

// Diagnostic summary/detail for the desired_state_management /
// is_dynamic_template_saved checks below (internal-task minor 1). These used to
// live in ModifyPlan (resource_modify_plan.go); see this function's doc
// comment for why they were moved here.
const (
	neverHonouredSummary = "Restriction setting is never applied by UEM"
	neverHonouredDetail  = "%s is never honoured by UEM for VPP purchased-app assignments on any platform: the server " +
		"silently ignores it, so the applied state can never match what is configured here, and Terraform " +
		"cannot converge. Remove it from configuration."
)

// ValidateConfig rejects a non-empty distribution.smart_groups, and any
// configured (non-null, known) distribution.effective_date, on any
// assignment. UEM silently drops smart_groups for VPP apps, so the only way
// to assign a group is vpp_app_details.license_usage; UEM likewise ignores
// effective_date for VPP apps, so honoring any configured value — including
// an explicit empty string — would silently do nothing.
//
// ValidateConfig also rejects restriction.desired_state_management and
// is_dynamic_template_saved whenever configured true: neither is ever
// honoured by UEM, on either platform (live-verified), so the outcome does
// not depend on platform and no lookup is needed. This check used to live in
// ModifyPlan, gated behind a platform lookup and an unknown-application_uuid
// early return; moving it here means it also runs on `terraform validate`
// (not just `terraform plan`) and is never skipped just because
// application_uuid happens to be unknown. The desired_state_management case
// is also caught by the post-apply backstop (compareRestrictionReadback in
// resource_restriction_backstop.go), but only after the PUT has already gone
// out and done nothing; is_dynamic_template_saved has no backstop at all
// (state.go's nullIfDefaultFalseBool keeps state null only when the *prior*
// value was null, so a planned true here reads back as an explicit false and
// Terraform core itself raises an opaque "provider produced inconsistent
// result after apply" error) — a config-time error is a much better
// experience for both.
//
// The config is walked as generic objects so unknown values (for example an
// assignments list built from a not-yet-created resource) are skipped rather
// than failing the decode.
//
// ValidateConfig also rejects an assignments[].priority list whose
// priorities are not exactly {0, ..., n-1} (assignmentpriority.KindGap /
// KindDuplicate), or that is a valid set but not listed in ascending
// priority order (assignmentpriority.KindOutOfOrder) — see the
// assignmentpriority package doc comment for why UEM requires both.
//
// Possible later improvement: this resource's state.go pairs the UEM
// readback with the configured assignments by list index rather than by
// priority; re-pairing by priority on Read would allow lifting the
// out-of-order check (KindOutOfOrder) here.
func (r *purchasedApplicationAssignmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var assignments types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("assignments"), &assignments)...)
	if resp.Diagnostics.HasError() || assignments.IsNull() || assignments.IsUnknown() {
		return
	}

	// Live-confirmed 2026-09-24 on as<internal-env> 26.2: UEM's 400 was live-probed for
	// both the {0..n-1} gap/duplicate case ([0,2], [0,0] rejected) and the
	// ascending-order case (UEM itself accepts [1,0]; the order check here is
	// a provider artifact of pairing Read's readback by list index). Evidence:
	// internal-design-doc
	if priorities, ok := assignmentpriority.CollectPriorities(assignments); ok {
		for _, finding := range assignmentpriority.Check(priorities) {
			summary, detail := assignmentpriority.Diagnostic(finding, len(priorities))
			resp.Diagnostics.AddAttributeError(
				path.Root("assignments").AtListIndex(finding.Index).AtName("priority"),
				summary,
				detail,
			)
		}
	}

	for i, element := range assignments.Elements() {
		assignment, ok := element.(types.Object)
		if !ok || assignment.IsNull() || assignment.IsUnknown() {
			continue
		}
		distribution, ok := assignment.Attributes()["distribution"].(types.Object)
		if !ok || distribution.IsNull() || distribution.IsUnknown() {
			continue
		}
		// Live-confirmed 2026-09-23 on as<internal-env> 26.2: a probe sent
		// distribution.smart_groups [Y] with license_usage [{X}]; Y was never
		// assigned and GET returned smart_groups []. Evidence:
		// internal-design-doc
		smartGroups, ok := distribution.Attributes()["smart_groups"].(types.List)
		if ok && !smartGroups.IsNull() && !smartGroups.IsUnknown() && len(smartGroups.Elements()) > 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("assignments").AtListIndex(i).AtName("distribution").AtName("smart_groups"),
				tf.DistributionSmartGroupsUnsupportedSummary,
				tf.DistributionSmartGroupsUnsupportedDetail,
			)
		}

		// Any non-null, known value is rejected — including an explicit
		// empty string, which UEM would still silently ignore rather than
		// honor as "no date" (mirrors the smart_groups check above).
		// Live-confirmed 2026-09-23 on as<internal-env> 26.2: VPP ignores
		// effective_date. Evidence:
		// internal-design-doc
		effectiveDate, ok := distribution.Attributes()["effective_date"].(types.String)
		if ok && !effectiveDate.IsNull() && !effectiveDate.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("assignments").AtListIndex(i).AtName("distribution").AtName("effective_date"),
				tf.DistributionEffectiveDateUnsupportedSummary,
				tf.DistributionEffectiveDateUnsupportedDetail,
			)
		}

		// Live-confirmed 2026-09-23/2026-09-24 on as<internal-env> 26.2: neither flag is
		// ever honoured by UEM on either platform. Evidence:
		// internal-design-doc;
		// internal-design-doc
		if restriction, ok := assignment.Attributes()["restriction"].(types.Object); ok && !restriction.IsNull() && !restriction.IsUnknown() {
			if dsm, ok := restriction.Attributes()["desired_state_management"].(types.Bool); ok && !dsm.IsNull() && !dsm.IsUnknown() && dsm.ValueBool() {
				resp.Diagnostics.AddAttributeError(
					path.Root("assignments").AtListIndex(i).AtName("restriction").AtName("desired_state_management"),
					neverHonouredSummary,
					fmt.Sprintf(neverHonouredDetail, "desired_state_management"),
				)
			}
		}

		if dts, ok := assignment.Attributes()["is_dynamic_template_saved"].(types.Bool); ok && !dts.IsNull() && !dts.IsUnknown() && dts.ValueBool() {
			resp.Diagnostics.AddAttributeError(
				path.Root("assignments").AtListIndex(i).AtName("is_dynamic_template_saved"),
				neverHonouredSummary,
				fmt.Sprintf(neverHonouredDetail, "is_dynamic_template_saved"),
			)
		}
	}
}
