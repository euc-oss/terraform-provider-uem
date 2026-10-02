package assignment

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// managedAccessTestPath is the attribute path managedAccessPlanModifier
// operates at for every fixture in this file: a single assignment at index 0.
var managedAccessTestPath = path.Root("assignments").AtListIndex(0).AtName("restriction").AtName("managed_access")

// --- unit tests: managedAccessPlanModifier in isolation (internal-task-65i) ------

// managedAccessSiblingConfig builds a tfsdk.Config with one assignment whose
// restriction sets make_app_mdm_managed/prevent_application_backup as given,
// so managedAccessPlanModifier's req.Config.GetAttribute sibling lookups have
// something real to read.
func managedAccessSiblingConfig(t *testing.T, st modifyPlanSchemaTypes, mamm, pab types.Bool) tfsdk.Config {
	t.Helper()
	return st.config(t, []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged:        mamm,
			PreventApplicationBackup: pab,
			ManagedAccess:            types.BoolNull(),
		}, types.BoolNull()),
	})
}

func runManagedAccessPlanModifier(t *testing.T, cfg tfsdk.Config, configValue, stateValue, planValue types.Bool) types.Bool {
	t.Helper()
	req := planmodifier.BoolRequest{
		Path:        managedAccessTestPath,
		Config:      cfg,
		ConfigValue: configValue,
		StateValue:  stateValue,
		PlanValue:   planValue,
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	managedAccessPlanModifier{}.PlanModifyBool(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	return resp.PlanValue
}

func TestManagedAccessPlanModifier_ConfigNull_PriorTrue_MAMMTrue_KeepsProposed(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolValue(true), types.BoolNull())
	// PlanValue simulates what Terraform core's proposed-new-state carried
	// forward for a null-config Optional+Computed attribute: the prior true.
	got := runManagedAccessPlanModifier(t, cfg, types.BoolNull(), types.BoolValue(true), types.BoolValue(true))
	if !got.Equal(types.BoolValue(true)) {
		t.Fatalf("expected proposed true kept, got %v", got)
	}
}

func TestManagedAccessPlanModifier_ConfigNull_PriorTrue_PABTrue_KeepsProposed(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolNull(), types.BoolValue(true))
	got := runManagedAccessPlanModifier(t, cfg, types.BoolNull(), types.BoolValue(true), types.BoolValue(true))
	if !got.Equal(types.BoolValue(true)) {
		t.Fatalf("expected proposed true kept, got %v", got)
	}
}

func TestManagedAccessPlanModifier_ConfigNull_PriorTrue_NeitherSibling_PlansFalse(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolNull(), types.BoolNull())
	got := runManagedAccessPlanModifier(t, cfg, types.BoolNull(), types.BoolValue(true), types.BoolValue(true))
	if !got.Equal(types.BoolValue(false)) {
		t.Fatalf("expected false (removal, neither sibling true), got %v", got)
	}
}

func TestManagedAccessPlanModifier_ConfigNull_PriorNull_PlansFalse_Create(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolNull(), types.BoolNull())
	got := runManagedAccessPlanModifier(t, cfg, types.BoolNull(), types.BoolNull(), types.BoolUnknown())
	if !got.Equal(types.BoolValue(false)) {
		t.Fatalf("expected false on create, got %v", got)
	}
}

func TestManagedAccessPlanModifier_ConfigNull_PriorFalse_MAMMTrue_PlansFalse(t *testing.T) {
	// ModifyPlan (modifyPlanForIOS) is what sets managed_access true in this
	// case, not the schema plan modifier: the modifier must behave exactly
	// like the removed Default(false) here.
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolValue(true), types.BoolNull())
	got := runManagedAccessPlanModifier(t, cfg, types.BoolNull(), types.BoolValue(false), types.BoolValue(false))
	if !got.Equal(types.BoolValue(false)) {
		t.Fatalf("expected false (ModifyPlan will force true afterwards), got %v", got)
	}
}

func TestManagedAccessPlanModifier_ExplicitConfigValues_Untouched(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolNull(), types.BoolNull())

	t.Run("explicit false", func(t *testing.T) {
		got := runManagedAccessPlanModifier(t, cfg, types.BoolValue(false), types.BoolValue(true), types.BoolValue(false))
		if !got.Equal(types.BoolValue(false)) {
			t.Fatalf("expected untouched false, got %v", got)
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		got := runManagedAccessPlanModifier(t, cfg, types.BoolValue(true), types.BoolValue(false), types.BoolValue(true))
		if !got.Equal(types.BoolValue(true)) {
			t.Fatalf("expected untouched true, got %v", got)
		}
	})
}

