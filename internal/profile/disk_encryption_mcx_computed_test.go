package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// useStateForUnknownDescription is the Description() text both
// boolplanmodifier.UseStateForUnknown() and objectplanmodifier.UseStateForUnknown()
// return (the framework's built-in modifier, not this package's custom
// useStateForNullBool/useStateForNullObject wrappers, whose Description is
// "Use the prior state value when the configuration value is null."). Used
// below to assert that mcx, its leaf, and filevault2.enable carry ONLY the
// built-in modifier and nothing else.
const useStateForUnknownDescription = "Once set, the value of this attribute in state will not change."

// B42 (live, as<internal-env> UEM 26.2): creating a uem_profile (AppleOsX) with
// disk_encryption.filevault2 set but disk_encryption.mcx left unset failed
// apply with "Provider produced inconsistent result after apply:
// .disk_encryption.mcx: was null, but now
// {\"destroy_fv_key_on_standby\":false}". UEM's DiskEncryption ctor always
// instantiates a non-null MCX sub-object with its own default, even when the
// whole block is left out of the configuration -- the same class of bug F13/B37
// fixed for scep_list/vpn_list/eas_microsoft_outlook/web_clips_list/
// kernel_extension/custom_attributes (see f13_computed_test.go). Mcx and its
// leaf, and filevault2.enable (UEM's FileVault2 ctor also defaults it, to
// true, canonical rules Q1), must be Optional+Computed with
// UseStateForUnknown like those payloads.
//
// B42 FOLLOW-UP (this file): the first fix additionally gave mcx and its leaf
// a create-time null-conversion modifier (useStateForNullObject /
// useStateForNullBool) that turned create's genuinely-unknown plan into an
// explicit null. That is what actually broke live create: Terraform core only
// allows the applied value to differ from the plan when the plan was
// UNKNOWN, never when it was null -- a planned null that comes back non-null
// is rejected outright. The fix here removes that create-time conversion and
// keeps only UseStateForUnknown, so this test now asserts its ABSENCE: each
// of the three attributes must carry exactly one plan modifier, and it must
// be the built-in UseStateForUnknown, not a null-conversion wrapper.
func TestDiskEncryptionMCXAndFileVaultEnableAreComputed(t *testing.T) {
	t.Parallel()

	var sr resource.SchemaResponse
	(&ProfileResource{}).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	ctx := context.Background()

	deAttr, ok := sr.Schema.Attributes["disk_encryption"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("disk_encryption: not a SingleNestedAttribute")
	}

	mcxAttr, ok := deAttr.Attributes["mcx"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("disk_encryption.mcx: not a SingleNestedAttribute")
	}
	if !mcxAttr.Optional || !mcxAttr.Computed {
		t.Errorf("disk_encryption.mcx: want Optional+Computed, got Optional=%v Computed=%v", mcxAttr.Optional, mcxAttr.Computed)
	}
	if len(mcxAttr.PlanModifiers) != 1 {
		t.Fatalf("disk_encryption.mcx: want exactly 1 plan modifier (UseStateForUnknown only, no null-conversion modifier), got %d", len(mcxAttr.PlanModifiers))
	}
	if got := mcxAttr.PlanModifiers[0].Description(ctx); got != useStateForUnknownDescription {
		t.Errorf("disk_encryption.mcx: plan modifier = %q, want the built-in UseStateForUnknown (%q); a null-conversion modifier is what broke live create (B42 follow-up)", got, useStateForUnknownDescription)
	}

	destroyAttr, ok := mcxAttr.Attributes["destroy_fv_key_on_standby"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("disk_encryption.mcx.destroy_fv_key_on_standby: not a BoolAttribute")
	}
	if !destroyAttr.Optional || !destroyAttr.Computed {
		t.Errorf("disk_encryption.mcx.destroy_fv_key_on_standby: want Optional+Computed, got Optional=%v Computed=%v", destroyAttr.Optional, destroyAttr.Computed)
	}
	if len(destroyAttr.PlanModifiers) != 1 {
		t.Fatalf("disk_encryption.mcx.destroy_fv_key_on_standby: want exactly 1 plan modifier, got %d", len(destroyAttr.PlanModifiers))
	}
	if got := destroyAttr.PlanModifiers[0].Description(ctx); got != useStateForUnknownDescription {
		t.Errorf("disk_encryption.mcx.destroy_fv_key_on_standby: plan modifier = %q, want the built-in UseStateForUnknown (%q)", got, useStateForUnknownDescription)
	}

	fvAttr, ok := deAttr.Attributes["filevault2"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("disk_encryption.filevault2: not a SingleNestedAttribute")
	}
	enableAttr, ok := fvAttr.Attributes["enable"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("disk_encryption.filevault2.enable: not a BoolAttribute")
	}
	if !enableAttr.Optional || !enableAttr.Computed {
		t.Errorf("disk_encryption.filevault2.enable: want Optional+Computed, got Optional=%v Computed=%v", enableAttr.Optional, enableAttr.Computed)
	}
	if len(enableAttr.PlanModifiers) != 1 {
		t.Fatalf("disk_encryption.filevault2.enable: want exactly 1 plan modifier, got %d", len(enableAttr.PlanModifiers))
	}
	if got := enableAttr.PlanModifiers[0].Description(ctx); got != useStateForUnknownDescription {
		t.Errorf("disk_encryption.filevault2.enable: plan modifier = %q, want the built-in UseStateForUnknown (%q)", got, useStateForUnknownDescription)
	}

	// Fields that stay Optional-only: each is either required whenever
	// disk_encryption is configured (recovery_type, filevault_user,
	// prompt_to_enable_filevault_at, airwatch.use_intelligent_hub -- so UEM
	// never has to fill a default a real plan could omit), or has no cited
	// UEM ctor default to plan against.
	for _, name := range []string{"show_recovery_key", "recovery_type", "filevault_enterprise_certificate", "filevault_user", "username", "prompt_to_enable_filevault_at", "number_of_times_user_can_bypass"} {
		a, ok := fvAttr.Attributes[name]
		if !ok {
			t.Fatalf("filevault2.%s: missing", name)
		}
		if a.IsComputed() {
			t.Errorf("filevault2.%s: expected Optional-only (no cited UEM default), got Computed", name)
		}
	}
}

