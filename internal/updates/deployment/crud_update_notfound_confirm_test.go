package deployment

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const confirmUpdateTestDeploymentUUID = "b28f7626-d8ea-4b28-9168-1f4491593111"

// updateOnlyNotFoundFake embeds counterUpdatesV1Service (defined in
// crud_notfound_confirm_test.go) so it inherits that fake's call-counting
// GetDeviceUpdateDeploymentDetails behavior unchanged, while overriding
// UpdateDeviceUpdateDeployment to answer a controllable, counted error --
// the write half of the Update not-found-confirm wiring (internal-task scope
// extension) that the GET-only counterUpdatesV1Service fake does not cover.
type updateOnlyNotFoundFake struct {
	*counterUpdatesV1Service
	updateCalls int
	updateErr   error
}

func (f *updateOnlyNotFoundFake) UpdateDeviceUpdateDeployment(context.Context, string, *sdk.DeviceUpdateDeploymentUpdateV1Model) (http.Header, error) {
	f.updateCalls++
	return nil, f.updateErr
}

// emptyResourcePlan mirrors emptyResourceState (crud_test.go) but returns a
// tfsdk.Plan with every attribute correctly-typed-null, ready for
// SetAttribute to fill in just the fields a test needs.
func emptyResourcePlan(t *testing.T) tfsdk.Plan {
	t.Helper()

	r := &updateDeploymentResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}

	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}

	return tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

// confirmUpdateDeploymentPlan builds a minimal-but-schema-valid Update plan:
// every required attribute set, everything else left correctly-typed-null.
func confirmUpdateDeploymentPlan(t *testing.T) resource.UpdateRequest {
	t.Helper()
	ctx := context.Background()

	plan := emptyResourcePlan(t)
	smartGroups, listDiags := types.ListValueFrom(ctx, types.StringType, []string{"666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c"})
	if listDiags.HasError() {
		t.Fatalf("failed to build smart_group_uuids: %v", listDiags)
	}

	diags := plan.SetAttribute(ctx, path.Root("id"), confirmUpdateTestDeploymentUUID)
	diags.Append(plan.SetAttribute(ctx, path.Root("update_uuid"), "70ad49f3-a0b8-03a4-c00d-c1cf16affdef")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("organization_group_uuid"), "69536499-21a1-7c80-64b8-4cfb04c8f4a9")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("name"), "confirm-update-test")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("deployment_type"), "DOWNLOAD_AND_INSTALL")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("deployment_start_time"), "2026-07-28T16:00:00.000Z")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("smart_group_uuids"), smartGroups)...)
	if diags.HasError() {
		t.Fatalf("failed to seed plan: %v", diags)
	}

	return resource.UpdateRequest{Plan: plan}
}

// TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError is the
// fail-on-revert wiring test for the internal-task Update-path scope extension:
// UpdateDeviceUpdateDeployment answers a plain 404, exactly like a
// genuinely-deleted deployment would. Without notfound.Confirm wired into
// Update, this would drop state on that single response. With it wired in,
// Update waits (zeroed by TestMain) and re-GETs once via
// fetchDeploymentDetails; the confirming re-GET succeeds, so Update must NOT
// drop state and must NOT retry the write -- it surfaces the "re-run apply"
// error instead (mid-update state is too delicate to resume inline; see
// confirmNotFoundUpdate's doc comment in crud.go).
func TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError(t *testing.T) {
	getSvc := &counterUpdatesV1Service{
		details: &sdk.DeviceUpdateDeploymentV1Model{UUID: confirmUpdateTestDeploymentUUID},
	}
	// Pre-charge one GET so the confirming re-GET Update triggers lands on
	// counterUpdatesV1Service's SECOND call, which its shared fake logic
	// answers with success. counterUpdatesV1Service's FIRST call always
	// answers err (what the Read-path tests in crud_notfound_confirm_test.go
	// need); this Update-path test doesn't want that -- the WRITE is what
	// fails here, not the GET, and the confirming GET should succeed
	// immediately on its one and only call.
	getSvc.getCalls = 1

	svc := &updateOnlyNotFoundFake{
		counterUpdatesV1Service: getSvc,
		updateErr:               &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	r := newResourceWithMockUpdatesService(svc)
	req := confirmUpdateDeploymentPlan(t)

	initialState := emptyResourceState(t)
	if diags := initialState.SetAttribute(context.Background(), path.Root("id"), confirmUpdateTestDeploymentUUID); diags.HasError() {
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
		t.Fatal("expected state to be KEPT (not RemoveResource'd): the confirming re-GET found the deployment still exists")
	}
	if svc.updateCalls != 1 {
		t.Fatalf("expected exactly 1 UpdateDeviceUpdateDeployment call (no blind retry of the write), got %d", svc.updateCalls)
	}
	if getSvc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceUpdateDeploymentDetails calls (1 pre-charge + 1 real confirming re-GET), got %d", getSvc.getCalls)
	}
}
