package assignment

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// --- fixture plumbing -------------------------------------------------------

// modifyPlanSchemaTypes caches the schema and the nested element types needed
// to hand-build tfsdk.Plan/tfsdk.Config values for ModifyPlan tests, mirroring
// the approach in resource_restriction_plan_test.go's partialRestrictionConfig.
type modifyPlanSchemaTypes struct {
	schemaResp     *resource.SchemaResponse
	assignmentElem types.ObjectType
	appConfigElem  attr.Type
	usageElem      attr.Type
}

func loadModifyPlanSchemaTypes(t *testing.T) modifyPlanSchemaTypes {
	t.Helper()
	r := &purchasedApplicationAssignmentResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	listType, ok := schemaResp.Schema.GetAttributes()["assignments"].GetType().(types.ListType)
	if !ok {
		t.Fatal("assignments is not a list type")
	}
	elemType, ok := listType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	distType, ok := elemType.AttrTypes["distribution"].(types.ObjectType)
	if !ok {
		t.Fatal("distribution is not an object type")
	}
	vppType, ok := distType.AttrTypes["vpp_app_details"].(types.ObjectType)
	if !ok {
		t.Fatal("vpp_app_details is not an object type")
	}
	usageListType, ok := vppType.AttrTypes["license_usage"].(types.ListType)
	if !ok {
		t.Fatal("license_usage is not a list type")
	}
	appConfigListType, ok := elemType.AttrTypes["application_configuration"].(types.ListType)
	if !ok {
		t.Fatal("application_configuration is not a list type")
	}

	return modifyPlanSchemaTypes{
		schemaResp:     schemaResp,
		assignmentElem: elemType,
		appConfigElem:  appConfigListType.ElemType,
		usageElem:      usageListType.ElemType,
	}
}

// assignment builds one assignment (priority 0 — every test in this file
// uses a single-element assignments list) with the given restriction and
// is_dynamic_template_saved value, leaving every other attribute null/empty.
func (st modifyPlanSchemaTypes) assignment(restriction *tf.PurchasedAppAssignmentRestrictionModel, dts types.Bool) tf.PurchasedAppAssignmentModel {
	return tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       types.StringNull(),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: types.ListNull(st.usageElem)},
		},
		Restriction:              restriction,
		ApplicationConfiguration: types.ListNull(st.appConfigElem),
		ApplicationAttributes:    types.ListNull(st.appConfigElem),
		IsDynamicTemplateSaved:   dts,
	}
}

func (st modifyPlanSchemaTypes) emptyRaw(ctx context.Context) tftypes.Value {
	schemaType := st.schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		panic("purchased application assignment schema type is not an Object")
	}
	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(schemaType, values)
}

// ruleModel builds the rule-level model for a set of assignments. A nil
// assignments slice plans/configures assignments as an explicit null list
// (distinct from an empty-but-present list) so the "assignments unknown/null"
// ModifyPlan short-circuit can be exercised.
func (st modifyPlanSchemaTypes) ruleModel(ctx context.Context, t *testing.T, appUUID string, assignments []tf.PurchasedAppAssignmentModel) tf.PurchasedAppAssignmentRuleModel {
	t.Helper()
	var assignmentsList types.List
	if assignments == nil {
		assignmentsList = types.ListNull(st.assignmentElem)
	} else {
		listVal, diags := types.ListValueFrom(ctx, st.assignmentElem, assignments)
		if diags.HasError() {
			t.Fatalf("assignments: %v", diags)
		}
		assignmentsList = listVal
	}
	return tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue(appUUID),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         assignmentsList,
	}
}

// plan builds a tfsdk.Plan for modifyPlanTestAppUUID (every test in this
// file uses that one application UUID).
func (st modifyPlanSchemaTypes) plan(t *testing.T, assignments []tf.PurchasedAppAssignmentModel) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	p := tfsdk.Plan{Schema: st.schemaResp.Schema, Raw: st.emptyRaw(ctx)}
	model := st.ruleModel(ctx, t, modifyPlanTestAppUUID, assignments)
	if diags := p.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return p
}

// Config builds a tfsdk.Config with the same shape as plan. Since tfsdk.Config
// has no Set method (only Plan/State do), the raw value is built via a
// throwaway Plan against the identical schema and then rewrapped.
func (st modifyPlanSchemaTypes) config(t *testing.T, assignments []tf.PurchasedAppAssignmentModel) tfsdk.Config {
	t.Helper()
	planT := st.plan(t, assignments)
	return tfsdk.Config{Schema: st.schemaResp.Schema, Raw: planT.Raw}
}

const modifyPlanTestAppUUID = "596b30c4-5fd8-f8a4-2f40-553c312b9f1a"

// fakeVppPlatformLookup is the mh5 test double for vppPlatformLookupAPI: it
// counts calls so tests can assert exactly when (and how often) ModifyPlan
// performs a lookup.
type fakeVppPlatformLookup struct {
	platform vppPlatform
	err      error
	calls    int
}

