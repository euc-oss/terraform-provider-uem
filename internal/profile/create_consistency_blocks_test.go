package profile

// Server-filled payload blocks (internal-ticket), second part. Live (a UEM
// 26.4 lab tenant, macOS) each of these made a create fail with "Provider produced
// inconsistent result after apply":
//
//   - gatekeeper.enforced_software_update_delay: UEM sets 30 whenever
//     enable_software_update_delay is true, and leaves it unset when false.
//   - system_extensions.allow_user_overrides, and the three
//     allowed_system_extension_types[].allow_*_extension_type fields: UEM
//     fills false.
//   - privacy_preferences[].static_code: UEM fills false.
//   - restrictions: setting one leaf makes UEM return every other sub-block
//     populated with defaults (11 sub-blocks in the camera-only case).
//
// The tests plan through the real framework server, run the real Create (and
// Update) against an httptest UEM through the real SDK client or the fake
// service, answer with the raw shapes observed live, and apply the check
// Terraform core runs after an apply (planApplyMismatches). Each case also
// keeps a "configured value that UEM changes still errors" test.

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- helpers ---

func topAttrType(t *testing.T, name string) tftypes.Type {
	t.Helper()
	obj, ok := getResourceSchema(t).Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	typ, ok := obj.AttributeTypes[name]
	if !ok {
		t.Fatalf("no attribute %q", name)
	}
	return typ
}

func subType(t *testing.T, parent tftypes.Type, name string) tftypes.Type {
	t.Helper()
	switch p := parent.(type) {
	case tftypes.Object:
		typ, ok := p.AttributeTypes[name]
		if !ok {
			t.Fatalf("no attribute %q", name)
		}
		return typ
	default:
		t.Fatalf("not an object: %T", parent)
	}
	return nil
}

func listElemType(t *testing.T, listType tftypes.Type) tftypes.Type {
	t.Helper()
	l, ok := listType.(tftypes.List)
	if !ok {
		t.Fatalf("not a list: %T", listType)
	}
	return l.ElementType
}

// nestedObj builds an object of typ, every attribute not given is null.
func nestedObj(t *testing.T, typ tftypes.Type, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(typ, fillMissingAttrs(t, typ, vals))
}

func listOf(elemType tftypes.Type, items ...tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: elemType}, items)
}

// fillDefaultsFalse fills every nil *bool with false and every nil pointer to a
// struct with a zero struct, recursively: UEM's "every other sub-block fully
// populated with false" answer for restrictions.
func fillDefaultsFalse(v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			switch v.Type().Elem().Kind() {
			case reflect.Bool:
				b := false
				v.Set(reflect.ValueOf(&b))
			case reflect.Struct:
				v.Set(reflect.New(v.Type().Elem()))
			default:
				return
			}
		}
		if v.Elem().Kind() == reflect.Struct {
			fillDefaultsFalse(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fillDefaultsFalse(v.Field(i))
			}
		}
	}
}

// uemFilledRestrictions is the live answer for a camera-only restrictions
// config: Applications.Camera as configured, every other sub-block present and
// filled with false (allow_app_store_app_adoption true), as observed.
func uemFilledRestrictions() *sdk.AppleOsXRestrictionsPayloadEntityV2 {
	r := &sdk.AppleOsXRestrictionsPayloadEntityV2{
		Applications: &sdk.AppleOsXRestrictionApplicationsPayloadEntityV2{
			Camera: &sdk.AppleOsXRestrictionCameraPayloadEntityV2{AllowUseOfBuiltInCamera: sdk.BoolPtr(false)},
		},
	}
	fillDefaultsFalse(reflect.ValueOf(r))
	r.Applications.AppStore.AllowAppStoreAppAdoption = sdk.BoolPtr(true)
	return r
}

// runUpdate runs the real Update with the given fake service.
func runUpdate(t *testing.T, fake profileServiceAPI, prior, planned tftypes.Value) *resource.UpdateResponse {
	t.Helper()
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
	return resp
}

// mismatchesUnder returns the plan-vs-applied mismatches whose path starts with prefix.
func mismatchesUnder(planned, applied tftypes.Value, prefix string) []string {
	var out []string
	for _, bad := range planApplyMismatches("", planned, applied) {
		if strings.HasPrefix(bad, prefix) {
			out = append(out, bad)
		}
	}
	return out
}

