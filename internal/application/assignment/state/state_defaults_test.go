package state

import (
	"context"
	"testing"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	client "github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
)

// --- excluded_smart_groups null-if-default (internal-task item 2) ---

func TestReadAPIIntoState_ExcludedSmartGroups_PriorNullServerEmpty_KeepsNull(t *testing.T) {
	ctx := context.Background()
	data := &tf.AppAssignmentRuleModel{ExcludedSmartGroups: types.ListNull(types.StringType)}
	diags := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !data.ExcludedSmartGroups.IsNull() {
		t.Fatalf("expected null excluded_smart_groups, got %v", data.ExcludedSmartGroups)
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_NoPriorServerEmpty_KeepsNull(t *testing.T) {
	// Import path: zero-value model has a null ExcludedSmartGroups.
	ctx := context.Background()
	data := &tf.AppAssignmentRuleModel{}
	diags := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !data.ExcludedSmartGroups.IsNull() {
		t.Fatalf("expected null excluded_smart_groups on import of empty server value, got %v", data.ExcludedSmartGroups)
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_PriorNonNullServerEmpty_TakesEmptyList(t *testing.T) {
	ctx := context.Background()
	emptyList, diags := types.ListValue(types.StringType, nil)
	if diags.HasError() {
		t.Fatalf("building empty list: %v", diags)
	}
	data := &tf.AppAssignmentRuleModel{ExcludedSmartGroups: emptyList}
	if d := ReadAPIIntoState(ctx, data, &sdk.AppAssignmentRuleV2Model{}); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	if data.ExcludedSmartGroups.IsNull() {
		t.Fatal("expected an explicit empty list, got null")
	}
	if len(data.ExcludedSmartGroups.Elements()) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(data.ExcludedSmartGroups.Elements()))
	}
}

func TestReadAPIIntoState_ExcludedSmartGroups_NonEmptyServer_AlwaysAsIs(t *testing.T) {
	ctx := context.Background()
	data := &tf.AppAssignmentRuleModel{ExcludedSmartGroups: types.ListNull(types.StringType)}
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

func TestDistributionFromAPI_AppDeliveryMethod_PriorNullServerOnDemand_KeepsNull(t *testing.T) {
	got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{
		Name: "n", AppDeliveryMethod: "on_demand",
	}, nil)
	if !got.AppDeliveryMethod.IsNull() {
		t.Fatalf("expected null app_delivery_method, got %v", got.AppDeliveryMethod)
	}
}

func TestDistributionFromAPI_AppDeliveryMethod_PriorNonNullServerOnDemand_TakesServer(t *testing.T) {
	prior := &tf.AppAssignmentDistributionModel{AppDeliveryMethod: types.StringValue("AUTO")}
	got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{
		Name: "n", AppDeliveryMethod: "on_demand",
	}, prior)
	if got.AppDeliveryMethod.ValueString() != "ON_DEMAND" {
		t.Fatalf("expected server value ON_DEMAND to surface, got %v", got.AppDeliveryMethod)
	}
}

func TestDistributionFromAPI_AppDeliveryMethod_NonDefaultValue_AlwaysAsIs(t *testing.T) {
	for name, prior := range map[string]*tf.AppAssignmentDistributionModel{
		"nil prior":  nil,
		"null prior": {AppDeliveryMethod: types.StringNull()},
	} {
		t.Run(name, func(t *testing.T) {
			got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{
				Name: "n", AppDeliveryMethod: "auto",
			}, prior)
			if got.AppDeliveryMethod.ValueString() != "AUTO" {
				t.Fatalf("expected non-default value AUTO to surface as-is, got %v", got.AppDeliveryMethod)
			}
		})
	}
}

// --- distribution.effective_date rule (D) (internal-task item 3) ---

func TestEffectiveDateFromAPI_NoPriorObject_ImportCapturesServerDate(t *testing.T) {
	when := mustParseTime(t)
	got := effectiveDateFromAPI(client.NewUEMTime(when), nil)
	if got.IsNull() {
		t.Fatal("expected the server's date to be captured on import, got null")
	}
	if got.ValueString() != "2026-04-23T00:00:00Z" {
		t.Fatalf("unexpected captured date: %v", got)
	}
}

