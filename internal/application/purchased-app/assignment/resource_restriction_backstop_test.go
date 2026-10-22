package assignment

import (
	"context"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// --- pure helper tests -----------------------------------------------------

func assignmentWithRestriction(priority int64, distName string, restriction *tf.PurchasedAppAssignmentRestrictionModel) tf.PurchasedAppAssignmentModel {
	return tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(priority),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name: types.StringValue(distName),
		},
		Restriction: restriction,
	}
}

func TestCompareRestrictionReadback_MismatchOnDroppedTrueFlags(t *testing.T) {
	planned := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(10, "App A", &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(true),
			PreventRemoval:           types.BoolValue(true),
			PreventApplicationBackup: types.BoolValue(true),
			MakeAppMdmManaged:        types.BoolValue(true),
			ManagedAccess:            types.BoolValue(true),
			DesiredStateManagement:   types.BoolValue(true),
		}),
	}
	actual := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(10, "App A", &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(true),
			PreventRemoval:           types.BoolValue(false),
			PreventApplicationBackup: types.BoolValue(false),
			MakeAppMdmManaged:        types.BoolValue(false),
			ManagedAccess:            types.BoolValue(false),
			DesiredStateManagement:   types.BoolValue(false),
		}),
	}

	mismatches := compareRestrictionReadback(planned, actual)
	if len(mismatches) != 1 {
		t.Fatalf("expected 1 assignment mismatch, got %d", len(mismatches))
	}
	got := map[string]restrictionFieldMismatch{}
	for _, f := range mismatches[0].Fields {
		got[f.Field] = f
	}
	for _, field := range []string{"prevent_removal", "prevent_application_backup", "make_app_mdm_managed", "managed_access", "desired_state_management"} {
		if _, ok := got[field]; !ok {
			t.Errorf("expected mismatch for field %q", field)
		}
	}
	if _, ok := got["remove_on_unenroll"]; ok {
		t.Errorf("remove_on_unenroll was honoured and should not be reported as a mismatch")
	}

	detail := formatRestrictionMismatchDetail(mismatches)
	if !strings.Contains(detail, reasonMacOSIgnored) {
		t.Errorf("detail missing macOS-ignored reason: %s", detail)
	}
	if n := strings.Count(detail, reasonMacOSIgnored); n != 1 {
		t.Errorf("macOS-ignored reason should appear exactly once (deduplicated), got %d: %s", n, detail)
	}
	if !strings.Contains(detail, reasonDesiredStateIgnored) {
		t.Errorf("detail missing desired_state_management reason: %s", detail)
	}
	if !strings.Contains(detail, reasonGuidance) {
		t.Errorf("detail missing guidance: %s", detail)
	}
}

func TestCompareRestrictionReadback_ManagedAccessDerivedOnIOS(t *testing.T) {
	planned := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(0, "App B", &tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolValue(false),
		}),
	}
	actual := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(0, "App B", &tf.PurchasedAppAssignmentRestrictionModel{
			MakeAppMdmManaged: types.BoolValue(true),
			ManagedAccess:     types.BoolValue(true),
		}),
	}

	mismatches := compareRestrictionReadback(planned, actual)
	if len(mismatches) != 1 || len(mismatches[0].Fields) != 1 {
		t.Fatalf("expected exactly one managed_access mismatch, got %+v", mismatches)
	}
	f := mismatches[0].Fields[0]
	if f.Field != "managed_access" || f.Planned != false || f.Actual != true {
		t.Fatalf("unexpected mismatch: %+v", f)
	}

	detail := formatRestrictionMismatchDetail(mismatches)
	if !strings.Contains(detail, reasonManagedAccessDerived) {
		t.Errorf("detail missing iOS derivation reason: %s", detail)
	}
}