// mustSingleNestedAttr fetches attrs[name] as a schema.SingleNestedAttribute,
// failing the test with a clear message instead of panicking on a bad
// assertion (forcetypeassert).
func mustSingleNestedAttr(t *testing.T, attrs map[string]schema.Attribute, name string) schema.SingleNestedAttribute {
	t.Helper()
	a, ok := attrs[name].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("%s: not a SingleNestedAttribute (got %T)", name, attrs[name])
	}
	return a
}

// mustBoolAttr is mustSingleNestedAttr's schema.BoolAttribute equivalent.
func mustBoolAttr(t *testing.T, attrs map[string]schema.Attribute, name string) schema.BoolAttribute {
	t.Helper()
	a, ok := attrs[name].(schema.BoolAttribute)
	if !ok {
		t.Fatalf("%s: not a BoolAttribute (got %T)", name, attrs[name])
	}
	return a
}

// runObjectPlanModifiers applies mods to req in sequence, exactly as the
// framework chains multiple plan modifiers on one attribute: each modifier
// sees the previous modifier's PlanValue as its own req.PlanValue.
func runObjectPlanModifiers(ctx context.Context, mods []planmodifier.Object, req planmodifier.ObjectRequest) planmodifier.ObjectResponse {
	resp := planmodifier.ObjectResponse{PlanValue: req.PlanValue}
	for _, m := range mods {
		req.PlanValue = resp.PlanValue
		m.PlanModifyObject(ctx, req, &resp)
	}
	return resp
}

// runBoolPlanModifiers is the Bool-typed equivalent of runObjectPlanModifiers.
func runBoolPlanModifiers(ctx context.Context, mods []planmodifier.Bool, req planmodifier.BoolRequest) planmodifier.BoolResponse {
	resp := planmodifier.BoolResponse{PlanValue: req.PlanValue}
	for _, m := range mods {
		req.PlanValue = resp.PlanValue
		m.PlanModifyBool(ctx, req, &resp)
	}
	return resp
}

