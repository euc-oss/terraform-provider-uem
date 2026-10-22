package smartgroup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestMain(m *testing.M) {
	// Read/Update not-found handling goes through notfound.Confirm; no
	// wall-clock wait in unit tests.
	notfound.Delay = 0
	os.Exit(m.Run())
}

// --- schema/value helpers: every attribute defaults to null except the
// overrides given, like the data source tests' buildConfig. ---

func getRSSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	(&SmartGroupResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp
}

func rsRaw(t *testing.T, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	objType, ok := getRSSchema(t).Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range overrides {
		values[name] = v
	}
	return tftypes.NewValue(objType, values)
}

func rsState(t *testing.T, overrides map[string]tftypes.Value) tfsdk.State {
	return tfsdk.State{Schema: getRSSchema(t).Schema, Raw: rsRaw(t, overrides)}
}

func rsConfig(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	return tfsdk.Config{Schema: getRSSchema(t).Schema, Raw: rsRaw(t, overrides)}
}

func rsPlan(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Plan {
	return tfsdk.Plan{Schema: getRSSchema(t).Schema, Raw: rsRaw(t, overrides)}
}

func str(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }

func strSet(vals ...string) tftypes.Value {
	elems := make([]tftypes.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, str(v))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elems)
}

var osTFType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"device_type": tftypes.String, "operator": tftypes.String, "value": tftypes.String,
}}

func osSet(rows ...[3]string) tftypes.Value {
	elems := make([]tftypes.Value, 0, len(rows))
	for _, r := range rows {
		elems = append(elems, tftypes.NewValue(osTFType, map[string]tftypes.Value{
			"device_type": str(r[0]), "operator": str(r[1]), "value": str(r[2]),
		}))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: osTFType}, elems)
}

func stateModel(t *testing.T, s tfsdk.State) SmartGroupResourceModel {
	t.Helper()
	var m SmartGroupResourceModel
	if d := s.Get(context.Background(), &m); d.HasError() {
		t.Fatalf("state get: %v", d)
	}
	return m
}

// --- fake SmartGroupsService ---

type fakeSmartGroupService struct {
	fakeSmartGroupSearch

	mu        sync.Mutex
	loads     []loadAnswer // answered in call order; the last one repeats
	loadCalls int
	updated   *sdk.SmartGroupEditV1Model
	deleted   int
	deleteErr error
}

type loadAnswer struct {
	group *sdk.SmartGroupV1
	err   error
}

func (f *fakeSmartGroupService) LoadSmartGroupAsync(_ context.Context, _ int) (http.Header, *sdk.SmartGroupV1, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.loads[min(f.loadCalls, len(f.loads)-1)]
	f.loadCalls++
	return nil, a.group, a.err
}

func (f *fakeSmartGroupService) CreateSmartGroupAsync(context.Context, *sdk.SmartGroupEditV1Model) (http.Header, *sdk.SmartGroupCreateResponseV1, error) {
	v := int64(5)
	return nil, &sdk.SmartGroupCreateResponseV1{Value: &v}, nil
}

func (f *fakeSmartGroupService) UpdateSmartGroupAsync(_ context.Context, _ int, req *sdk.SmartGroupEditV1Model) (http.Header, error) {
	f.updated = req
	return nil, nil
}

func (f *fakeSmartGroupService) DeleteAsync(_ context.Context, id int) (http.Header, error) {
	f.deleted = id
	return nil, f.deleteErr
}

func group(mut func(*sdk.SmartGroupV1)) *sdk.SmartGroupV1 {
	id := 5
	g := &sdk.SmartGroupV1{
		SmartGroupID:                   &id,
		SmartGroupUUID:                 "9af645a8-fef3-3e6d-3408-5cc69e0937d4",
		Name:                           "sg",
		ManagedByOrganizationGroupID:   "7",
		ManagedByOrganizationGroupName: "OG",
		ManagedByOrganizationGroupUUID: "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c",
		CriteriaType:                   "All",
	}
	if mut != nil {
		mut(g)
	}
	return g
}