func TestCompareRestrictionReadback_AllHonoured_NoMismatch(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(true),
		ManagedAccess:    types.BoolValue(true),
	}
	planned := []tf.PurchasedAppAssignmentModel{assignmentWithRestriction(1, "App C", restriction)}
	actual := []tf.PurchasedAppAssignmentModel{assignmentWithRestriction(1, "App C", &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(true),
		ManagedAccess:    types.BoolValue(true),
	})}

	if mismatches := compareRestrictionReadback(planned, actual); len(mismatches) != 0 {
		t.Fatalf("expected no mismatches, got %+v", mismatches)
	}
}

func TestCompareRestrictionReadback_UnsetFlagsNoFalsePositive(t *testing.T) {
	planned := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(2, "App D", &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(false),
			PreventRemoval:           types.BoolNull(),
			PreventApplicationBackup: types.BoolNull(),
			MakeAppMdmManaged:        types.BoolNull(),
			ManagedAccess:            types.BoolNull(),
			DesiredStateManagement:   types.BoolNull(),
		}),
	}
	actual := []tf.PurchasedAppAssignmentModel{
		assignmentWithRestriction(2, "App D", &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(false),
			PreventRemoval:           types.BoolValue(false),
			PreventApplicationBackup: types.BoolValue(false),
			MakeAppMdmManaged:        types.BoolValue(false),
			ManagedAccess:            types.BoolValue(false),
			DesiredStateManagement:   types.BoolValue(false),
		}),
	}

	if mismatches := compareRestrictionReadback(planned, actual); len(mismatches) != 0 {
		t.Fatalf("expected no mismatches for null/false planned flags, got %+v", mismatches)
	}
}

func TestCompareRestrictionReadback_NilPlannedRestrictionSkipped(t *testing.T) {
	planned := []tf.PurchasedAppAssignmentModel{assignmentWithRestriction(3, "App E", nil)}
	actual := []tf.PurchasedAppAssignmentModel{assignmentWithRestriction(3, "App E", &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(true),
	})}

	if mismatches := compareRestrictionReadback(planned, actual); len(mismatches) != 0 {
		t.Fatalf("expected no mismatches when nothing was planned, got %+v", mismatches)
	}
}

// --- resource-level Create/Update tests -------------------------------------

// transformAssignmentService fakes the UEM PUT+GET cycle: it stores the last
// PUT body and returns it (optionally mutated by transform) on GET, the way
// a real backend would echo what it actually persisted.
type transformAssignmentService struct {
	last      *sdk.AppAssignmentRuleV2Model
	transform func([]sdk.AppAssignmentV2Model) []sdk.AppAssignmentV2Model
}

func (s *transformAssignmentService) GetAssignmentRuleAsync(context.Context, string) (http.Header, *sdk.AppAssignmentRuleV2Model, error) {
	if s.last == nil {
		return nil, nil, nil
	}
	out := *s.last
	assignments := make([]sdk.AppAssignmentV2Model, len(s.last.Assignments))
	copy(assignments, s.last.Assignments)
	if s.transform != nil {
		assignments = s.transform(assignments)
	}
	out.Assignments = assignments
	return nil, &out, nil
}

func (s *transformAssignmentService) UpdateAssignmentRuleAsync(_ context.Context, _ string, request *sdk.AppAssignmentRuleV2Model) (http.Header, error) {
	s.last = request
	return nil, nil
}

// dropAllRestrictionFlags simulates a macOS VPP app: every restriction flag
// the request set comes back false.
func dropAllRestrictionFlags(assignments []sdk.AppAssignmentV2Model) []sdk.AppAssignmentV2Model {
	out := make([]sdk.AppAssignmentV2Model, len(assignments))
	falseVal := false
	for i, a := range assignments {
		if a.Restriction != nil {
			r := *a.Restriction
			r.RemoveOnUnenroll = &falseVal
			r.PreventRemoval = &falseVal
			r.PreventApplicationBackup = &falseVal
			r.MakeAppMdmManaged = &falseVal
			r.ManagedAccess = &falseVal
			r.DesiredStateManagement = &falseVal
			a.Restriction = &r
		}
		out[i] = a
	}
	return out
}