func (f *fakeVppPlatformLookup) LookupVppPlatform(_ context.Context, _ string) (vppPlatform, error) {
	f.calls++
	if f.err != nil {
		return vppPlatformUnknown, f.err
	}
	return f.platform, nil
}

func newModifyPlanResource(fake *fakeVppPlatformLookup) *purchasedApplicationAssignmentResource {
	return &purchasedApplicationAssignmentResource{
		client: &sdk.Client{},
		newVppPlatformLookup: func(*sdk.Client) vppPlatformLookupAPI {
			return fake
		},
	}
}

func runModifyPlan(t *testing.T, r *purchasedApplicationAssignmentResource, planT tfsdk.Plan, configT tfsdk.Config) *resource.ModifyPlanResponse {
	t.Helper()
	ctx := context.Background()
	resp := &resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: planT.Schema, Raw: planT.Raw.Copy()},
	}
	req := resource.ModifyPlanRequest{Plan: planT, Config: configT}
	r.ModifyPlan(ctx, req, resp)
	return resp
}

// --- ModifyPlan behavioural tests -------------------------------------------

func TestModifyPlan_Destroy_NoOp(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	ctx := context.Background()
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	planT := st.plan(t, []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{PreventRemoval: types.BoolValue(true)}, types.BoolNull()),
	})
	configT := st.config(t, []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{PreventRemoval: types.BoolValue(true)}, types.BoolNull()),
	})

	destroyPlan := tfsdk.Plan{Schema: planT.Schema, Raw: tftypes.NewValue(planT.Raw.Type(), nil)}

	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: planT.Schema, Raw: destroyPlan.Raw.Copy()}}
	req := resource.ModifyPlanRequest{Plan: destroyPlan, Config: configT}
	r.ModifyPlan(ctx, req, resp)

	if resp.Diagnostics.HasError() || len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics for a destroy plan, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls for a destroy plan, got %d", fake.calls)
	}
}

func TestModifyPlan_NoLookupWhenNoFlagTrue(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	// Only remove_on_unenroll is true: honoured on every platform, so this
	// must never trigger a lookup.
	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{RemoveOnUnenroll: types.BoolValue(true)}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls when no platform-sensitive flag is true, got %d", fake.calls)
	}
}

func TestModifyPlan_MacOSRejected(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(true),
			PreventRemoval:           types.BoolValue(true),
			PreventApplicationBackup: types.BoolValue(true),
			MakeAppMdmManaged:        types.BoolValue(true),
			ManagedAccess:            types.BoolValue(true),
		}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	if fake.calls != 1 {
		t.Fatalf("expected exactly 1 lookup call, got %d", fake.calls)
	}

	errs := resp.Diagnostics.Errors()
	if len(errs) != 4 {
		t.Fatalf("expected 4 errors (one per macOS-ignored flag), got %d: %v", len(errs), resp.Diagnostics)
	}
	// remove_on_unenroll must not be flagged; every error must be the
	// macOS-ignored diagnostic, each naming a different offending flag.
	wantFlags := map[string]bool{
		"prevent_removal":            false,
		"prevent_application_backup": false,
		"make_app_mdm_managed":       false,
		"managed_access":             false,
	}
	for _, d := range errs {
		if d.Summary() != macOSIgnoredSummary {
			t.Fatalf("unexpected error summary %q", d.Summary())
		}
		found := false
		for flag := range wantFlags {
			if !wantFlags[flag] && (d.Detail() == fmt.Sprintf(macOSIgnoredDetail, flag)) {
				wantFlags[flag] = true
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("error detail did not match any expected flag: %q", d.Detail())
		}
	}
	for flag, seen := range wantFlags {
		if !seen {
			t.Fatalf("expected an error naming flag %q, none found", flag)
		}
	}
}

func TestModifyPlan_IOSAllowed_NoDiags(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll: types.BoolValue(true),
			PreventRemoval:   types.BoolValue(true),
			ManagedAccess:    types.BoolValue(true),
		}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got: %v", resp.Diagnostics)
	}
	if fake.calls != 1 {
		t.Fatalf("expected exactly 1 lookup call, got %d", fake.calls)
	}
}

func managedAccessFromPlan(t *testing.T, planT tfsdk.Plan) types.Bool {
	t.Helper()
	ctx := context.Background()
	var v types.Bool
	diags := planT.GetAttribute(ctx, path.Root("assignments").AtListIndex(0).AtName("restriction").AtName("managed_access"), &v)
	if diags.HasError() {
		t.Fatalf("get managed_access: %v", diags)
	}
	return v
}

func TestModifyPlan_IOS_ManagedAccessAutoSet_MakeAppMdmManaged(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	r := newModifyPlanResource(fake)

	planAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolValue(false), // Default(false) plan value before ModifyPlan.
		}, types.BoolNull()),
	}
	configAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolNull(), // not set in config
		}, types.BoolNull()),
	}
	planT := st.plan(t, planAssignments)
	configT := st.config(t, configAssignments)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	ma := managedAccessFromPlan(t, resp.Plan)
	if ma.IsNull() || ma.IsUnknown() || !ma.ValueBool() {
		t.Fatalf("expected plan managed_access = true, got %v", ma)
	}
}