// readWith runs Read against prior state and one server answer.
func readWith(t *testing.T, prior map[string]tftypes.Value, answers ...loadAnswer) (*resource.ReadResponse, *fakeSmartGroupService) {
	t.Helper()
	base := map[string]tftypes.Value{
		"id": str("5"), "name": str("sg"), "managed_by_org_group_id": str("7"), "criteria_type": str("All"),
	}
	for k, v := range prior {
		base[k] = v
	}
	svc := &fakeSmartGroupService{loads: answers}
	r := &SmartGroupResource{svc: svc}
	st := rsState(t, base)
	resp := &resource.ReadResponse{State: st}
	r.Read(context.Background(), resource.ReadRequest{State: st}, resp)
	return resp, svc
}

// 1. Regression check: reverting reconcileOwnerships' known-default mapping
// makes the unset case read back as {"allownerships"} (a perpetual diff
// against the null config); reverting to raw server values breaks the
// explicit-casing cases.
func TestSmartGroupRead_AllOwnershipsServerDefaultIsNullWhenUnset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	resp, _ := readWith(t, nil, loadAnswer{group: group(func(g *sdk.SmartGroupV1) { g.Ownerships = []string{"allownerships"} })})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if got := stateModel(t, resp.State).Ownerships; !got.IsNull() {
		t.Fatalf("unset ownerships with server default [allownerships] must read back null, got %s", got)
	}

	for _, tc := range []struct {
		name       string
		configured []string
		server     []string
	}{
		{"explicit value reads back verbatim", []string{"CorporateDedicated"}, []string{"corporatededicated"}},
		{"explicit AllOwnerships is not special-cased", []string{"AllOwnerships"}, []string{"allownerships"}},
	} {
		resp, _ := readWith(t, map[string]tftypes.Value{"ownerships": strSet(tc.configured...)},
			loadAnswer{group: group(func(g *sdk.SmartGroupV1) { g.Ownerships = tc.server })})
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: read: %v", tc.name, resp.Diagnostics)
		}
		// With prior non-null, reconcileOwnerships' default mapping does not
		// apply, and ownerships is read back exactly as the server returned it
		// (reconcileSetVerbatim) -- there is no live evidence the ownerships
		// endpoint is case-insensitive, so the configured casing is never
		// substituted back in.
		want, _ := types.SetValueFrom(ctx, types.StringType, tc.server)
		if got := stateModel(t, resp.State).Ownerships; !got.Equal(want) {
			t.Errorf("%s: ownerships = %s, want %s (verbatim, case-folding must not be reintroduced)", tc.name, got, want)
		}
	}
}

// 2. Regression check: set semantics. The server returns id-list sets
// (tag_ids, user_exclusions) re-ordered; state must equal the prior values
// exactly regardless of order, because Set attributes are order-independent.
// Switching either attribute to a list fails this.
func TestSmartGroupRead_ReorderedIDSetsProduceNoDiff(t *testing.T) {
	t.Parallel()
	prior := map[string]tftypes.Value{
		"criteria_type":   str("All"),
		"tag_ids":         strSet("1", "2"),
		"user_exclusions": strSet("10", "11"),
	}
	resp, _ := readWith(t, prior, loadAnswer{group: group(func(g *sdk.SmartGroupV1) {
		g.Tags = []sdk.SmartGroupTagV1{{ID: "2", Name: "b"}, {ID: "1", Name: "a"}}
		g.UserExclusions = []sdk.SmartGroupUserV1{{ID: "11"}, {ID: "10"}}
	})})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	for name, want := range prior {
		got, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		gotVal, ok := got.(tftypes.Value)
		if !ok {
			t.Fatalf("%s: state step is %T, not a tftypes.Value", name, got)
		}
		if !want.Equal(gotVal) {
			t.Errorf("%s: state %s differs from prior %s (would be a plan diff)", name, got, want)
		}
	}
}

