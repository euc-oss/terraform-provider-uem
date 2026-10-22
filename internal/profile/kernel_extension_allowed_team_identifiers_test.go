package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// B48 (live, as<internal-env>): creating a uem_profile (AppleOsX) with kernel_extension
// set but kernel_extension.allowed_team_identifiers left unset failed apply
// with "Provider produced inconsistent result after apply:
// .kernel_extension.allowed_team_identifiers: was null, but now [\"\"]". UEM's
// own ctor echoes a non-null (if degenerate, [""]) AllowedTeamIdentifiers even
// when the create request never sent one -- the same class of bug B42 fixed
// for disk_encryption.mcx/filevault2.enable (see disk_encryption_mcx_computed_test.go).
//
// The prior schema entry additionally carried useStateForNullList() alongside
// listplanmodifier.UseStateForUnknown(), which turned create's genuinely
// unknown plan into an explicit null. That is what broke live create:
// Terraform core only allows the applied value to differ from the plan when
// the plan was UNKNOWN, never when it was null. This test asserts the
// null-conversion modifier's ABSENCE: allowed_team_identifiers must carry
// exactly one plan modifier, and it must be the built-in UseStateForUnknown.
func TestKernelExtensionAllowedTeamIdentifiers_PlanModifierIsUseStateForUnknownOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sr := getResourceSchema(t)
	keAttr := mustSingleNestedAttr(t, sr.Schema.Attributes, "kernel_extension")

	idsAttr, ok := keAttr.Attributes["allowed_team_identifiers"].(schema.ListAttribute)
	if !ok {
		t.Fatal("kernel_extension.allowed_team_identifiers: not a ListAttribute")
	}
	if !idsAttr.Optional || !idsAttr.Computed {
		t.Errorf("kernel_extension.allowed_team_identifiers: want Optional+Computed, got Optional=%v Computed=%v", idsAttr.Optional, idsAttr.Computed)
	}
	if len(idsAttr.PlanModifiers) != 1 {
		t.Fatalf("kernel_extension.allowed_team_identifiers: want exactly 1 plan modifier (UseStateForUnknown only, no null-conversion modifier), got %d", len(idsAttr.PlanModifiers))
	}
	if got := idsAttr.PlanModifiers[0].Description(ctx); got != useStateForUnknownDescription {
		t.Errorf("kernel_extension.allowed_team_identifiers: plan modifier = %q, want the built-in UseStateForUnknown (%q); a null-conversion modifier is what broke live create (B48)", got, useStateForUnknownDescription)
	}
}

// runListPlanModifiers is the List-typed equivalent of
// runObjectPlanModifiers/runBoolPlanModifiers (disk_encryption_mcx_computed_test.go):
// it chains multiple plan modifiers on one attribute exactly as the framework
// does, feeding each modifier the previous modifier's PlanValue.
func runListPlanModifiers(ctx context.Context, mods []planmodifier.List, req planmodifier.ListRequest) planmodifier.ListResponse {
	resp := planmodifier.ListResponse{PlanValue: req.PlanValue}
	for _, m := range mods {
		req.PlanValue = resp.PlanValue
		m.PlanModifyList(ctx, req, &resp)
	}
	return resp
}

// TestKernelExtensionAllowedTeamIdentifiersPlanModifier_CreateConfigNullStaysUnknown
// drives allowed_team_identifiers' ACTUAL schema plan modifiers (not a
// hand-copied re-implementation) through a create scenario: no prior state
// (State.Raw wholly null, as the framework presents it when no resource
// instance exists yet) and configuration omits allowed_team_identifiers. The
// plan must stay UNKNOWN, never null -- this is the exact live create failure
// this fix addresses (B48, as<internal-env>). Because this test walks the real
// schema.PlanModifiers slice, re-adding a null-conversion modifier (like the
// removed useStateForNullList) to this schema entry would make this test fail
// again -- see the revert check in the internal-ticket report for this fix.
func TestKernelExtensionAllowedTeamIdentifiersPlanModifier_CreateConfigNullStaysUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sr := getResourceSchema(t)
	keAttr := mustSingleNestedAttr(t, sr.Schema.Attributes, "kernel_extension")
	idsAttr, ok := keAttr.Attributes["allowed_team_identifiers"].(schema.ListAttribute)
	if !ok {
		t.Fatal("kernel_extension.allowed_team_identifiers: not a ListAttribute")
	}

	req := planmodifier.ListRequest{
		Path:        path.Root("kernel_extension").AtName("allowed_team_identifiers"),
		ConfigValue: types.ListNull(types.StringType),
		PlanValue:   types.ListUnknown(types.StringType), // the framework's own default for an unconfigured Computed attribute
		State:       nullResourceState(t),                // create: no prior state at all
		StateValue:  types.ListNull(types.StringType),
	}

	resp := runListPlanModifiers(ctx, idsAttr.PlanModifiers, req)

	if resp.PlanValue.IsNull() {
		t.Fatal("allowed_team_identifiers plan modifiers turned create's unknown into an explicit null -- this is exactly the B48 live failure (\"was null, but now [\\\"\\\"]\"); a planned null can never differ from the applied value, only a planned unknown can")
	}
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("expected allowed_team_identifiers plan to stay unknown on create, got a known value %v", resp.PlanValue)
	}
}

