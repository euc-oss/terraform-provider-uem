package macscript

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const confirmUpdateTestScriptUUID = "b28f7626-d8ea-4b28-9168-1f4491593111"

// updateOnlyNotFoundFake embeds counterScriptService (defined in
// crud_notfound_confirm_test.go) so it inherits that fake's call-counting
// GetScriptAsync behavior unchanged, while overriding
// ReplaceScriptDefinitionAsync to answer a controllable, counted error --
// the write half of the Update not-found-confirm wiring (internal-task scope
// extension) that the GET-only counterScriptService fake does not cover.
type updateOnlyNotFoundFake struct {
	*counterScriptService
	updateCalls int
	updateErr   error
}

func (f *updateOnlyNotFoundFake) ReplaceScriptDefinitionAsync(context.Context, string, *sdk.UpdateScriptV1) (http.Header, error) {
	f.updateCalls++
	return nil, f.updateErr
}

// confirmUpdateScriptPlan builds a minimal-but-schema-valid Update plan by
// reusing minimalScriptModel (crud_server_defaults_test.go), setting only ID
// (Read/Update need a script UUID; Create leaves it null).
func confirmUpdateScriptPlan(t *testing.T) resource.UpdateRequest {
	t.Helper()
	m := minimalScriptModel()
	m.ID = types.StringValue(confirmUpdateTestScriptUUID)
	return resource.UpdateRequest{Plan: planFromModel(t, m)}
}

// TestUpdate_Flaky404ThenFound_ReturnsReRunApplyError is the fail-on-revert
// wiring test for the internal-task Update-path scope extension:
// ReplaceScriptDefinitionAsync answers a plain 404, exactly like a
// genuinely-deleted script would. Without notfound.Confirm wired into
// Update, this would drop state on that single response. With it wired in,
// Update waits (zeroed by TestMain) and re-GETs once via
// fetchMacscriptDetails; the confirming re-GET succeeds, so Update must NOT
// drop state and must NOT retry the write -- it surfaces the "re-run apply"
// error instead (mid-update state is too delicate to resume inline; see
// confirmNotFoundUpdate's doc comment in crud.go). This uses the same plain
// isNotFoundAPIError classifier Update already used before this change, not
// Read's combined ambiguous-gone classifier.
func TestUpdate_Flaky404ThenFound_ReturnsReRunApplyError(t *testing.T) {
	getSvc := &counterScriptService{
		script: confirmTestScript(),
	}
	// Pre-charge one GET so the confirming re-GET Update triggers lands on
	// counterScriptService's SECOND call, which its shared fake logic
	// answers with success. counterScriptService's FIRST call always
	// answers err (what the Read-path tests in crud_notfound_confirm_test.go
	// need); this Update-path test doesn't want that -- the WRITE is what
	// fails here, not the GET, and the confirming GET should succeed
	// immediately on its one and only call.
	getSvc.getCalls = 1

	svc := &updateOnlyNotFoundFake{
		counterScriptService: getSvc,
		updateErr:            &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	r := newResourceWithMockService(svc)
	req := confirmUpdateScriptPlan(t)

	initialState := emptyResourceState(t)
	if diags := initialState.SetAttribute(context.Background(), path.Root("id"), confirmUpdateTestScriptUUID); diags.HasError() {
		t.Fatalf("failed to seed initial state: %v", diags)
	}
	resp := &resource.UpdateResponse{State: initialState}

	r.Update(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a re-run-apply Diagnostics error, got none")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Detail() == "UEM returned not-found then found during update; re-run apply" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the exact re-run-apply error message, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT (not RemoveResource'd): the confirming re-GET found the script still exists")
	}
	if svc.updateCalls != 1 {
		t.Fatalf("expected exactly 1 ReplaceScriptDefinitionAsync call (no blind retry of the write), got %d", svc.updateCalls)
	}
	if getSvc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAsync calls (1 pre-charge + 1 real confirming re-GET), got %d", getSvc.getCalls)
	}
	if getSvc.listCalls != 0 {
		t.Fatalf("expected no OG-list call: Update uses the plain isNotFoundAPIError classifier only, got %d", getSvc.listCalls)
	}
}
