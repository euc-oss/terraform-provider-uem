package profile

// Create-consistency tests (internal-ticket).
//
// Live (a UEM 26.4 lab tenant), a macOS uem_profile create failed with
// "Provider produced inconsistent result after apply" for two reasons:
//
//   (a) a PARTIAL gatekeeper block: UEM fills a default for the true/false
//       fields the configuration leaves out (enable_app_software_update_delay,
//       enable_software_update_delay, ...). They were Optional only, so the
//       plan said null and the read-back said false.
//   (b) profile_scope = "Production": UEM accepts it and reads back "".
//
// These tests run the production path end to end: PlanResourceChange through
// the real framework server (so the schema's plan modifiers run), then the
// real ProfileResource.Create against an httptest server through the real SDK
// client, then the same plan-vs-applied check Terraform core runs after every
// apply (assertAppliedMatchesPlan below; the framework does not run it).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// planApplyMismatches mirrors Terraform core's post-apply check
// (plans.AssertObjectCompatible): every attribute whose planned value is
// known must come back from apply exactly equal; an unknown planned value may
// become anything. It returns the attribute paths that violate that. Sets and
// maps are compared exactly (stricter than core); nothing asserted here uses them.
func planApplyMismatches(path string, planned, applied tftypes.Value) []string {
	if !planned.IsKnown() {
		// Core also requires the applied value to be wholly known.
		if !applied.IsFullyKnown() {
			return []string{path}
		}
		return nil
	}
	if planned.IsNull() || applied.IsNull() || !applied.IsKnown() {
		if planned.IsNull() && applied.IsNull() {
			return nil
		}
		return []string{path}
	}
	var out []string
	switch {
	case planned.Type().Is(tftypes.Object{}):
		var pm, am map[string]tftypes.Value
		if planned.As(&pm) != nil || applied.As(&am) != nil {
			return []string{path}
		}
		for k, pv := range pm {
			out = append(out, planApplyMismatches(path+"."+k, pv, am[k])...)
		}
	case planned.Type().Is(tftypes.List{}), planned.Type().Is(tftypes.Tuple{}):
		var pl, al []tftypes.Value
		if planned.As(&pl) != nil || applied.As(&al) != nil || len(pl) != len(al) {
			return []string{path}
		}
		for i := range pl {
			out = append(out, planApplyMismatches(path+"[]", pl[i], al[i])...)
		}
	default:
		if !planned.Equal(applied) {
			out = append(out, path)
		}
	}
	return out
}

// assertAppliedMatchesPlan fails the test, the way Terraform core would fail
// the apply, when the new state is not compatible with the plan.
func assertAppliedMatchesPlan(t *testing.T, planned, applied tftypes.Value) {
	t.Helper()
	if bad := planApplyMismatches("", planned, applied); len(bad) > 0 {
		t.Fatalf("Provider produced inconsistent result after apply: %s", strings.Join(bad, ", "))
	}
}

// createConfigAppleOsX is the create config of a macOS profile with the given
// gatekeeper and profile_scope; everything else the user does not set is null.
func createConfigAppleOsX(t *testing.T, gatekeeper, profileScope tftypes.Value) tftypes.Value {
	t.Helper()
	return createResourceConfig(t, map[string]tftypes.Value{
		"name":          stringVal("Create Consistency"),
		"platform":      stringVal(sdk.PlatformAppleOsX),
		"org_group_id":  stringVal("14165"),
		"profile_scope": profileScope,
		"gatekeeper":    gatekeeper,
	}).Raw
}

// uemCreateServer is a fake UEM: it records the create request body and
// answers the read-back GET with the given General fields and top-level
// payload sections (for example "GateKeeper", "Restrictions").
type uemCreateServer struct {
	sent map[string]any
}