func createConfigWith(t *testing.T, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return createResourceConfig(t, mergeValues(map[string]tftypes.Value{
		"name":         stringVal("Create Consistency"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"),
	}, vals)).Raw
}

func updateConfigWith(t *testing.T, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return createResourceConfig(t, mergeValues(map[string]tftypes.Value{
		"name":         stringVal("Renamed"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"),
	}, vals)).Raw
}

// appliedAsPrior turns the state a Create produced into the prior state of a
// later update (id and uuid are what UEM returned).
func appliedAsPrior(applied tftypes.Value) tftypes.Value { return applied }

// updateFake answers every Get with a fresh macOS entity plus the mutation.
type updateFake struct {
	freshGetService
	mutate func(*sdk.AppleOsXDeviceProfileEntityV2)
}

func (f *updateFake) Get(ctx context.Context, id int) (*sdk.ProfileResult, error) {
	ent := liveAppleOsXEntity()
	// The live fixture carries a Restrictions.Desktop of its own; start clean.
	ent.Restrictions = nil
	if f.mutate != nil {
		f.mutate(ent)
	}
	return &sdk.ProfileResult{AppleOsX: ent}, nil
}

func sentEntity(t *testing.T, v any) *sdk.AppleOsXDeviceProfileEntityV2 {
	t.Helper()
	e, ok := v.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("sent entity is %T, want *sdk.AppleOsXDeviceProfileEntityV2", v)
	}
	return e
}

// firstOf returns the first element of a decoded JSON array of objects.
func firstOf(t *testing.T, v any) map[string]any {
	t.Helper()
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		t.Fatalf("expected a non-empty JSON array, got %T (%v)", v, v)
	}
	return mapOf(t, arr[0])
}

// --- gatekeeper.enforced_software_update_delay ---

func gkUEM(enable bool, enforced *int) map[string]any {
	m := map[string]any{
		"AllowAutoUnlock":              true,
		"AllowFingerprintForUnlock":    true,
		"AllowHandoff":                 true,
		"AllowScreenCapture":           true,
		"EnableAppSoftwareUpdateDelay": false,
		"EnableSoftwareUpdateDelay":    enable,
	}
	if enforced != nil {
		m["EnforcedSoftwareUpdateDelay"] = *enforced
	}
	return m
}

func intp(n int) *int { return &n }

// Live case 3: enable_software_update_delay = true, number unset -> UEM sets 30.
func TestCreate_EnforcedDelay_FilledWhenToggleOn(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{"enable_software_update_delay": boolVal(true)}),
	})
	planned, applied, resp, uem := planAndCreate(t, config, nil, gkUEM(true, intp(30)))
	requireNoDiagErrors(t, resp)
	if objAttr(t, objAttr(t, planned, "gatekeeper"), "enforced_software_update_delay").IsKnown() {
		t.Error("planned enforced_software_update_delay is known, want unknown")
	}
	if v, present := mapOf(t, uem.sent["GateKeeper"])["EnforcedSoftwareUpdateDelay"]; present {
		t.Errorf("sent EnforcedSoftwareUpdateDelay = %v, want it omitted (not configured)", v)
	}
	var n int64
	if err := objAttr(t, objAttr(t, applied, "gatekeeper"), "enforced_software_update_delay").As(&n); err == nil && n == 0 {
		t.Error("state enforced_software_update_delay is zero")
	}
	got := objAttr(t, objAttr(t, applied, "gatekeeper"), "enforced_software_update_delay")
	if want := tftypes.NewValue(tftypes.Number, 30); !got.Equal(want) {
		t.Errorf("state enforced_software_update_delay = %v, want 30", got)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// A configured number that UEM changes is still reported.
func TestCreate_EnforcedDelay_ConfiguredValueChangedByUEMStillSurfaces(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{
			"enable_software_update_delay":   boolVal(true),
			"enforced_software_update_delay": int64Val(45),
		}),
	})
	planned, applied, resp, uem := planAndCreate(t, config, nil, gkUEM(true, intp(30)))
	requireNoDiagErrors(t, resp)
	if v, ok := mapOf(t, uem.sent["GateKeeper"])["EnforcedSoftwareUpdateDelay"].(float64); !ok || v != 45 {
		t.Errorf("sent EnforcedSoftwareUpdateDelay = %v, want the configured 45", v)
	}
	bad := mismatchesUnder(planned, applied, ".gatekeeper")
	if len(bad) != 1 || bad[0] != ".gatekeeper.enforced_software_update_delay" {
		t.Fatalf("mismatches = %v, want exactly [.gatekeeper.enforced_software_update_delay]", bad)
	}
}