func TestEffectiveDateFromAPI_NoPriorObject_ServerZero_KeepsNull(t *testing.T) {
	got := effectiveDateFromAPI(client.UEMTime{}, nil)
	if !got.IsNull() {
		t.Fatalf("expected null for a zero server date on import, got %v", got)
	}
}

func TestEffectiveDateFromAPI_PriorObjectCreateReadback_NullDate_KeepsNull(t *testing.T) {
	// Create readback: prior is the just-applied plan, which never set a date.
	when := mustParseTime(t)
	prior := &tf.AppAssignmentDistributionModel{EffectiveDate: types.StringNull()}
	got := effectiveDateFromAPI(client.NewUEMTime(when), prior)
	if !got.IsNull() {
		t.Fatalf("expected null even though the server stamped a date, got %v", got)
	}
}

func TestEffectiveDateFromAPI_PriorObjectRemovalReadback_NullDate_KeepsNull(t *testing.T) {
	// Removal readback: prior is last-known state with effective_date already
	// cleared by the nullWhenConfigNullString modifier.
	when := mustParseTime(t)
	prior := &tf.AppAssignmentDistributionModel{
		AppDeliveryMethod: types.StringValue("AUTO"),
		EffectiveDate:     types.StringNull(),
	}
	got := effectiveDateFromAPI(client.NewUEMTime(when), prior)
	if !got.IsNull() {
		t.Fatalf("expected null even though the server stamped a date, got %v", got)
	}
}

func TestEffectiveDateFromAPI_PriorObjectUserSetDate_KeptAsIs(t *testing.T) {
	when := mustParseTime(t)
	prior := &tf.AppAssignmentDistributionModel{EffectiveDate: types.StringValue("2026-04-23T00:00:00Z")}
	got := effectiveDateFromAPI(client.NewUEMTime(when), prior)
	if got.IsNull() || got.ValueString() != "2026-04-23T00:00:00Z" {
		t.Fatalf("expected the user-set date to surface as-is, got %v", got)
	}
}

// --- ReadAPIIntoState-level index pairing for rule (D) ---

func TestReadAPIIntoState_EffectiveDate_Import_NoPriorAssignments_CapturesDate(t *testing.T) {
	ctx := context.Background()
	data := &tf.AppAssignmentRuleModel{} // no prior assignments at all
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name:          "n",
				EffectiveDate: client.NewUEMTime(mustParseTime(t)),
			},
		}},
	}
	if d := ReadAPIIntoState(ctx, data, api); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	var assigns []tf.AppAssignmentModel
	if d := data.Assignments.ElementsAs(ctx, &assigns, false); d.HasError() {
		t.Fatalf("decode: %v", d)
	}
	if assigns[0].Distribution.EffectiveDate.IsNull() {
		t.Fatal("expected the import to capture the server's effective_date")
	}
}

func TestReadAPIIntoState_EffectiveDate_CreateReadback_PriorNullDate_KeepsNull(t *testing.T) {
	ctx := context.Background()
	prior := tf.AppAssignmentModel{Distribution: tf.AppAssignmentDistributionModel{
		Name:          types.StringValue("n"),
		SmartGroups:   types.ListNull(types.StringType),
		EffectiveDate: types.StringNull(),
	}}
	priorList, diags := types.ListValueFrom(ctx, assignmentElemType, []tf.AppAssignmentModel{prior})
	if diags.HasError() {
		t.Fatalf("building prior list: %v", diags)
	}
	data := &tf.AppAssignmentRuleModel{Assignments: priorList}
	api := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name:          "n",
				EffectiveDate: client.NewUEMTime(mustParseTime(t)),
			},
		}},
	}
	if d := ReadAPIIntoState(ctx, data, api); d.HasError() {
		t.Fatalf("unexpected diagnostics: %v", d)
	}
	var assigns []tf.AppAssignmentModel
	if d := data.Assignments.ElementsAs(ctx, &assigns, false); d.HasError() {
		t.Fatalf("decode: %v", d)
	}
	if !assigns[0].Distribution.EffectiveDate.IsNull() {
		t.Fatalf("expected null even though the server stamped a date, got %v", assigns[0].Distribution.EffectiveDate)
	}
}

func mustParseTime(t *testing.T) time.Time {
	t.Helper()
	const s = "2026-04-23T00:00:00Z"
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing time %q: %v", s, err)
	}
	return parsed
}

