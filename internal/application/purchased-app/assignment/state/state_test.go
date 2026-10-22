package state

import (
	"context"
	"testing"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	client "github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

func TestEmptyPurchasedAppAssignmentRuleToAPI(t *testing.T) {
	api := EmptyPurchasedAppAssignmentRuleToAPI()
	if api == nil {
		t.Fatal("expected non-nil API payload")
	}
	if len(api.Assignments) != 0 {
		t.Fatalf("expected 0 assignments, got %d", len(api.Assignments))
	}
	if len(api.ExcludedSmartGroups) != 0 {
		t.Fatalf("expected 0 excluded smart groups, got %d", len(api.ExcludedSmartGroups))
	}
}

func TestSetMinimalStateNormalizesUUID(t *testing.T) {
	data := &tf.PurchasedAppAssignmentRuleModel{}
	SetMinimalState(data, "  ABC-DEF  ")
	if data.ApplicationUUID.ValueString() != "abc-def" {
		t.Fatalf("unexpected application_uuid: %q", data.ApplicationUUID.ValueString())
	}
	if data.ID.ValueString() != "abc-def" {
		t.Fatalf("unexpected id: %q", data.ID.ValueString())
	}
}

func TestPurchasedAppAssignmentRuleToAPI_VPPLicenseUsage(t *testing.T) {
	ctx := context.Background()

	licenseUsage, luDiags := types.ListValueFrom(ctx, vppLicenseUsageElemType, []tf.VppLicenseUsageModel{{
		SmartGroupUUID: types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
		Allocated:      types.Int64Value(2),
		Redeemed:       types.Int64Null(),
	}})
	if luDiags.HasError() {
		t.Fatalf("failed to build license usage: %v", luDiags)
	}

	assignment := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:        types.StringValue("All users"),
			Description: types.StringValue("VPP assignment"),
			SmartGroups: types.ListValueMust(types.StringType, []attr.Value{}),
			VppAppDetails: tf.VppAppDetailsModel{
				LicenseUsage: licenseUsage,
			},
		},
		Restriction: &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll: types.BoolValue(true),
			ManagedAccess:    types.BoolValue(true),
		},
		Tunnel: &tf.PurchasedAppAssignmentTunnelModel{
			PerAppVpnProfileUUID: types.StringValue("f6caf44b-5fc7-1880-f09a-4a5bb560d5d8"),
		},
		ApplicationConfiguration: types.ListValueMust(appConfigurationElemType, []attr.Value{}),
		ApplicationAttributes:    types.ListValueMust(appConfigurationElemType, []attr.Value{}),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}

	assignments, aDiags := types.ListValueFrom(ctx, assignmentElemType, []tf.PurchasedAppAssignmentModel{assignment})
	if aDiags.HasError() {
		t.Fatalf("failed to build assignments list: %v", aDiags)
	}

	excluded, exDiags := types.ListValueFrom(ctx, types.StringType, []string{"SG-EXCLUDED"})
	if exDiags.HasError() {
		t.Fatalf("failed to build excluded list: %v", exDiags)
	}

	model := &tf.PurchasedAppAssignmentRuleModel{
		ExcludedSmartGroups: excluded,
		Assignments:         assignments,
	}

	api, diags := PurchasedAppAssignmentRuleToAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(api.ExcludedSmartGroups) != 1 || api.ExcludedSmartGroups[0] != "sg-excluded" {
		t.Fatalf("unexpected excluded smart groups: %#v", api.ExcludedSmartGroups)
	}
	if len(api.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %d", len(api.Assignments))
	}

	got := api.Assignments[0]
	if got.Distribution.VppAppDetails == nil || len(got.Distribution.VppAppDetails.LicenseUsage) != 1 {
		t.Fatalf("expected VPP license usage, got %#v", got.Distribution.VppAppDetails)
	}
	lu := got.Distribution.VppAppDetails.LicenseUsage[0]
	if lu.SmartGroupUUID != "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7" {
		t.Fatalf("unexpected smart_group_uuid: %q", lu.SmartGroupUUID)
	}
	if lu.Allocated == nil || *lu.Allocated != 2 {
		t.Fatalf("unexpected allocated: %#v", lu.Allocated)
	}
	if got.Tunnel == nil || got.Tunnel.PerAppVpnProfileUUID != "f6caf44b-5fc7-1880-f09a-4a5bb560d5d8" {
		t.Fatalf("unexpected tunnel mapping: %#v", got.Tunnel)
	}
}

