package macsensor

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
// through Read) run with no wall-clock wait.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

const confirmTestSensorUUID2 = "0b0d6fc4-838a-e89b-d117-57f62678207d"

// counterDeviceSensorsV2Service answers GetDeviceSensorAsync with err on
// its first call and sensor on every subsequent call ("always" keeps
// answering err on every call), counting calls so tests can drive Read's
// confirming re-GET (internal-task, internal/common/notfound) deterministically.
type counterDeviceSensorsV2Service struct {
	getCalls int
	err      error
	sensor   *sdk.DeviceSensorResponseV2Model
	always   bool
}

func (f *counterDeviceSensorsV2Service) CreateDeviceSensorAsync(context.Context, *sdk.DeviceSensorRequestV2Model) (http.Header, *sdk.BaseModelV2, error) {
	return nil, nil, nil
}

func (f *counterDeviceSensorsV2Service) GetDeviceSensorAsync(context.Context, string) (http.Header, *sdk.DeviceSensorResponseV2Model, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.sensor, nil
}

func (f *counterDeviceSensorsV2Service) UpdateDeviceSensorAsync(context.Context, string, *sdk.DeviceSensorUpdateV2Model) (http.Header, error) {
	return nil, nil
}

func confirmSensorReadState(t *testing.T) resource.ReadRequest {
	t.Helper()
	state := emptyResourceState(t)
	diags := state.SetAttribute(context.Background(), path.Root("id"), confirmTestSensorUUID2)
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return resource.ReadRequest{State: state}
}

// TestRead_FlakyNotFoundThenFound_KeepsState is the fail-on-revert wiring
// test for internal-task: Read's first GetDeviceSensorAsync answers a plain 404,
// exactly like a genuinely-deleted macOS device sensor would. Without
// notfound.Confirm wired in, Read would drop state on that single
// response. With it wired in, Read waits (zeroed here) and re-GETs once;
// the confirming re-GET succeeds, so the sensor must stay in state and
// GetDeviceSensorAsync must have been called exactly twice.
func TestRead_FlakyNotFoundThenFound_KeepsState(t *testing.T) {
	svc := &counterDeviceSensorsV2Service{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		sensor: &sdk.DeviceSensorResponseV2Model{UUID: confirmTestSensorUUID2},
	}
	r := newResourceWithMockService(svc)
	req := confirmSensorReadState(t)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky 404 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky 404 followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceSensorAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestRead_ConfirmedNotFound_RemovesState proves the "gone twice" path
// still exercises notfound.Confirm (not around it): both the original and
// the confirming GetDeviceSensorAsync answer 404, so the sensor must be
// dropped from state.
func TestRead_ConfirmedNotFound_RemovesState(t *testing.T) {
	svc := &counterDeviceSensorsV2Service{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		always: true,
	}
	r := newResourceWithMockService(svc)
	req := confirmSensorReadState(t)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error dropping a confirmed-gone sensor, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming Get classify as not-found")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceSensorAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}