func (u *uemCreateServer) handler(general map[string]any, sections map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/create"):
			raw, _ := io.ReadAll(r.Body)
			u.sent = map[string]any{}
			_ = json.Unmarshal(raw, &u.sent)
			_ = json.NewEncoder(w).Encode(55555)
		case r.Method == http.MethodGet:
			g := map[string]any{
				"ProfileId":              55555,
				"Name":                   "Create Consistency",
				"ProfileUuid":            "uuid-create-consistency",
				"ProfileContext":         "Device",
				"AssignmentType":         "Auto",
				"IsActive":               true,
				"ManagedLocationGroupID": 14165,
			}
			for k, v := range general {
				g[k] = v
			}
			body := map[string]any{"General": g}
			for k, v := range sections {
				body[k] = v
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// planAndCreate plans a create through the real framework, runs the real
// Create against the fake UEM, and returns the planned and applied objects.
func planAndCreate(t *testing.T, config tftypes.Value, general, gatekeeper map[string]any) (planned, applied tftypes.Value, resp *resource.CreateResponse, uem *uemCreateServer) {
	t.Helper()
	var sections map[string]any
	if gatekeeper != nil {
		sections = map[string]any{"GateKeeper": gatekeeper}
	}
	return planAndCreateSections(t, config, general, sections)
}

// planAndCreateSections is planAndCreate with arbitrary top-level sections in
// the read-back.
func planAndCreateSections(t *testing.T, config tftypes.Value, general, sections map[string]any) (planned, applied tftypes.Value, resp *resource.CreateResponse, uem *uemCreateServer) {
	t.Helper()
	planned = planProfileValue(t, nullResourceState(t).Raw, config)

	uem = &uemCreateServer{}
	c, server := createTestClient(t, uem.handler(general, sections))
	t.Cleanup(server.Close)

	schemaResp := getResourceSchema(t)
	res := &ProfileResource{client: c}
	resp = &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: planned},
	}, resp)
	return planned, resp.State.Raw, resp, uem
}

func requireNoDiagErrors(t *testing.T, resp *resource.CreateResponse) {
	t.Helper()
	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("Create returned errors: %v", msgs)
	}
}

func objAttr(t *testing.T, obj tftypes.Value, name string) tftypes.Value {
	t.Helper()
	var m map[string]tftypes.Value
	if err := obj.As(&m); err != nil {
		t.Fatalf("object.As: %v", err)
	}
	v, ok := m[name]
	if !ok {
		t.Fatalf("attribute %q missing", name)
	}
	return v
}

func boolOf(t *testing.T, v tftypes.Value) bool {
	t.Helper()
	var b bool
	if err := v.As(&b); err != nil {
		t.Fatalf("value %v is not a known bool: %v", v, err)
	}
	return b
}

func mapOf(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object, got %T (%v)", v, v)
	}
	return m
}

// partialGatekeeper configures four of the six true/false fields and leaves
// enable_app_software_update_delay and enable_software_update_delay out, the
// shape that failed live.
func partialGatekeeper() tftypes.Value {
	return gatekeeperVal(map[string]tftypes.Value{
		"allow_auto_unlock":            boolVal(true),
		"allow_fingerprint_for_unlock": boolVal(true),
		"allow_handoff":                boolVal(false),
		"allow_screen_capture":         boolVal(true),
	})
}

// uemGatekeeperWithDefaults is what UEM answers for the partial block: the
// four configured values echoed and the two unset fields filled with their
// defaults (seen live).
func uemGatekeeperWithDefaults() map[string]any {
	return map[string]any{
		"AllowAutoUnlock":              true,
		"AllowFingerprintForUnlock":    true,
		"AllowHandoff":                 false,
		"AllowScreenCapture":           true,
		"EnableAppSoftwareUpdateDelay": false,
		"EnableSoftwareUpdateDelay":    false,
	}
}

