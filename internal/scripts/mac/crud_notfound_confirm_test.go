package macscript

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait. This also speeds up the
// pre-existing TestRead_5001000_ConfirmedAbsent_RemovesFromState and
// TestRead_5001000_AbsentAcrossTwoPages_RemovesFromState in
// crud_absent_test.go, which now also flow through notfound.Confirm.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// counterScriptService answers GetScriptAsync with err on its first call
// and script on every subsequent call ("always" keeps answering err on
// every call), counting calls so tests can drive Read's confirming re-GET
// (internal-task, internal/common/notfound) deterministically. GetScriptsByOrganizationGroupAsync
// delegates to listFn for the scripts/mac Read's second removal site
// (ambiguous 500/1000 confirmed gone via the OG-scoped list).
type counterScriptService struct {
	getCalls int
	err      error
	script   *sdk.ScriptResourceV1
	always   bool

	listFn    func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error)
	listCalls int
}

func (f *counterScriptService) GetScriptAsync(context.Context, string) (http.Header, *sdk.ScriptResourceV1, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.script, nil
}

func (f *counterScriptService) GetScriptsByOrganizationGroupAsync(
	_ context.Context,
	og string,
	opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	f.listCalls++
	if f.listFn == nil {
		return nil, &sdk.ScriptsSearchResultV1{}, nil
	}
	res, err := f.listFn(og, opts)
	return nil, res, err
}

func (f *counterScriptService) CreateScriptAsync(context.Context, string, *sdk.CreateScriptV1) (http.Header, error) {
	return nil, nil
}

func (f *counterScriptService) ScriptBulkDeleteAsync(context.Context, string, *[]string) (http.Header, *sdk.DeleteScriptResourceV1, error) {
	return nil, nil, nil
}

func (f *counterScriptService) ReplaceScriptDefinitionAsync(context.Context, string, *sdk.UpdateScriptV1) (http.Header, error) {
	return nil, nil
}

func confirmTestScript() *sdk.ScriptResourceV1 {
	f := false
	return &sdk.ScriptResourceV1{
		ScriptUUID:            notFoundTestScriptUUID,
		OrganizationGroupUUID: absentTestOrgGroupUUID,
		Name:                  "confirm-test-script",
		Platform:              "APPLE_OSX",
		ScriptType:            "BASH",
		ExecutionContext:      "SYSTEM",
		ScriptData:            "ZWNobyBoaQ==",
		AllowedInCatalog:      &f,
		UserInteraction:       &f,
	}
}

func confirmTestReadState(t *testing.T, orgGroupUUID string) resource.ReadRequest {
	t.Helper()
	state := emptyResourceState(t)
	diags := state.SetAttribute(context.Background(), path.Root("id"), notFoundTestScriptUUID)
	if orgGroupUUID != "" {
		diags.Append(state.SetAttribute(context.Background(), path.Root("organization_group_uuid"), orgGroupUUID)...)
	}
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return resource.ReadRequest{State: state}
}

// TestRead_Flaky404ThenFound_KeepsState is the fail-on-revert wiring test
// for internal-task, site 1 (plain 404 classification): Read's first
// GetScriptAsync answers a plain 404, exactly like a genuinely-deleted
// script would. Without notfound.Confirm wired in, Read would drop state
// on that single response. With it wired in, Read waits (zeroed here) and
// re-GETs once; the confirming re-GET succeeds, so the script must stay in
// state and GetScriptAsync must have been called exactly twice.
func TestRead_Flaky404ThenFound_KeepsState(t *testing.T) {
	svc := &counterScriptService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		script: confirmTestScript(),
	}
	r := newResourceWithMockService(svc)
	req := confirmTestReadState(t, absentTestOrgGroupUUID)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky 404 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky 404 followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
	if svc.listCalls != 0 {
		t.Fatalf("expected no OG-list call on the plain-404 site, got %d", svc.listCalls)
	}
}

// TestRead_FlakyAmbiguous5001000ThenFound_KeepsState is the fail-on-revert
// wiring test for internal-task, site 2 (ambiguous 500/1000 confirmed gone by
// the OG-scoped list): the first GetScriptAsync answers 500/1000, and
// confirmScriptGone's list scan confirms the uuid absent — exactly the
// shape that, before this fix, dropped state immediately. With
// notfound.Confirm wired in AFTER that decision, Read waits (zeroed here)
// and re-GETs once more; that confirming re-GET succeeds, so the script
// must stay in state. Because the confirming refetch succeeds, Confirm
// never re-invokes confirmScriptGone's list scan, so exactly 1 list call is
// expected (the initial decision only).
func TestRead_FlakyAmbiguous5001000ThenFound_KeepsState(t *testing.T) {
	svc := &counterScriptService{
		err:    err5001000(),
		script: confirmTestScript(),
		listFn: listPages([]string{"other-uuid"}), // notFoundTestScriptUUID absent from the OG list
	}
	r := newResourceWithMockService(svc)
	req := confirmTestReadState(t, absentTestOrgGroupUUID)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky ambiguous 500/1000 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky ambiguous 500/1000 followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
	if svc.listCalls != 1 {
		t.Fatalf("expected exactly 1 OG-list call (the initial ambiguous-gone decision only; the confirming re-GET succeeded so Confirm never re-invoked confirmScriptGone), got %d", svc.listCalls)
	}
}

// TestRead_ConfirmedAmbiguous5001000_RemovesState proves the "gone twice"
// path for site 2 still exercises notfound.Confirm (not around it): both
// the original and the confirming GetScriptAsync answer the ambiguous
// 500/1000 shape, and the list confirms absence both times, so the script
// must be dropped from state — see also the updated call-count assertions
// in TestRead_5001000_ConfirmedAbsent_RemovesFromState (crud_absent_test.go),
// which already covers this path with the mockScriptService fake.
func TestRead_ConfirmedAmbiguous5001000_RemovesState(t *testing.T) {
	svc := &counterScriptService{
		err:    err5001000(),
		always: true,
		listFn: listPages([]string{"other-uuid"}),
	}
	r := newResourceWithMockService(svc)
	req := confirmTestReadState(t, absentTestOrgGroupUUID)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error dropping a confirmed-gone script, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming GetScriptAsync classify as ambiguous-gone")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
	if svc.listCalls != 2 {
		t.Fatalf("expected exactly 2 OG-list calls (initial decision + Confirm's re-verification), got %d", svc.listCalls)
	}
}
