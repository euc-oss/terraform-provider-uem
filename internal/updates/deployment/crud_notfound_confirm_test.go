package deployment

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

const confirmTestDeploymentUUID = "0b0d6fc4-838a-e89b-d117-57f62678207d"

// counterUpdatesV1Service answers GetDeviceUpdateDeploymentDetails with err
// on its first call and details on every subsequent call ("always" keeps
// answering err on every call), counting calls so tests can drive Read's
// confirming re-GET (internal-task, internal/common/notfound) deterministically.
type counterUpdatesV1Service struct {
	getCalls int
	err      error
	details  *sdk.DeviceUpdateDeploymentV1Model
	always   bool
}

func (f *counterUpdatesV1Service) CreateUpdateDeployment(context.Context, string, string, *sdk.DeviceUpdateDeploymentBaseV1Model) (http.Header, *sdk.BaseModelV1, error) {
	return nil, nil, nil
}

func (f *counterUpdatesV1Service) GetDeviceUpdateDeploymentDetails(context.Context, string) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.details, nil
}

func (f *counterUpdatesV1Service) UpdateDeviceUpdateDeployment(context.Context, string, *sdk.DeviceUpdateDeploymentUpdateV1Model) (http.Header, error) {
	return nil, nil
}

func (f *counterUpdatesV1Service) DeleteDeviceUpdateDeployment(context.Context, string) (http.Header, error) {
	return nil, nil
}

func newResourceWithMockUpdatesService(svc UpdatesV1API) *updateDeploymentResource {
	return &updateDeploymentResource{
		client: &sdk.Client{},
		newUpdatesV1: func(*sdk.Client) UpdatesV1API {
			return svc
		},
	}
}

func confirmDeploymentReadState(t *testing.T) resource.ReadRequest {
	t.Helper()
	state := emptyResourceState(t)
	diags := state.SetAttribute(context.Background(), path.Root("id"), confirmTestDeploymentUUID)
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return resource.ReadRequest{State: state}
}

// TestRead_FlakyNotFoundThenFound_KeepsState is the fail-on-revert wiring
// test for internal-task: Read's first GetDeviceUpdateDeploymentDetails answers
// a plain 404, exactly like a genuinely-deleted update deployment would.
// Without notfound.Confirm wired in, Read would drop state on that single
// response. With it wired in, Read waits (zeroed here) and re-GETs once;
// the confirming re-GET succeeds, so the deployment must stay in state and
// GetDeviceUpdateDeploymentDetails must have been called exactly twice.
func TestRead_FlakyNotFoundThenFound_KeepsState(t *testing.T) {
	svc := &counterUpdatesV1Service{
		err:     &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		details: &sdk.DeviceUpdateDeploymentV1Model{},
	}
	r := newResourceWithMockUpdatesService(svc)
	req := confirmDeploymentReadState(t)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky 404 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky 404 followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceUpdateDeploymentDetails calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestRead_ConfirmedNotFound_RemovesState proves the "gone twice" path
// still exercises notfound.Confirm (not around it): both the original and
// the confirming GetDeviceUpdateDeploymentDetails answer 404, so the
// deployment must be dropped from state.
func TestRead_ConfirmedNotFound_RemovesState(t *testing.T) {
	svc := &counterUpdatesV1Service{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		always: true,
	}
	r := newResourceWithMockUpdatesService(svc)
	req := confirmDeploymentReadState(t)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error dropping a confirmed-gone deployment, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming Get classify as not-found")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetDeviceUpdateDeploymentDetails calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}