// Toggle off: UEM leaves the number unset, the unknown plan resolves to null.
func TestCreate_EnforcedDelay_NullWhenToggleOff(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{"enable_software_update_delay": boolVal(false)}),
	})
	planned, applied, resp, _ := planAndCreate(t, config, nil, gkUEM(false, nil))
	requireNoDiagErrors(t, resp)
	if !objAttr(t, objAttr(t, applied, "gatekeeper"), "enforced_software_update_delay").IsNull() {
		t.Error("state enforced_software_update_delay is not null")
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// Later updates: a 30 UEM set is carried and sent unchanged; a null stays
// unknown in the plan and nothing is sent for it.
func TestUpdate_EnforcedDelay_CarriedOrOmitted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		state     tftypes.Value
		wantKnown bool
	}{
		{"state 30 is carried", int64Val(30), true},
		{"state null stays unknown", tftypes.NewValue(tftypes.Number, nil), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stateGK := gatekeeperVal(map[string]tftypes.Value{
				"allow_auto_unlock": boolVal(true), "allow_fingerprint_for_unlock": boolVal(true),
				"allow_handoff": boolVal(true), "allow_screen_capture": boolVal(true),
				"enable_app_software_update_delay": boolVal(false),
				"enable_software_update_delay":     boolVal(tc.wantKnown),
				"enforced_software_update_delay":   tc.state,
			})
			prior, planned := gatekeeperUpdateFixture(t, stateGK, gatekeeperVal(map[string]tftypes.Value{
				"enable_software_update_delay": boolVal(tc.wantKnown),
			}))
			got := objAttr(t, objAttr(t, planned, "gatekeeper"), "enforced_software_update_delay")
			if got.IsKnown() != tc.wantKnown {
				t.Fatalf("planned enforced_software_update_delay known=%v, want %v (%v)", got.IsKnown(), tc.wantKnown, got)
			}
			var enforced *int
			if tc.wantKnown {
				enforced = intp(30)
			}
			fake := &updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) {
				e.GateKeeper = &sdk.MacOsGatekeeperPayloadV2Entity{
					AllowAutoUnlock: sdk.BoolPtr(true), AllowFingerprintForUnlock: sdk.BoolPtr(true),
					AllowHandoff: sdk.BoolPtr(true), AllowScreenCapture: sdk.BoolPtr(true),
					EnableAppSoftwareUpdateDelay: sdk.BoolPtr(false), EnableSoftwareUpdateDelay: sdk.BoolPtr(tc.wantKnown),
					EnforcedSoftwareUpdateDelay: enforced,
				}
			}}
			resp := runUpdate(t, fake, prior, planned)
			sent := sentEntity(t, fake.updateEntity)
			if tc.wantKnown && (sent.GateKeeper.EnforcedSoftwareUpdateDelay == nil || *sent.GateKeeper.EnforcedSoftwareUpdateDelay != 30) {
				t.Errorf("sent EnforcedSoftwareUpdateDelay = %v, want the carried 30", sent.GateKeeper.EnforcedSoftwareUpdateDelay)
			}
			if !tc.wantKnown && sent.GateKeeper.EnforcedSoftwareUpdateDelay != nil {
				t.Errorf("sent EnforcedSoftwareUpdateDelay = %d, want nothing sent for a null state", *sent.GateKeeper.EnforcedSoftwareUpdateDelay)
			}
			if bad := mismatchesUnder(planned, resp.State.Raw, ".gatekeeper"); len(bad) > 0 {
				t.Errorf("Provider produced inconsistent result after apply: %v", bad)
			}
		})
	}
}

// --- system_extensions ---

func sysExtTypeEntry(t *testing.T, team string, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	sysType := topAttrType(t, "system_extensions")
	elem := listElemType(t, subType(t, sysType, "allowed_system_extension_types"))
	return nestedObj(t, elem, mergeValues(map[string]tftypes.Value{"team_identifier": stringVal(team)}, vals))
}

func sysExtVal(t *testing.T, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	return nestedObj(t, topAttrType(t, "system_extensions"), vals)
}

func sysExtList(t *testing.T, items ...tftypes.Value) tftypes.Value {
	t.Helper()
	return listOf(listElemType(t, subType(t, topAttrType(t, "system_extensions"), "allowed_system_extension_types")), items...)
}

func allowedExtList(t *testing.T) tftypes.Value {
	t.Helper()
	elem := listElemType(t, subType(t, topAttrType(t, "system_extensions"), "allowed_system_extensions"))
	return listOf(elem, nestedObj(t, elem, map[string]tftypes.Value{
		"team_identifier": stringVal("TEAM1"), "bundle_identifier": stringVal("com.example.ext"),
	}))
}

// uemSysExt is the live answer: the three type booleans filled with false per
// type entry, allow_user_overrides filled, and UEM's own "*" entry added.
func uemSysExt(overrides bool, teams ...string) map[string]any {
	types := make([]map[string]any, 0, len(teams)+1)
	for _, tm := range teams {
		types = append(types, map[string]any{"TeamIdentifier": tm, "AllowDriverExtensionType": false, "AllowEndpointSecurityExtensionType": false, "AllowNetworkExtensionType": false})
	}
	types = append(types, map[string]any{"TeamIdentifier": "*", "AllowDriverExtensionType": false, "AllowEndpointSecurityExtensionType": false, "AllowNetworkExtensionType": false})
	return map[string]any{
		"AllowUserOverrides":          overrides,
		"AllowedSystemExtensionTypes": types,
		"AllowedSystemExtensions":     []map[string]any{{"TeamIdentifier": "TEAM1", "BundleIdentifier": "com.example.ext"}},
	}
}

