package macscript

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
)

// uix: creating a uem_mac_script without an optional attribute failed apply
// with "Provider produced inconsistent result after apply" because the
// post-create readback mapped UEM's server default into state. These tests
// drive the real Create/Read paths through the fake service.
//
// platform_architecture is still Optional-only (not Computed), so a plan
// that omits it is genuinely Null (never Unknown) and must still read back
// as null after create -- its tests below assert that.
//
// timeout and user_interaction are Optional+Computed as of B34: a plan that
// omits either is Unknown, not Null (the framework core marks any Computed
// attribute Unknown when its config is null), and the post-create readback
// now takes UEM's server default back verbatim instead of mapping it to
// null -- an Unknown planned value may resolve to anything without
// "inconsistent result after apply"; only a Null one may not. Their tests
// below assert the verbatim readback.

const serverDefaultsTestScriptUUID = "b28f7626-d8ea-4b28-9168-1f4491593111"

// minimalScriptModel returns a model with only the required attributes set;
// every optional attribute is null, as in a config that omits them.
func minimalScriptModel() tf.MacScriptResourceModel {
	return tf.MacScriptResourceModel{
		ID:                    types.StringNull(),
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("uix"),
		Description:           types.StringNull(),
		Platform:              types.StringValue("APPLE_OSX"),
		ScriptType:            types.StringValue("BASH"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		PlatformArchitecture:  types.StringNull(),
		ScriptData:            types.StringValue("ZWNobyBoaQ=="),
		Timeout:               types.Int64Null(),
		AllowedInCatalog:      types.BoolValue(false),
		UserInteraction:       types.BoolNull(),
	}
}

// serverDefaultsAPI returns the GET body UEM sends for a script created with
// every optional attribute omitted (live as<internal-env> readback).
func serverDefaultsAPI() *sdk.ScriptResourceV1 {
	f := false
	timeout := 30
	return &sdk.ScriptResourceV1{
		ScriptUUID:            serverDefaultsTestScriptUUID,
		OrganizationGroupUUID: "90cd82d2-e5f5-ce88-e288-e50bdbf629b8",
		Name:                  "uix",
		Platform:              "APPLE_OSX",
		ScriptType:            "BASH",
		ExecutionContext:      "SYSTEM",
		ScriptData:            "ZWNobyBoaQ==",
		AllowedInCatalog:      &f,
		UserInteraction:       &f,
		Timeout:               &timeout,
		// QUIRK-11: an unset architecture arrives as the JSON number 0.
		PlatformArchitecture: &client.IntOrString{IsStr: false, IntVal: 0},
	}
}

func resourceSchema(t *testing.T) (resp resource.SchemaResponse) {
	t.Helper()
	(&macscriptResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp
}

func planFromModel(t *testing.T, m tf.MacScriptResourceModel) tfsdk.Plan {
	t.Helper()
	s := resourceSchema(t).Schema
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := plan.Set(context.Background(), &m); diags.HasError() {
		t.Fatalf("failed to build plan: %v", diags)
	}
	return plan
}

func stateFromModel(t *testing.T, m tf.MacScriptResourceModel) tfsdk.State {
	t.Helper()
	state := emptyResourceState(t)
	if diags := state.Set(context.Background(), &m); diags.HasError() {
		t.Fatalf("failed to build state: %v", diags)
	}
	return state
}

func modelFromState(t *testing.T, state tfsdk.State) tf.MacScriptResourceModel {
	t.Helper()
	var got tf.MacScriptResourceModel
	if diags := state.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("failed to read state: %v", diags)
	}
	return got
}

// createWithAPI runs the real Create path (create, then the post-apply
// readback through refreshIntoState) with the fake GET returning api.
func createWithAPI(t *testing.T, plan tf.MacScriptResourceModel, api *sdk.ScriptResourceV1) tf.MacScriptResourceModel {
	t.Helper()
	got, _ := createWithAPIRequest(t, plan, api)
	return got
}

// createWithAPIRequest is createWithAPI that also returns the captured
// create request.
func createWithAPIRequest(t *testing.T, plan tf.MacScriptResourceModel, api *sdk.ScriptResourceV1) (tf.MacScriptResourceModel, *sdk.CreateScriptV1) {
	t.Helper()
	svc := &mockScriptService{
		createHeaders: http.Header{"Location": []string{"/API/mdm/groups/111/scripts/" + serverDefaultsTestScriptUUID}},
		fetchScript:   api,
	}
	r := newResourceWithMockService(svc)
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	r.Create(context.Background(), resource.CreateRequest{Plan: planFromModel(t, plan)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: unexpected diagnostics: %v", resp.Diagnostics)
	}
	return modelFromState(t, resp.State), svc.createReq
}

// readWithAPI runs the real Read path from prior state with the fake GET
// returning api.
func readWithAPI(t *testing.T, prior tf.MacScriptResourceModel, api *sdk.ScriptResourceV1) tf.MacScriptResourceModel {
	t.Helper()
	prior.ID = types.StringValue(serverDefaultsTestScriptUUID)
	svc := &mockScriptService{fetchScript: api}
	r := newResourceWithMockService(svc)
	state := stateFromModel(t, prior)
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: unexpected diagnostics: %v", resp.Diagnostics)
	}
	return modelFromState(t, resp.State)
}

// B34: user_interaction is now Optional+Computed, so a real terraform plan
// for a new resource whose config omits it presents Unknown (not Null) --
// the framework core marks any Computed attribute Unknown in the plan
// whenever its config is null, regardless of prior state (there is none on
// create). Create must still omit it from the request in that case (verified
// by TestCreate_NullOptionals_OmittedFromRequest below), and the post-create
// readback now takes UEM's response verbatim -- an Unknown planned value can
// resolve to anything without an "inconsistent result after apply" error, so
// there is no need to fabricate null once the schema is Computed.
func TestCreate_UnknownUserInteraction_ReadsBackServerDefaultVerbatim(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.UserInteraction = types.BoolUnknown()

	created := createWithAPI(t, plan, serverDefaultsAPI())
	if !created.UserInteraction.Equal(types.BoolValue(false)) {
		t.Fatalf("Create: user_interaction = %v, want false (UEM's server default, read verbatim)", created.UserInteraction)
	}

	refreshed := readWithAPI(t, created, serverDefaultsAPI())
	if !refreshed.UserInteraction.Equal(types.BoolValue(false)) {
		t.Fatalf("Read after create: user_interaction = %v, want false (no drift)", refreshed.UserInteraction)
	}
}

func TestCreate_ExplicitUserInteraction_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, v := range []bool{false, true} {
		plan := minimalScriptModel()
		plan.UserInteraction = types.BoolValue(v)
		api := serverDefaultsAPI()
		api.UserInteraction = &v

		created := createWithAPI(t, plan, api)
		if !created.UserInteraction.Equal(types.BoolValue(v)) {
			t.Errorf("Create: user_interaction = %v, want %v", created.UserInteraction, v)
		}
		if refreshed := readWithAPI(t, created, api); !refreshed.UserInteraction.Equal(types.BoolValue(v)) {
			t.Errorf("Read: user_interaction = %v, want %v", refreshed.UserInteraction, v)
		}
	}
}

