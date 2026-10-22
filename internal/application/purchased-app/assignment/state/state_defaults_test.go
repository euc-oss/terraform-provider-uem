package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// --- excluded_smart_groups null-if-default (internal-task item 2) ---

func TestReadAPIIntoState_ExcludedSmartGroups_PriorNullServerEmpty_KeepsNull(t *testing.T) {
	ctx := t.Context()
	data := &tf.PurchasedAppAssignmentRuleModel{ExcludedSmartGroups: types.ListNull(types.StringType)}
	if d := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{}); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	if !data.ExcludedSmartGroups.IsNull() {
		t.Fatalf("expected null excluded_smart_groups, got %v", data.ExcludedSmartGroups)
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_NoPriorServerEmpty_KeepsNull(t *testing.T) {
	ctx := t.Context()
	data := &tf.PurchasedAppAssignmentRuleModel{} // import path: zero value
	if d := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{}); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	if !data.ExcludedSmartGroups.IsNull() {
		t.Fatalf("expected null excluded_smart_groups on import of empty server value, got %v", data.ExcludedSmartGroups)
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_PriorNonNullServerEmpty_TakesEmptyList(t *testing.T) {
	ctx := t.Context()
	emptyList, diags := types.ListValue(types.StringType, nil)
	if diags.HasError() {
		t.Fatalf("building empty list: %v", diags)
	}
	data := &tf.PurchasedAppAssignmentRuleModel{ExcludedSmartGroups: emptyList}
	if d := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{}); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	if data.ExcludedSmartGroups.IsNull() {
		t.Fatal("expected an explicit empty list, got null")
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_NonEmptyServer_AlwaysAsIs(t *testing.T) {
	ctx := t.Context()
	data := &tf.PurchasedAppAssignmentRuleModel{ExcludedSmartGroups: types.ListNull(types.StringType)}
	api := &sdk.AppAssignmentRuleV2Model{ExcludedSmartGroups: []string{"SG-1"}}
	if d := ReadAPIIntoState(ctx, data, api); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	var got []types.String
	if d := data.ExcludedSmartGroups.ElementsAs(ctx, &got, false); d.HasError() {
		t.Fatalf("decode: %v", d)
	}
	if len(got) != 1 || got[0].ValueString() != "sg-1" {
		t.Fatalf("expected server drift to surface, got %#v", got)
	}
}

// --- distribution.app_delivery_method null-if-default (internal-task item 2) ---

func TestPurchasedDistributionFromAPI_AppDeliveryMethod_PriorNullServerOnDemand_KeepsNull(t *testing.T) {
	got := appDeliveryMethodFromAPI("on_demand", types.StringNull())
	if !got.IsNull() {
		t.Fatalf("expected null app_delivery_method, got %v", got)
	}
}

func TestPurchasedDistributionFromAPI_AppDeliveryMethod_PriorNonNullServerOnDemand_TakesServer(t *testing.T) {
	got := appDeliveryMethodFromAPI("on_demand", types.StringValue("SEAMLESS"))
	if got.ValueString() != "ON_DEMAND" {
		t.Fatalf("expected server value ON_DEMAND to surface, got %v", got)
	}
}

func TestPurchasedDistributionFromAPI_AppDeliveryMethod_NonDefaultValue_AlwaysAsIs(t *testing.T) {
	for name, prior := range map[string]types.String{
		"null prior": types.StringNull(),
		"set prior":  types.StringValue("SEAMLESS"),
	} {
		t.Run(name, func(t *testing.T) {
			got := appDeliveryMethodFromAPI("seamless", prior)
			if got.ValueString() != "SEAMLESS" {
				t.Fatalf("expected non-default value SEAMLESS to surface as-is, got %v", got)
			}
		})
	}
}

// --- restriction flags with no schema Default: null-if-default (internal-task item 2) ---

func TestNullIfDefaultFalseBool_PriorNullServerFalse_KeepsNull(t *testing.T) {
	f := false
	got := nullIfDefaultFalseBool(&f, types.BoolNull())
	if !got.IsNull() {
		t.Fatalf("expected null, got %v", got)
	}
}

func TestNullIfDefaultFalseBool_PriorNonNullServerFalse_TakesServer(t *testing.T) {
	f := false
	got := nullIfDefaultFalseBool(&f, types.BoolValue(true))
	if got.IsNull() || got.ValueBool() {
		t.Fatalf("expected explicit false to surface, got %v", got)
	}
}

func TestNullIfDefaultFalseBool_NonDefaultTrue_AlwaysAsIs(t *testing.T) {
	tru := true
	for name, prior := range map[string]types.Bool{
		"null prior": types.BoolNull(),
		"set prior":  types.BoolValue(false),
	} {
		t.Run(name, func(t *testing.T) {
			got := nullIfDefaultFalseBool(&tru, prior)
			if got.IsNull() || !got.ValueBool() {
				t.Fatalf("expected true to surface as-is, got %v", got)
			}
		})
	}
}

func TestNullIfDefaultFalseBool_ServerNil_ReturnsNull(t *testing.T) {
	got := nullIfDefaultFalseBool(nil, types.BoolValue(true))
	if !got.IsNull() {
		t.Fatalf("expected null when the server omits the field, got %v", got)
	}
}

// TestReadAPIIntoState_RestrictionFields_PerFieldNullIfDefault exercises the
// per-field collapse inside a non-null restriction block (the block-level
// all-default collapse is covered by state_restriction_prior_test.go).
func TestReadAPIIntoState_RestrictionFields_PerFieldNullIfDefault(t *testing.T) {
	tr := true
	f := false
	// A non-default RemoveOnUnenroll keeps the restriction block non-nil, so
	// the per-field collapse on the three no-Default flags is observable.
	api := apiRule("", &sdk.AppAssignmentRestrictionV1ModelV2{
		RemoveOnUnenroll:         &tr,
		PreventRemoval:           &f,
		PreventApplicationBackup: &f,
		MakeAppMdmManaged:        &f,
		ManagedAccess:            &f,
		DesiredStateManagement:   &f,
	})
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), nil), api)
	if got.Restriction == nil {
		t.Fatal("expected a non-nil restriction block")
	}
	if !got.Restriction.PreventApplicationBackup.IsNull() {
		t.Fatalf("expected prevent_application_backup to collapse to null, got %v", got.Restriction.PreventApplicationBackup)
	}
	if !got.Restriction.MakeAppMdmManaged.IsNull() {
		t.Fatalf("expected make_app_mdm_managed to collapse to null, got %v", got.Restriction.MakeAppMdmManaged)
	}
	if !got.Restriction.DesiredStateManagement.IsNull() {
		t.Fatalf("expected desired_state_management to collapse to null, got %v", got.Restriction.DesiredStateManagement)
	}
	// remove_on_unenroll/prevent_removal/managed_access keep KeepStateBool
	// (Default(false) unchanged): explicit server values surface as-is.
	if got.Restriction.RemoveOnUnenroll.IsNull() || !got.Restriction.RemoveOnUnenroll.ValueBool() {
		t.Fatalf("expected remove_on_unenroll=true, got %v", got.Restriction.RemoveOnUnenroll)
	}
	if got.Restriction.PreventRemoval.IsNull() || got.Restriction.PreventRemoval.ValueBool() {
		t.Fatalf("expected prevent_removal=false (explicit), got %v", got.Restriction.PreventRemoval)
	}
}

