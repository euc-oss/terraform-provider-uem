package assignment

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// Reason strings for the restriction-backstop diagnostic. These are UEM's
// documented, live-verified VPP behaviours (internal-task): the server silently
// ignores some restriction flags depending on platform, and the provider has
// no way to know an app's platform ahead of time, so the mismatch can only be
// caught after the fact by comparing the plan to what UEM actually stored.
// Live-confirmed 2026-09-23 on as<internal-env> 26.2: macOS honours only
// remove_on_unenroll (writes of the other flags succeed but read back
// false), desired_state_management is never honoured on either platform, and
// iOS managed_access is forced/derived. Evidence:
// internal-design-doc
const (
	restrictionBackstopSummary = "UEM did not apply requested restriction settings"

	reasonMacOSIgnored = "On macOS VPP apps UEM honours only remove_on_unenroll; prevent_removal, " +
		"prevent_application_backup, make_app_mdm_managed and managed_access are ignored."
	reasonDesiredStateIgnored  = "desired_state_management is not applied by UEM for VPP apps."
	reasonManagedAccessDerived = "On iOS UEM forces managed_access on when make_app_mdm_managed or " +
		"prevent_application_backup is true; set managed_access = true."
	reasonGuidance = "Terraform state has been set to the values UEM actually stored; " +
		"remove or change these settings in configuration."
)

// restrictionFieldMismatch describes a single restriction flag whose
// read-back value from UEM disagrees with what was planned.
type restrictionFieldMismatch struct {
	Field   string
	Planned bool
	Actual  bool
	Reason  string
}

// restrictionMismatch groups the field mismatches found for one assignment.
type restrictionMismatch struct {
	Index            int
	Priority         int64
	DistributionName string
	Fields           []restrictionFieldMismatch
}

// restrictionFieldAccessor names a restriction flag and how to read it off
// the model.
type restrictionFieldAccessor struct {
	name string
	get  func(*tf.PurchasedAppAssignmentRestrictionModel) types.Bool
}

// restrictionTrueOnlyFields lists the flags that only ever produce a mismatch
// when planned true and read back false (or null/absent): UEM never turns
// these on by itself, it only ever silently drops a requested true.
var restrictionTrueOnlyFields = []restrictionFieldAccessor{
	{"remove_on_unenroll", func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.RemoveOnUnenroll }},
	{"prevent_removal", func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.PreventRemoval }},
	{"prevent_application_backup", func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.PreventApplicationBackup }},
	{"make_app_mdm_managed", func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.MakeAppMdmManaged }},
	{"desired_state_management", func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.DesiredStateManagement }},
}

// isKnownTrue reports whether a plan/state bool is known, non-null, and true.
func isKnownTrue(b types.Bool) bool {
	return !b.IsNull() && !b.IsUnknown() && b.ValueBool()
}

// isKnownNonNull reports whether a plan/state bool has a concrete value.
func isKnownNonNull(b types.Bool) bool {
	return !b.IsNull() && !b.IsUnknown()
}

// restrictionBoolValue returns the effective bool for a restriction flag,
// treating a nil restriction block or a null/unknown flag as false (UEM's
// own zero-value shape for an unconfigured restriction). Live-confirmed
// 2026-09-23 on as<internal-env> 26.2: a null restriction reads back with all six
// flags false. Evidence:
// internal-design-doc;
// internal-design-doc
func restrictionBoolValue(r *tf.PurchasedAppAssignmentRestrictionModel, get func(*tf.PurchasedAppAssignmentRestrictionModel) types.Bool) bool {
	if r == nil {
		return false
	}
	v := get(r)
	if v.IsNull() || v.IsUnknown() {
		return false
	}
	return v.ValueBool()
}

// compareRestrictionReadback compares the PLANNED restriction flags for each
// assignment against what UEM actually stored (the post-PUT readback). It is
// a pure function so the mismatch logic is unit-testable without a network
// call. Only known, non-null planned values are considered: unset/null flags
// never produce a false positive.
func compareRestrictionReadback(planned, actual []tf.PurchasedAppAssignmentModel) []restrictionMismatch {
	var mismatches []restrictionMismatch

	limit := len(planned)
	if len(actual) < limit {
		limit = len(actual)
	}

	for i := 0; i < limit; i++ {
		plannedRestriction := planned[i].Restriction
		if plannedRestriction == nil {
			continue
		}
		actualRestriction := actual[i].Restriction

		var fields []restrictionFieldMismatch

		for _, f := range restrictionTrueOnlyFields {
			plannedVal := f.get(plannedRestriction)
			if !isKnownTrue(plannedVal) {
				continue
			}
			actualVal := restrictionBoolValue(actualRestriction, f.get)
			if actualVal {
				continue
			}
			reason := ""
			switch f.name {
			case "prevent_removal", "prevent_application_backup", "make_app_mdm_managed":
				reason = reasonMacOSIgnored
			case "desired_state_management":
				reason = reasonDesiredStateIgnored
			}
			fields = append(fields, restrictionFieldMismatch{
				Field: f.name, Planned: true, Actual: false, Reason: reason,
			})
		}

		plannedManagedAccess := plannedRestriction.ManagedAccess
		if isKnownNonNull(plannedManagedAccess) {
			plannedVal := plannedManagedAccess.ValueBool()
			actualVal := restrictionBoolValue(actualRestriction, func(r *tf.PurchasedAppAssignmentRestrictionModel) types.Bool { return r.ManagedAccess })
			if plannedVal != actualVal {
				reason := reasonMacOSIgnored
				if actualVal && !plannedVal {
					reason = reasonManagedAccessDerived
				}
				fields = append(fields, restrictionFieldMismatch{
					Field: "managed_access", Planned: plannedVal, Actual: actualVal, Reason: reason,
				})
			}
		}

		if len(fields) == 0 {
			continue
		}

		var priority int64
		if !planned[i].Priority.IsNull() && !planned[i].Priority.IsUnknown() {
			priority = planned[i].Priority.ValueInt64()
		}
		mismatches = append(mismatches, restrictionMismatch{
			Index:            i,
			Priority:         priority,
			DistributionName: planned[i].Distribution.Name.ValueString(),
			Fields:           fields,
		})
	}

	return mismatches
}

// formatRestrictionMismatchDetail renders the mismatches found by
// compareRestrictionReadback into a diagnostic detail string naming, per
// assignment, each planned->actual flag and why UEM likely ignored it.
func formatRestrictionMismatchDetail(mismatches []restrictionMismatch) string {
	var b strings.Builder
	seenReasons := make(map[string]bool)
	var reasons []string

	for _, m := range mismatches {
		fmt.Fprintf(&b, "assignments[%d] (priority %d, distribution %q):", m.Index, m.Priority, m.DistributionName)
		for _, f := range m.Fields {
			fmt.Fprintf(&b, " %s planned=%t actual=%t;", f.Field, f.Planned, f.Actual)
			if f.Reason != "" && !seenReasons[f.Reason] {
				seenReasons[f.Reason] = true
				reasons = append(reasons, f.Reason)
			}
		}
		b.WriteString("\n")
	}

	for _, reason := range reasons {
		b.WriteString(reason)
		b.WriteString("\n")
	}

	b.WriteString(reasonGuidance)

	return b.String()
}
