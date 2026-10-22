package smartgroup

import (
	"context"
	"sort"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// allOwnershipsDefault is the Ownerships value UEM stores when none is sent.
const allOwnershipsDefault = "allownerships"

var operatingSystemAttrTypes = map[string]attr.Type{
	"device_type": types.StringType,
	"operator":    types.StringType,
	"value":       types.StringType,
}

var operatingSystemObjectType = types.ObjectType{AttrTypes: operatingSystemAttrTypes}

// setStrings returns a set's string elements, sorted so the request body is
// deterministic. A null or unknown set yields nil.
func setStrings(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := s.ElementsAs(ctx, &out, false)
	sort.Strings(out)
	return out, diags
}

// buildEditModel builds the whole-object request body for Create and Update
// from the plan. Every managed field is sent on every call; OEMAndModels is
// left for the caller (Update copies the server's current value in).
//
// Clearing a criteria or member set: every slice field of
// sdk.SmartGroupEditV1Model is tagged `json:",omitempty"`, so a set that goes
// from non-empty to empty (or null) builds a nil slice and the field is
// OMITTED from the PUT body entirely -- no `"Platforms": []` is sent.
// TestBuildEditModel_EmptiedSetIsOmittedFromPUTBody pins this against a
// captured request body. Live-confirmed: UEM then keeps the field's existing
// value rather than clearing it (live run 2: enrollment_categories cleared
// to [] still returned ["DepEnrolled"], with an inconsistent-result error).
// ModifyPlan (resource.go) now catches this at plan time -- before Update
// ever builds this body -- and errors, naming the attribute and pointing at
// the console or `-replace` instead of silently recording a clear that did
// not happen.
func buildEditModel(ctx context.Context, m *SmartGroupResourceModel) (*sdk.SmartGroupEditV1Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	strs := func(s types.Set) []string {
		v, d := setStrings(ctx, s)
		diags.Append(d...)
		return v
	}

	req := &sdk.SmartGroupEditV1Model{
		Name:                         m.Name.ValueString(),
		ManagedByOrganizationGroupID: m.ManagedByOrgGroupID.ValueString(),
		CriteriaType:                 m.CriteriaType.ValueString(),
		Platforms:                    strs(m.Platforms),
		Ownerships:                   strs(m.Ownerships),
		Models:                       strs(m.Models),
		ManagementTypes:              strs(m.ManagementTypes),
		EnrollmentCategories:         strs(m.EnrollmentCategories),
		CPUArchitectures:             strs(m.CPUArchitectures),
	}
	for _, id := range strs(m.OrganizationGroupIDs) {
		req.OrganizationGroups = append(req.OrganizationGroups, sdk.SmartGroupOGV1{ID: id})
	}
	for _, id := range strs(m.UserGroupIDs) {
		req.UserGroups = append(req.UserGroups, sdk.SmartGroupUserGroupV1{ID: id})
	}
	for _, id := range strs(m.TagIDs) {
		req.Tags = append(req.Tags, sdk.SmartGroupTagV1{ID: id})
	}
	for _, id := range strs(m.UserAdditions) {
		req.UserAdditions = append(req.UserAdditions, sdk.SmartGroupUserV1{ID: id})
	}
	for _, id := range strs(m.DeviceAdditions) {
		req.DeviceAdditions = append(req.DeviceAdditions, sdk.SmartGroupDeviceV1{ID: id})
	}
	for _, id := range strs(m.UserExclusions) {
		req.UserExclusions = append(req.UserExclusions, sdk.SmartGroupUserV1{ID: id})
	}
	for _, id := range strs(m.DeviceExclusions) {
		req.DeviceExclusions = append(req.DeviceExclusions, sdk.SmartGroupDeviceV1{ID: id})
	}
	for _, id := range strs(m.UserGroupExclusions) {
		req.UserGroupExclusions = append(req.UserGroupExclusions, sdk.SmartGroupUserGroupV1{ID: id})
	}

	if !m.OperatingSystems.IsNull() && !m.OperatingSystems.IsUnknown() {
		var oses []OperatingSystemModel
		diags.Append(m.OperatingSystems.ElementsAs(ctx, &oses, false)...)
		for _, os := range oses {
			req.OperatingSystems = append(req.OperatingSystems, sdk.SmartGroupOperatingSystemV1{
				DeviceType: os.DeviceType.ValueString(),
				Operator:   os.Operator.ValueString(),
				Value:      os.Value.ValueString(),
			})
		}
		sort.Slice(req.OperatingSystems, func(i, j int) bool {
			a, b := req.OperatingSystems[i], req.OperatingSystems[j]
			return a.DeviceType+"\x00"+a.Operator+"\x00"+a.Value < b.DeviceType+"\x00"+b.Operator+"\x00"+b.Value
		})
	}
	return req, diags
}

// reconcileStringSet turns the server's values into a set, keeping the prior
// (plan or state) casing of any element that matches case-insensitively, so
// "WindowsPhone" configured and "windowsphone" returned is no diff. An empty
// server list becomes null, unless prior is a known empty set (an explicit
// `[]` in config), which is kept to avoid a null-vs-empty diff.
func reconcileStringSet(ctx context.Context, prior types.Set, server []string) (types.Set, diag.Diagnostics) {
	if len(server) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior, nil
		}
		return types.SetNull(types.StringType), nil
	}
	casing := map[string]string{}
	priorVals, diags := setStrings(ctx, prior)
	for _, p := range priorVals {
		if _, ok := casing[strings.ToLower(p)]; !ok {
			casing[strings.ToLower(p)] = p
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range server {
		key := strings.ToLower(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		if p, ok := casing[key]; ok {
			s = p
		}
		out = append(out, s)
	}
	set, d := types.SetValueFrom(ctx, types.StringType, out)
	diags.Append(d...)
	return set, diags
}

// reconcileOwnerships is reconcileSetVerbatim plus one known server default:
// with ownerships unset in prior, UEM's automatic ["allownerships"] (any
// casing) reads back as null. Configured ownerships are never special-cased,
// and are otherwise read back exactly as UEM returns them -- there is no
// live evidence that the ownerships endpoint is case-insensitive on write,
// so the provider does not paper over a casing mismatch by preserving the
// configured casing (see reconcileSetVerbatim). Live-confirmed on as<internal-env>
// 26.2: with ownerships unset, UEM returns ["allownerships"] (gate record:
// smart-group live run 1, 2026-09-26).
func reconcileOwnerships(ctx context.Context, prior types.Set, server []string) (types.Set, diag.Diagnostics) {
	if prior.IsNull() && len(server) == 1 && strings.EqualFold(server[0], allOwnershipsDefault) {
		return types.SetNull(types.StringType), nil
	}
	return reconcileSetVerbatim(ctx, prior, server)
}

// reconcileSetVerbatim turns the server's values into a set, read back
// exactly as UEM returned them -- no case reconciliation with prior. Used
// for criteria attributes that are not actually case-insensitive on the UEM
// side: live evidence showed UEM echoing back whatever casing was
// configured, and a lowercase platform sent on update returned HTTP 500, so
// the provider must not paper over a casing mismatch by preserving the
// configured casing. An empty server list becomes null, unless prior is a
// known empty set (an explicit `[]` in config), which is kept to avoid a
// null-vs-empty diff.
func reconcileSetVerbatim(ctx context.Context, prior types.Set, server []string) (types.Set, diag.Diagnostics) {
	if len(server) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior, nil
		}
		return types.SetNull(types.StringType), nil
	}
	set, diags := types.SetValueFrom(ctx, types.StringType, server)
	return set, diags
}