// TestCreate_PartialGatekeeper_ServerFilledDefaults is the fix for (a): the two
// unset fields plan as unknown, are not sent, and take UEM's filled defaults
// without an inconsistent-result error.
func TestCreate_PartialGatekeeper_ServerFilledDefaults(t *testing.T) {
	t.Parallel()
	config := createConfigAppleOsX(t, partialGatekeeper(), nullString())
	planned, applied, resp, uem := planAndCreate(t, config, nil, uemGatekeeperWithDefaults())
	requireNoDiagErrors(t, resp)

	pgk := objAttr(t, planned, "gatekeeper")
	for _, f := range []string{"enable_app_software_update_delay", "enable_software_update_delay"} {
		if objAttr(t, pgk, f).IsKnown() {
			t.Errorf("planned gatekeeper.%s = %v, want unknown (UEM fills it)", f, objAttr(t, pgk, f))
		}
	}

	// Faithful send: the configured fields go out as configured and the
	// unset ones are not invented.
	gk := mapOf(t, uem.sent["GateKeeper"])
	for k, want := range map[string]bool{"AllowAutoUnlock": true, "AllowFingerprintForUnlock": true, "AllowHandoff": false, "AllowScreenCapture": true} {
		if got, ok := gk[k].(bool); !ok || got != want {
			t.Errorf("sent GateKeeper.%s = %v, want %v", k, gk[k], want)
		}
	}
	for _, k := range []string{"EnableAppSoftwareUpdateDelay", "EnableSoftwareUpdateDelay", "EnforcedSoftwareUpdateDelay"} {
		if v, present := gk[k]; present {
			t.Errorf("sent GateKeeper.%s = %v, want it omitted (not configured)", k, v)
		}
	}

	// What UEM filled lands in state, and Terraform core would accept it.
	agk := objAttr(t, applied, "gatekeeper")
	for _, f := range []string{"enable_app_software_update_delay", "enable_software_update_delay"} {
		v := objAttr(t, agk, f)
		if !v.IsKnown() || v.IsNull() || boolOf(t, v) {
			t.Errorf("state gatekeeper.%s = %v, want the UEM default false", f, v)
		}
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// TestCreate_PartialGatekeeper_ConfiguredValueChangedByUEMStillSurfaces proves
// the fix does not hide a real mismatch: a value the user configured that UEM
// returns changed is still an inconsistent result.
func TestCreate_PartialGatekeeper_ConfiguredValueChangedByUEMStillSurfaces(t *testing.T) {
	t.Parallel()
	config := createConfigAppleOsX(t, partialGatekeeper(), nullString())
	changed := uemGatekeeperWithDefaults()
	changed["AllowHandoff"] = true // configured false
	planned, applied, resp, _ := planAndCreate(t, config, nil, changed)
	requireNoDiagErrors(t, resp)

	bad := planApplyMismatches("", planned, applied)
	if len(bad) != 1 || bad[0] != ".gatekeeper.allow_handoff" {
		t.Fatalf("mismatches = %v, want exactly [.gatekeeper.allow_handoff]", bad)
	}
}

// TestCreate_FullGatekeeper_StillConsistent is the control: a fully configured
// block behaves as before.
func TestCreate_FullGatekeeper_StillConsistent(t *testing.T) {
	t.Parallel()
	full := gatekeeperVal(map[string]tftypes.Value{
		"allow_auto_unlock":                boolVal(true),
		"allow_fingerprint_for_unlock":     boolVal(true),
		"allow_handoff":                    boolVal(false),
		"allow_screen_capture":             boolVal(true),
		"enable_app_software_update_delay": boolVal(false),
		"enable_software_update_delay":     boolVal(false),
	})
	config := createConfigAppleOsX(t, full, nullString())
	planned, applied, resp, uem := planAndCreate(t, config, nil, uemGatekeeperWithDefaults())
	requireNoDiagErrors(t, resp)
	gk := mapOf(t, uem.sent["GateKeeper"])
	if v, ok := gk["EnableSoftwareUpdateDelay"].(bool); !ok || v {
		t.Errorf("sent GateKeeper.EnableSoftwareUpdateDelay = %v, want the configured false", gk["EnableSoftwareUpdateDelay"])
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// TestPlan_PartialGatekeeper_UpdateKeepsServerFilledValues: on an update the
// fields left unset in the configuration plan as the refreshed state value
// (not "known after apply" and not a diff), whichever value UEM holds; a
// configured field still wins.
func TestPlan_PartialGatekeeper_UpdateKeepsServerFilledValues(t *testing.T) {
	t.Parallel()
	stateGK := gatekeeperVal(map[string]tftypes.Value{
		"allow_auto_unlock":                boolVal(true),
		"allow_fingerprint_for_unlock":     boolVal(true),
		"allow_handoff":                    boolVal(false),
		"allow_screen_capture":             boolVal(true),
		"enable_app_software_update_delay": boolVal(false),
		"enable_software_update_delay":     boolVal(true), // set in the console
	})
	prior := createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"gatekeeper": stateGK,
	})).Raw
	config := createResourceConfig(t, map[string]tftypes.Value{
		"name":         stringVal("Renamed"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"),
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{
			"allow_handoff": boolVal(true), // configured: changes
		}),
	}).Raw
	planned := planProfileValue(t, prior, config)
	pgk := objAttr(t, planned, "gatekeeper")
	if got := objAttr(t, pgk, "allow_handoff"); !got.IsKnown() || !boolOf(t, got) {
		t.Errorf("configured allow_handoff = %v, want the configured true", got)
	}
	for f, want := range map[string]bool{"allow_auto_unlock": true, "allow_fingerprint_for_unlock": true, "allow_screen_capture": true, "enable_app_software_update_delay": false, "enable_software_update_delay": true} {
		got := objAttr(t, pgk, f)
		if !got.IsKnown() || got.IsNull() || boolOf(t, got) != want {
			t.Errorf("unset gatekeeper.%s planned %v, want the state value %v", f, got, want)
		}
	}
}

// gatekeeperUpdateFixture returns a prior state whose gatekeeper is stateGK
// and the plan the real framework produces for a rename plus the given
// gatekeeper config.
func gatekeeperUpdateFixture(t *testing.T, stateGK, configGK tftypes.Value) (prior tftypes.Value, planned tftypes.Value) {
	t.Helper()
	prior = createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"gatekeeper": stateGK,
	})).Raw
	config := createResourceConfig(t, map[string]tftypes.Value{
		"name":         stringVal("Renamed"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"),
		"gatekeeper":   configGK,
	}).Raw
	return prior, planProfileValue(t, prior, config)
}