// Live cases 4f/4g: types with only team_identifier, allow_user_overrides unset.
func TestCreate_SystemExtensions_ServerFilledBooleans(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"system_extensions": sysExtVal(t, map[string]tftypes.Value{
			"allowed_system_extension_types": sysExtList(t, sysExtTypeEntry(t, "TEAM1", nil)),
			"allowed_system_extensions":      allowedExtList(t),
		}),
	})
	planned, applied, resp, uem := planAndCreateSections(t, config, nil, map[string]any{"SystemExtensions": uemSysExt(false, "TEAM1")})
	requireNoDiagErrors(t, resp)
	se := mapOf(t, uem.sent["SystemExtensions"])
	if v, present := se["AllowUserOverrides"]; present {
		t.Errorf("sent AllowUserOverrides = %v, want it omitted", v)
	}
	ty := firstOf(t, se["AllowedSystemExtensionTypes"])
	for _, k := range []string{"AllowDriverExtensionType", "AllowEndpointSecurityExtensionType", "AllowNetworkExtensionType"} {
		if v, present := ty[k]; present {
			t.Errorf("sent %s = %v, want it omitted", k, v)
		}
	}
	if ty["TeamIdentifier"] != "TEAM1" {
		t.Errorf("sent TeamIdentifier = %v", ty["TeamIdentifier"])
	}
	se2 := objAttr(t, applied, "system_extensions")
	if got := objAttr(t, se2, "allow_user_overrides"); !got.Equal(boolVal(false)) {
		t.Errorf("state allow_user_overrides = %v, want false", got)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// Configured values UEM changes still surface.
func TestCreate_SystemExtensions_ConfiguredValuesChangedByUEMStillSurface(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"system_extensions": sysExtVal(t, map[string]tftypes.Value{
			"allow_user_overrides":           boolVal(true),
			"allowed_system_extension_types": sysExtList(t, sysExtTypeEntry(t, "TEAM1", map[string]tftypes.Value{"allow_driver_extension_type": boolVal(true)})),
			"allowed_system_extensions":      allowedExtList(t),
		}),
	})
	planned, applied, resp, _ := planAndCreateSections(t, config, nil, map[string]any{"SystemExtensions": uemSysExt(false, "TEAM1")})
	requireNoDiagErrors(t, resp)
	bad := mismatchesUnder(planned, applied, ".system_extensions")
	want := map[string]bool{
		".system_extensions.allow_user_overrides":                                         true,
		".system_extensions.allowed_system_extension_types[].allow_driver_extension_type": true,
	}
	if len(bad) != len(want) {
		t.Fatalf("mismatches = %v, want %v", bad, want)
	}
	for _, b := range bad {
		if !want[b] {
			t.Fatalf("unexpected mismatch %s in %v", b, bad)
		}
	}
}