// 2b. Decided/faithful: criteria_type, platforms, models, management_types,
// enrollment_categories, cpu_architectures and operating_systems are read
// back exactly as UEM returns them, with no case reconciliation against the
// prior (plan/state) casing. Live evidence: UEM echoes back the configured
// casing on read, and a lowercase platform sent on update returned HTTP 500,
// so the provider must not paper over a casing mismatch. This is the revert
// check: if case-folding (preserveCase / the casing map in reconcileStringSet
// or reconcileOperatingSystems) is reintroduced for these attributes, this
// test fails because it would then expect the prior's casing instead of the
// server's.
func TestSmartGroupRead_CriteriaAttributesReadBackVerbatim(t *testing.T) {
	t.Parallel()
	prior := map[string]tftypes.Value{
		"criteria_type":         str("All"),
		"platforms":             strSet("AppleOsX"),
		"models":                strSet("IPad"),
		"management_types":      strSet("MdmEnrolled"),
		"enrollment_categories": strSet("DepEnrolled"),
		"cpu_architectures":     strSet("Arm64"),
		"operating_systems":     osSet([3]string{"AppleOsX", "GreaterThan", "14.0"}),
	}
	resp, _ := readWith(t, prior, loadAnswer{group: group(func(g *sdk.SmartGroupV1) {
		g.CriteriaType = "all"
		g.Platforms = []string{"appleosx"}
		g.Models = []string{"ipad"}
		g.ManagementTypes = []string{"mdmenrolled"}
		g.EnrollmentCategories = []string{"depenrolled"}
		g.CpuArchitectures = []string{"arm64"}
		g.OperatingSystems = []sdk.SmartGroupOperatingSystemV1{{DeviceType: "appleosx", Operator: "greaterthan", Value: "14.0"}}
	})})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	want := map[string]tftypes.Value{
		"criteria_type":         str("all"),
		"platforms":             strSet("appleosx"),
		"models":                strSet("ipad"),
		"management_types":      strSet("mdmenrolled"),
		"enrollment_categories": strSet("depenrolled"),
		"cpu_architectures":     strSet("arm64"),
		"operating_systems":     osSet([3]string{"appleosx", "greaterthan", "14.0"}),
	}
	for name, wantVal := range want {
		got, err := resp.State.Raw.ApplyTerraform5AttributePathStep(tftypes.AttributeName(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		gotVal, ok := got.(tftypes.Value)
		if !ok {
			t.Fatalf("%s: state step is %T, not a tftypes.Value", name, got)
		}
		if !wantVal.Equal(gotVal) {
			t.Errorf("%s: state %s, want the server's exact casing %s (case-folding must not be reintroduced)", name, got, wantVal)
		}
	}
}

func validate(t *testing.T, cfg map[string]tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()
	base := map[string]tftypes.Value{"name": str("sg"), "managed_by_org_group_id": str("7")}
	for k, v := range cfg {
		base[k] = v
	}
	resp := &resource.ValidateConfigResponse{}
	(&SmartGroupResource{}).ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: rsConfig(t, base)}, resp)
	return resp
}

func hasAttrError(resp *resource.ValidateConfigResponse, attr string) bool {
	for _, d := range resp.Diagnostics.Errors() {
		if p, ok := d.(interface{ Path() path.Path }); ok && p.Path().Equal(path.Root(attr)) {
			return true
		}
	}
	return false
}

// 3. Decided: the "additions only valid with UserDevice" cross-field rule
// was removed (unconfirmed against live UEM behaviour); user_additions and
// device_additions alongside criteria_type "All" must pass ValidateConfig
// with no error now, and let UEM validate the combination at apply.
// Reintroducing the check fails this.
func TestSmartGroupValidateConfig_AdditionsWithAllIsNotValidatedByProvider(t *testing.T) {
	t.Parallel()
	for _, attr := range []string{"user_additions", "device_additions"} {
		resp := validate(t, map[string]tftypes.Value{"criteria_type": str("All"), "platforms": strSet("AppleOsX"), attr: strSet("1")})
		if hasAttrError(resp, attr) {
			t.Errorf("%s with criteria_type All: want no provider-side error (pass-through to UEM), got %v", attr, resp.Diagnostics)
		}
	}
	// The exclusion sets are valid with All.
	if resp := validate(t, map[string]tftypes.Value{"criteria_type": str("All"), "platforms": strSet("AppleOsX"), "user_exclusions": strSet("1")}); resp.Diagnostics.HasError() {
		t.Errorf("user_exclusions with All must be valid, got %v", resp.Diagnostics)
	}
}

