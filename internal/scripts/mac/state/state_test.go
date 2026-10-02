package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
)

// categoriesListValue builds a known categories types.List from string
// values, for tests that need to construct a MacCatalogDisplayModel by hand.
func categoriesListValue(t *testing.T, values ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elems = append(elems, types.StringValue(v))
	}
	l, diags := types.ListValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatalf("categoriesListValue: %v", diags)
	}
	return l
}

// categoriesStrings unpacks a known categories types.List into plain strings,
// for asserting on its contents.
func categoriesStrings(t *testing.T, l types.List) []string {
	t.Helper()
	elems := l.Elements()
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		s, ok := e.(types.String)
		if !ok {
			t.Fatalf("categoriesStrings: element has unexpected type %T", e)
		}
		out = append(out, s.ValueString())
	}
	return out
}

func TestToCreateScriptV1_roundTripFields(t *testing.T) {
	timeout := int64(300)
	userInteraction := true
	allowed := true
	cat1 := 42

	model := &tf.MacScriptResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("Deploy app"),
		Description:           types.StringValue("Installs software"),
		Platform:              types.StringValue("APPLE_OSX"),
		ScriptType:            types.StringValue("BASH"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		PlatformArchitecture:  types.StringValue("ARM64"),
		ScriptData:            types.StringValue("ZWNobyBoZWxsbw=="),
		Timeout:               types.Int64Value(timeout),
		ScriptVariables: []tf.MacScriptVariableModel{
			{Name: types.StringValue("FOO"), Value: types.StringValue("bar")},
		},
		AllowedInCatalog: types.BoolValue(allowed),
		CatalogDisplay: &tf.MacCatalogDisplayModel{
			DisplayName:    types.StringValue("Deploy"),
			DisplayDesc:    types.StringValue("Run deploy script"),
			PreActionText:  types.StringValue("This script will run on your device."),
			PostActionText: types.StringValue("The script has finished."),
			ActionType:     types.StringValue("INSTALL"),
			CatalogIconURL: types.StringValue("https://example/icon.png"),
			Categories:     categoriesListValue(t, "42"),
		},
		UserInteraction: types.BoolValue(userInteraction),
	}

	api, diags := ToCreateScriptV1(model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api == nil {
		t.Fatal("expected non-nil API request")
	}

	if api.Name != "Deploy app" {
		t.Errorf("Name: got %q", api.Name)
	}
	if api.Platform != "APPLE_OSX" || api.ScriptType != "BASH" {
		t.Errorf("platform/script_type: got %q / %q", api.Platform, api.ScriptType)
	}
	if api.Timeout == nil || *api.Timeout != 300 {
		t.Errorf("Timeout: got %v", api.Timeout)
	}
	if len(api.ScriptVariables) != 1 || api.ScriptVariables[0].Name != "FOO" {
		t.Errorf("ScriptVariables: got %+v", api.ScriptVariables)
	}
	if !api.AllowedInCatalog {
		t.Errorf("AllowedInCatalog: got %v", api.AllowedInCatalog)
	}
	if api.CatalogDisplay == nil || len(api.CatalogDisplay.Categories) != 1 || *api.CatalogDisplay.Categories[0] != cat1 {
		t.Errorf("CatalogDisplay: got %+v", api.CatalogDisplay)
	}
	if api.CatalogDisplay.PreActionText != "This script will run on your device." {
		t.Errorf("PreActionText: got %q", api.CatalogDisplay.PreActionText)
	}
	if api.CatalogDisplay.PostActionText != "The script has finished." {
		t.Errorf("PostActionText: got %q", api.CatalogDisplay.PostActionText)
	}
	if api.UserInteraction == nil || !*api.UserInteraction {
		t.Errorf("UserInteraction: got %v", api.UserInteraction)
	}
}

func TestToCreateScriptV1_invalidCategory(t *testing.T) {
	model := &tf.MacScriptResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("x"),
		Platform:              types.StringValue("APPLE_OSX"),
		ScriptType:            types.StringValue("BASH"),
		ScriptData:            types.StringValue("eA=="),
		AllowedInCatalog:      types.BoolValue(false),
		CatalogDisplay: &tf.MacCatalogDisplayModel{
			PreActionText:  types.StringValue("before"),
			PostActionText: types.StringValue("after"),
			Categories:     categoriesListValue(t, "not-a-number"),
		},
	}

	_, diags := ToCreateScriptV1(model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid category")
	}
}