// List-element leaves are not carried between entries (state is matched by
// index, so a removal would hand one entry another entry's value): they plan
// unknown when the plan changes, which is consistent for every list change.
func TestUpdate_SystemExtensionTypes_ListChangesStayConsistent(t *testing.T) {
	t.Parallel()
	explicitDriver := map[string]tftypes.Value{"allow_driver_extension_type": boolVal(true)}
	stateTypes := sysExtList(t,
		sysExtTypeEntry(t, "TEAM-A", map[string]tftypes.Value{
			"allow_driver_extension_type": boolVal(true), "allow_endpoint_security_extension_type": boolVal(false), "allow_network_extension_type": boolVal(false)}),
		sysExtTypeEntry(t, "TEAM-B", map[string]tftypes.Value{
			"allow_driver_extension_type": boolVal(false), "allow_endpoint_security_extension_type": boolVal(false), "allow_network_extension_type": boolVal(false)}),
	)
	stateSE := sysExtVal(t, map[string]tftypes.Value{
		"allow_user_overrides":           boolVal(false),
		"allowed_system_extension_types": stateTypes,
		"allowed_system_extensions":      allowedExtList(t),
	})
	prior := createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"system_extensions": stateSE})).Raw

	for _, tc := range []struct {
		name  string
		types tftypes.Value
		teams []string
	}{
		{"unchanged list", sysExtList(t, sysExtTypeEntry(t, "TEAM-A", explicitDriver), sysExtTypeEntry(t, "TEAM-B", nil)), []string{"TEAM-A", "TEAM-B"}},
		{"entry added", sysExtList(t, sysExtTypeEntry(t, "TEAM-A", explicitDriver), sysExtTypeEntry(t, "TEAM-B", nil), sysExtTypeEntry(t, "TEAM-C", nil)), []string{"TEAM-A", "TEAM-B", "TEAM-C"}},
		{"first entry removed", sysExtList(t, sysExtTypeEntry(t, "TEAM-B", nil)), []string{"TEAM-B"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := updateConfigWith(t, map[string]tftypes.Value{
				"system_extensions": sysExtVal(t, map[string]tftypes.Value{
					"allowed_system_extension_types": tc.types,
					"allowed_system_extensions":      allowedExtList(t),
				}),
			})
			planned := planProfileValue(t, prior, config)

			// No entry may adopt a value from another entry: every unset leaf
			// is unknown (or, for TEAM-A, the configured true).
			var plannedTypes []tftypes.Value
			if err := objAttr(t, objAttr(t, planned, "system_extensions"), "allowed_system_extension_types").As(&plannedTypes); err != nil {
				t.Fatal(err)
			}
			for i, el := range plannedTypes {
				team := ""
				_ = objAttr(t, el, "team_identifier").As(&team)
				for _, leaf := range []string{"allow_endpoint_security_extension_type", "allow_network_extension_type"} {
					if objAttr(t, el, leaf).IsKnown() {
						t.Errorf("entry %d (%s) %s planned known (%v), want unknown", i, team, leaf, objAttr(t, el, leaf))
					}
				}
				if team == "TEAM-B" && objAttr(t, el, "allow_driver_extension_type").IsKnown() {
					t.Errorf("TEAM-B allow_driver_extension_type planned %v: it adopted another entry's value", objAttr(t, el, "allow_driver_extension_type"))
				}
				if team == "TEAM-A" && !objAttr(t, el, "allow_driver_extension_type").Equal(boolVal(true)) {
					t.Errorf("TEAM-A configured allow_driver_extension_type = %v, want true", objAttr(t, el, "allow_driver_extension_type"))
				}
			}

			// UEM answers with its own fills; the TEAM-A driver value is the configured true.
			fake := &updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) {
				e.SystemExtensions = &sdk.MacOsSystemExtensionsPayloadV2Model{AllowUserOverrides: sdk.BoolPtr(false)}
				for _, team := range tc.teams {
					e.SystemExtensions.AllowedSystemExtensionTypes = append(e.SystemExtensions.AllowedSystemExtensionTypes, sdk.MacOsAllowedSystemExtensionTypesV2Model{
						TeamIdentifier:                     team,
						AllowDriverExtensionType:           sdk.BoolPtr(team == "TEAM-A"),
						AllowEndpointSecurityExtensionType: sdk.BoolPtr(false),
						AllowNetworkExtensionType:          sdk.BoolPtr(false),
					})
				}
				e.SystemExtensions.AllowedSystemExtensions = []sdk.MacOsAllowedSystemExtensionV2Model{{TeamIdentifier: "TEAM1", BundleIdentifier: "com.example.ext"}}
			}}
			resp := runUpdate(t, fake, prior, planned)
			sent := sentEntity(t, fake.updateEntity)
			for _, ty := range sent.SystemExtensions.AllowedSystemExtensionTypes {
				if ty.AllowEndpointSecurityExtensionType != nil || ty.AllowNetworkExtensionType != nil {
					t.Errorf("sent an unset leaf for %s: %+v", ty.TeamIdentifier, ty)
				}
				if ty.TeamIdentifier == "TEAM-B" && ty.AllowDriverExtensionType != nil {
					t.Errorf("TEAM-B allow_driver_extension_type sent as %v although unset", *ty.AllowDriverExtensionType)
				}
			}
			if bad := mismatchesUnder(planned, resp.State.Raw, ".system_extensions"); len(bad) > 0 {
				t.Errorf("Provider produced inconsistent result after apply: %v", bad)
			}
		})
	}
}

// allow_user_overrides is an object-level leaf and IS carried on updates.
func TestPlan_SystemExtensions_AllowUserOverridesCarried(t *testing.T) {
	t.Parallel()
	stateSE := sysExtVal(t, map[string]tftypes.Value{
		"allow_user_overrides":           boolVal(false),
		"allowed_system_extension_types": sysExtList(t, sysExtTypeEntry(t, "TEAM1", nil)),
		"allowed_system_extensions":      allowedExtList(t),
	})
	prior := createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"system_extensions": stateSE})).Raw
	config := updateConfigWith(t, map[string]tftypes.Value{
		"system_extensions": sysExtVal(t, map[string]tftypes.Value{
			"allowed_system_extension_types": sysExtList(t, sysExtTypeEntry(t, "TEAM1", nil)),
			"allowed_system_extensions":      allowedExtList(t),
		}),
	})
	planned := planProfileValue(t, prior, config)
	if got := objAttr(t, objAttr(t, planned, "system_extensions"), "allow_user_overrides"); !got.Equal(boolVal(false)) {
		t.Errorf("planned allow_user_overrides = %v, want the state value false", got)
	}
}

// --- privacy_preferences[].static_code ---

func privacyEntry(t *testing.T, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	elem := listElemType(t, topAttrType(t, "privacy_preferences"))
	return nestedObj(t, elem, mergeValues(map[string]tftypes.Value{
		"identifier":       stringVal("com.example.app"),
		"identifier_type":  stringVal("bundleID"),
		"code_requirement": stringVal(`identifier "com.example.app" and anchor apple`),
		"accessibility":    stringVal("Allow"),
	}, vals))
}

func privacyList(t *testing.T, items ...tftypes.Value) tftypes.Value {
	t.Helper()
	return listOf(listElemType(t, topAttrType(t, "privacy_preferences")), items...)
}