func TestReadAPIIntoState_NormalizesMixedCase(t *testing.T) {
	ctx := context.Background()
	allocated := 2
	redeemed := 1
	api := &sdk.AppAssignmentRuleV2Model{
		ExcludedSmartGroups: []string{"SG-EXCLUDED"},
		Assignments: []sdk.AppAssignmentV2Model{
			{
				Priority: 0,
				Distribution: sdk.AppAssignmentDistributionV2Model{
					Name: "VPP",
					VppAppDetails: &sdk.AppAssignmentVppV1ModelV2{
						LicenseUsage: []sdk.AssignmentLicenseUsageV1ModelV2{{
							SmartGroupUUID: "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7",
							Allocated:      &allocated,
							Redeemed:       &redeemed,
						}},
					},
				},
				ApplicationConfiguration: []sdk.AppConfigurationV1ModelV2{{
					Key:   "t1",
					Value: "true",
					Type:  "3",
				}},
			},
		},
	}

	data := &tf.PurchasedAppAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var excluded []types.String
	diags = data.ExcludedSmartGroups.ElementsAs(ctx, &excluded, false)
	if diags.HasError() {
		t.Fatalf("failed to decode excluded_smart_groups: %v", diags)
	}
	if len(excluded) != 1 || excluded[0].ValueString() != "sg-excluded" {
		t.Fatalf("unexpected excluded_smart_groups: %#v", excluded)
	}

	var assigns []tf.PurchasedAppAssignmentModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}

	var usage []tf.VppLicenseUsageModel
	diags = assigns[0].Distribution.VppAppDetails.LicenseUsage.ElementsAs(ctx, &usage, false)
	if diags.HasError() {
		t.Fatalf("failed to decode license_usage: %v", diags)
	}
	if len(usage) != 1 || usage[0].SmartGroupUUID.ValueString() != "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7" {
		t.Fatalf("unexpected license_usage: %#v", usage)
	}
	if usage[0].Redeemed.ValueInt64() != 1 {
		t.Fatalf("expected redeemed=1, got %d", usage[0].Redeemed.ValueInt64())
	}

	var configs []tf.AppConfigurationEntryModel
	diags = assigns[0].ApplicationConfiguration.ElementsAs(ctx, &configs, false)
	if diags.HasError() {
		t.Fatalf("failed to decode application_configuration: %v", diags)
	}
	if len(configs) != 1 || configs[0].Type.ValueString() != "BOOLEAN" {
		t.Fatalf("unexpected application_configuration: %#v", configs)
	}
}

func TestReadAPIIntoState_EmptyAppConfigAndAttributesAreEmptyLists(t *testing.T) {
	ctx := context.Background()
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{
			{
				Priority: 0,
				Distribution: sdk.AppAssignmentDistributionV2Model{
					Name: "VPP",
				},
				ApplicationConfiguration: []sdk.AppConfigurationV1ModelV2{},
				ApplicationAttributes:    []sdk.AppConfigurationV1ModelV2{},
			},
		},
	}

	data := &tf.PurchasedAppAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var assigns []tf.PurchasedAppAssignmentModel
	diags = data.Assignments.ElementsAs(ctx, &assigns, false)
	if diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if len(assigns) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assigns))
	}

	if assigns[0].ApplicationConfiguration.IsNull() {
		t.Error("application_configuration should be empty list, not null")
	}
	if assigns[0].ApplicationAttributes.IsNull() {
		t.Error("application_attributes should be empty list, not null")
	}

	var configs []tf.AppConfigurationEntryModel
	diags = assigns[0].ApplicationConfiguration.ElementsAs(ctx, &configs, false)
	if diags.HasError() {
		t.Fatalf("failed to decode application_configuration: %v", diags)
	}
	if len(configs) != 0 {
		t.Fatalf("expected 0 application_configuration entries, got %d", len(configs))
	}
}