// TestUpdate_AddingPartialGatekeeper_ServerFilledDefaults covers the update
// that ADDS a partial gatekeeper block to a profile that has none: the unset
// leaves must plan unknown (the framework's stock UseStateForUnknown planned
// them null here, because the leaf's state is null), and the real Update must
// accept the defaults UEM fills.
func TestUpdate_AddingPartialGatekeeper_ServerFilledDefaults(t *testing.T) {
	t.Parallel()
	prior, planned := gatekeeperUpdateFixture(t, nullGatekeeper(), partialGatekeeper())
	pgk := objAttr(t, planned, "gatekeeper")
	for _, f := range []string{"enable_app_software_update_delay", "enable_software_update_delay"} {
		if objAttr(t, pgk, f).IsKnown() {
			t.Fatalf("planned gatekeeper.%s = %v, want unknown when the block is added", f, objAttr(t, pgk, f))
		}
	}

	fake := &freshGetService{gatekeeper: &sdk.MacOsGatekeeperPayloadV2Entity{
		AllowAutoUnlock:              sdk.BoolPtr(true),
		AllowFingerprintForUnlock:    sdk.BoolPtr(true),
		AllowHandoff:                 sdk.BoolPtr(false),
		AllowScreenCapture:           sdk.BoolPtr(true),
		EnableAppSoftwareUpdateDelay: sdk.BoolPtr(false),
		EnableSoftwareUpdateDelay:    sdk.BoolPtr(false),
	}}
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	t.Cleanup(server.Close)
	res := &ProfileResource{
		client: c,
		newProfileService: func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error) {
			return fake, nil
		},
	}
	schemaResp := getResourceSchema(t)
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: planned},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: prior},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok || sent.GateKeeper == nil {
		t.Fatalf("no GateKeeper sent: %+v", fake.updateEntity)
	}
	if sent.GateKeeper.EnableSoftwareUpdateDelay != nil || sent.GateKeeper.EnableAppSoftwareUpdateDelay != nil {
		t.Errorf("unset gatekeeper fields were sent on an add: %+v", sent.GateKeeper)
	}
	for _, bad := range planApplyMismatches("", planned, resp.State.Raw) {
		if strings.HasPrefix(bad, ".gatekeeper") {
			t.Errorf("Provider produced inconsistent result after apply: %s", bad)
		}
	}
}