func TestManagedAccessPlanModifier_ConfigUnknown_Untouched(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	cfg := managedAccessSiblingConfig(t, st, types.BoolNull(), types.BoolNull())
	got := runManagedAccessPlanModifier(t, cfg, types.BoolUnknown(), types.BoolValue(true), types.BoolUnknown())
	if !got.IsUnknown() {
		t.Fatalf("expected plan left unknown when config is unknown, got %v", got)
	}
}

// --- pipeline tests: through the real framework server (internal-task-65i) ------

// managedAccessTestAppUUID/managedAccessTestSmartGroupUUID/managedAccessTestID
// are fixed fixture identifiers reused across the pipeline tests below.
const (
	managedAccessTestAppUUID        = "596b30c4-5fd8-f8a4-2f40-553c312b9f1a"
	managedAccessTestSmartGroupUUID = "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"
	managedAccessTestID             = "assignment-rule-mh5-65i"
)

// managedAccessStateAssignment builds a post-apply assignment for PriorState:
// application_attributes/application_configuration are non-null empty lists,
// license_usage has one entry with redeemed known (0), and effective_date is
// null -- exactly the shape the live bug report showed drifting on every
// subsequent plan.
func managedAccessStateAssignment(t *testing.T, st modifyPlanSchemaTypes, managedAccess, makeAppMdmManaged types.Bool) tf.PurchasedAppAssignmentModel {
	t.Helper()
	ctx := context.Background()
	usageElem, ok := st.usageElem.(types.ObjectType)
	if !ok {
		t.Fatalf("license_usage element is not an object type: %T", st.usageElem)
	}
	usage, diags := types.ListValue(usageElem, []attr.Value{
		types.ObjectValueMust(usageElem.AttrTypes, map[string]attr.Value{
			"smart_group_uuid": types.StringValue(managedAccessTestSmartGroupUUID),
			"allocated":        types.Int64Value(1),
			"redeemed":         types.Int64Value(0),
		}),
	})
	if diags.HasError() {
		t.Fatalf("license_usage: %v", diags)
	}
	appConfigElemType, ok := st.appConfigElem.(types.ObjectType)
	if !ok {
		t.Fatalf("application_configuration element is not an object type: %T", st.appConfigElem)
	}
	emptyAppConfig, diags := types.ListValue(appConfigElemType, []attr.Value{})
	if diags.HasError() {
		t.Fatalf("empty application_configuration: %v", diags)
	}
	_ = ctx
	return tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       types.StringNull(),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: usage},
		},
		Restriction: &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(false),
			PreventRemoval:           types.BoolValue(false),
			PreventApplicationBackup: types.BoolNull(),
			MakeAppMdmManaged:        makeAppMdmManaged,
			ManagedAccess:            managedAccess,
			DesiredStateManagement:   types.BoolNull(),
		},
		ApplicationConfiguration: emptyAppConfig,
		ApplicationAttributes:    emptyAppConfig,
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
}

// managedAccessConfigAssignment builds the corresponding CONFIG assignment:
// every Computed-only/Optional+Computed leaf that the HCL never sets is null
// (application_attributes, application_configuration, effective_date,
// license_usage[].redeemed), while the Required leaves mirror the state
// fixture so the two are talking about the same assignment.
func managedAccessConfigAssignment(t *testing.T, st modifyPlanSchemaTypes, managedAccess, makeAppMdmManaged types.Bool) tf.PurchasedAppAssignmentModel {
	t.Helper()
	usageElem, ok := st.usageElem.(types.ObjectType)
	if !ok {
		t.Fatalf("license_usage element is not an object type: %T", st.usageElem)
	}
	usage, diags := types.ListValue(usageElem, []attr.Value{
		types.ObjectValueMust(usageElem.AttrTypes, map[string]attr.Value{
			"smart_group_uuid": types.StringValue(managedAccessTestSmartGroupUUID),
			"allocated":        types.Int64Value(1),
			"redeemed":         types.Int64Null(),
		}),
	})
	if diags.HasError() {
		t.Fatalf("license_usage config: %v", diags)
	}
	return tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       types.StringNull(),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: usage},
		},
		Restriction: &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolNull(),
			PreventRemoval:           types.BoolNull(),
			PreventApplicationBackup: types.BoolNull(),
			MakeAppMdmManaged:        makeAppMdmManaged,
			ManagedAccess:            managedAccess,
			DesiredStateManagement:   types.BoolNull(),
		},
		ApplicationConfiguration: types.ListNull(st.appConfigElem),
		ApplicationAttributes:    types.ListNull(st.appConfigElem),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
}