// TestMCXObjectPlanModifier_CreateConfigNullStaysUnknown drives disk_encryption.mcx's
// ACTUAL schema plan modifiers (not a hand-copied re-implementation) through a
// create scenario: no prior state (State.Raw wholly null, as the framework
// presents it when no resource instance exists yet) and configuration omits
// mcx. The plan must stay UNKNOWN, never null -- Terraform core only accepts
// an applied value that differs from the plan when the plan was unknown,
// never when it was null (this is the exact live create failure this fix
// addresses: B42 follow-up, as<internal-env> UEM 26.2). Because this test walks the real
// schema.PlanModifiers slice, re-adding a null-conversion modifier (like the
// removed useStateForNullObject) to the mcx schema entry would make this test
// fail again -- see the revert check in the internal-ticket report for this fix.
func TestMCXObjectPlanModifier_CreateConfigNullStaysUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sr := getResourceSchema(t)
	deAttr := mustSingleNestedAttr(t, sr.Schema.Attributes, "disk_encryption")
	mcxAttr := mustSingleNestedAttr(t, deAttr.Attributes, "mcx")

	attrTypes := profilemodels.DiskEncryptionMCXAttrTypes
	req := planmodifier.ObjectRequest{
		Path:        path.Root("disk_encryption").AtName("mcx"),
		ConfigValue: types.ObjectNull(attrTypes),
		PlanValue:   types.ObjectUnknown(attrTypes), // the framework's own default for an unconfigured Computed attribute
		State:       nullResourceState(t),           // create: no prior state at all
		StateValue:  types.ObjectNull(attrTypes),
	}

	resp := runObjectPlanModifiers(ctx, mcxAttr.PlanModifiers, req)

	if resp.PlanValue.IsNull() {
		t.Fatal("mcx plan modifiers turned create's unknown into an explicit null -- this is exactly the B42 live failure (\"was null, but now {...}\"); a planned null can never differ from the applied value, only a planned unknown can")
	}
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("expected mcx plan to stay unknown on create, got a known value %v", resp.PlanValue)
	}
}

// TestMCXObjectPlanModifier_UpdateConfigNullUsesPriorState mirrors the test
// above for an update: prior state holds a known mcx value and configuration
// omits mcx. UseStateForUnknown must carry the prior state value forward
// (matching the removed useStateForNullBool/useStateForNullObject's Update
// behavior, which this fix keeps -- only the create-time null conversion was
// wrong).
func TestMCXObjectPlanModifier_UpdateConfigNullUsesPriorState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sr := getResourceSchema(t)
	deAttr := mustSingleNestedAttr(t, sr.Schema.Attributes, "disk_encryption")
	mcxAttr := mustSingleNestedAttr(t, deAttr.Attributes, "mcx")

	attrTypes := profilemodels.DiskEncryptionMCXAttrTypes
	priorMCX := types.ObjectValueMust(attrTypes, map[string]attr.Value{
		"destroy_fv_key_on_standby": types.BoolValue(true),
	})
	state := createResourceState(t, map[string]tftypes.Value{
		"disk_encryption": diskEncryptionVal(map[string]tftypes.Value{
			"mcx": mcxVal(map[string]tftypes.Value{"destroy_fv_key_on_standby": boolVal(true)}),
		}),
	})

	req := planmodifier.ObjectRequest{
		Path:        path.Root("disk_encryption").AtName("mcx"),
		ConfigValue: types.ObjectNull(attrTypes),
		PlanValue:   types.ObjectUnknown(attrTypes),
		State:       state,
		StateValue:  priorMCX,
	}

	resp := runObjectPlanModifiers(ctx, mcxAttr.PlanModifiers, req)

	if resp.PlanValue.IsUnknown() || resp.PlanValue.IsNull() {
		t.Fatalf("expected the prior state value to carry forward on update, got %v", resp.PlanValue)
	}
	if !resp.PlanValue.Equal(priorMCX) {
		t.Errorf("expected plan = prior state %v, got %v", priorMCX, resp.PlanValue)
	}
}

// TestFileVaultEnableAndMCXLeafPlanModifier_CreateConfigNullStaysUnknown is the
// Bool-typed equivalent of TestMCXObjectPlanModifier_CreateConfigNullStaysUnknown,
// covering both remaining Optional+Computed bool leaves this fix touches:
// mcx.destroy_fv_key_on_standby and filevault2.enable.
func TestFileVaultEnableAndMCXLeafPlanModifier_CreateConfigNullStaysUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sr := getResourceSchema(t)
	deAttr := mustSingleNestedAttr(t, sr.Schema.Attributes, "disk_encryption")
	mcxAttr := mustSingleNestedAttr(t, deAttr.Attributes, "mcx")
	destroyAttr := mustBoolAttr(t, mcxAttr.Attributes, "destroy_fv_key_on_standby")
	fvAttr := mustSingleNestedAttr(t, deAttr.Attributes, "filevault2")
	enableAttr := mustBoolAttr(t, fvAttr.Attributes, "enable")

	for _, tc := range []struct {
		name string
		path path.Path
		mods []planmodifier.Bool
	}{
		{"mcx.destroy_fv_key_on_standby", path.Root("disk_encryption").AtName("mcx").AtName("destroy_fv_key_on_standby"), destroyAttr.PlanModifiers},
		{"filevault2.enable", path.Root("disk_encryption").AtName("filevault2").AtName("enable"), enableAttr.PlanModifiers},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.BoolRequest{
				Path:        tc.path,
				ConfigValue: types.BoolNull(),
				PlanValue:   types.BoolUnknown(),
				State:       nullResourceState(t),
				StateValue:  types.BoolNull(),
			}
			resp := runBoolPlanModifiers(ctx, tc.mods, req)
			if resp.PlanValue.IsNull() {
				t.Fatalf("%s: plan modifiers turned create's unknown into an explicit null -- the same class of bug as the B42 live failure", tc.name)
			}
			if !resp.PlanValue.IsUnknown() {
				t.Errorf("%s: expected plan to stay unknown on create, got known value %v", tc.name, resp.PlanValue)
			}
		})
	}
}