// uemPrivacy is live case 7b's answer: static_code filled with false,
// apple_events_list [] and the AE* strings "".
func uemPrivacy(staticCode bool) map[string]any {
	return map[string]any{"Identities": []map[string]any{{
		"Identifier": "com.example.app", "IdentifierType": "bundleID",
		"CodeRequirement": `identifier "com.example.app" and anchor apple`, "Accessibility": "Allow",
		"StaticCode": staticCode, "AppleEventsList": []any{},
		"AEReceiverCodeRequirement": "", "AEReceiverIdentifier": "", "AEReceiverIdentifierType": "",
	}}}
}

func TestCreate_PrivacyPreferences_StaticCodeFilled(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{"privacy_preferences": privacyList(t, privacyEntry(t, nil))})
	planned, applied, resp, uem := planAndCreateSections(t, config, nil, map[string]any{"PrivacyPreferences": uemPrivacy(false)})
	requireNoDiagErrors(t, resp)
	ident := firstOf(t, mapOf(t, uem.sent["PrivacyPreferences"])["Identities"])
	if v, present := ident["StaticCode"]; present {
		t.Errorf("sent StaticCode = %v, want it omitted (not configured)", v)
	}
	if ident["CodeRequirement"] == nil || ident["Accessibility"] != "Allow" {
		t.Errorf("configured fields not sent faithfully: %v", ident)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

func TestCreate_PrivacyPreferences_ConfiguredStaticCodeChangedByUEMStillSurfaces(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{"privacy_preferences": privacyList(t, privacyEntry(t, map[string]tftypes.Value{"static_code": boolVal(true)}))})
	planned, applied, resp, _ := planAndCreateSections(t, config, nil, map[string]any{"PrivacyPreferences": uemPrivacy(false)})
	requireNoDiagErrors(t, resp)
	bad := mismatchesUnder(planned, applied, ".privacy_preferences")
	if len(bad) != 1 || bad[0] != ".privacy_preferences[].static_code" {
		t.Fatalf("mismatches = %v, want exactly [.privacy_preferences[].static_code]", bad)
	}
}

// Update with an unchanged list, an entry added and the first entry removed.
func TestUpdate_PrivacyPreferences_ListChangesStayConsistent(t *testing.T) {
	t.Parallel()
	entry := func(id string, vals map[string]tftypes.Value) tftypes.Value {
		return privacyEntry(t, mergeValues(map[string]tftypes.Value{"identifier": stringVal(id)}, vals))
	}
	statePrivacy := privacyList(t,
		entry("com.example.one", map[string]tftypes.Value{"static_code": boolVal(true)}),
		entry("com.example.two", map[string]tftypes.Value{"static_code": boolVal(false)}))
	prior := createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"privacy_preferences": statePrivacy})).Raw
	for _, tc := range []struct {
		name string
		list tftypes.Value
		ids  []string
	}{
		{"unchanged list", privacyList(t, entry("com.example.one", map[string]tftypes.Value{"static_code": boolVal(true)}), entry("com.example.two", nil)), []string{"com.example.one", "com.example.two"}},
		{"entry added", privacyList(t, entry("com.example.one", map[string]tftypes.Value{"static_code": boolVal(true)}), entry("com.example.two", nil), entry("com.example.three", nil)), []string{"com.example.one", "com.example.two", "com.example.three"}},
		{"first entry removed", privacyList(t, entry("com.example.two", nil)), []string{"com.example.two"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planned := planProfileValue(t, prior, updateConfigWith(t, map[string]tftypes.Value{"privacy_preferences": tc.list}))

			// An unset static_code never adopts another entry's value.
			var items []tftypes.Value
			if err := objAttr(t, planned, "privacy_preferences").As(&items); err != nil {
				t.Fatal(err)
			}
			for i, el := range items {
				var id string
				_ = objAttr(t, el, "identifier").As(&id)
				if id != "com.example.one" && objAttr(t, el, "static_code").IsKnown() {
					t.Errorf("entry %d (%s) static_code planned %v, want unknown", i, id, objAttr(t, el, "static_code"))
				}
			}
			fake := &updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) {
				e.PrivacyPreferences = &sdk.MacOsPrivacyPreferencesPayloadV2Model{}
				for _, id := range tc.ids {
					e.PrivacyPreferences.Identities = append(e.PrivacyPreferences.Identities, sdk.MacOsPrivacyPreferencesV2Model{
						Identifier: id, IdentifierType: "bundleID", CodeRequirement: `identifier "com.example.app" and anchor apple`,
						Accessibility: "Allow", StaticCode: sdk.BoolPtr(id == "com.example.one"),
					})
				}
			}}
			resp := runUpdate(t, fake, prior, planned)
			sent := sentEntity(t, fake.updateEntity)
			for _, id := range sent.PrivacyPreferences.Identities {
				if (id.Identifier == "com.example.one") != (id.StaticCode != nil) {
					t.Errorf("sent StaticCode for %s = %v (only the configured entry may carry it)", id.Identifier, id.StaticCode)
				}
			}
			if bad := mismatchesUnder(planned, resp.State.Raw, ".privacy_preferences"); len(bad) > 0 {
				t.Errorf("Provider produced inconsistent result after apply: %v", bad)
			}
		})
	}
}

// --- restrictions ---