// TestScriptVariablesToAPI_PassesThroughEveryEntry covers B16 decision table
// row #164 (REMOVE — dead code cleanup): script_variables.name is
// schema-Required (internal/scripts/mac/schema.go), so a null/unknown name
// is unreachable via normal Terraform validation; scriptVariablesToAPI must
// no longer silently skip an entry on that basis.
func TestScriptVariablesToAPI_PassesThroughEveryEntry(t *testing.T) {
	vars := []tf.MacScriptVariableModel{
		{Name: types.StringValue("FOO"), Value: types.StringValue("bar")},
		{Name: types.StringNull(), Value: types.StringValue("orphaned-value")},
		{Name: types.StringUnknown(), Value: types.StringValue("also-orphaned")},
	}
	got := scriptVariablesToAPI(vars)
	if len(got) != 3 {
		t.Fatalf("expected all 3 entries to pass through, got %d: %+v", len(got), got)
	}
	if got[0].Name != "FOO" || got[0].Value != "bar" {
		t.Errorf("unexpected first entry: %+v", got[0])
	}
	if got[1].Value != "orphaned-value" {
		t.Errorf("expected null-name entry to still pass through, got %+v", got[1])
	}
	if got[2].Value != "also-orphaned" {
		t.Errorf("expected unknown-name entry to still pass through, got %+v", got[2])
	}
}

func TestToUpdateScriptV1_setsScriptUUID(t *testing.T) {
	model := &tf.MacScriptResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("updated"),
		Platform:              types.StringValue("APPLE_OSX"),
		ScriptType:            types.StringValue("BASH"),
		ScriptData:            types.StringValue("dXBkYXRlZA=="),
		AllowedInCatalog:      types.BoolValue(true),
	}

	const scriptUUID = "bafde89c-041e-1756-082b-933aaf16cad8"
	api, diags := ToUpdateScriptV1(model, scriptUUID)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.ScriptUUID != scriptUUID {
		t.Errorf("ScriptUUID: got %q", api.ScriptUUID)
	}
	if api.OrganizationGroupUUID != model.OrganizationGroupUUID.ValueString() {
		t.Errorf("OrganizationGroupUUID: got %q", api.OrganizationGroupUUID)
	}
	if api.Name != "updated" {
		t.Errorf("Name: got %q", api.Name)
	}
}

func TestReadAPIIntoState_mapsScriptResource(t *testing.T) {
	timeout := 300
	allowed := false
	userInteraction := false
	cat := 7

	api := &sdk.ScriptResourceV1{
		ScriptUUID:           "bafde89c-041e-1756-082b-933aaf16cad8",
		Name:                 "Sample script 1",
		Description:          "Test script",
		Platform:             "APPLE_OSX",
		ScriptType:           "BASH",
		ExecutionContext:     "USER",
		PlatformArchitecture: &client.IntOrString{IsStr: true, StrVal: "X64"},
		AllowedInCatalog:     &allowed,
		UserInteraction:      &userInteraction,
		ScriptData:           "R2V0LURhdGUgfCBPdXQtRmlsZSBDOlx0ZXN0LnR4dA==",
		Timeout:              &timeout,
		CatalogDisplay: &sdk.CatalogDisplayV1{
			DisplayName:    "Sample Script",
			DisplayDesc:    "Test",
			PreActionText:  "Run this script?",
			PostActionText: "Script complete.",
			Categories:     []*int{&cat},
		},
		ScriptVariables: []sdk.ScriptVariablesV1{
			{Name: "ENV", Value: "prod"},
		},
	}

	var data tf.MacScriptResourceModel
	diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if data.ID.ValueString() != api.ScriptUUID {
		t.Errorf("ID: got %q", data.ID.ValueString())
	}
	if data.Name.ValueString() != api.Name {
		t.Errorf("Name: got %q", data.Name.ValueString())
	}
	if data.Timeout.ValueInt64() != 300 {
		t.Errorf("Timeout: got %d", data.Timeout.ValueInt64())
	}
	if data.CatalogDisplay == nil || data.CatalogDisplay.DisplayName.ValueString() != "Sample Script" {
		t.Errorf("CatalogDisplay: got %+v", data.CatalogDisplay)
	}
	if got := categoriesStrings(t, data.CatalogDisplay.Categories); len(got) != 1 || got[0] != "7" {
		t.Errorf("Categories: got %+v", got)
	}
	if len(data.ScriptVariables) != 1 || data.ScriptVariables[0].Name.ValueString() != "ENV" {
		t.Errorf("ScriptVariables: got %+v", data.ScriptVariables)
	}
}