// 4. Decided: the "criteria only valid with All" cross-field rule was
// removed (unconfirmed against live UEM behaviour); a criterion alongside
// criteria_type "UserDevice" must pass ValidateConfig with no error now, and
// let UEM validate the combination at apply. Reintroducing the check fails
// this.
func TestSmartGroupValidateConfig_CriteriaWithUserDeviceIsNotValidatedByProvider(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		attr string
		val  tftypes.Value
	}{
		{"platforms", strSet("AppleOsX")},
		{"tag_ids", strSet("3")},
		{"operating_systems", osSet([3]string{"AppleOsX", "GreaterThan", "14.0"})},
	} {
		resp := validate(t, map[string]tftypes.Value{"criteria_type": str("UserDevice"), "user_additions": strSet("1"), tc.attr: tc.val})
		if hasAttrError(resp, tc.attr) {
			t.Errorf("%s with criteria_type UserDevice: want no provider-side error (pass-through to UEM), got %v", tc.attr, resp.Diagnostics)
		}
	}
	if resp := validate(t, map[string]tftypes.Value{"criteria_type": str("UserDevice"), "device_additions": strSet("1")}); resp.Diagnostics.HasError() {
		t.Errorf("device_additions with UserDevice must be valid, got %v", resp.Diagnostics)
	}
}

// 5. Regression check: "All" with no criteria is a WARNING about matching
// every device, never an error. Removing the warning, or turning it into an
// error, fails this.
func TestSmartGroupValidateConfig_AllWithNoCriteriaWarns(t *testing.T) {
	t.Parallel()
	resp := validate(t, map[string]tftypes.Value{"criteria_type": str("All")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("want no error, got %v", resp.Diagnostics)
	}
	warnings := resp.Diagnostics.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0].Detail(), "EVERY device") {
		t.Fatalf("want one warning about matching every device, got %v", resp.Diagnostics)
	}
	if resp := validate(t, map[string]tftypes.Value{"criteria_type": str("All"), "platforms": strSet("AppleOsX")}); len(resp.Diagnostics) != 0 {
		t.Errorf("All with a criterion must not warn, got %v", resp.Diagnostics)
	}
}

func TestSmartGroupValidateConfig_NonNumericOrgGroupIsError(t *testing.T) {
	t.Parallel()
	resp := validate(t, map[string]tftypes.Value{"criteria_type": str("All"), "platforms": strSet("AppleOsX"), "managed_by_org_group_id": str("abc")})
	if !hasAttrError(resp, "managed_by_org_group_id") {
		t.Fatalf("want an attribute error, got %v", resp.Diagnostics)
	}
}