func restrictionsCameraConfig(t *testing.T, camera bool) tftypes.Value {
	t.Helper()
	rt := topAttrType(t, "restrictions")
	at := subType(t, rt, "applications")
	cameraVal := nestedObj(t, subType(t, at, "camera"), map[string]tftypes.Value{"allow_use_of_built_in_camera": boolVal(camera)})
	return nestedObj(t, rt, map[string]tftypes.Value{
		"applications": nestedObj(t, at, map[string]tftypes.Value{"camera": cameraVal}),
	})
}

var restrictionsSiblingPaths = []string{
	".restrictions.widgets", ".restrictions.desktop", ".restrictions.functionality", ".restrictions.media",
	".restrictions.preferences", ".restrictions.sharing", ".restrictions.applications.safari",
	".restrictions.applications.app_store", ".restrictions.applications.apple_music",
	".restrictions.applications.game_centre",
}

// Live case 8: restrictions = { applications = { camera = {...} } } only.
func TestCreate_Restrictions_ServerFilledSiblingSubBlocks(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{"restrictions": restrictionsCameraConfig(t, false)})
	planned, applied, resp, uem := planAndCreateSections(t, config, nil, map[string]any{"Restrictions": uemFilledRestrictions()})
	requireNoDiagErrors(t, resp)

	pr := objAttr(t, planned, "restrictions")
	for _, p := range []string{"widgets", "desktop", "functionality", "media", "preferences", "sharing"} {
		if objAttr(t, pr, p).IsKnown() {
			t.Errorf("planned restrictions.%s is known, want unknown (UEM fills it)", p)
		}
	}
	// Faithful send: only the configured camera goes out.
	rs := mapOf(t, uem.sent["Restrictions"])
	for _, p := range []string{"Widgets", "Desktop", "Functionality", "Media", "Preferences", "Sharing"} {
		if v, present := rs[p]; present {
			t.Errorf("sent Restrictions.%s = %v, want it omitted (not configured)", p, v)
		}
	}
	apps := mapOf(t, rs["Applications"])
	if _, ok := apps["Camera"]; !ok {
		t.Error("configured Applications.Camera not sent")
	}
	for _, p := range []string{"Safari", "AppStore", "AppleMusic", "GameCentre"} {
		if v, present := apps[p]; present {
			t.Errorf("sent Applications.%s = %v, want it omitted", p, v)
		}
	}
	// UEM's filled sub-blocks land in state.
	ar := objAttr(t, applied, "restrictions")
	for _, p := range []string{"widgets", "desktop", "functionality", "media", "preferences", "sharing"} {
		if objAttr(t, ar, p).IsNull() {
			t.Errorf("state restrictions.%s is null, want UEM's filled sub-block", p)
		}
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// The configured camera value, and a configured leaf inside a configured
// sub-block, still surface when UEM changes them.
func TestCreate_Restrictions_ConfiguredValueChangedByUEMStillSurfaces(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{"restrictions": restrictionsCameraConfig(t, true)})
	planned, applied, resp, _ := planAndCreateSections(t, config, nil, map[string]any{"Restrictions": uemFilledRestrictions()})
	requireNoDiagErrors(t, resp)
	bad := mismatchesUnder(planned, applied, ".restrictions")
	if len(bad) != 1 || bad[0] != ".restrictions.applications.camera.allow_use_of_built_in_camera" {
		t.Fatalf("mismatches = %v, want exactly [.restrictions.applications.camera.allow_use_of_built_in_camera]", bad)
	}
}

// A sub-block configured with one leaf: UEM fills the sibling leaves.
func TestCreate_Restrictions_PartialSubBlockLeavesFilled(t *testing.T) {
	t.Parallel()
	rt := topAttrType(t, "restrictions")
	ft := subType(t, rt, "functionality")
	passwords := nestedObj(t, subType(t, ft, "passwords"), map[string]tftypes.Value{"allow_password_auto_fill": boolVal(false)})
	config := createConfigWith(t, map[string]tftypes.Value{"restrictions": nestedObj(t, rt, map[string]tftypes.Value{
		"functionality": nestedObj(t, ft, map[string]tftypes.Value{"passwords": passwords}),
	})})
	planned, applied, resp, uem := planAndCreateSections(t, config, nil, map[string]any{"Restrictions": uemFilledRestrictions()})
	requireNoDiagErrors(t, resp)
	pw := mapOf(t, mapOf(t, mapOf(t, uem.sent["Restrictions"])["Functionality"])["Passwords"])
	if len(pw) != 1 {
		t.Errorf("sent Passwords = %v, want only the configured leaf", pw)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// Update of a profile whose state holds UEM's filled sub-blocks (a Create's
// result): unset siblings are carried and sent back unchanged.
func TestUpdate_Restrictions_FilledSiblingsCarried(t *testing.T) {
	t.Parallel()
	createCfg := createConfigWith(t, map[string]tftypes.Value{"restrictions": restrictionsCameraConfig(t, false)})
	_, applied, resp, _ := planAndCreateSections(t, createCfg, nil, map[string]any{"Restrictions": uemFilledRestrictions()})
	requireNoDiagErrors(t, resp)
	prior := appliedAsPrior(applied)

	planned := planProfileValue(t, prior, updateConfigWith(t, map[string]tftypes.Value{"restrictions": restrictionsCameraConfig(t, false)}))
	pr := objAttr(t, planned, "restrictions")
	for _, p := range []string{"widgets", "desktop", "functionality", "media", "preferences", "sharing"} {
		if !objAttr(t, pr, p).IsKnown() {
			t.Errorf("planned restrictions.%s is unknown, want the state value carried", p)
		}
	}
	fake := &updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) { e.Restrictions = uemFilledRestrictions() }}
	r := runUpdate(t, fake, prior, planned)
	sent := sentEntity(t, fake.updateEntity)
	if sent.Restrictions == nil || sent.Restrictions.Widgets == nil || sent.Restrictions.Applications == nil || sent.Restrictions.Applications.Safari == nil {
		t.Fatalf("carried siblings not sent: %+v", sent.Restrictions)
	}
	if bad := mismatchesUnder(planned, r.State.Raw, ".restrictions"); len(bad) > 0 {
		t.Errorf("Provider produced inconsistent result after apply: %v", bad)
	}
	_ = restrictionsSiblingPaths
}