// TestReadAPIIntoState_LicenseUsageNullNotEmptyList covers b16 decision
// table row #141 (CORRECT). UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Helpers/AppsControllerHelper.cs:3108-3119,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/AppAssignmentVppV1Model.cs:29-30
// (canonical Q19): license_usage is populated only when
// AssignmentLicenseUsage?.Count > 0; otherwise it is omitted (null) on the
// wire, never an empty array. This must read back as null, not [].
func TestReadAPIIntoState_LicenseUsageNullNotEmptyList(t *testing.T) {
	ctx := context.Background()

	t.Run("no vpp_app_details object at all", func(t *testing.T) {
		api := &sdk.AppAssignmentRuleV2Model{
			Assignments: []sdk.AppAssignmentV2Model{{
				Priority:     0,
				Distribution: sdk.AppAssignmentDistributionV2Model{Name: "VPP"},
			}},
		}
		data := &tf.PurchasedAppAssignmentRuleModel{}
		if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		var assigns []tf.PurchasedAppAssignmentModel
		if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
			t.Fatalf("failed to decode assignments: %v", diags)
		}
		if !assigns[0].Distribution.VppAppDetails.LicenseUsage.IsNull() {
			t.Fatalf("expected license_usage to be null, got %#v", assigns[0].Distribution.VppAppDetails.LicenseUsage)
		}
	})

	t.Run("vpp_app_details object present but with an empty license usage list", func(t *testing.T) {
		api := &sdk.AppAssignmentRuleV2Model{
			Assignments: []sdk.AppAssignmentV2Model{{
				Priority: 0,
				Distribution: sdk.AppAssignmentDistributionV2Model{
					Name:          "VPP",
					VppAppDetails: &sdk.AppAssignmentVppV1ModelV2{LicenseUsage: []sdk.AssignmentLicenseUsageV1ModelV2{}},
				},
			}},
		}
		data := &tf.PurchasedAppAssignmentRuleModel{}
		if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		var assigns []tf.PurchasedAppAssignmentModel
		if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
			t.Fatalf("failed to decode assignments: %v", diags)
		}
		if !assigns[0].Distribution.VppAppDetails.LicenseUsage.IsNull() {
			t.Fatalf("expected license_usage to be null, got %#v", assigns[0].Distribution.VppAppDetails.LicenseUsage)
		}
	})

	t.Run("populated license usage still reads as a concrete list", func(t *testing.T) {
		api := &sdk.AppAssignmentRuleV2Model{
			Assignments: []sdk.AppAssignmentV2Model{{
				Priority: 0,
				Distribution: sdk.AppAssignmentDistributionV2Model{
					Name: "VPP",
					VppAppDetails: &sdk.AppAssignmentVppV1ModelV2{
						LicenseUsage: []sdk.AssignmentLicenseUsageV1ModelV2{{SmartGroupUUID: "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"}},
					},
				},
			}},
		}
		data := &tf.PurchasedAppAssignmentRuleModel{}
		if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		var assigns []tf.PurchasedAppAssignmentModel
		if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
			t.Fatalf("failed to decode assignments: %v", diags)
		}
		lu := assigns[0].Distribution.VppAppDetails.LicenseUsage
		if lu.IsNull() {
			t.Fatal("expected license_usage to be a concrete list, got null")
		}
		var usage []tf.VppLicenseUsageModel
		if diags := lu.ElementsAs(ctx, &usage, false); diags.HasError() {
			t.Fatalf("failed to decode license_usage: %v", diags)
		}
		if len(usage) != 1 || usage[0].SmartGroupUUID.ValueString() != "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7" {
			t.Fatalf("unexpected license_usage: %#v", usage)
		}
	})
}

func TestPurchasedAppAssignmentRuleToAPI_EmptyAppConfigArrays(t *testing.T) {
	ctx := context.Background()

	assignment := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:        types.StringValue("All users"),
			SmartGroups: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("sg-1")}),
			VppAppDetails: tf.VppAppDetailsModel{
				LicenseUsage: types.ListValueMust(vppLicenseUsageElemType, []attr.Value{}),
			},
		},
		ApplicationConfiguration: types.ListNull(appConfigurationElemType),
		ApplicationAttributes:    types.ListNull(appConfigurationElemType),
	}

	assignments, aDiags := types.ListValueFrom(ctx, assignmentElemType, []tf.PurchasedAppAssignmentModel{assignment})
	if aDiags.HasError() {
		t.Fatalf("failed to build assignments list: %v", aDiags)
	}

	model := &tf.PurchasedAppAssignmentRuleModel{Assignments: assignments}
	api, diags := PurchasedAppAssignmentRuleToAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(api.Assignments) != 1 {
		t.Fatalf("expected one assignment, got %d", len(api.Assignments))
	}

	got := api.Assignments[0]
	if got.ApplicationConfiguration == nil {
		t.Fatal("expected non-nil application_configuration slice")
	}
	if len(got.ApplicationConfiguration) != 0 {
		t.Fatalf("expected empty application_configuration, got %#v", got.ApplicationConfiguration)
	}
	if got.ApplicationAttributes == nil {
		t.Fatal("expected non-nil application_attributes slice")
	}
	if len(got.ApplicationAttributes) != 0 {
		t.Fatalf("expected empty application_attributes, got %#v", got.ApplicationAttributes)
	}
}

// TestReadAPIIntoState_KeepsServerDistributionSmartGroups proves Read maps a
// server-returned distribution.smart_groups into state unchanged (normalized
// only), so drift is still detected even though ValidateConfig forbids
// configuring the field.
func TestReadAPIIntoState_KeepsServerDistributionSmartGroups(t *testing.T) {
	ctx := context.Background()
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Priority: 0,
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name:        "VPP",
				SmartGroups: []string{"cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"},
			},
		}},
	}

	data := &tf.PurchasedAppAssignmentRuleModel{}
	if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	var groups []string
	if diags := assigns[0].Distribution.SmartGroups.ElementsAs(ctx, &groups, false); diags.HasError() {
		t.Fatalf("failed to decode smart_groups: %v", diags)
	}
	if len(groups) != 1 || groups[0] != "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7" {
		t.Fatalf("unexpected distribution.smart_groups in state: %#v", groups)
	}
}