// TestCreate_ProfileScopeProduction_UEMReadsBackEmpty is the fix for (b) on
// macOS: the configured value is sent as configured, UEM reads back "", and the
// apply is consistent with the plan.
func TestCreate_ProfileScopeProduction_UEMReadsBackEmpty(t *testing.T) {
	t.Parallel()
	config := createConfigAppleOsX(t, nullGatekeeper(), stringVal("Production"))
	planned, applied, resp, uem := planAndCreate(t, config, map[string]any{"ProfileScope": ""}, nil)
	requireNoDiagErrors(t, resp)

	if got := mapOf(t, uem.sent["General"])["ProfileScope"]; got != "Production" {
		t.Errorf("sent General.ProfileScope = %v, want the configured \"Production\"", got)
	}
	var scope string
	if err := objAttr(t, applied, "profile_scope").As(&scope); err != nil || scope != "Production" {
		t.Errorf("state profile_scope = %v (err %v), want the configured value kept", objAttr(t, applied, "profile_scope"), err)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// TestCreate_ProfileScope_UEMChangesToDifferentValueStillSurfaces: only an
// EMPTY read-back is read as "UEM reports none". A different non-empty value is
// real and must still be an inconsistent result.
func TestCreate_ProfileScope_UEMChangesToDifferentValueStillSurfaces(t *testing.T) {
	t.Parallel()
	config := createConfigAppleOsX(t, nullGatekeeper(), stringVal("Production"))
	planned, applied, resp, _ := planAndCreate(t, config, map[string]any{"ProfileScope": "Staging"}, nil)
	requireNoDiagErrors(t, resp)
	bad := planApplyMismatches("", planned, applied)
	if len(bad) != 1 || bad[0] != ".profile_scope" {
		t.Fatalf("mismatches = %v, want exactly [.profile_scope]", bad)
	}
}

// TestCreate_ProfileScopeUnset_UEMReadsBackEmpty is the control: with the
// attribute unset nothing is sent and the empty read-back is stored as "".
func TestCreate_ProfileScopeUnset_UEMReadsBackEmpty(t *testing.T) {
	t.Parallel()
	config := createConfigAppleOsX(t, nullGatekeeper(), nullString())
	planned, applied, resp, uem := planAndCreate(t, config, map[string]any{"ProfileScope": ""}, nil)
	requireNoDiagErrors(t, resp)
	if v, present := mapOf(t, uem.sent["General"])["ProfileScope"]; present {
		t.Errorf("sent General.ProfileScope = %v, want it omitted", v)
	}
	var scope string
	if err := objAttr(t, applied, "profile_scope").As(&scope); err != nil || scope != "" {
		t.Errorf("state profile_scope = %v, want \"\"", objAttr(t, applied, "profile_scope"))
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// TestRead_ProfileScope_EmptyReadBackKeepsStateOnMacOS: a refresh must not
// flip the kept value to "" (that would plan a pointless update every run),
// while a different non-empty value from UEM is real drift.
func TestRead_ProfileScope_EmptyReadBackKeepsStateOnMacOS(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, uemScope, prior, want string
	}{
		{"empty keeps state", "", "Production", "Production"},
		{"different value is drift", "Staging", "Production", "Staging"},
		{"empty with empty state", "", "", ""},
		{"same value", "Production", "Production", "Production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			live := liveAppleOsXEntity()
			live.General.ProfileScope = tc.uemScope
			data := profilemodels.ProfileResourceModel{}
			data.ProfileScope = types.StringValue(tc.prior)
			profilestate.ReadProfileIntoState(context.Background(), &data, &sdk.ProfileResult{AppleOsX: live})
			if data.ProfileScope.ValueString() != tc.want || data.ProfileScope.IsNull() {
				t.Errorf("profile_scope = %v, want %q", data.ProfileScope, tc.want)
			}
		})
	}
}

// TestRead_ProfileScope_OtherPlatformsUnchanged guards the scope of the rule:
// it was observed on macOS only, so iOS (a separate reader) still stores UEM's
// empty value verbatim.
func TestRead_ProfileScope_OtherPlatformsUnchanged(t *testing.T) {
	t.Parallel()
	live := liveAppleiOSEntity()
	live.General.ProfileScope = ""
	data := profilemodels.ProfileResourceModel{}
	data.ProfileScope = types.StringValue("Production")
	profilestate.ReadProfileIntoState(context.Background(), &data, &sdk.ProfileResult{AppleiOS: live})
	if data.ProfileScope.ValueString() != "" {
		t.Errorf("iOS profile_scope = %q, want UEM's empty value stored as-is", data.ProfileScope.ValueString())
	}
}

// freshGetService answers every Get with a new macOS entity whose ProfileScope
// is empty, as UEM does: the Update overlay mutates the entity it fetched, so a
// shared pointer would echo the sent value back and hide the read-back.
type freshGetService struct {
	fakeProfileService
	gatekeeper *sdk.MacOsGatekeeperPayloadV2Entity // GateKeeper UEM answers with, if any
}

func (f *freshGetService) Get(ctx context.Context, id int) (*sdk.ProfileResult, error) {
	ent := liveAppleOsXEntity()
	if f.gatekeeper != nil {
		gk := *f.gatekeeper
		ent.GateKeeper = &gk
	}
	return &sdk.ProfileResult{AppleOsX: ent}, nil
}

// TestUpdate_ProfileScope_EmptyReadBackKeepsPlannedValueOnMacOS runs the real
// Update: the planned profile_scope is sent, UEM reads back "", and the new
// state equals the plan (no inconsistent result on update either).
func TestUpdate_ProfileScope_EmptyReadBackKeepsPlannedValueOnMacOS(t *testing.T) {
	t.Parallel()
	fake := &freshGetService{}
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	t.Cleanup(server.Close)
	res := &ProfileResource{
		client: c,
		newProfileService: func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error) {
			return fake, nil
		},
	}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"name":          stringVal("Renamed"),
		"profile_scope": stringVal("Production"),
	})
	plan := createResourcePlan(t, planValues)
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok || sent.General == nil || sent.General.ProfileScope != "Production" {
		t.Fatalf("sent General.ProfileScope not the planned \"Production\": %+v", fake.updateEntity)
	}
	// The live fixture's other fields (name, smart groups) differ from this
	// plan, so check profile_scope only.
	for _, bad := range planApplyMismatches("", plan.Raw, resp.State.Raw) {
		if bad == ".profile_scope" {
			t.Fatal("Provider produced inconsistent result after apply: .profile_scope")
		}
	}
}