// Adding restrictions to a profile that has none: the sub-blocks plan unknown
// (the plan decode must cope) and the update is consistent.
func TestUpdate_AddingRestrictions_ServerFilledSiblings(t *testing.T) {
	t.Parallel()
	prior := createResourceState(t, rmwBasePlanValues(sdk.PlatformAppleOsX)).Raw
	planned := planProfileValue(t, prior, updateConfigWith(t, map[string]tftypes.Value{"restrictions": restrictionsCameraConfig(t, false)}))
	if objAttr(t, objAttr(t, planned, "restrictions"), "widgets").IsKnown() {
		t.Fatal("planned restrictions.widgets is known on an add, want unknown")
	}
	fake := &updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) { e.Restrictions = uemFilledRestrictions() }}
	r := runUpdate(t, fake, prior, planned)
	sent := sentEntity(t, fake.updateEntity)
	if sent.Restrictions == nil || sent.Restrictions.Widgets != nil || sent.Restrictions.Desktop != nil {
		t.Errorf("an unset sub-block was sent on an add: %+v", sent.Restrictions)
	}
	if bad := mismatchesUnder(planned, r.State.Raw, ".restrictions"); len(bad) > 0 {
		t.Errorf("Provider produced inconsistent result after apply: %v", bad)
	}
}

// UEM omits the Desktop sub-block (it is only returned when
// lock_desktop_picture is true): the provider restores the planned Desktop, and
// the Computed desktop_picture_path the user did not set must not stay unknown
// in the applied state.
func TestCreate_Restrictions_DesktopOmittedByUEMLeavesNoUnknown(t *testing.T) {
	t.Parallel()
	rt := topAttrType(t, "restrictions")
	dt := subType(t, rt, "desktop")
	config := createConfigWith(t, map[string]tftypes.Value{"restrictions": nestedObj(t, rt, map[string]tftypes.Value{
		"desktop": nestedObj(t, dt, map[string]tftypes.Value{"lock_desktop_picture": boolVal(false)}),
	})})
	answer := uemFilledRestrictions()
	answer.Desktop = nil
	planned, applied, resp, _ := planAndCreateSections(t, config, nil, map[string]any{"Restrictions": answer})
	requireNoDiagErrors(t, resp)
	if !applied.IsFullyKnown() {
		t.Fatal("applied state still holds an unknown value (Terraform core rejects that)")
	}
	if got := objAttr(t, objAttr(t, objAttr(t, applied, "restrictions"), "desktop"), "desktop_picture_path"); !got.IsNull() {
		t.Errorf("state desktop_picture_path = %v, want null", got)
	}
	assertAppliedMatchesPlan(t, planned, applied)
}

// Every true/false, number and string attribute under restrictions is
// Optional+Computed; only the list attributes are left alone.
func TestSchema_RestrictionsLeavesAreServerFilled(t *testing.T) {
	t.Parallel()
	var walk func(path string, attrs map[string]schema.Attribute)
	walk = func(path string, attrs map[string]schema.Attribute) {
		for name, a := range attrs {
			p := path + "." + name
			switch v := a.(type) {
			case schema.ListAttribute:
				// left unchanged on purpose
			case schema.SingleNestedAttribute:
				if !v.Optional || !v.Computed {
					t.Errorf("%s: want Optional+Computed", p)
				}
				walk(p, v.Attributes)
			default:
				if !a.IsOptional() || !a.IsComputed() {
					t.Errorf("%s (%T): want Optional+Computed", p, a)
				}
			}
		}
	}
	r, ok := getResourceSchema(t).Schema.Attributes["restrictions"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("restrictions is not a SingleNestedAttribute")
	}
	walk("restrictions", r.Attributes)
}