// The live create shape: a macOS profile whose kernel_extension is configured
// but allowed_team_identifiers is left entirely unset. With this fix, a fresh
// Computed ListAttribute left out of config plans as genuinely UNKNOWN
// (listplanmodifier.UseStateForUnknown() alone, no create-time null
// conversion). This is what Create() actually receives in real Terraform
// usage. Create must decode the unknown allowed_team_identifiers plan value
// without error (the model field is types.List, which holds unknown
// directly), must omit AllowedTeamIdentifiers from the request body (an
// unknown/null list means "let UEM apply its own default"), and must land
// UEM's echoed [""] verbatim in state.
func TestProfileResourceCreate_KernelExtensionAllowedTeamIdentifiersUnknown(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(55555)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              55555,
					"Name":                   "macOS Kernel Extension Only",
					"ProfileUuid":            "uuid-b48",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"KernelExtension": map[string]interface{}{
					// UEM's own default, echoed even though the create
					// request never sent AllowedTeamIdentifiers
					// (live-confirmed: as<internal-env>).
					"AllowedTeamIdentifiers": []string{""},
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

	schemaResp := getResourceSchema(t)
	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	keType, ok := objType.AttributeTypes["kernel_extension"].(tftypes.Object)
	if !ok {
		t.Fatal("kernel_extension attribute type is not an Object")
	}

	kernelExtVal := tftypes.NewValue(keType, map[string]tftypes.Value{
		"allow_user_overrides":      tftypes.NewValue(keType.AttributeTypes["allow_user_overrides"], nil),
		"allowed_kernel_extensions": tftypes.NewValue(keType.AttributeTypes["allowed_kernel_extensions"], nil),
		// allowed_team_identifiers left entirely unset in configuration.
		// ModifyPlan (listplanmodifier.UseStateForUnknown(), the ONLY
		// modifier on this attribute after this fix) leaves a fresh Computed
		// ListAttribute with no prior state as genuinely unknown.
		"allowed_team_identifiers": tftypes.NewValue(keType.AttributeTypes["allowed_team_identifiers"], tftypes.UnknownValue),
	})

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"name":             stringVal("macOS Kernel Extension Only"),
		"platform":         stringVal("AppleOsX"),
		"org_group_id":     stringVal("14165"),
		"kernel_extension": kernelExtVal,
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
	keTop, ok := capturedBody["KernelExtension"].(map[string]interface{})
	if !ok {
		t.Fatal("expected KernelExtension in request body")
	}
	if _, present := keTop["AllowedTeamIdentifiers"]; present {
		t.Errorf("expected AllowedTeamIdentifiers to be omitted from the request body, got %v", keTop["AllowedTeamIdentifiers"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "55555" {
		t.Errorf("expected ID '55555', got '%s'", model.ID.ValueString())
	}
	if model.KernelExtension == nil {
		t.Fatal("expected KernelExtension to be set in state after read-back")
	}
	ids := model.KernelExtension.AllowedTeamIdentifiers
	if ids.IsNull() || ids.IsUnknown() {
		t.Fatalf("expected a known, non-null AllowedTeamIdentifiers, got %v", ids)
	}
	elems := ids.Elements()
	if len(elems) != 1 {
		t.Fatalf("expected AllowedTeamIdentifiers = [\"\"] (UEM's echoed value), got %v", elems)
	}
	s, ok := elems[0].(types.String)
	if !ok || s.ValueString() != "" {
		t.Errorf("expected AllowedTeamIdentifiers[0] = \"\" (UEM's echoed value), got %v", elems[0])
	}
}