// --- is_dynamic_template_saved null-if-default (internal-task item 2) ---

func TestReadAPIIntoState_IsDynamicTemplateSaved_PriorNullServerFalse_KeepsNull(t *testing.T) {
	got := readSingleAssignment(t, priorRuleWithDynamicTemplate(t, types.BoolNull()), apiRuleWithDynamicTemplate(false))
	if !got.IsDynamicTemplateSaved.IsNull() {
		t.Fatalf("expected null, got %v", got.IsDynamicTemplateSaved)
	}
}

func TestReadAPIIntoState_IsDynamicTemplateSaved_PriorNonNullServerFalse_TakesServer(t *testing.T) {
	got := readSingleAssignment(t, priorRuleWithDynamicTemplate(t, types.BoolValue(true)), apiRuleWithDynamicTemplate(false))
	if got.IsDynamicTemplateSaved.IsNull() || got.IsDynamicTemplateSaved.ValueBool() {
		t.Fatalf("expected explicit false to surface, got %v", got.IsDynamicTemplateSaved)
	}
}

func TestReadAPIIntoState_IsDynamicTemplateSaved_ServerTrue_AlwaysAsIs(t *testing.T) {
	got := readSingleAssignment(t, priorRuleWithDynamicTemplate(t, types.BoolNull()), apiRuleWithDynamicTemplate(true))
	if got.IsDynamicTemplateSaved.IsNull() || !got.IsDynamicTemplateSaved.ValueBool() {
		t.Fatalf("expected true to surface as-is, got %v", got.IsDynamicTemplateSaved)
	}
}