// B39: description is now Optional+Computed (same shape as B34 gave
// timeout/user_interaction), so a real terraform plan for a new resource
// whose config omits it presents Unknown (not Null) -- see the
// user_interaction counterpart above for the full explanation. Live (as<internal-env>
// 26.2): before this fix description was Optional-only, so an omitted config
// planned a fixed null and apply failed with "Provider produced inconsistent
// result after apply" once UEM's readback came back "" instead.
func TestCreate_UnknownDescription_ReadsBackUEMValueVerbatim(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.Description = types.StringUnknown()

	created := createWithAPI(t, plan, serverDefaultsAPI())
	if !created.Description.Equal(types.StringValue("")) {
		t.Fatalf("Create: description = %v, want \"\" (UEM's readback, read verbatim, not null)", created.Description)
	}

	refreshed := readWithAPI(t, created, serverDefaultsAPI())
	if !refreshed.Description.Equal(types.StringValue("")) {
		t.Fatalf("Read after create: description = %v, want \"\" (no drift)", refreshed.Description)
	}
}

func TestCreate_ExplicitDescription_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"", "a description"} {
		plan := minimalScriptModel()
		plan.Description = types.StringValue(v)
		api := serverDefaultsAPI()
		api.Description = v

		created := createWithAPI(t, plan, api)
		if !created.Description.Equal(types.StringValue(v)) {
			t.Errorf("Create: description = %v, want %q", created.Description, v)
		}
		if refreshed := readWithAPI(t, created, api); !refreshed.Description.Equal(types.StringValue(v)) {
			t.Errorf("Read: description = %v, want %q", refreshed.Description, v)
		}
	}
}