// 6. Regression check: import by UUID resolves through the shared hardened
// walk (a multi-page, double-walked search here) and matches the stored uuid
// case-insensitively. A case-sensitive compare, or a walk that stops after
// page 0, fails this.
func TestSmartGroupImportState_UUIDCaseInsensitiveViaSharedWalk(t *testing.T) {
	t.Parallel()
	total := 3
	svc := &fakeSmartGroupService{fakeSmartGroupSearch: fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "a", "98ae6a26-50c7-926b-2018-a9d778bb2b8a"), sg(2, "b", "5b34837c-d60d-e277-ba49-af2b9f170279")}, 0, 2, &total),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(42, "c", "5927dbb3-765a-5914-ecb9-5d06939655d9")}, 1, 2, &total),
	}}}
	r := &SmartGroupResource{svc: svc, pageSize: 2}
	resp := &resource.ImportStateResponse{State: rsState(t, nil)}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "5927dbb3-765a-5914-ecb9-5d06939655d9"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	if got := stateModel(t, resp.State).ID.ValueString(); got != "42" {
		t.Fatalf("id = %q, want 42", got)
	}
	// Two independent walks of two pages each: the shared double-walk ran.
	if len(svc.pagesRequested) != 4 {
		t.Errorf("want the hardened double walk (4 page requests), got %v", svc.pagesRequested)
	}

	// A numeric id imports directly, with no search.
	svc2 := &fakeSmartGroupService{}
	resp = &resource.ImportStateResponse{State: rsState(t, nil)}
	(&SmartGroupResource{svc: svc2}).ImportState(context.Background(), resource.ImportStateRequest{ID: "17"}, resp)
	if resp.Diagnostics.HasError() || stateModel(t, resp.State).ID.ValueString() != "17" || len(svc2.pagesRequested) != 0 {
		t.Errorf("numeric import: diags=%v pages=%v", resp.Diagnostics, svc2.pagesRequested)
	}

	// Neither shape is an error.
	resp = &resource.ImportStateResponse{State: rsState(t, nil)}
	(&SmartGroupResource{svc: svc2}).ImportState(context.Background(), resource.ImportStateRequest{ID: "sg-name"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("a non-numeric, non-uuid import id must be an error")
	}
}

// 7. Regression check: a confirmed not-found on Read removes the resource
// from state instead of erroring. Dropping the client.IsNotFound branch makes
// Read error instead.
func TestSmartGroupRead_NotFoundRemovesResource(t *testing.T) {
	t.Parallel()
	nf := &sdk.APIError{StatusCode: http.StatusBadRequest, ErrorCode: "6", Message: "Smart Group not found with Id: 5 or User does not have access to it."}
	resp, svc := readWith(t, nil, loadAnswer{err: nf})
	if resp.Diagnostics.HasError() {
		t.Fatalf("want no error, got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("want the resource removed from state")
	}
	if svc.loadCalls != 2 {
		t.Errorf("want the not-found confirmed with a second load, got %d loads", svc.loadCalls)
	}

	// A different error is a real error and keeps state.
	resp, _ = readWith(t, nil, loadAnswer{err: &sdk.APIError{StatusCode: http.StatusInternalServerError, Message: "boom"}})
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 500 must be an error")
	}
}

// 8. Regression check: the counters are Computed-only (no Optional, no plan
// modifier) and follow the server, while every configurable attribute is
// unchanged by a count-only drift. Adding UseStateForUnknown or making them
// configurable fails the schema half; mapping them into anything else fails
// the read half.
func TestSmartGroupRead_CountDriftProducesNoDiff(t *testing.T) {
	t.Parallel()
	s := getRSSchema(t).Schema
	for _, name := range []string{"devices_count", "assignments_count", "exclusions_count"} {
		a := s.Attributes[name]
		if !a.IsComputed() || a.IsOptional() || a.IsRequired() {
			t.Errorf("%s must be Computed only", name)
		}
		ia, ok := a.(schema.Int64Attribute)
		if !ok || len(ia.PlanModifiers) > 0 {
			t.Errorf("%s must be an Int64Attribute with no plan modifiers", name)
		}
	}

	prior := map[string]tftypes.Value{"platforms": strSet("AppleOsX")}
	answer := func(devices int) loadAnswer {
		return loadAnswer{group: group(func(g *sdk.SmartGroupV1) {
			g.Platforms = []string{"AppleOsX"}
			g.Devices, g.Assignments, g.Exclusions = &devices, &devices, &devices
		})}
	}
	first, _ := readWith(t, prior, answer(3))
	second, _ := readWith(t, prior, answer(7))
	a, b := stateModel(t, first.State), stateModel(t, second.State)
	if a.DevicesCount.ValueInt64() != 3 || b.DevicesCount.ValueInt64() != 7 || b.AssignmentsCount.ValueInt64() != 7 || b.ExclusionsCount.ValueInt64() != 7 {
		t.Fatalf("counts not refreshed: %v / %v", a.DevicesCount, b.DevicesCount)
	}
	a.DevicesCount, a.AssignmentsCount, a.ExclusionsCount = b.DevicesCount, b.AssignmentsCount, b.ExclusionsCount
	if !a.Platforms.Equal(b.Platforms) || !a.Name.Equal(b.Name) || !a.CriteriaType.Equal(b.CriteriaType) || !a.Ownerships.Equal(b.Ownerships) {
		t.Errorf("a count-only drift changed a configurable attribute: %+v vs %+v", a, b)
	}
}