// B35: UEM returns "categories": [] for a real, in-catalog script with no
// categories assigned (live-confirmed, root-cause report). That must read
// back as a known, non-null empty list -- not null, which would trip the
// (now-removed) Required schema constraint on catalog_display.categories.
//
// Revert check: restoring the len(api.Categories) > 0 guard in
// catalogDisplayFromAPI makes this test fail.
func TestReadAPIIntoState_categoriesEmptyList(t *testing.T) {
	api := &sdk.ScriptResourceV1{
		ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8",
		CatalogDisplay: &sdk.CatalogDisplayV1{
			DisplayName:    "Sample Script",
			DisplayDesc:    "Test",
			PreActionText:  "Run this script?",
			PostActionText: "Script complete.",
			Categories:     []*int{}, // non-nil, zero-length: UEM's live shape for "no categories"
		},
	}

	var data tf.MacScriptResourceModel
	if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.CatalogDisplay == nil {
		t.Fatal("CatalogDisplay: got nil")
	}
	if data.CatalogDisplay.Categories.IsNull() {
		t.Fatal("Categories: got null, want a known empty list")
	}
	if data.CatalogDisplay.Categories.IsUnknown() {
		t.Fatal("Categories: got unknown, want a known empty list")
	}
	if len(data.CatalogDisplay.Categories.Elements()) != 0 {
		t.Errorf("Categories: got %+v, want empty", data.CatalogDisplay.Categories)
	}
}

// B35: a nil api.Categories (the field genuinely absent from UEM's response)
// still reads back as null -- this fix only stops collapsing a present,
// empty list; it does not manufacture one out of nothing.
func TestReadAPIIntoState_categoriesAbsentStaysNull(t *testing.T) {
	api := &sdk.ScriptResourceV1{
		ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8",
		CatalogDisplay: &sdk.CatalogDisplayV1{
			DisplayName:    "Sample Script",
			DisplayDesc:    "Test",
			PreActionText:  "Run this script?",
			PostActionText: "Script complete.",
			Categories:     nil,
		},
	}

	var data tf.MacScriptResourceModel
	if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.CatalogDisplay == nil {
		t.Fatal("CatalogDisplay: got nil")
	}
	if !data.CatalogDisplay.Categories.IsNull() {
		t.Errorf("Categories: got %+v, want null", data.CatalogDisplay.Categories)
	}
}