// B39/B40 crud-level revert check: creating a script whose config leaves
// description unset (Unknown at plan time, now that description is
// Optional+Computed -- B39) while platform carries UEM's real wire spelling
// (APPLE_OSX, not the V2 Profiles API's AppleOsX -- B40) must decode without
// error and read back exactly what UEM returns for both fields. This is the
// combined shape of the two live "Provider produced inconsistent result
// after apply" failures this fix addresses.
func TestCreate_UnknownDescriptionAndUEMPlatform_NoDecodeErrorReadsBackVerbatim(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.Description = types.StringUnknown()
	plan.Platform = types.StringValue("APPLE_OSX")

	api := serverDefaultsAPI()
	api.Platform = "APPLE_OSX"
	api.Description = ""

	created := createWithAPI(t, plan, api)
	if !created.Description.Equal(types.StringValue("")) {
		t.Errorf("Create: description = %v, want \"\" (UEM's readback, not null)", created.Description)
	}
	if !created.Platform.Equal(types.StringValue("APPLE_OSX")) {
		t.Errorf("Create: platform = %v, want APPLE_OSX (UEM's wire value)", created.Platform)
	}

	refreshed := readWithAPI(t, created, api)
	if !refreshed.Description.Equal(types.StringValue("")) {
		t.Errorf("Read after create: description = %v, want \"\"", refreshed.Description)
	}
	if !refreshed.Platform.Equal(types.StringValue("APPLE_OSX")) {
		t.Errorf("Read after create: platform = %v, want APPLE_OSX", refreshed.Platform)
	}
}

// B34: timeout is now Optional+Computed, so a real terraform plan for a new
// resource whose config omits it presents Unknown (not Null) -- see the
// user_interaction counterpart above for the full explanation.
func TestCreate_UnknownTimeout_ReadsBackServerDefaultVerbatim(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.Timeout = types.Int64Unknown()

	created := createWithAPI(t, plan, serverDefaultsAPI())
	if !created.Timeout.Equal(types.Int64Value(30)) {
		t.Fatalf("Create: timeout = %v, want 30 (UEM's server default, read verbatim)", created.Timeout)
	}

	refreshed := readWithAPI(t, created, serverDefaultsAPI())
	if !refreshed.Timeout.Equal(types.Int64Value(30)) {
		t.Fatalf("Read after create: timeout = %v, want 30 (no drift)", refreshed.Timeout)
	}
}

func TestCreate_ExplicitTimeout_RoundTrips(t *testing.T) {
	t.Parallel()

	for _, v := range []int{30, 45} {
		plan := minimalScriptModel()
		plan.Timeout = types.Int64Value(int64(v))
		api := serverDefaultsAPI()
		api.Timeout = &v

		created := createWithAPI(t, plan, api)
		if !created.Timeout.Equal(types.Int64Value(int64(v))) {
			t.Errorf("Create: timeout = %v, want %d", created.Timeout, v)
		}
		if refreshed := readWithAPI(t, created, api); !refreshed.Timeout.Equal(types.Int64Value(int64(v))) {
			t.Errorf("Read: timeout = %v, want %d", refreshed.Timeout, v)
		}
	}
}