func TestSmartGroupRead_OEMAndModelsIsWarningNotError(t *testing.T) {
	t.Parallel()
	resp, _ := readWith(t, nil, loadAnswer{group: group(func(g *sdk.SmartGroupV1) {
		g.OEMAndModels = []sdk.OEMAndModelV1{{OEM: &sdk.OEMV1{ID: "1", Name: "Apple"}}}
	})})
	if resp.Diagnostics.HasError() || len(resp.Diagnostics.Warnings()) != 1 {
		t.Fatalf("want exactly one warning, got %v", resp.Diagnostics)
	}
}

// GAP-1: the real SDK request body for an Update where platforms goes from
// ["AppleOsX"] to [] and tag_ids from ["3"] to null. Every edit-model slice is
// omitempty, so neither field is sent at all -- not as [] and not as null --
// while the fields still set are. The update also carries the server's
// OEMAndModels through. If the SDK ever starts sending `"Platforms":[]`,
// this fails and the buildEditModel doc comment must be revisited.
func TestBuildEditModel_EmptiedSetIsOmittedFromPUTBody(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var putBody []byte
	serverGroup := `{"SmartGroupID":5,"SmartGroupUuid":"9af645a8-fef3-3e6d-3408-5cc69e0937d4","Name":"sg","ManagedByOrganizationGroupId":"7",` +
		`"CriteriaType":"All","Platforms":["AppleOsX"],"Tags":[{"Id":"3"}],"Ownerships":["CorporateDedicated"],` +
		`"OEMAndModels":[{"OEM":{"Id":"1","Name":"Apple"}}]}`
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			putBody = b
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, serverGroup)
	})
	defer server.Close()

	r := &SmartGroupResource{svc: sdk.NewSmartGroupsService(c)}
	common := map[string]tftypes.Value{
		"id": str("5"), "name": str("sg"), "managed_by_org_group_id": str("7"), "criteria_type": str("All"),
		"ownerships": strSet("CorporateDedicated"),
	}
	stateVals := map[string]tftypes.Value{"platforms": strSet("AppleOsX"), "tag_ids": strSet("3")}
	planVals := map[string]tftypes.Value{"platforms": strSet()}
	for k, v := range common {
		stateVals[k], planVals[k] = v, v
	}
	resp := &resource.UpdateResponse{State: rsState(t, stateVals)}
	r.Update(context.Background(), resource.UpdateRequest{Plan: rsPlan(t, planVals), State: rsState(t, stateVals)}, resp)
	// The fake server still returns Platforms/Tags after the PUT, so the
	// re-read reports them; only the request body matters here.

	mu.Lock()
	defer mu.Unlock()
	if putBody == nil {
		t.Fatalf("no PUT captured; diags: %v", resp.Diagnostics)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(putBody, &body); err != nil {
		t.Fatalf("PUT body %s: %v", putBody, err)
	}
	for _, absent := range []string{"Platforms", "Tags"} {
		if v, ok := body[absent]; ok {
			t.Errorf("%s was emptied in the plan; want it omitted from the PUT body, got %s", absent, v)
		}
	}
	if string(body["Ownerships"]) != `["CorporateDedicated"]` {
		t.Errorf("Ownerships = %s, want [\"CorporateDedicated\"]", body["Ownerships"])
	}
	if !strings.Contains(string(body["OEMAndModels"]), `"Apple"`) {
		t.Errorf("OEMAndModels not carried through the PUT: %s", putBody)
	}
}