// deriveManagedAccessOnIOS simulates an iOS VPP app: make_app_mdm_managed is
// honoured as sent, but managed_access is forced true whenever
// make_app_mdm_managed is true, regardless of what was requested.
func deriveManagedAccessOnIOS(assignments []sdk.AppAssignmentV2Model) []sdk.AppAssignmentV2Model {
	out := make([]sdk.AppAssignmentV2Model, len(assignments))
	trueVal := true
	for i, a := range assignments {
		if a.Restriction != nil && a.Restriction.MakeAppMdmManaged != nil && *a.Restriction.MakeAppMdmManaged {
			r := *a.Restriction
			r.ManagedAccess = &trueVal
			a.Restriction = &r
		}
		out[i] = a
	}
	return out
}

func restrictionPlanState(t *testing.T, restriction *tf.PurchasedAppAssignmentRestrictionModel) (resource.CreateRequest, *resource.CreateResponse) {
	t.Helper()
	ctx := context.Background()

	r := &purchasedApplicationAssignmentResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	empty := emptyPurchasedAssignmentState(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: empty.Raw.Copy()}

	elemType, ok := schemaResp.Schema.GetAttributes()["assignments"].GetType().(types.ListType).ElemType.(types.ObjectType)
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
	usageElemType, ok := usageListType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatal("license_usage element is not an object type")
	}
	appConfigListType, ok := elemType.AttrTypes["application_configuration"].(types.ListType)
	if !ok {
		t.Fatal("application_configuration is not a list type")
	}

	usage, diags := types.ListValue(usageElemType, []attr.Value{
		types.ObjectValueMust(usageElemType.AttrTypes, map[string]attr.Value{
			"smart_group_uuid": types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
			"allocated":        types.Int64Value(1),
			"redeemed":         types.Int64Null(),
		}),
	})
	if diags.HasError() {
		t.Fatalf("license_usage: %v", diags)
	}

	a := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(10),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP App"),
			Description:       types.StringNull(),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: usage},
		},
		Restriction:              restriction,
		ApplicationConfiguration: types.ListNull(appConfigListType.ElemType),
		ApplicationAttributes:    types.ListNull(appConfigListType.ElemType),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
	assignments, diags := types.ListValueFrom(ctx, elemType, []tf.PurchasedAppAssignmentModel{a})
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}

	model := tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue("596b30c4-5fd8-f8a4-2f40-553c312b9f1a"),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         assignments,
	}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyPurchasedAssignmentState(t)}
	return req, resp
}

func runCreateWithService(t *testing.T, restriction *tf.PurchasedAppAssignmentRestrictionModel, svc purchasedAppAssignmentServiceAPI) *resource.CreateResponse {
	t.Helper()
	req, resp := restrictionPlanState(t, restriction)
	r := &purchasedApplicationAssignmentResource{
		client:                           &sdk.Client{},
		newPurchasedAppAssignmentService: func(*sdk.Client) purchasedAppAssignmentServiceAPI { return svc },
	}
	r.Create(context.Background(), req, resp)
	return resp
}

func runUpdateWithService(t *testing.T, restriction *tf.PurchasedAppAssignmentRestrictionModel, svc purchasedAppAssignmentServiceAPI) *resource.UpdateResponse {
	t.Helper()
	createReq, _ := restrictionPlanState(t, restriction)

	updateReq := resource.UpdateRequest{
		Plan:  createReq.Plan,
		State: emptyPurchasedAssignmentState(t),
	}
	resp := &resource.UpdateResponse{State: emptyPurchasedAssignmentState(t)}
	r := &purchasedApplicationAssignmentResource{
		client:                           &sdk.Client{},
		newPurchasedAppAssignmentService: func(*sdk.Client) purchasedAppAssignmentServiceAPI { return svc },
	}
	r.Update(context.Background(), updateReq, resp)
	return resp
}