func priorRuleWithDynamicTemplate(t *testing.T, saved types.Bool) *tf.PurchasedAppAssignmentRuleModel {
	t.Helper()
	rule := priorRule(t, types.StringNull(), nil)
	var assigns []tf.PurchasedAppAssignmentModel
	if diags := rule.Assignments.ElementsAs(t.Context(), &assigns, false); diags.HasError() {
		t.Fatalf("decode prior: %v", diags)
	}
	assigns[0].IsDynamicTemplateSaved = saved
	list, diags := types.ListValueFrom(t.Context(), assignmentElemType, assigns)
	if diags.HasError() {
		t.Fatalf("rebuild prior: %v", diags)
	}
	rule.Assignments = list
	return rule
}

func apiRuleWithDynamicTemplate(saved bool) *sdk.AppAssignmentRuleV2Model {
	api := apiRule("", nil)
	api.Assignments[0].IsDynamicTemplateSaved = &saved
	return api
}

// --- distribution.smart_groups null-if-empty (u3a2 live regression) ---

// UEM always echoes [] for a purchased app's distribution.smart_groups. With
// the removal modifier planning null when it is unset, the readback must stay
// null, or create fails with "inconsistent result after apply" (seen live).
func TestSmartGroupsFromAPI_NullPriorEmptyServer_StaysNull(t *testing.T) {
	cases := []struct {
		name  string
		prior *tf.PurchasedAppAssignmentDistributionModel
	}{
		{"no prior object (import)", nil},
		{"prior null (create/removal readback)", &tf.PurchasedAppAssignmentDistributionModel{SmartGroups: types.ListNull(types.StringType)}},
	}
	for _, tc := range cases {
		if got := smartGroupsFromAPI(nil, tc.prior); !got.IsNull() {
			t.Errorf("%s: smart_groups = %s, want null", tc.name, got)
		}
		if got := smartGroupsFromAPI([]string{}, tc.prior); !got.IsNull() {
			t.Errorf("%s ([] from server): smart_groups = %s, want null", tc.name, got)
		}
	}
}

func TestSmartGroupsFromAPI_NonNullPriorOrValues_TakenAsIs(t *testing.T) {
	prior := &tf.PurchasedAppAssignmentDistributionModel{SmartGroups: types.ListValueMust(types.StringType, nil)}
	if got := smartGroupsFromAPI(nil, prior); got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("explicit [] prior: smart_groups = %s, want []", got)
	}
	if got := smartGroupsFromAPI([]string{"ABC"}, nil); got.IsNull() || len(got.Elements()) != 1 {
		t.Errorf("server value: smart_groups = %s, want 1 element", got)
	}
}