// B35: a config that explicitly sets categories = [] sends a non-nil, empty
// slice through to the SDK request shape (categoryStringsToAPI no longer
// collapses an empty result to nil) -- see catalogDisplayToAPI's comment for
// why this still can't reach the wire as "[]" given the SDK's omitempty tag.
func TestCatalogDisplayToAPI_categoriesEmptyListIsNonNil(t *testing.T) {
	m := &tf.MacCatalogDisplayModel{
		DisplayName:    types.StringValue("Deploy"),
		DisplayDesc:    types.StringValue("desc"),
		PreActionText:  types.StringValue("pre"),
		PostActionText: types.StringValue("post"),
		ActionType:     types.StringValue("INSTALL"),
		CatalogIconURL: types.StringValue("https://example/icon.png"),
		Categories:     categoriesListValue(t), // config explicitly set categories = []
	}

	api, diags := catalogDisplayToAPI(m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.Categories == nil {
		t.Error("Categories: got nil, want a non-nil empty slice (config set categories = [])")
	}
	if len(api.Categories) != 0 {
		t.Errorf("Categories: got %+v, want empty", api.Categories)
	}
}

// B35: a config that omits categories entirely (nil) sends nothing.
func TestCatalogDisplayToAPI_categoriesNilIsOmitted(t *testing.T) {
	m := &tf.MacCatalogDisplayModel{
		DisplayName:    types.StringValue("Deploy"),
		DisplayDesc:    types.StringValue("desc"),
		PreActionText:  types.StringValue("pre"),
		PostActionText: types.StringValue("post"),
		ActionType:     types.StringValue("INSTALL"),
		CatalogIconURL: types.StringValue("https://example/icon.png"),
		Categories:     types.ListNull(types.StringType),
	}

	api, diags := catalogDisplayToAPI(m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.Categories != nil {
		t.Errorf("Categories: got %+v, want nil (omitted)", api.Categories)
	}
}

// uix: a plan whose categories is Unknown (Computed, config omitted it, no
// prior state to carry forward -- e.g. a real Create plan) also sends
// nothing; only a known list is converted.
func TestCatalogDisplayToAPI_categoriesUnknownIsOmitted(t *testing.T) {
	m := &tf.MacCatalogDisplayModel{
		DisplayName:    types.StringValue("Deploy"),
		DisplayDesc:    types.StringValue("desc"),
		PreActionText:  types.StringValue("pre"),
		PostActionText: types.StringValue("post"),
		ActionType:     types.StringValue("INSTALL"),
		CatalogIconURL: types.StringValue("https://example/icon.png"),
		Categories:     types.ListUnknown(types.StringType),
	}

	api, diags := catalogDisplayToAPI(m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.Categories != nil {
		t.Errorf("Categories: got %+v, want nil (omitted)", api.Categories)
	}
}

// v1n.4.1: organization_group_uuid comes from the API on every Read. An
// import-time Read (null prior state) must populate it, and a Read must never
// keep a stale prior value the API no longer reports.
func TestReadAPIIntoState_organizationGroupUUIDFromAPI(t *testing.T) {
	const ogUUID = "e91c83f7-97b0-825a-dea5-b30cfda2ce05"

	var imported tf.MacScriptResourceModel
	api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", OrganizationGroupUUID: ogUUID}
	if diags := ReadAPIIntoState(context.Background(), &imported, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := imported.OrganizationGroupUUID.ValueString(); got != ogUUID {
		t.Errorf("import Read: OrganizationGroupUUID = %q, want %q", got, ogUUID)
	}

	stale := tf.MacScriptResourceModel{OrganizationGroupUUID: types.StringValue("stale-og-uuid")}
	api.OrganizationGroupUUID = ""
	if diags := ReadAPIIntoState(context.Background(), &stale, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := stale.OrganizationGroupUUID.ValueString(); got != "" {
		t.Errorf("Read kept stale prior OrganizationGroupUUID %q; want the API value %q", got, "")
	}
}

func TestReadAPIIntoState_roundTrip(t *testing.T) {
	original := &tf.MacScriptResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("round-trip"),
		Description:           types.StringValue("desc"),
		Platform:              types.StringValue("APPLE_OSX"),
		ScriptType:            types.StringValue("ZSH"),
		ExecutionContext:      types.StringValue("USER"),
		PlatformArchitecture:  types.StringValue("X64"),
		ScriptData:            types.StringValue("c2NyaXB0"),
		Timeout:               types.Int64Value(120),
		ScriptVariables: []tf.MacScriptVariableModel{
			{Name: types.StringValue("A"), Value: types.StringValue("1")},
		},
		AllowedInCatalog: types.BoolValue(true),
		CatalogDisplay: &tf.MacCatalogDisplayModel{
			DisplayName:    types.StringValue("RT"),
			DisplayDesc:    types.StringValue("round trip"),
			ActionType:     types.StringValue("INSTALL"),
			CatalogIconURL: types.StringValue("https://example/icon.png"),
			PreActionText:  types.StringValue("Proceed?"),
			PostActionText: types.StringValue("Done."),
			Categories:     categoriesListValue(t, "3"),
		},
		UserInteraction: types.BoolValue(false),
	}

	createReq, diags := ToCreateScriptV1(original)
	if diags.HasError() {
		t.Fatalf("ToCreateScriptV1: %v", diags)
	}

	api := &sdk.ScriptResourceV1{
		ScriptUUID:           "05d17100-b346-c29d-6760-a0fdedcf8623",
		Name:                 createReq.Name,
		Description:          createReq.Description,
		Platform:             createReq.Platform,
		ScriptType:           createReq.ScriptType,
		ExecutionContext:     createReq.ExecutionContext,
		PlatformArchitecture: createReq.PlatformArchitecture,
		ScriptData:           createReq.ScriptData,
		Timeout:              createReq.Timeout,
		ScriptVariables:      createReq.ScriptVariables,
		AllowedInCatalog:     &createReq.AllowedInCatalog,
		CatalogDisplay:       createReq.CatalogDisplay,
		UserInteraction:      createReq.UserInteraction,
	}

	var roundTripped tf.MacScriptResourceModel
	diags = ReadAPIIntoState(context.Background(), &roundTripped, api, api.ScriptUUID)
	if diags.HasError() {
		t.Fatalf("ReadAPIIntoState: %v", diags)
	}

	if roundTripped.Name.ValueString() != original.Name.ValueString() {
		t.Errorf("Name mismatch: %q vs %q", roundTripped.Name.ValueString(), original.Name.ValueString())
	}
	if roundTripped.ScriptData.ValueString() != original.ScriptData.ValueString() {
		t.Errorf("ScriptData mismatch")
	}
	if len(roundTripped.ScriptVariables) != len(original.ScriptVariables) {
		t.Errorf("ScriptVariables length mismatch")
	}
	if roundTripped.CatalogDisplay == nil || roundTripped.CatalogDisplay.DisplayName.ValueString() != "RT" {
		t.Errorf("CatalogDisplay mismatch: %+v", roundTripped.CatalogDisplay)
	}
}

// am3 (B16 (b)-row #161 removal): Read no longer disambiguates "" using the
// prior value -- that rule was transplanted from uem_profile's
// setDescriptionFromAPI with no evidence for this endpoint. Read now stores
// whatever the API returned, faithfully, regardless of what the prior value
// was. A script created/updated with description unset reads back as ""
// (UEM's wire model cannot represent "no description" any other way), not
// null. B39 made the schema attribute Optional+Computed (see schema.go) so
// that no-longer-collapsed "" no longer perpetually diffs against an omitted
// description in config -- an omitted config now plans Unknown, not null,
// and UEM's "" resolves it without an "inconsistent result after apply".
func TestReadAPIIntoState_descriptionPassthrough(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prior types.String
		api   string
		want  types.String
	}{
		{"null prior, empty API -> empty (pass-through)", types.StringNull(), "", types.StringValue("")},
		{"empty prior, empty API -> empty", types.StringValue(""), "", types.StringValue("")},
		{"null prior, non-empty API -> API value", types.StringNull(), "x", types.StringValue("x")},
		{"non-empty prior, empty API -> empty (pass-through, prior ignored)", types.StringValue("old"), "", types.StringValue("")},
		{"empty prior, non-empty API -> API value", types.StringValue(""), "x", types.StringValue("x")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tf.MacScriptResourceModel{Description: tc.prior}
			api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", Description: tc.api}
			if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !data.Description.Equal(tc.want) {
				t.Errorf("Description = %v, want %v", data.Description, tc.want)
			}
		})
	}
}

// B34: user_interaction is Optional+Computed, and UEM's response is read back
// verbatim regardless of any prior value -- including a server-default false,
// which used to collapse to null whenever the prior was itself null. The
// no-diff-for-an-omitted-config behaviour that collapse used to provide is
// now the schema's job (Optional+Computed + UseStateForUnknown; see
// TestUserInteractionSchema_OptionalComputedUseStateForUnknown and
// TestUserInteractionPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange).
func TestReadAPIIntoState_userInteractionServerDefault(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	for _, tc := range []struct {
		name  string
		prior types.Bool
		api   *bool
		want  types.Bool
	}{
		{"null prior, server default false -> false (verbatim)", types.BoolNull(), boolPtr(false), types.BoolValue(false)},
		{"explicit false prior, server false -> false", types.BoolValue(false), boolPtr(false), types.BoolValue(false)},
		{"null prior, non-default true -> true", types.BoolNull(), boolPtr(true), types.BoolValue(true)},
		{"true prior, server default false -> false (verbatim, drift)", types.BoolValue(true), boolPtr(false), types.BoolValue(false)},
		{"true prior, server true -> true", types.BoolValue(true), boolPtr(true), types.BoolValue(true)},
		{"nil API -> null", types.BoolValue(true), nil, types.BoolNull()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tf.MacScriptResourceModel{UserInteraction: tc.prior}
			api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", UserInteraction: tc.api}
			if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !data.UserInteraction.Equal(tc.want) {
				t.Errorf("UserInteraction = %v, want %v", data.UserInteraction, tc.want)
			}
		})
	}
}

// B34 (the bug): an import-time Read starts from an all-null model
// (ImportState only ever sets id, internal/scripts/mac/crud.go), so the prior
// being null on import carries no meaning -- unlike a null prior from a
// config that genuinely omitted the attribute. Reading UEM's server-default
// false as null here (the pre-fix behaviour) would discard a real, present
// value; import must show exactly what UEM returned, false included.
//
// Revert check: reintroducing the removed prior-based collapse in
// userInteractionFromAPI (i.e. mapping a false API value to null whenever
// data.UserInteraction is null, as it always is here) makes this test fail.
func TestReadAPIIntoState_userInteractionImport(t *testing.T) {
	f := false
	var imported tf.MacScriptResourceModel
	api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", UserInteraction: &f}
	if diags := ReadAPIIntoState(context.Background(), &imported, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if imported.UserInteraction.IsNull() || imported.UserInteraction.ValueBool() != false {
		t.Errorf("import Read: UserInteraction = %v, want false (verbatim, not null)", imported.UserInteraction)
	}
}

// B34: timeout is Optional+Computed, and UEM's response is read back
// verbatim regardless of any prior value -- including a server-default 30,
// which used to collapse to null whenever the prior was itself null. The
// no-diff-for-an-omitted-config behaviour that collapse used to provide is
// now the schema's job (Optional+Computed + UseStateForUnknown; see
// TestTimeoutSchema_OptionalComputedUseStateForUnknown and
// TestTimeoutPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange).
func TestReadAPIIntoState_timeoutServerDefault(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	for _, tc := range []struct {
		name  string
		prior types.Int64
		api   *int
		want  types.Int64
	}{
		{"null prior, server default 30 -> 30 (verbatim)", types.Int64Null(), intPtr(30), types.Int64Value(30)},
		{"explicit 30 prior, server 30 -> 30", types.Int64Value(30), intPtr(30), types.Int64Value(30)},
		{"null prior, non-default 45 -> 45", types.Int64Null(), intPtr(45), types.Int64Value(45)},
		{"60 prior, server default 30 -> 30 (verbatim, drift)", types.Int64Value(60), intPtr(30), types.Int64Value(30)},
		{"60 prior, server 60 -> 60", types.Int64Value(60), intPtr(60), types.Int64Value(60)},
		{"nil API -> null", types.Int64Value(60), nil, types.Int64Null()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tf.MacScriptResourceModel{Timeout: tc.prior}
			api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", Timeout: tc.api}
			if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !data.Timeout.Equal(tc.want) {
				t.Errorf("Timeout = %v, want %v", data.Timeout, tc.want)
			}
		})
	}
}

// B34 (the bug): an import-time Read starts from an all-null model
// (ImportState only ever sets id, internal/scripts/mac/crud.go), so the prior
// being null on import carries no meaning -- unlike a null prior from a
// config that genuinely omitted the attribute. Reading UEM's server-default
// 30 as null here (the pre-fix behaviour) would discard a real, present
// value; import must show exactly what UEM returned, 30 included. This is
// exactly the live shape from the root-cause report: every script sampled on
// that tenant currently sits at timeout=30 explicitly.
//
// Revert check: reintroducing the removed prior-based collapse in
// timeoutFromAPI (i.e. mapping a 30 API value to null whenever data.Timeout
// is null, as it always is here) makes this test fail.
func TestReadAPIIntoState_timeoutImport(t *testing.T) {
	d := 30
	var imported tf.MacScriptResourceModel
	api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", Timeout: &d}
	if diags := ReadAPIIntoState(context.Background(), &imported, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if imported.Timeout.IsNull() || imported.Timeout.ValueInt64() != 30 {
		t.Errorf("import Read: Timeout = %v, want 30 (verbatim, not null)", imported.Timeout)
	}
}

// uix: platform_architecture is a QUIRK-11 IntOrString. UEM returns the int 0
// when unset, which must read as null (not "0"); the string form is taken
// as-is whatever the prior, so drift shows.
func TestReadAPIIntoState_platformArchitectureServerDefault(t *testing.T) {
	unset := &client.IntOrString{IsStr: false, IntVal: 0}
	for _, tc := range []struct {
		name  string
		prior types.String
		api   *client.IntOrString
		want  types.String
	}{
		{"null prior, server int 0 -> null", types.StringNull(), unset, types.StringNull()},
		{"null prior, string LEGACY -> LEGACY", types.StringNull(), &client.IntOrString{IsStr: true, StrVal: "LEGACY"}, types.StringValue("LEGACY")},
		{"explicit prior, same string -> kept", types.StringValue("ARM64"), &client.IntOrString{IsStr: true, StrVal: "ARM64"}, types.StringValue("ARM64")},
		{"explicit prior, different string -> API value (drift)", types.StringValue("ARM64"), &client.IntOrString{IsStr: true, StrVal: "UNKNOWN"}, types.StringValue("UNKNOWN")},
		{"explicit prior, server int 0 -> null (drift)", types.StringValue("ARM64"), unset, types.StringNull()},
		{"nil API -> null", types.StringValue("ARM64"), nil, types.StringNull()},
		{"null prior, non-zero int 2 -> \"2\"", types.StringNull(), &client.IntOrString{IsStr: false, IntVal: 2}, types.StringValue("2")},
		{"explicit prior, non-zero int 2 -> \"2\" (drift)", types.StringValue("ARM64"), &client.IntOrString{IsStr: false, IntVal: 2}, types.StringValue("2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tf.MacScriptResourceModel{PlatformArchitecture: tc.prior}
			api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", PlatformArchitecture: tc.api}
			if diags := ReadAPIIntoState(context.Background(), &data, api, api.ScriptUUID); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !data.PlatformArchitecture.Equal(tc.want) {
				t.Errorf("PlatformArchitecture = %v, want %v", data.PlatformArchitecture, tc.want)
			}
		})
	}
}

// uix: an import-time Read (all-null model) reads the unset int 0 as null.
func TestReadAPIIntoState_platformArchitectureImport(t *testing.T) {
	var imported tf.MacScriptResourceModel
	api := &sdk.ScriptResourceV1{ScriptUUID: "bafde89c-041e-1756-082b-933aaf16cad8", PlatformArchitecture: &client.IntOrString{}}
	if diags := ReadAPIIntoState(context.Background(), &imported, api, api.ScriptUUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !imported.PlatformArchitecture.IsNull() {
		t.Errorf("import Read: PlatformArchitecture = %v, want null", imported.PlatformArchitecture)
	}
}

// B36: faithful pass-through, exactly like Create -- a null or unknown plan
// value omits Timeout from the update request instead of substituting an
// uncited default (30); a known value is sent as-is.
func TestToUpdateScriptV1_timeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan types.Int64
		want *int
	}{
		{"null -> omitted", types.Int64Null(), nil},
		{"unknown -> omitted", types.Int64Unknown(), nil},
		{"explicit 45 -> 45", types.Int64Value(45), intPtr(45)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &tf.MacScriptResourceModel{
				OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
				Name:                  types.StringValue("updated"),
				Platform:              types.StringValue("APPLE_OSX"),
				ScriptType:            types.StringValue("BASH"),
				ScriptData:            types.StringValue("dXBkYXRlZA=="),
				Timeout:               tc.plan,
			}
			api, diags := ToUpdateScriptV1(model, "bafde89c-041e-1756-082b-933aaf16cad8")
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if tc.want == nil {
				if api.Timeout != nil {
					t.Errorf("Timeout = %v, want omitted (nil)", *api.Timeout)
				}
			} else {
				if api.Timeout == nil || *api.Timeout != *tc.want {
					t.Errorf("Timeout = %v, want %d", api.Timeout, *tc.want)
				}
			}
			// A PUT that omits user_interaction already resets it to false,
			// so a null user_interaction stays omitted (unlike timeout).
			if api.UserInteraction != nil {
				t.Errorf("UserInteraction = %v, want omitted (nil)", *api.UserInteraction)
			}
		})
	}
}

func intPtr(v int) *int { return &v }