func TestCreate_NullPlatformArchitecture_ServerDefaultStaysNull(t *testing.T) {
	t.Parallel()

	created := createWithAPI(t, minimalScriptModel(), serverDefaultsAPI())
	if !created.PlatformArchitecture.IsNull() {
		t.Fatalf("Create: platform_architecture = %v, want null (plan was null; inconsistent result after apply)", created.PlatformArchitecture)
	}

	refreshed := readWithAPI(t, created, serverDefaultsAPI())
	if !refreshed.PlatformArchitecture.IsNull() {
		t.Fatalf("Read after create: platform_architecture = %v, want null (no drift)", refreshed.PlatformArchitecture)
	}
}

func TestCreate_ExplicitPlatformArchitecture_RoundTrips(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.PlatformArchitecture = types.StringValue("LEGACY")
	api := serverDefaultsAPI()
	api.PlatformArchitecture = &client.IntOrString{IsStr: true, StrVal: "LEGACY"}

	created := createWithAPI(t, plan, api)
	if !created.PlatformArchitecture.Equal(types.StringValue("LEGACY")) {
		t.Errorf("Create: platform_architecture = %v, want LEGACY", created.PlatformArchitecture)
	}
	if refreshed := readWithAPI(t, created, api); !refreshed.PlatformArchitecture.Equal(types.StringValue("LEGACY")) {
		t.Errorf("Read: platform_architecture = %v, want LEGACY", refreshed.PlatformArchitecture)
	}
}

