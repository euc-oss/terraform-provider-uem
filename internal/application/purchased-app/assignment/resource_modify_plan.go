package assignment

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.ResourceWithModifyPlan = &purchasedApplicationAssignmentResource{}

// Diagnostic summaries/details for the VPP restriction plan-time checks (internal-task). These fire ahead of apply, before UEM has had a chance to silently
// drop a restriction flag or force managed_access on — see the doc comment on
// ModifyPlan for why plan time is preferable to the post-apply backstop in
// resource_restriction_backstop.go for the cases this file can catch.
const (
	macOSIgnoredSummary = "Restriction setting is not applied by UEM for macOS VPP apps"
	macOSIgnoredDetail  = "%s is not applied by UEM for macOS VPP apps; only remove_on_unenroll is honoured. " +
		"Remove it or set it to false."

	iosManagedAccessForcedSummary = "managed_access is forced on by UEM for this iOS VPP assignment"
	iosManagedAccessForcedDetail  = "UEM forces managed_access on for iOS VPP apps when make_app_mdm_managed or " +
		"prevent_application_backup is true; set managed_access = true or remove it."

	vppPlatformLookupFailedSummary = "Could not determine VPP app platform"
	vppPlatformLookupFailedDetail  = "could not determine the VPP app's platform; restriction flags will be checked after apply: %s"
)

// planRestrictionFlags is the subset of one assignment's restriction block
// that ModifyPlan's platform-sensitive checks need, read generically off
// types.List/types.Object (see collectPlanRestrictionFlags) rather than via
// ElementsAs. Reading only this subset means an unknown value elsewhere in
// the assignment (distribution, tunnel, vpp_app_details, and so on) never
// blocks the walk.
type planRestrictionFlags struct {
	hasRestriction           bool
	preventRemoval           bool
	preventApplicationBackup bool
	makeAppMdmManaged        bool
	managedAccess            bool
}

// configManagedAccess is the CONFIG-side counterpart used by
// modifyPlanForIOS: whether restriction.managed_access was explicitly set in
// configuration and, if so, to what value.
type configManagedAccess struct {
	set   bool
	value bool
}

// restrictionWalkResult is one assignment's restriction object as read
// generically from the plan/config, or the zero value (absent=true) when the
// assignment has no restriction block configured.
type restrictionWalkResult struct {
	obj     types.Object
	present bool
}

// walkAssignmentRestrictions reads assignments[*].restriction as generic
// types.Object values, without decoding any other part of the assignment.
// It returns ok=false — meaning "an unknown value was found; skip the
// platform check entirely" — the moment any list element or any element's
// restriction object is itself unknown. A null element or a null/absent
// restriction is not an error: it is reported as restrictionWalkResult{} in
// that slot (present=false), matching "no restriction configured".
//
// Unknown values elsewhere in an assignment (distribution, tunnel,
// vpp_app_details, application_configuration, and so on) are never inspected
// here and therefore never block the walk: only the assignments themselves
// and their restriction block matter to the checks in this file.
func walkAssignmentRestrictions(list types.List) (results []restrictionWalkResult, ok bool) {
	elements := list.Elements()
	results = make([]restrictionWalkResult, len(elements))
	for i, element := range elements {
		assignment, isObject := element.(types.Object)
		if !isObject || assignment.IsUnknown() {
			return nil, false
		}
		if assignment.IsNull() {
			continue
		}
		restriction, isRestrictionObject := assignment.Attributes()["restriction"].(types.Object)
		if !isRestrictionObject || restriction.IsNull() {
			continue
		}
		if restriction.IsUnknown() {
			return nil, false
		}
		results[i] = restrictionWalkResult{obj: restriction, present: true}
	}
	return results, true
}

// boolAttr reads a named types.Bool attribute off a restriction object,
// reporting unknown separately from its effective (known) value.
func boolAttr(restriction types.Object, name string) (value types.Bool, isBool bool) {
	value, isBool = restriction.Attributes()[name].(types.Bool)
	return value, isBool
}