// reconcileOperatingSystemsVerbatim is reconcileSetVerbatim for
// operating_systems: every field is read back exactly as UEM returned it,
// with no case reconciliation with prior.
func reconcileOperatingSystemsVerbatim(ctx context.Context, prior types.Set, server []sdk.SmartGroupOperatingSystemV1) (types.Set, diag.Diagnostics) {
	if len(server) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior, nil
		}
		return types.SetNull(operatingSystemObjectType), nil
	}
	out := make([]OperatingSystemModel, 0, len(server))
	for _, s := range server {
		out = append(out, OperatingSystemModel{
			DeviceType: types.StringValue(s.DeviceType),
			Operator:   types.StringValue(s.Operator),
			Value:      types.StringValue(s.Value),
		})
	}
	set, diags := types.SetValueFrom(ctx, operatingSystemObjectType, out)
	return set, diags
}

func int64OrNull(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// smartGroupToModel builds the new state from a LoadSmartGroupAsync result.
// The prior argument is the plan (Create/Update) or the prior state (Read/import); it only
// supplies casing and the null-vs-default choices described on the
// reconcile helpers, never values the server did not return. The counters
// are always taken from the server; they are Computed-only, so a count that
// changes between reads never produces a plan diff.
func smartGroupToModel(ctx context.Context, sg *sdk.SmartGroupV1, prior *SmartGroupResourceModel) (SmartGroupResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	// set reconciles the numeric id-list attributes (organization/user
	// group/tag/addition/exclusion sets) through reconcileStringSet; its
	// case-insensitive matching is a no-op on digits and is shared with
	// reconcileOwnerships' default handling.
	set := func(prior types.Set, server []string) types.Set {
		v, d := reconcileStringSet(ctx, prior, server)
		diags.Append(d...)
		return v
	}
	// verbatim reconciles a criterion attribute UEM does not treat as
	// case-insensitive: read back exactly as returned (reconcileSetVerbatim).
	verbatim := func(prior types.Set, server []string) types.Set {
		v, d := reconcileSetVerbatim(ctx, prior, server)
		diags.Append(d...)
		return v
	}

	m := SmartGroupResourceModel{
		ID:                    prior.ID,
		UUID:                  types.StringValue(sg.SmartGroupUUID),
		Name:                  types.StringValue(sg.Name),
		ManagedByOrgGroupID:   prior.ManagedByOrgGroupID,
		ManagedByOrgGroupUUID: types.StringValue(sg.ManagedByOrganizationGroupUUID),
		ManagedByOrgGroupName: types.StringValue(sg.ManagedByOrganizationGroupName),
		CriteriaType:          types.StringValue(sg.CriteriaType),
		Platforms:             verbatim(prior.Platforms, sg.Platforms),
		Models:                verbatim(prior.Models, sg.Models),
		ManagementTypes:       verbatim(prior.ManagementTypes, sg.ManagementTypes),
		EnrollmentCategories:  verbatim(prior.EnrollmentCategories, sg.EnrollmentCategories),
		CPUArchitectures:      verbatim(prior.CPUArchitectures, sg.CpuArchitectures),
		DevicesCount:          int64OrNull(sg.Devices),
		AssignmentsCount:      int64OrNull(sg.Assignments),
		ExclusionsCount:       int64OrNull(sg.Exclusions),
	}
	if sg.SmartGroupID != nil {
		m.ID = types.StringValue(strconv.Itoa(*sg.SmartGroupID))
	}
	if sg.ManagedByOrganizationGroupID != "" {
		m.ManagedByOrgGroupID = types.StringValue(sg.ManagedByOrganizationGroupID)
	}

	ownerships, d := reconcileOwnerships(ctx, prior.Ownerships, sg.Ownerships)
	diags.Append(d...)
	m.Ownerships = ownerships

	oses, d := reconcileOperatingSystemsVerbatim(ctx, prior.OperatingSystems, sg.OperatingSystems)
	diags.Append(d...)
	m.OperatingSystems = oses

	m.OrganizationGroupIDs = set(prior.OrganizationGroupIDs, idsOf(sg.OrganizationGroups, func(v sdk.SmartGroupOGV1) string { return v.ID }))
	m.UserGroupIDs = set(prior.UserGroupIDs, idsOf(sg.UserGroups, func(v sdk.SmartGroupUserGroupV1) string { return v.ID }))
	m.TagIDs = set(prior.TagIDs, idsOf(sg.Tags, func(v sdk.SmartGroupTagV1) string { return v.ID }))
	m.UserAdditions = set(prior.UserAdditions, idsOf(sg.UserAdditions, func(v sdk.SmartGroupUserV1) string { return v.ID }))
	m.DeviceAdditions = set(prior.DeviceAdditions, idsOf(sg.DeviceAdditions, func(v sdk.SmartGroupDeviceV1) string { return v.ID }))
	m.UserExclusions = set(prior.UserExclusions, idsOf(sg.UserExclusions, func(v sdk.SmartGroupUserV1) string { return v.ID }))
	m.DeviceExclusions = set(prior.DeviceExclusions, idsOf(sg.DeviceExclusions, func(v sdk.SmartGroupDeviceV1) string { return v.ID }))
	m.UserGroupExclusions = set(prior.UserGroupExclusions, idsOf(sg.UserGroupExclusions, func(v sdk.SmartGroupUserGroupV1) string { return v.ID }))
	return m, diags
}

// idsOf extracts the Id of each {Id, ...} reference object.
func idsOf[T any](items []T, id func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, id(it))
	}
	return out
}