// --- distribution.description null-if-empty (u3a2 live import finding) ---

// description is Optional-only and UEM echoes "" for an unset description,
// so a null prior must stay null (an import of an assignment without a
// description planned "" -> null live).
func TestDistributionFromAPI_Description_NullPriorEmptyServer_StaysNull(t *testing.T) {
	for name, prior := range map[string]*tf.AppAssignmentDistributionModel{
		"no prior object (import)":             nil,
		"prior null (create/removal readback)": {Description: types.StringNull(), AppDeliveryMethod: types.StringNull(), EffectiveDate: types.StringNull()},
	} {
		got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{Description: ""}, prior)
		if !got.Description.IsNull() {
			t.Errorf("%s: description = %s, want null", name, got.Description)
		}
	}
}

func TestDistributionFromAPI_Description_ExplicitEmptyOrValue_TakenAsIs(t *testing.T) {
	prior := &tf.AppAssignmentDistributionModel{Description: types.StringValue(""), AppDeliveryMethod: types.StringNull(), EffectiveDate: types.StringNull()}
	if got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{Description: ""}, prior); got.Description.IsNull() || got.Description.ValueString() != "" {
		t.Errorf("explicit \"\" prior: description = %s, want \"\"", got.Description)
	}
	if got := distributionFromAPI(sdk.AppAssignmentDistributionV2Model{Description: "x"}, nil); got.Description.ValueString() != "x" {
		t.Errorf("server value: description = %s, want \"x\"", got.Description)
	}
}

// --- restriction block null-if-default (u3a2 live finding) ---

// UEM always echoes restriction {remove_on_unenroll: false}. An assignment
// created without a restriction block must read back nil, or apply fails with
// "inconsistent result after apply" (seen live).
func TestRestrictionFromAPI_NullIfDefault(t *testing.T) {
	f, tr := false, true
	cases := []struct {
		name     string
		api      *sdk.AppAssignmentRestrictionV1ModelV2
		prior    *tf.AppAssignmentRestrictionModel
		wantNil  bool
		wantNull bool
		wantVal  bool
	}{
		{"no prior block, server default false -> nil block", &sdk.AppAssignmentRestrictionV1ModelV2{RemoveOnUnenroll: &f}, nil, true, false, false},
		{"no prior block, server flag absent -> nil block", &sdk.AppAssignmentRestrictionV1ModelV2{}, nil, true, false, false},
		{"no prior block, server true (drift/import) -> captured", &sdk.AppAssignmentRestrictionV1ModelV2{RemoveOnUnenroll: &tr}, nil, false, false, true},
		{"prior block with null flag, server false -> flag null", &sdk.AppAssignmentRestrictionV1ModelV2{RemoveOnUnenroll: &f}, &tf.AppAssignmentRestrictionModel{RemoveOnUnenroll: types.BoolNull()}, false, true, false},
		{"prior block with explicit false, server false -> false kept", &sdk.AppAssignmentRestrictionV1ModelV2{RemoveOnUnenroll: &f}, &tf.AppAssignmentRestrictionModel{RemoveOnUnenroll: types.BoolValue(false)}, false, false, false},
		{"prior block with true, server true -> true", &sdk.AppAssignmentRestrictionV1ModelV2{RemoveOnUnenroll: &tr}, &tf.AppAssignmentRestrictionModel{RemoveOnUnenroll: types.BoolValue(true)}, false, false, true},
	}
	for _, tc := range cases {
		got := restrictionFromAPI(tc.api, tc.prior)
		if tc.wantNil {
			if got != nil {
				t.Errorf("%s: got %+v, want nil block", tc.name, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("%s: got nil block", tc.name)
			continue
		}
		if tc.wantNull {
			if !got.RemoveOnUnenroll.IsNull() {
				t.Errorf("%s: remove_on_unenroll = %s, want null", tc.name, got.RemoveOnUnenroll)
			}
			continue
		}
		if got.RemoveOnUnenroll.IsNull() || got.RemoveOnUnenroll.ValueBool() != tc.wantVal {
			t.Errorf("%s: remove_on_unenroll = %s, want %v", tc.name, got.RemoveOnUnenroll, tc.wantVal)
		}
	}
}