// collectPlanRestrictionFlags reads assignments[*].restriction's four
// platform-sensitive flags (prevent_removal, prevent_application_backup,
// make_app_mdm_managed, managed_access) off the PLAN. It returns ok=false if
// the list, any element, any element's restriction object, or any of those
// four flag values is unknown — in every such case the platform check must
// be skipped rather than guess, since the apply-time backstop
// (resource_restriction_backstop.go) covers whatever ModifyPlan cannot see
// yet.
func collectPlanRestrictionFlags(list types.List) (flags []planRestrictionFlags, ok bool) {
	walked, ok := walkAssignmentRestrictions(list)
	if !ok {
		return nil, false
	}
	flags = make([]planRestrictionFlags, len(walked))
	for i, w := range walked {
		if !w.present {
			continue
		}
		flags[i].hasRestriction = true
		for _, spec := range []struct {
			name string
			dst  *bool
		}{
			{"prevent_removal", &flags[i].preventRemoval},
			{"prevent_application_backup", &flags[i].preventApplicationBackup},
			{"make_app_mdm_managed", &flags[i].makeAppMdmManaged},
			{"managed_access", &flags[i].managedAccess},
		} {
			b, isBool := boolAttr(w.obj, spec.name)
			if !isBool {
				continue
			}
			if b.IsUnknown() {
				return nil, false
			}
			*spec.dst = !b.IsNull() && b.ValueBool()
		}
	}
	return flags, true
}

// collectConfigManagedAccess reads assignments[*].restriction.managed_access
// off the CONFIG — the only config-side flag modifyPlanForIOS needs. Same
// unknown-skip contract as collectPlanRestrictionFlags: ok=false means an
// unknown was found and the platform check must be skipped entirely.
func collectConfigManagedAccess(list types.List) (values []configManagedAccess, ok bool) {
	walked, ok := walkAssignmentRestrictions(list)
	if !ok {
		return nil, false
	}
	values = make([]configManagedAccess, len(walked))
	for i, w := range walked {
		if !w.present {
			continue
		}
		b, isBool := boolAttr(w.obj, "managed_access")
		if !isBool {
			continue
		}
		if b.IsUnknown() {
			return nil, false
		}
		if !b.IsNull() {
			values[i] = configManagedAccess{set: true, value: b.ValueBool()}
		}
	}
	return values, true
}

// platformSensitiveRestrictionPlannedTrue reports whether any assignment
// plans one of the four restriction flags whose UEM behaviour depends on the
// VPP app's platform: prevent_removal, prevent_application_backup,
// make_app_mdm_managed, or managed_access. Note that remove_on_unenroll is
// excluded, since it is honoured on every platform and never needs a
// platform lookup.
func platformSensitiveRestrictionPlannedTrue(flags []planRestrictionFlags) bool {
	for _, f := range flags {
		if !f.hasRestriction {
			continue
		}
		if f.preventRemoval || f.preventApplicationBackup || f.makeAppMdmManaged || f.managedAccess {
			return true
		}
	}
	return false
}

// ModifyPlan catches, at plan time, VPP restriction configurations that UEM
// is known to silently misbehave on (internal-task), rather than letting the user
// discover it only after resource_restriction_backstop.go's post-apply
// comparison fires.
//
// Both desired_state_management and is_dynamic_template_saved are handled in
// ValidateConfig (resource_validate.go), not here — see that file's doc
// comment for why a config-only check, run before ModifyPlan and unaffected
// by an unknown application_uuid, is strictly better for a check that never
// needs a platform lookup.
//
// The checks that remain here (macOS ignoring most restriction flags; iOS
// forcing managed_access on) do depend on platform, which requires one
// lookup call per ModifyPlan invocation. That lookup is skipped entirely
// unless some assignment plans one of the platform-sensitive flags true, and
// — since UEM's behaviour cannot be predicted for a value Terraform hasn't
// resolved yet — also skipped whenever the assignments list, any element, or
// any of the specific flags this file reads (see collectPlanRestrictionFlags
// / collectConfigManagedAccess) is unknown. Unknown values are common when
// assignments is built from a not-yet-created resource; the apply-time
// backstop in resource_restriction_backstop.go still catches whatever this
// skip lets through.
func (r *purchasedApplicationAssignmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// Destroy plan: nothing to check.
		return
	}

	var appUUID types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("application_uuid"), &appUUID)...)
	if resp.Diagnostics.HasError() || appUUID.IsUnknown() || appUUID.IsNull() {
		return
	}

	var planAssignmentsList types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("assignments"), &planAssignmentsList)...)
	if resp.Diagnostics.HasError() || planAssignmentsList.IsUnknown() || planAssignmentsList.IsNull() {
		return
	}

	planFlags, ok := collectPlanRestrictionFlags(planAssignmentsList)
	if !ok {
		// Some plan-side restriction flag (or its containing list/element/
		// object) is unknown: cannot determine whether a platform-sensitive
		// flag is planned true, so skip entirely rather than guess.
		return
	}
	if !platformSensitiveRestrictionPlannedTrue(planFlags) {
		return
	}

	var configAssignmentsList types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("assignments"), &configAssignmentsList)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var configManagedAccessValues []configManagedAccess
	if !configAssignmentsList.IsNull() && !configAssignmentsList.IsUnknown() {
		values, ok := collectConfigManagedAccess(configAssignmentsList)
		if !ok {
			// CONFIG's managed_access is unknown for some assignment: skip
			// entirely, same rationale as the plan-side skip above.
			return
		}
		configManagedAccessValues = values
	}

	svc, err := r.vppPlatformLookupService(ctx)
	if err != nil {
		resp.Diagnostics.AddWarning(vppPlatformLookupFailedSummary, fmt.Sprintf(vppPlatformLookupFailedDetail, err.Error()))
		return
	}

	platform, err := svc.LookupVppPlatform(ctx, appUUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddWarning(vppPlatformLookupFailedSummary, fmt.Sprintf(vppPlatformLookupFailedDetail, err.Error()))
		return
	}

	switch platform {
	case vppPlatformMacOS:
		r.modifyPlanForMacOS(planFlags, resp)
	case vppPlatformIOS:
		r.modifyPlanForIOS(ctx, planFlags, configManagedAccessValues, resp)
	default:
		// Unknown platform: no diagnostics, no plan change.
	}
}

