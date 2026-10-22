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

const confirmTestAppUUID = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// counterAppAssignmentService answers GetAssignmentRuleAsync with err on
// its first call and result on every subsequent call ("always" keeps
// answering err on every call), counting calls so tests can drive Read's
// confirming re-GET (internal-task, internal/common/notfound) deterministically.
type counterAppAssignmentService struct {
	getCalls int
	err      error
	result   *sdk.AppAssignmentRuleV2Model
	always   bool
}

func (f *counterAppAssignmentService) GetAssignmentRuleAsync(context.Context, string) (http.Header, *sdk.AppAssignmentRuleV2Model, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.result, nil
}

func (f *counterAppAssignmentService) UpdateAssignmentRuleAsync(context.Context, string, *sdk.AppAssignmentRuleV2Model) (http.Header, error) {
	return nil, nil
}

func confirmAssignmentState(t *testing.T) tfsdk.State {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&applicationAssignmentResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	ctx := context.Background()
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil)}
	diags := state.SetAttribute(ctx, path.Root("id"), confirmTestAppUUID)
	diags.Append(state.SetAttribute(ctx, path.Root("application_uuid"), confirmTestAppUUID)...)
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return state
}

// TestRead_FlakyNotFoundThenFound_KeepsState is the fail-on-revert wiring
// test for internal-task: Read's first GetAssignmentRuleAsync answers a plain
// 404, exactly like a genuinely-deleted assignment rule would. Without
// notfound.Confirm wired in, Read would drop state on that single
// response. With it wired in, Read waits (zeroed here) and re-GETs once;
// the confirming re-GET succeeds, so the assignment must stay in state and
// GetAssignmentRuleAsync must have been called exactly twice.
func TestRead_FlakyNotFoundThenFound_KeepsState(t *testing.T) {
	svc := &counterAppAssignmentService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		result: &sdk.AppAssignmentRuleV2Model{},
	}
	r := &applicationAssignmentResource{
		client:                  &sdk.Client{},
		newAppAssignmentService: func(*sdk.Client) appAssignmentServiceAPI { return svc },
	}
	state := confirmAssignmentState(t)
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
		t.Fatalf("expected exactly 2 GetAssignmentRuleAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestRead_ConfirmedNotFound_RemovesState proves the "gone twice" path
// still exercises notfound.Confirm (not around it): both the original and
// the confirming GetAssignmentRuleAsync answer 404, so the assignment must
// be dropped from state.
func TestRead_ConfirmedNotFound_RemovesState(t *testing.T) {
	svc := &counterAppAssignmentService{
		err:    &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"},
		always: true,
	}
	r := &applicationAssignmentResource{
		client:                  &sdk.Client{},
		newAppAssignmentService: func(*sdk.Client) appAssignmentServiceAPI { return svc },
	}
	state := confirmAssignmentState(t)
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
		t.Fatalf("expected exactly 2 GetAssignmentRuleAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}