func managedAccessBuildState(t *testing.T, st modifyPlanSchemaTypes, id string, assignments []tf.PurchasedAppAssignmentModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	s := tfsdk.State{Schema: st.schemaResp.Schema, Raw: st.emptyRaw(ctx)}
	listVal, diags := types.ListValueFrom(ctx, st.assignmentElem, assignments)
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}
	model := tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringValue(id),
		ApplicationUUID:     types.StringValue(managedAccessTestAppUUID),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         listVal,
	}
	if diags := s.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return s
}

func managedAccessBuildConfig(t *testing.T, st modifyPlanSchemaTypes, assignments []tf.PurchasedAppAssignmentModel) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	p := tfsdk.Plan{Schema: st.schemaResp.Schema, Raw: st.emptyRaw(ctx)}
	listVal, diags := types.ListValueFrom(ctx, st.assignmentElem, assignments)
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}
	model := tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue(managedAccessTestAppUUID),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         listVal,
	}
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}
	return tfsdk.Config{Schema: st.schemaResp.Schema, Raw: p.Raw}
}

// managedAccessProposedNewState approximates Terraform core's proposed-new-
// value algorithm for this nested schema: a non-null config (sub)value wins;
// a null config (sub)value carries the prior value forward wholesale. This is
// accurate for every attribute these tests vary (all Computed or
// Optional+Computed), which is what the real bug/fix hinges on; the tests
// hold every purely-Optional attribute (e.g. distribution.description,
// tunnel, vpp_app_details) identical between prior and config so this
// approximation never has to arbitrate a real removal for one of those.
func managedAccessProposedNewState(t *testing.T, prior, config tftypes.Value) tftypes.Value {
	t.Helper()
	if config.IsNull() {
		return prior
	}
	if prior.IsNull() {
		return config
	}
	switch ty := config.Type().(type) {
	case tftypes.Object:
		var priorAttrs, configAttrs map[string]tftypes.Value
		if err := prior.As(&priorAttrs); err != nil {
			t.Fatalf("prior object: %v", err)
		}
		if err := config.As(&configAttrs); err != nil {
			t.Fatalf("config object: %v", err)
		}
		out := make(map[string]tftypes.Value, len(ty.AttributeTypes))
		for name, at := range ty.AttributeTypes {
			cv, ok := configAttrs[name]
			if !ok {
				cv = tftypes.NewValue(at, nil)
			}
			pv, ok := priorAttrs[name]
			if !ok {
				pv = tftypes.NewValue(at, nil)
			}
			out[name] = managedAccessProposedNewState(t, pv, cv)
		}
		return tftypes.NewValue(ty, out)
	case tftypes.List:
		var priorList, configList []tftypes.Value
		if err := prior.As(&priorList); err != nil {
			t.Fatalf("prior list: %v", err)
		}
		if err := config.As(&configList); err != nil {
			t.Fatalf("config list: %v", err)
		}
		out := make([]tftypes.Value, len(configList))
		for i, cv := range configList {
			if i < len(priorList) {
				out[i] = managedAccessProposedNewState(t, priorList[i], cv)
			} else {
				out[i] = cv
			}
		}
		return tftypes.NewValue(ty, out)
	default:
		return config
	}
}

func managedAccessDynamicValue(t *testing.T, v tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	dv, err := tfprotov6.NewDynamicValue(v.Type(), v)
	if err != nil {
		t.Fatalf("NewDynamicValue: %v", err)
	}
	return &dv
}

func managedAccessFailOnError(t *testing.T, diags []*tfprotov6.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("plan error diagnostic: %s: %s", d.Summary, d.Detail)
		}
	}
}