func restrictionLeafBool(t *testing.T, state tfsdk.State, field string) bool {
	t.Helper()
	var data tf.PurchasedAppAssignmentRuleModel
	if diags := state.Get(context.Background(), &data); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := data.Assignments.ElementsAs(context.Background(), &assigns, false); diags.HasError() {
		t.Fatalf("read assignments: %v", diags)
	}
	if len(assigns) != 1 || assigns[0].Restriction == nil {
		t.Fatalf("expected one assignment with a restriction block, got %+v", assigns)
	}
	r := assigns[0].Restriction
	switch field {
	case "remove_on_unenroll":
		return r.RemoveOnUnenroll.ValueBool()
	case "prevent_removal":
		return r.PreventRemoval.ValueBool()
	case "managed_access":
		return r.ManagedAccess.ValueBool()
	case "make_app_mdm_managed":
		return r.MakeAppMdmManaged.ValueBool()
	}
	t.Fatalf("unknown field %q", field)
	return false
}

func TestCreate_RestrictionBackstop_DroppedFlags(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll:         types.BoolValue(true),
		PreventRemoval:           types.BoolValue(true),
		PreventApplicationBackup: types.BoolValue(true),
		MakeAppMdmManaged:        types.BoolValue(true),
		ManagedAccess:            types.BoolValue(true),
		DesiredStateManagement:   types.BoolValue(true),
	}
	svc := &transformAssignmentService{transform: dropAllRestrictionFlags}
	resp := runCreateWithService(t, restriction, svc)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a backstop error diagnostic")
	}
	errCount := 0
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == restrictionBackstopSummary {
			errCount++
		}
	}
	if errCount != 1 {
		t.Fatalf("expected exactly one backstop error diagnostic, got %d (all: %v)", errCount, resp.Diagnostics)
	}
	if got := restrictionLeafBool(t, resp.State, "prevent_removal"); got {
		t.Errorf("expected state to reflect UEM's actual (false) value, got %v", got)
	}
}

func TestCreate_RestrictionBackstop_ManagedAccessDerived(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		MakeAppMdmManaged: types.BoolValue(true),
		ManagedAccess:     types.BoolValue(false),
	}
	svc := &transformAssignmentService{transform: deriveManagedAccessOnIOS}
	resp := runCreateWithService(t, restriction, svc)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a backstop error diagnostic")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == restrictionBackstopSummary && strings.Contains(d.Detail(), reasonManagedAccessDerived) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected diagnostic mentioning iOS managed_access derivation, got %v", resp.Diagnostics)
	}
	if got := restrictionLeafBool(t, resp.State, "managed_access"); !got {
		t.Errorf("expected state managed_access=true (UEM's actual value), got %v", got)
	}
}

func TestCreate_RestrictionBackstop_AllHonoured_NoDiagnostic(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(true),
		ManagedAccess:    types.BoolValue(true),
	}
	svc := &transformAssignmentService{}
	resp := runCreateWithService(t, restriction, svc)

	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect any diagnostics error: %v", resp.Diagnostics)
	}
}

func TestUpdate_RestrictionBackstop_DroppedFlags(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(true),
		PreventRemoval:   types.BoolValue(true),
	}
	svc := &transformAssignmentService{transform: dropAllRestrictionFlags}
	resp := runUpdateWithService(t, restriction, svc)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a backstop error diagnostic on Update")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == restrictionBackstopSummary {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected backstop diagnostic, got %v", resp.Diagnostics)
	}
	if got := restrictionLeafBool(t, resp.State, "prevent_removal"); got {
		t.Errorf("expected Update state to reflect UEM's actual (false) value, got %v", got)
	}
}

func TestUpdate_RestrictionBackstop_UnsetFlags_NoDiagnostic(t *testing.T) {
	restriction := &tf.PurchasedAppAssignmentRestrictionModel{
		RemoveOnUnenroll: types.BoolValue(false),
		PreventRemoval:   types.BoolNull(),
	}
	svc := &transformAssignmentService{}
	resp := runUpdateWithService(t, restriction, svc)

	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect any diagnostics error: %v", resp.Diagnostics)
	}
}