// TestReadAPIIntoState_DistributionEffectiveDateAlwaysReadsNull proves
// distribution.effective_date reads back null even when the server returns a
// non-zero date (internal-task item 4): UEM ignores effective_date for VPP apps,
// so a server-echoed value must never surface, or an import could generate a
// state value the plan-time validator then rejects.
func TestReadAPIIntoState_DistributionEffectiveDateAlwaysReadsNull(t *testing.T) {
	ctx := context.Background()
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Priority: 0,
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name:          "VPP",
				EffectiveDate: client.NewUEMTime(time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)),
			},
		}},
	}

	data := &tf.PurchasedAppAssignmentRuleModel{}
	if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	if !assigns[0].Distribution.EffectiveDate.IsNull() {
		t.Fatalf("expected distribution.effective_date to read back null, got %v", assigns[0].Distribution.EffectiveDate)
	}
}

// TestReadAPIIntoState_TunnelClearedNotKeptFromPrior covers b16 decision
// table row #140 (CORRECT, keep-prior part). UEM source: AirWatch API/AW.Mam.Api/AW.Mam.Api/Helpers/AppsControllerHelper.cs:3570-3572,3806-3810
// (canonical Q18): the server's full assignment-replace flow CLEARS rather
// than merges an omitted tunnel UUID. When a later GET no longer returns a
// tunnel UUID that was previously configured, state must show it cleared
// (null), not silently keep echoing the stale prior value forever.
func TestReadAPIIntoState_TunnelClearedNotKeptFromPrior(t *testing.T) {
	ctx := context.Background()

	priorAssignment := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:          types.StringValue("VPP"),
			SmartGroups:   types.ListValueMust(types.StringType, []attr.Value{}),
			VppAppDetails: tf.VppAppDetailsModel{LicenseUsage: types.ListValueMust(vppLicenseUsageElemType, []attr.Value{})},
		},
		Tunnel: &tf.PurchasedAppAssignmentTunnelModel{
			PerAppVpnProfileUUID:    types.StringValue("f6caf44b-5fc7-1880-f09a-4a5bb560d5d8"),
			AfwPerAppVpnProfileUUID: types.StringValue("bafde89c-041e-1756-082b-933aaf16cad8"),
		},
		ApplicationConfiguration: types.ListValueMust(appConfigurationElemType, []attr.Value{}),
		ApplicationAttributes:    types.ListValueMust(appConfigurationElemType, []attr.Value{}),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
	priorList, pDiags := types.ListValueFrom(ctx, assignmentElemType, []tf.PurchasedAppAssignmentModel{priorAssignment})
	if pDiags.HasError() {
		t.Fatalf("failed to build prior assignments: %v", pDiags)
	}

	data := &tf.PurchasedAppAssignmentRuleModel{Assignments: priorList}

	// Server response: the tunnel object is still present (so the
	// all-fields-empty "no tunnel" short-circuit doesn't fire), but
	// per_app_vpn_profile_uuid is no longer returned — UEM cleared it.
	// afw_per_app_vpn_profile_uuid is still returned, to prove per-field
	// independence.
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Priority: 0,
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name: "VPP",
			},
			Tunnel: &sdk.AppAssignmentTunnelV1ModelV2{
				AfwPerAppVpnProfileUUID: "bafde89c-041e-1756-082b-933aaf16cad8",
			},
		}},
	}

	if diags := ReadAPIIntoState(ctx, data, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := data.Assignments.ElementsAs(ctx, &assigns, false); diags.HasError() {
		t.Fatalf("failed to decode assignments: %v", diags)
	}
	tunnel := assigns[0].Tunnel
	if tunnel == nil {
		t.Fatal("expected tunnel block to remain (afw uuid still set)")
	}
	if !tunnel.PerAppVpnProfileUUID.IsNull() {
		t.Fatalf(
			"expected cleared per_app_vpn_profile_uuid to read back null, not the stale prior value; got %q",
			tunnel.PerAppVpnProfileUUID.ValueString(),
		)
	}
	if tunnel.AfwPerAppVpnProfileUUID.ValueString() != "bafde89c-041e-1756-082b-933aaf16cad8" {
		t.Fatalf("expected afw uuid to read from API, got %q", tunnel.AfwPerAppVpnProfileUUID.ValueString())
	}
}