func TestSmartGroupCreate_ReadsBackAfterCreate(t *testing.T) {
	t.Parallel()
	svc := &fakeSmartGroupService{loads: []loadAnswer{{group: group(func(g *sdk.SmartGroupV1) {
		g.Platforms = []string{"appleosx"}
		g.Ownerships = []string{"AllOwnerships"}
	})}}}
	plan := rsPlan(t, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "uuid": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name": str("sg"), "managed_by_org_group_id": str("7"), "criteria_type": str("All"), "platforms": strSet("AppleOsX"),
	})
	resp := &resource.CreateResponse{State: rsState(t, nil)}
	(&SmartGroupResource{svc: svc}).Create(context.Background(), resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: %v", resp.Diagnostics)
	}
	m := stateModel(t, resp.State)
	// platforms is read back verbatim (the server's "appleosx", not the
	// configured "AppleOsX"); ownerships keeps its own reconcileOwnerships
	// default handling and reads back as unset for "AllOwnerships".
	want, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"appleosx"})
	if m.ID.ValueString() != "5" || m.UUID.ValueString() == "" || !m.Platforms.Equal(want) || !m.Ownerships.IsNull() {
		t.Errorf("unexpected state after create: %+v", m)
	}
}

func TestSmartGroupDelete_NotFoundIsSuccess(t *testing.T) {
	t.Parallel()
	svc := &fakeSmartGroupService{deleteErr: &sdk.APIError{StatusCode: http.StatusNotFound}}
	st := rsState(t, map[string]tftypes.Value{"id": str("5")})
	resp := &resource.DeleteResponse{State: st}
	(&SmartGroupResource{svc: svc}).Delete(context.Background(), resource.DeleteRequest{State: st}, resp)
	if resp.Diagnostics.HasError() || svc.deleted != 5 {
		t.Fatalf("diags=%v deleted=%d", resp.Diagnostics, svc.deleted)
	}
}

// --- ModifyPlan: clearing a criterion/member/exclusion set is live-confirmed
// broken (live run 2: clearing enrollment_categories left UEM at
// ["DepEnrolled"] and produced an inconsistent-result error), so ModifyPlan
// must catch a non-empty-to-empty(or null) transition at plan time. ---

// rsNullState builds a wholly null resource state/object, the shape Terraform
// uses for a resource that does not exist yet (Create) or is being destroyed.
func rsNullState(t *testing.T) tfsdk.State {
	t.Helper()
	objType, ok := getRSSchema(t).Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	return tfsdk.State{Schema: getRSSchema(t).Schema, Raw: tftypes.NewValue(objType, nil)}
}

func runModifyPlan(t *testing.T, state tfsdk.State, planOverrides map[string]tftypes.Value) *resource.ModifyPlanResponse {
	t.Helper()
	plan := rsPlan(t, planOverrides)
	req := resource.ModifyPlanRequest{State: state, Plan: plan, Config: tfsdk.Config(plan)}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	(&SmartGroupResource{}).ModifyPlan(context.Background(), req, resp)
	return resp
}

// 9. Non-empty (state) -> empty (plan): errors, naming the attribute.
func TestSmartGroupModifyPlan_NonEmptyToEmptyIsError(t *testing.T) {
	t.Parallel()
	state := rsState(t, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet("AppleOsX", "WindowsPhone")})
	resp := runModifyPlan(t, state, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet()})
	if !hasAttrError(&resource.ValidateConfigResponse{Diagnostics: resp.Diagnostics}, "platforms") {
		t.Fatalf("platforms non-empty -> empty: want an attribute error, got %v", resp.Diagnostics)
	}
}

