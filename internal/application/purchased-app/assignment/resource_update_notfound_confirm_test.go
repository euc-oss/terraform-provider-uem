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

// updateOnlyNotFoundFake embeds counterPurchasedAppAssignmentService
// (defined in resource_notfound_confirm_test.go) so it inherits that fake's
// call-counting GetAssignmentRuleAsync behavior unchanged, while overriding
// UpdateAssignmentRuleAsync to answer a controllable, counted error -- the
// write half of the Update not-found-confirm wiring (internal-task scope
// extension) that the GET-only counterPurchasedAppAssignmentService fake
// does not cover.
type updateOnlyNotFoundFake struct {
	*counterPurchasedAppAssignmentService
	updateCalls int
	updateErr   error
}

func (f *updateOnlyNotFoundFake) UpdateAssignmentRuleAsync(context.Context, string, *sdk.AppAssignmentRuleV2Model) (http.Header, error) {
	f.updateCalls++
	return nil, f.updateErr
}

// confirmUpdatePurchasedAssignmentPlan builds a minimal-but-schema-valid
// Update plan: application_uuid set, assignments left null (Optional).
func confirmUpdatePurchasedAssignmentPlan(t *testing.T) resource.UpdateRequest {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&purchasedApplicationAssignmentResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	state := confirmPurchasedAssignmentState(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: state.Raw}

	diags := plan.SetAttribute(ctx, path.Root("application_uuid"), confirmTestPurchasedAppUUID)
	if diags.HasError() {
		t.Fatalf("failed to seed plan: %v", diags)
	}

	return resource.UpdateRequest{Plan: plan}
}

// TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError is the
// fail-on-revert wiring test for the internal-task Update-path scope extension:
// UpdateAssignmentRuleAsync answers a plain 404, exactly like a
// genuinely-deleted assignment rule would. Without notfound.Confirm wired
// into Update, this would drop state on that single response. With it wired
// in, Update waits (zeroed by TestMain) and re-GETs once via
// GetAssignmentRuleAsync; the confirming re-GET succeeds, so Update must NOT
// drop state and must NOT retry the write -- it surfaces the "re-run apply"
// error instead (mid-update state is too delicate to resume inline; see
// confirmNotFoundUpdate's doc comment in resource_crud.go).
func TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError(t *testing.T) {
	getSvc := &counterPurchasedAppAssignmentService{
		result: &sdk.AppAssignmentRuleV2Model{},
	}
	// Pre-charge one GET so the confirming re-GET Update triggers lands on
	// counterPurchasedAppAssignmentService's SECOND call, which its shared
	// fake logic answers with success. counterPurchasedAppAssignmentService's
	// FIRST call always answers err (what the Read-path tests in
	// resource_notfound_confirm_test.go need); this Update-path test doesn't
	// want that -- the WRITE is what fails here, not the GET, and the
	// confirming GET should succeed immediately on its one and only call.
	getSvc.getCalls = 1

	svc := &updateOnlyNotFoundFake{
		counterPurchasedAppAssignmentService: getSvc,
		updateErr:                            &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	r := &purchasedApplicationAssignmentResource{
		client:                           &sdk.Client{},
		newPurchasedAppAssignmentService: func(*sdk.Client) purchasedAppAssignmentServiceAPI { return svc },
	}
	req := confirmUpdatePurchasedAssignmentPlan(t)

	initialState := confirmPurchasedAssignmentState(t)
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
		t.Fatal("expected state to be KEPT (not RemoveResource'd): the confirming re-GET found the assignment rule still exists")
	}
	if svc.updateCalls != 1 {
		t.Fatalf("expected exactly 1 UpdateAssignmentRuleAsync call (no blind retry of the write), got %d", svc.updateCalls)
	}
	if getSvc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetAssignmentRuleAsync calls (1 pre-charge + 1 real confirming re-GET), got %d", getSvc.getCalls)
	}
}