// managedAccessNewServer wires a protocol-6 server exposing the resource with
// a fake VPP platform lookup, mirroring restrictionTestProvider's use in
// resource_restriction_plan_test.go.
func managedAccessNewServer(t *testing.T, fake *fakeVppPlatformLookup) tfprotov6.ProviderServer {
	t.Helper()
	svc := &serverDefaultingAssignmentService{}
	r := &purchasedApplicationAssignmentResource{
		client: &sdk.Client{},
		newPurchasedAppAssignmentService: func(*sdk.Client) purchasedAppAssignmentServiceAPI {
			return svc
		},
		newVppPlatformLookup: func(*sdk.Client) vppPlatformLookupAPI {
			return fake
		},
	}
	server, err := providerserver.NewProtocol6WithError(&restrictionTestProvider{r: r})()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}
	return server
}

// managedAccessRestrictionLeaf decodes assignments[0].restriction.<name> from
// a resource object value.
func managedAccessRestrictionLeaf(t *testing.T, v tftypes.Value, name string) tftypes.Value {
	t.Helper()
	leaves := restrictionLeaves(t, v)
	leaf, ok := leaves[name]
	if !ok {
		t.Fatalf("restriction has no %q leaf", name)
	}
	return leaf
}

// TestManagedAccessPipeline_IOSDerivedTrue_NoUnknowns is the real repro test
// (internal-task-65i): an iOS VPP assignment with managed_access=true in state
// (forced on by modifyPlanForIOS on a prior apply) and make_app_mdm_managed
// still true in config, but managed_access itself left unset. Before the
// fix, TransformDefaults stomped the carried-forward true back to false
// (config is null and managed_access had a schema Default), forcing plan !=
// prior state and marking every other computed attribute unknown. After the
// fix the planned state must equal the prior state exactly: no diff, no
// unknowns, anywhere.
func TestManagedAccessPipeline_IOSDerivedTrue_NoUnknowns(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	server := managedAccessNewServer(t, fake)

	priorState := managedAccessBuildState(t, st, managedAccessTestID, []tf.PurchasedAppAssignmentModel{
		managedAccessStateAssignment(t, st, types.BoolValue(true), types.BoolValue(true)),
	})
	config := managedAccessBuildConfig(t, st, []tf.PurchasedAppAssignmentModel{
		managedAccessConfigAssignment(t, st, types.BoolNull(), types.BoolValue(true)),
	})
	proposed := managedAccessProposedNewState(t, priorState.Raw, config.Raw)

	ctx := context.Background()
	planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_purchased_application_assignment",
		PriorState:       managedAccessDynamicValue(t, priorState.Raw),
		ProposedNewState: managedAccessDynamicValue(t, proposed),
		Config:           managedAccessDynamicValue(t, config.Raw),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	managedAccessFailOnError(t, planResp.Diagnostics)

	planned, err := planResp.PlannedState.Unmarshal(priorState.Raw.Type())
	if err != nil {
		t.Fatalf("unmarshal planned: %v", err)
	}

	if !planned.Equal(priorState.Raw) {
		t.Fatalf("expected planned state to equal prior state exactly (no unknowns/diff)\nplanned: %s\nprior:   %s", planned, priorState.Raw)
	}
	if fake.calls == 0 {
		t.Fatal("expected a platform lookup since make_app_mdm_managed is planned true")
	}
}