// B34 (behaviour change): timeout is now Optional+Computed, so a real
// terraform plan for an existing resource whose config omits timeout no
// longer presents Null -- the framework core's UseStateForUnknown plan
// modifier copies the known prior state (60) forward instead, exactly like
// TestTimeoutPlanModifier_OmittedConfigAgainstKnownStatePlansNoChange
// verifies at the modifier level. So the plan built here uses 60 (what a
// real plan would actually contain), not Null (what it would have contained
// before this fix, when the attribute was Optional-only). The old behaviour
// this test used to cover -- config-omits-timeout always PUTs UEM's default,
// resetting a stale non-default value -- was already unreachable through a
// real plan once the schema became Computed (removing timeout from an
// existing resource's configuration shows no diff at all, so nothing is ever
// sent as a change), and B36 additionally removed that force-send from
// ToUpdateScriptV1 itself: it was an uncited default (no UEM source, nothing
// live-confirmed, that a PUT requires Timeout or that omitting it resets the
// value). A literal `timeout = null` in config -- exercised directly by
// TestToUpdateScriptV1_timeout in internal/scripts/mac/state -- now omits
// Timeout from the request too, exactly like Create.
func TestUpdate_OmittedTimeoutAgainstKnownState_KeepsCurrentValue(t *testing.T) {
	t.Parallel()

	prior := minimalScriptModel()
	prior.ID = types.StringValue(serverDefaultsTestScriptUUID)
	prior.Timeout = types.Int64Value(60)
	plan := minimalScriptModel()
	plan.ID = types.StringValue(serverDefaultsTestScriptUUID)
	plan.Timeout = types.Int64Value(60) // UseStateForUnknown's real output for an omitted config

	api := serverDefaultsAPI()
	sixty := 60
	api.Timeout = &sixty

	svc := &mockScriptService{fetchScript: api}
	r := newResourceWithMockService(svc)
	resp := &resource.UpdateResponse{State: stateFromModel(t, prior)}
	r.Update(context.Background(), resource.UpdateRequest{Plan: planFromModel(t, plan), State: stateFromModel(t, prior)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: unexpected diagnostics: %v", resp.Diagnostics)
	}

	if svc.updateReq == nil || svc.updateReq.Timeout == nil || *svc.updateReq.Timeout != 60 {
		t.Fatalf("PUT timeout = %v, want 60 (config omitted timeout, but the plan carries the current value forward)", svc.updateReq)
	}
	if got := modelFromState(t, resp.State); !got.Timeout.Equal(types.Int64Value(60)) {
		t.Fatalf("Update: timeout = %v, want 60 (verbatim, no drift)", got.Timeout)
	}
}

// uix: a null (or, post-B34, an unknown) plan value is omitted from the
// create request, so UEM applies its own default; the read side (B34) now
// takes that default back verbatim instead of mapping it to null.
func TestCreate_NullOptionals_OmittedFromRequest(t *testing.T) {
	t.Parallel()

	_, req := createWithAPIRequest(t, minimalScriptModel(), serverDefaultsAPI())
	if req == nil {
		t.Fatal("no create request captured")
	}
	if req.Timeout != nil {
		t.Errorf("create Timeout = %d, want omitted (nil)", *req.Timeout)
	}
	if req.UserInteraction != nil {
		t.Errorf("create UserInteraction = %v, want omitted (nil)", *req.UserInteraction)
	}
	if req.Description != "" {
		t.Errorf("create Description = %q, want omitted (empty)", req.Description)
	}
	if req.PlatformArchitecture != nil {
		t.Errorf("create PlatformArchitecture = %v, want omitted (nil)", req.PlatformArchitecture)
	}
}

// uix: catalog_display.categories is now Optional+Computed with
// UseStateForUnknown, so the model field is a types.List rather than a Go
// slice -- a real Create plan for a new resource whose config sets
// catalog_display but omits categories presents Unknown for it (Computed, no
// prior state to carry forward, exactly like timeout/user_interaction
// above). Before the types.List change, req.Plan.Get errored trying to
// decode that Unknown value into a []types.String; this must now decode
// cleanly, the create request must omit categories (UEM applies its own
// default), and the post-create readback must take UEM's response back
// verbatim -- here, the empty list UEM returns for a catalog script with no
// categories assigned.
func TestCreate_UnknownCategories_OmitsRequestAndReadsBackUEMValue(t *testing.T) {
	t.Parallel()

	plan := minimalScriptModel()
	plan.AllowedInCatalog = types.BoolValue(true)
	plan.CatalogDisplay = &tf.MacCatalogDisplayModel{
		DisplayName:    types.StringValue("Deploy"),
		DisplayDesc:    types.StringValue("Run deploy"),
		PreActionText:  types.StringValue("Proceed?"),
		PostActionText: types.StringValue("Done."),
		ActionType:     types.StringValue("INSTALL"),
		CatalogIconURL: types.StringValue("https://example/icon.png"),
		Categories:     types.ListUnknown(types.StringType),
	}

	allowed := true
	api := serverDefaultsAPI()
	api.AllowedInCatalog = &allowed
	api.CatalogDisplay = &sdk.CatalogDisplayV1{
		DisplayName:    "Deploy",
		DisplayDesc:    "Run deploy",
		PreActionText:  "Proceed?",
		PostActionText: "Done.",
		ActionType:     "INSTALL",
		CatalogIconURL: "https://example/icon.png",
		Categories:     []*int{}, // UEM's live shape for "no categories"
	}

	created, req := createWithAPIRequest(t, plan, api)
	if req == nil {
		t.Fatal("no create request captured")
	}
	if req.CatalogDisplay == nil {
		t.Fatal("create request CatalogDisplay: got nil")
	}
	if req.CatalogDisplay.Categories != nil {
		t.Errorf("create request categories = %+v, want omitted (nil)", req.CatalogDisplay.Categories)
	}
	if created.CatalogDisplay == nil {
		t.Fatal("Create: CatalogDisplay got nil")
	}
	if created.CatalogDisplay.Categories.IsNull() || created.CatalogDisplay.Categories.IsUnknown() {
		t.Fatalf("Create: categories = %v, want a known empty list (UEM's readback)", created.CatalogDisplay.Categories)
	}
	if len(created.CatalogDisplay.Categories.Elements()) != 0 {
		t.Errorf("Create: categories = %v, want empty", created.CatalogDisplay.Categories)
	}
}

// uix: only the int 0 "unset" form reads as null; any other int keeps its
// string form, so a Read of a null-state script surfaces it as drift.
func TestRead_NonZeroIntPlatformArchitecture_Kept(t *testing.T) {
	t.Parallel()

	api := serverDefaultsAPI()
	api.PlatformArchitecture = &client.IntOrString{IsStr: false, IntVal: 2}
	if got := readWithAPI(t, minimalScriptModel(), api); !got.PlatformArchitecture.Equal(types.StringValue("2")) {
		t.Fatalf("Read: platform_architecture = %v, want \"2\"", got.PlatformArchitecture)
	}
}
