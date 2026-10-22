package assignment

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

// updateOnlyNotFoundFake embeds counterAssignmentService (defined in
// resource_notfound_confirm_test.go) so it inherits that fake's
// call-counting GetScriptAssignmentsAsync behavior unchanged, while
// overriding BulkUpdateScriptAssignmentsAsync to answer a controllable,
// counted error -- the write half of the Update not-found-confirm wiring
// (internal-task scope extension) that the GET-only counterAssignmentService
// fake does not cover.
type updateOnlyNotFoundFake struct {
	*counterAssignmentService
	updateCalls int
	updateErr   error
}

func (f *updateOnlyNotFoundFake) BulkUpdateScriptAssignmentsAsync(context.Context, string, *sdk.BulkUpdateScriptAssignmentV1) (http.Header, error) {
	f.updateCalls++
	return nil, f.updateErr
}

// confirmUpdateAssignmentPlan builds a minimal-but-schema-valid Update plan:
// script_uuid set, assignments and organization_group_uuid left null
// (Optional/Computed).
func confirmUpdateAssignmentPlan(t *testing.T) resource.UpdateRequest {
	t.Helper()
	ctx := context.Background()
	sch := testSchemaResponse(t).Schema
	state := seededState(t, "")
	plan := tfsdk.Plan{Schema: sch, Raw: state.Raw}

	diags := plan.SetAttribute(ctx, path.Root("script_uuid"), testScriptUUID)
	if diags.HasError() {
		t.Fatalf("failed to seed plan: %v", diags)
	}

	return resource.UpdateRequest{Plan: plan}
}

// TestUpdate_Flaky404ThenFound_ReturnsReRunApplyError is the fail-on-revert
// wiring test for the internal-task Update-path scope extension:
// BulkUpdateScriptAssignmentsAsync answers a plain 404, exactly like a
// genuinely-deleted script's assignments would. Without notfound.Confirm
// wired into Update, this would drop state on that single response. With it
// wired in, Update waits (zeroed by TestMain) and re-GETs once via
// GetScriptAssignmentsAsync; the confirming re-GET succeeds, so Update must
// NOT drop state and must NOT retry the write -- it surfaces the "re-run
// apply" error instead (mid-update state is too delicate to resume inline;
// see confirmNotFoundUpdate's doc comment in resource_crud.go). This uses
// the same plain isNotFoundAPIError classifier Update already used before
// this change, not Read's combined ambiguous-gone classifier.
func TestUpdate_Flaky404ThenFound_ReturnsReRunApplyError(t *testing.T) {
	getSvc := &counterAssignmentService{
		result: confirmTestAssignmentResult(),
	}
	// Pre-charge one GET so the confirming re-GET Update triggers lands on
	// counterAssignmentService's SECOND call, which its shared fake logic
	// answers with success. counterAssignmentService's FIRST call always
	// answers err (what the Read-path tests in resource_notfound_confirm_test.go
	// need); this Update-path test doesn't want that -- the WRITE is what
	// fails here, not the GET, and the confirming GET should succeed
	// immediately on its one and only call.
	getSvc.getCalls = 1

	a := &updateOnlyNotFoundFake{
		counterAssignmentService: getSvc,
		updateErr:                &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	s := &fakeScriptService{} // unused by Update; only Read consults the parent script
	r := &scriptAssignmentResource{
		client:                     &sdk.Client{},
		newScriptAssignmentService: func(*sdk.Client) scriptAssignmentServiceAPI { return a },
		newScriptService:           func(*sdk.Client) scriptServiceAPI { return s },
	}
	req := confirmUpdateAssignmentPlan(t)

	initialState := seededState(t, "")
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
		t.Fatal("expected state to be KEPT (not RemoveResource'd): the confirming re-GET found the script assignments still exist")
	}
	if a.updateCalls != 1 {
		t.Fatalf("expected exactly 1 BulkUpdateScriptAssignmentsAsync call (no blind retry of the write), got %d", a.updateCalls)
	}
	if getSvc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetScriptAssignmentsAsync calls (1 pre-charge + 1 real confirming re-GET), got %d", getSvc.getCalls)
	}
	if s.listCalls != 0 {
		t.Fatalf("expected no OG-list call: Update uses the plain isNotFoundAPIError classifier only, got %d", s.listCalls)
	}
}
