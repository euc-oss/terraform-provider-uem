package assignment

import (
	"context"
	"net/http"
	"os"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
)

const confirmTestSensorUUID = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// counterSensorAssignmentService answers GetDeviceSensorAssignmentsAsync
// with err on its first call and result on every subsequent call ("always"
// keeps answering err on every call), counting calls so tests can drive
// Read's confirming re-GET (internal-task, internal/common/notfound)
// deterministically. Only the methods Read exercises are implemented with
// real behavior; the rest are unused stubs.
type counterSensorAssignmentService struct {
	getCalls int
	err      error
	result   *[]sdk.DeviceSensorAssignmentResponseV1ModelV2
	always   bool
}

func (f *counterSensorAssignmentService) AddDeviceSensorAssignmentAsync(context.Context, string, *sdk.DeviceSensorAssignmentRequestV1ModelV2) (http.Header, *sdk.BaseModelV2, error) {
	return nil, nil, nil
}

func (f *counterSensorAssignmentService) GetDeviceSensorAssignmentsAsync(context.Context, string) (http.Header, *[]sdk.DeviceSensorAssignmentResponseV1ModelV2, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func (f *counterSensorAssignmentService) GetDeviceSensorAssignmentAsync(context.Context, string) (http.Header, *sdk.DeviceSensorAssignmentResponseV1ModelV2, error) {
	return nil, nil, nil
}

func (f *counterSensorAssignmentService) UpdateDeviceSensorAssignmentAsync(context.Context, string, *sdk.DeviceSensorAssignmentRequestV1ModelV2) (http.Header, *sdk.BaseExceptionModelV2, error) {
	return nil, nil, nil
}

func (f *counterSensorAssignmentService) DeleteDeviceSensorAssignmentAsync(context.Context, string) (http.Header, error) {
	return nil, nil
}

func (f *counterSensorAssignmentService) BulkUpdateDeviceSensorAssignmentRankingsAsync(context.Context, string, *[]sdk.DeviceSensorAssignmentRankingV1ModelV2, *sdk.DeviceSensorsV2BulkUpdateDeviceSensorAssignmentRankingsAsyncOptions) (http.Header, error) {
	return nil, nil
}

func confirmSensorAssignmentState(t *testing.T) tfsdk.State {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&sensorAssignmentResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	ctx := context.Background()
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	diags := state.SetAttribute(ctx, path.Root("id"), confirmTestSensorUUID)
	diags.Append(state.SetAttribute(ctx, path.Root("sensor_uuid"), confirmTestSensorUUID)...)
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return state
}

// TestRead_FlakyNotFoundThenFound_KeepsState is the fail-on-revert wiring
// test for internal-task: Read's first GetDeviceSensorAssignmentsAsync answers a
// plain 404, exactly like a genuinely-deleted sensor assignment would.
// Without notfound.Confirm wired in, Read would drop state on that single
// response. With it wired in, Read waits (zeroed here) and re-GETs once;
// the confirming re-GET succeeds, so the assignment must stay in state and
// GetDeviceSensorAssignmentsAsync must have been called exactly twice.
func TestRead_FlakyNotFoundThenFound_KeepsState(t *testing.T) {
	empty := []sdk.DeviceSensorAssignmentResponseV1ModelV2{}
	svc := &counterSensorAssignmentService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		result: &empty,
	}
	r := &sensorAssignmentResource{
		client:                     &sdk.Client{},
		newSensorAssignmentService: func(*sdk.Client) sensorAssignmentServiceAPI { return svc },
	}
	state := confirmSensorAssignmentState(t)
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky not-found followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky not-found followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceSensorAssignmentsAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestRead_ConfirmedNotFound_RemovesState proves the "gone twice" path
// still exercises notfound.Confirm (not around it): both the original and
// the confirming GetDeviceSensorAssignmentsAsync answer 404, so the
// assignment must be dropped from state.
func TestRead_ConfirmedNotFound_RemovesState(t *testing.T) {
	svc := &counterSensorAssignmentService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		always: true,
	}
	r := &sensorAssignmentResource{
		client:                     &sdk.Client{},
		newSensorAssignmentService: func(*sdk.Client) sensorAssignmentServiceAPI { return svc },
	}
	state := confirmSensorAssignmentState(t)
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error dropping a confirmed-gone assignment, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming Get classify as not-found")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceSensorAssignmentsAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}