func TestModifyPlan_IOS_ManagedAccessAutoSet_PreventApplicationBackup(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	r := newModifyPlanResource(fake)

	planAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			PreventApplicationBackup: types.BoolValue(true),
			ManagedAccess:            types.BoolValue(false),
		}, types.BoolNull()),
	}
	configAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			PreventApplicationBackup: types.BoolValue(true),
			ManagedAccess:            types.BoolNull(),
		}, types.BoolNull()),
	}
	planT := st.plan(t, planAssignments)
	configT := st.config(t, configAssignments)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	ma := managedAccessFromPlan(t, resp.Plan)
	if ma.IsNull() || ma.IsUnknown() || !ma.ValueBool() {
		t.Fatalf("expected plan managed_access = true, got %v", ma)
	}
}

// TestModifyPlan_IOS_ManagedAccessAutoSet_NoPerpetualDiff exercises the same
// scenario as above but simulates "next plan" conditions: prior state already
// holds managed_access = true (mirrored into the plan since config is still
// null for it, matching what Optional+Computed with Default(false) would
// carry forward), config still leaves managed_access unset, and
// make_app_mdm_managed is still true. ModifyPlan must set managed_access =
// true again (idempotent), not leave it or null it, so there is no perpetual
// diff between applies.
func TestModifyPlan_IOS_ManagedAccessAutoSet_NoPerpetualDiff(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	r := newModifyPlanResource(fake)

	planAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolValue(true), // carried forward from prior state
		}, types.BoolNull()),
	}
	configAssignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolNull(),
		}, types.BoolNull()),
	}
	planT := st.plan(t, planAssignments)
	configT := st.config(t, configAssignments)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	ma := managedAccessFromPlan(t, resp.Plan)
	if ma.IsNull() || ma.IsUnknown() || !ma.ValueBool() {
		t.Fatalf("expected plan managed_access = true on repeat plan, got %v", ma)
	}
}

func TestModifyPlan_IOS_ExplicitManagedAccessFalse_Errors(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformIOS}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolValue(false),
		}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if errs[0].Summary() != iosManagedAccessForcedSummary {
		t.Fatalf("unexpected summary: %q", errs[0].Summary())
	}
}

func TestModifyPlan_LookupError_WarningOnly(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{err: errors.New("boom")}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{PreventRemoval: types.BoolValue(true)}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics, only a warning, got: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics.Warnings()) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(resp.Diagnostics.Warnings()), resp.Diagnostics)
	}
	if resp.Diagnostics.Warnings()[0].Summary() != vppPlatformLookupFailedSummary {
		t.Fatalf("unexpected warning summary: %q", resp.Diagnostics.Warnings()[0].Summary())
	}
	// Plan restriction must be unchanged (still whatever was planned).
	pr := managedAccessFromPlan(t, resp.Plan)
	if !pr.IsNull() && !pr.IsUnknown() && pr.ValueBool() {
		t.Fatalf("expected managed_access to be unmodified (default false), got %v", pr)
	}
}

func TestModifyPlan_UnknownPlatform_NoOp(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformUnknown}
	r := newModifyPlanResource(fake)

	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolNull(),
		}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	resp := runModifyPlan(t, r, planT, configT)
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics for unknown platform, got: %v", resp.Diagnostics)
	}
	if fake.calls != 1 {
		t.Fatalf("expected exactly 1 lookup call, got %d", fake.calls)
	}
}

// desired_state_management / is_dynamic_template_saved were checked in
// ModifyPlan prior to internal-task minor 1; see
// TestValidateConfigDesiredStateManagementTrue_Errors and
// TestValidateConfigIsDynamicTemplateSavedTrue_Errors in
// resource_validate_test.go for their current (ValidateConfig) coverage.

func TestModifyPlan_ApplicationUUIDUnknown_NoOp(t *testing.T) {
	st := loadModifyPlanSchemaTypes(t)
	fake := &fakeVppPlatformLookup{platform: vppPlatformMacOS}
	r := newModifyPlanResource(fake)

	ctx := context.Background()
	assignments := []tf.PurchasedAppAssignmentModel{
		st.assignment(&tf.PurchasedAppAssignmentRestrictionModel{PreventRemoval: types.BoolValue(true)}, types.BoolNull()),
	}
	planT := st.plan(t, assignments)
	configT := st.config(t, assignments)

	// Force application_uuid unknown on the plan, as Terraform core would for
	// an interpolated value not yet known.
	diags := planT.SetAttribute(ctx, path.Root("application_uuid"), types.StringUnknown())
	if diags.HasError() {
		t.Fatalf("set application_uuid unknown: %v", diags)
	}

	resp := runModifyPlan(t, r, planT, configT)
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got: %v", resp.Diagnostics)
	}
	if fake.calls != 0 {
		t.Fatalf("expected 0 lookup calls, got %d", fake.calls)
	}
}