// The live create shape: a macOS profile whose disk_encryption.filevault2 is
// configured but disk_encryption.mcx is left entirely unset. With this fix's
// fix, a fresh Computed SingleNestedAttribute left out of config plans as
// genuinely UNKNOWN (objectplanmodifier.UseStateForUnknown() alone, no
// create-time null conversion -- see TestMCXObjectPlanModifier_CreateConfigNullStaysUnknown
// above). This is what Create() actually receives in real Terraform usage
// (ModifyPlan always runs first, and would leave mcx unknown here). Create
// must decode the unknown mcx plan value without error (the model field is
// now types.Object, which holds unknown directly -- see
// profilemodels.DiskEncryptionModel.MCX's doc comment -- unlike the old
// *DiskEncryptionMCXModel pointer field, which could not), must omit
// DiskEncryptionMCX from the request body (an unknown/null mcx means "let UEM
// apply its own default"), and must land UEM's echoed default in state.
func TestProfileResourceCreate_FileVaultSetMCXUnknown(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(44444)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              44444,
					"Name":                   "macOS FileVault Only",
					"ProfileUuid":            "uuid-b42",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"DiskEncryption": map[string]interface{}{
					"DiskEncryptionFileVault2": map[string]interface{}{
						"Enable":                     true,
						"ShowRecoveryKey":            true,
						"RecoveryType":               float64(1),
						"FileVaultUser":              "CurrentOrNextLoginUser",
						"PromptToEnableFileVaultAt":  "BothLoginAndLogout",
						"NumberOfTimesUserCanBypass": float64(3),
					},
					// UEM's own default, echoed even though the create
					// request never sent a DiskEncryptionMCX section
					// (live-confirmed: as<internal-env>, UEM 26.2).
					"DiskEncryptionMCX": map[string]interface{}{
						"DestroyFVKeyOnStandby": false,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS FileVault Only"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption": diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(map[string]tftypes.Value{
				"enable":                          boolVal(true),
				"show_recovery_key":               boolVal(true),
				"recovery_type":                   int64Val(1),
				"filevault_user":                  int64Val(1),
				"prompt_to_enable_filevault_at":   int64Val(1),
				"number_of_times_user_can_bypass": int64Val(3),
			}),
			// disk_encryption.mcx left entirely unset in configuration.
			// ModifyPlan (objectplanmodifier.UseStateForUnknown(), the ONLY
			// modifier on mcx after this fix) leaves a fresh
			// Computed SingleNestedAttribute with no prior state as
			// genuinely unknown -- it does not force it to null. This is
			// what Create() actually receives in real Terraform usage.
			"mcx": tftypes.NewValue(diskEncryptionMCXType(), tftypes.UnknownValue),
		}),
		"gatekeeper":      nullGatekeeper(),
		"restrictions":    nullRestrictions(),
		"uuid":            nullString(),
		"profile_context": nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors (want no decode error): %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	deTop, ok := capturedBody["DiskEncryption"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryption in request body")
	}
	if _, present := deTop["DiskEncryptionMCX"]; present {
		t.Errorf("expected DiskEncryptionMCX to be omitted from the request body, got %v", deTop["DiskEncryptionMCX"])
	}
	if _, ok := deTop["DiskEncryptionFileVault2"]; !ok {
		t.Fatal("expected DiskEncryptionFileVault2 in request body")
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "44444" {
		t.Errorf("expected ID '44444', got '%s'", model.ID.ValueString())
	}
	if model.DiskEncryption == nil {
		t.Fatal("expected DiskEncryption to be set in state after read-back")
	}
	if model.DiskEncryption.MCX.IsNull() {
		t.Fatal("expected MCX to be set in state from UEM's echoed default")
	}
	destroy := mcxDestroyFVKeyOnStandby(t, model.DiskEncryption.MCX)
	if destroy.IsNull() || destroy.IsUnknown() {
		t.Fatalf("expected a known, non-null DestroyFVKeyOnStandby, got %v", destroy)
	}
	if destroy.ValueBool() != false {
		t.Errorf("expected DestroyFVKeyOnStandby false (UEM's echoed default), got %v", destroy.ValueBool())
	}
}