// modifyPlanForMacOS adds an error for every planned-true restriction flag
// that macOS VPP apps ignore. One error is added per offending flag (not one
// combined error per assignment), each at that flag's own attribute path, so
// `terraform plan` output points directly at the attribute to fix.
//
// Live-confirmed 2026-09-23/2026-09-24 on as<internal-env> 26.2: macOS VPP apps honour
// ONLY remove_on_unenroll; prevent_removal, prevent_application_backup and
// make_app_mdm_managed are ignored. Evidence:
// internal-design-doc;
// internal-design-doc
func (r *purchasedApplicationAssignmentResource) modifyPlanForMacOS(planFlags []planRestrictionFlags, resp *resource.ModifyPlanResponse) {
	for i, f := range planFlags {
		if !f.hasRestriction {
			continue
		}
		for _, offending := range []struct {
			name string
			set  bool
		}{
			{"prevent_removal", f.preventRemoval},
			{"prevent_application_backup", f.preventApplicationBackup},
			{"make_app_mdm_managed", f.makeAppMdmManaged},
			{"managed_access", f.managedAccess},
		} {
			if !offending.set {
				continue
			}
			resp.Diagnostics.AddAttributeError(
				path.Root("assignments").AtListIndex(i).AtName("restriction").AtName(offending.name),
				macOSIgnoredSummary,
				fmt.Sprintf(macOSIgnoredDetail, offending.name),
			)
		}
	}
}

// modifyPlanForIOS handles the one iOS-specific behaviour: UEM forces
// managed_access on whenever make_app_mdm_managed or
// prevent_application_backup is true. When CONFIG left managed_access unset,
// the plan is updated to true so there is no diff noise; when CONFIG
// explicitly set managed_access = false, that is rejected as unsatisfiable.
//
// Live-confirmed 2026-09-23/2026-09-24 on as<internal-env> 26.2: managed_access on iOS
// is derived (managed_access OR make_app_mdm_managed OR
// prevent_application_backup); an iOS MAMM config plans managed_access=true,
// and an explicit false is rejected. Evidence:
// internal-design-doc;
// internal-design-doc
func (r *purchasedApplicationAssignmentResource) modifyPlanForIOS(ctx context.Context, planFlags []planRestrictionFlags, configManagedAccessValues []configManagedAccess, resp *resource.ModifyPlanResponse) {
	for i, f := range planFlags {
		if !f.hasRestriction {
			continue
		}
		if !f.makeAppMdmManaged && !f.preventApplicationBackup {
			continue
		}

		maPath := path.Root("assignments").AtListIndex(i).AtName("restriction").AtName("managed_access")

		var configManagedAccessValue configManagedAccess
		if i < len(configManagedAccessValues) {
			configManagedAccessValue = configManagedAccessValues[i]
		}

		switch {
		case !configManagedAccessValue.set:
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, maPath, true)...)
		case !configManagedAccessValue.value:
			resp.Diagnostics.AddAttributeError(maPath, iosManagedAccessForcedSummary, iosManagedAccessForcedDetail)
		}
		// configManagedAccessValue explicitly true: already correct.
	}
}
