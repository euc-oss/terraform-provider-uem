package macsensor

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const confirmUpdateTestSensorUUID = "b28f7626-d8ea-4b28-9168-1f4491593111"

// updateOnlyNotFoundFake embeds counterDeviceSensorsV2Service (defined in
// crud_notfound_confirm_test.go) so it inherits that fake's call-counting
// GetDeviceSensorAsync behavior unchanged, while overriding
// UpdateDeviceSensorAsync to answer a controllable, counted error -- the
// write half of the Update not-found-confirm wiring (internal-task scope
// extension) that the GET-only counterDeviceSensorsV2Service fake does not
// cover.
type updateOnlyNotFoundFake struct {
	*counterDeviceSensorsV2Service
	updateCalls int
	updateErr   error
}

func (f *updateOnlyNotFoundFake) UpdateDeviceSensorAsync(context.Context, string, *sdk.DeviceSensorUpdateV2Model) (http.Header, error) {
	f.updateCalls++
	return nil, f.updateErr
}

// emptyResourcePlan mirrors emptyResourceState (crud_test.go) but returns a
// tfsdk.Plan with every attribute correctly-typed-null, ready for
// SetAttribute to fill in just the fields a test needs.
func emptyResourcePlan(t *testing.T) tfsdk.Plan {
	t.Helper()

	r := &macsensorResource{}
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

// confirmUpdateSensorPlan builds a minimal-but-schema-valid Update plan:
// every required attribute set, everything else left correctly-typed-null.
func confirmUpdateSensorPlan(t *testing.T) resource.UpdateRequest {
	t.Helper()
	ctx := context.Background()

	plan := emptyResourcePlan(t)
	diags := plan.SetAttribute(ctx, path.Root("id"), confirmUpdateTestSensorUUID)
	diags.Append(plan.SetAttribute(ctx, path.Root("organization_group_uuid"), "70ad49f3-a0b8-03a4-c00d-c1cf16affdef")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("name"), "confirm_update_test")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("language"), "BASH")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("execution_context"), "SYSTEM")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("execution_architecture"), "EITHER64OR32BIT")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("response_data_type"), "STRING")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("code"), "ZWNobyBoaQ==")...)
	diags.Append(plan.SetAttribute(ctx, path.Root("is_read_only"), false)...)
	if diags.HasError() {
		t.Fatalf("failed to seed plan: %v", diags)
	}

	return resource.UpdateRequest{Plan: plan}
}

// TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError is the
// fail-on-revert wiring test for the internal-task Update-path scope extension:
// UpdateDeviceSensorAsync answers a plain 404, exactly like a
// genuinely-deleted sensor would. Without notfound.Confirm wired into
// Update, this would drop state on that single response. With it wired in,
// Update waits (zeroed by TestMain) and re-GETs once via
// fetchMacSensorDetails; the confirming re-GET succeeds, so Update must NOT
// drop state and must NOT retry the write -- it surfaces the "re-run apply"
// error instead (mid-update state is too delicate to resume inline; see
// confirmNotFoundUpdate's doc comment in crud.go).
func TestUpdate_FlakyNotFoundThenFound_ReturnsReRunApplyError(t *testing.T) {
	getSvc := &counterDeviceSensorsV2Service{
		sensor: &sdk.DeviceSensorResponseV2Model{UUID: confirmUpdateTestSensorUUID},
	}
	// Pre-charge one GET so the confirming re-GET Update triggers lands on
	// counterDeviceSensorsV2Service's SECOND call, which its shared fake
	// logic answers with success. counterDeviceSensorsV2Service's FIRST call
	// always answers err (what the Read-path tests in
	// crud_notfound_confirm_test.go need); this Update-path test doesn't
	// want that -- the WRITE is what fails here, not the GET, and the
	// confirming GET should succeed immediately on its one and only call.
	getSvc.getCalls = 1

	svc := &updateOnlyNotFoundFake{
		counterDeviceSensorsV2Service: getSvc,
		updateErr:                     &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
	}
	r := newResourceWithMockService(svc)
	req := confirmUpdateSensorPlan(t)

	initialState := emptyResourceState(t)
	if diags := initialState.SetAttribute(context.Background(), path.Root("id"), confirmUpdateTestSensorUUID); diags.HasError() {
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
		t.Fatal("expected state to be KEPT (not RemoveResource'd): the confirming re-GET found the sensor still exists")
	}
	if svc.updateCalls != 1 {
		t.Fatalf("expected exactly 1 UpdateDeviceSensorAsync call (no blind retry of the write), got %d", svc.updateCalls)
	}
	if getSvc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceSensorAsync calls (1 pre-charge + 1 real confirming re-GET), got %d", getSvc.getCalls)
	}
}