// 10. Non-empty (state) -> null (plan): errors the same way.
func TestSmartGroupModifyPlan_NonEmptyToNullIsError(t *testing.T) {
	t.Parallel()
	state := rsState(t, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet("AppleOsX")})
	resp := runModifyPlan(t, state, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All")}) // platforms defaults to null
	if !hasAttrError(&resource.ValidateConfigResponse{Diagnostics: resp.Diagnostics}, "platforms") {
		t.Fatalf("platforms non-empty -> null: want an attribute error, got %v", resp.Diagnostics)
	}
}

// 11. Empty (state, an explicit []) -> empty (plan): no error.
func TestSmartGroupModifyPlan_EmptyToEmptyIsNoError(t *testing.T) {
	t.Parallel()
	state := rsState(t, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet()})
	resp := runModifyPlan(t, state, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet()})
	if resp.Diagnostics.HasError() {
		t.Fatalf("empty -> empty: want no error, got %v", resp.Diagnostics)
	}
}

// 12. Create (no prior state): never errors, whatever the plan holds.
func TestSmartGroupModifyPlan_CreateIsNoError(t *testing.T) {
	t.Parallel()
	resp := runModifyPlan(t, rsNullState(t), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "criteria_type": str("All"), "platforms": strSet("AppleOsX"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: want no error, got %v", resp.Diagnostics)
	}
}

// 13. Shrinking a set but keeping it non-empty: no error (only a clear to
// empty/null is caught).
func TestSmartGroupModifyPlan_ShrunkButNonEmptyIsNoError(t *testing.T) {
	t.Parallel()
	state := rsState(t, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet("AppleOsX", "WindowsPhone")})
	resp := runModifyPlan(t, state, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("All"), "platforms": strSet("AppleOsX")})
	if resp.Diagnostics.HasError() {
		t.Fatalf("shrunk but non-empty: want no error, got %v", resp.Diagnostics)
	}
}

// 14. Every set attribute ModifyPlan is supposed to guard is actually
// checked, not just platforms: a table over smartGroupSetAttributes. Removing
// an entry from that list, or breaking the loop, fails this.
func TestSmartGroupModifyPlan_EveryGuardedAttributeIsChecked(t *testing.T) {
	t.Parallel()
	for _, name := range smartGroupSetAttributes {
		var stateVal, planVal tftypes.Value
		if name == "operating_systems" {
			stateVal = osSet([3]string{"AppleOsX", "GreaterThan", "14.0"})
			planVal = osSet()
		} else {
			stateVal = strSet("1")
			planVal = strSet()
		}
		state := rsState(t, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("UserDevice"), name: stateVal})
		resp := runModifyPlan(t, state, map[string]tftypes.Value{"id": str("5"), "criteria_type": str("UserDevice"), name: planVal})
		if !hasAttrError(&resource.ValidateConfigResponse{Diagnostics: resp.Diagnostics}, name) {
			t.Errorf("%s: non-empty -> empty must be an attribute error, got %v", name, resp.Diagnostics)
		}
	}
}

// B30: a uuid that UEM's search doesn't return (live: an organization
// group's own smart group) fails with an error that explains the search gap
// and points at the numeric-id import that does work.
func TestSmartGroupImportState_UUIDNotInSearchPointsToNumericImport(t *testing.T) {
	t.Parallel()
	total := 1
	svc := &fakeSmartGroupService{fakeSmartGroupSearch: fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1178905, "Paul - macOS", "9c197ba4-0eb8-7efa-f72b-35769e2754ef")}, 0, 500, &total),
	}}}
	resp := &resource.ImportStateResponse{State: rsState(t, nil)}
	(&SmartGroupResource{svc: svc}).ImportState(context.Background(), resource.ImportStateRequest{ID: "df852f6f-c018-9f1d-28a1-25097af7617f"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("want a not-found error")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"omits an organization group's own smart group", "no by-UUID read", "import it by its numeric id"} {
		if !strings.Contains(detail, want) {
			t.Errorf("error detail missing %q: %s", want, detail)
		}
	}
}