// TestManagedAccessPipeline_PlainNullConfigPlansFalse covers the two cases
// the maintainer called out as needing the modifier to behave EXACTLY like
// the removed Default(false): a create, and an update from a prior false,
// both with no make_app_mdm_managed/prevent_application_backup in play.
func TestManagedAccessPipeline_PlainNullConfigPlansFalse(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		st := loadModifyPlanSchemaTypes(t)
		fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
		server := managedAccessNewServer(t, fake)

		config := managedAccessBuildConfig(t, st, []tf.PurchasedAppAssignmentModel{
			managedAccessConfigAssignment(t, st, types.BoolNull(), types.BoolNull()),
		})
		priorRaw := tftypes.NewValue(config.Raw.Type(), nil)

		ctx := context.Background()
		planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
			TypeName:         "uem_purchased_application_assignment",
			PriorState:       managedAccessDynamicValue(t, priorRaw),
			ProposedNewState: managedAccessDynamicValue(t, config.Raw),
			Config:           managedAccessDynamicValue(t, config.Raw),
		})
		if err != nil {
			t.Fatalf("PlanResourceChange: %v", err)
		}
		managedAccessFailOnError(t, planResp.Diagnostics)

		planned, err := planResp.PlannedState.Unmarshal(config.Raw.Type())
		if err != nil {
			t.Fatalf("unmarshal planned: %v", err)
		}
		got := managedAccessRestrictionLeaf(t, planned, "managed_access")
		want := tftypes.NewValue(tftypes.Bool, false)
		if !got.Equal(want) {
			t.Fatalf("managed_access = %s, want %s", got, want)
		}
		if fake.calls != 0 {
			t.Fatalf("expected no platform lookup with no restriction flags true, got %d calls", fake.calls)
		}
	})

	t.Run("update from prior false", func(t *testing.T) {
		st := loadModifyPlanSchemaTypes(t)
		fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
		server := managedAccessNewServer(t, fake)

		priorState := managedAccessBuildState(t, st, managedAccessTestID, []tf.PurchasedAppAssignmentModel{
			managedAccessStateAssignment(t, st, types.BoolValue(false), types.BoolNull()),
		})
		config := managedAccessBuildConfig(t, st, []tf.PurchasedAppAssignmentModel{
			managedAccessConfigAssignment(t, st, types.BoolNull(), types.BoolNull()),
		})
		proposed := managedAccessProposedNewState(t, priorState.Raw, config.Raw)

		ctx := context.Background()
		planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
			TypeName:         "uem_purchased_application_assignment",
			PriorState:       managedAccessDynamicValue(t, priorState.Raw),
			ProposedNewState: managedAccessDynamicValue(t, proposed),
			Config:           managedAccessDynamicValue(t, config.Raw),
		})
		if err != nil {
			t.Fatalf("PlanResourceChange: %v", err)
		}
		managedAccessFailOnError(t, planResp.Diagnostics)

		planned, err := planResp.PlannedState.Unmarshal(priorState.Raw.Type())
		if err != nil {
			t.Fatalf("unmarshal planned: %v", err)
		}
		got := managedAccessRestrictionLeaf(t, planned, "managed_access")
		want := tftypes.NewValue(tftypes.Bool, false)
		if !got.Equal(want) {
			t.Fatalf("managed_access = %s, want %s", got, want)
		}
		if !planned.Equal(priorState.Raw) {
			t.Fatalf("expected no diff at all (false == false)\nplanned: %s\nprior:   %s", planned, priorState.Raw)
		}
	})
}

// TestManagedAccessPipeline_MAMMRemoved_PlansFalseAndMAMMNull covers removing
// make_app_mdm_managed (and managed_access) from configuration after both
// were true in state: managed_access must NOT stay stuck true just because
// the prior state had it true -- it must plan false (item 3, maintainer
// follow-up), and make_app_mdm_managed itself plans null (unrelated
// nullWhenConfigNullBoolModifier behaviour, unaffected by this fix).
func TestManagedAccessPipeline_MAMMRemoved_PlansFalseAndMAMMNull(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	server := managedAccessNewServer(t, fake)

	priorState := managedAccessBuildState(t, st, managedAccessTestID, []tf.PurchasedAppAssignmentModel{
		managedAccessStateAssignment(t, st, types.BoolValue(true), types.BoolValue(true)),
	})
	config := managedAccessBuildConfig(t, st, []tf.PurchasedAppAssignmentModel{
		managedAccessConfigAssignment(t, st, types.BoolNull(), types.BoolNull()),
	})
	proposed := managedAccessProposedNewState(t, priorState.Raw, config.Raw)

	ctx := context.Background()
	planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_purchased_application_assignment",
		PriorState:       managedAccessDynamicValue(t, priorState.Raw),
		ProposedNewState: managedAccessDynamicValue(t, proposed),
		Config:           managedAccessDynamicValue(t, config.Raw),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	managedAccessFailOnError(t, planResp.Diagnostics)

	planned, err := planResp.PlannedState.Unmarshal(priorState.Raw.Type())
	if err != nil {
		t.Fatalf("unmarshal planned: %v", err)
	}

	gotManagedAccess := managedAccessRestrictionLeaf(t, planned, "managed_access")
	wantManagedAccess := tftypes.NewValue(tftypes.Bool, false)
	if !gotManagedAccess.Equal(wantManagedAccess) {
		t.Fatalf("managed_access = %s, want %s (not stuck true)", gotManagedAccess, wantManagedAccess)
	}

	gotMAMM := managedAccessRestrictionLeaf(t, planned, "make_app_mdm_managed")
	if !gotMAMM.IsNull() {
		t.Fatalf("make_app_mdm_managed = %s, want null", gotMAMM)
	}
}
