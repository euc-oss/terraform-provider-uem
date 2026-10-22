package macapplication

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
)

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// counterInternalAppsV1Service answers GetInternalAppByIdAsync with err on
// its first call and model on every subsequent call, so tests can drive
// Read's confirming re-GET (internal-task, internal/common/notfound)
// deterministically. On the "always" variant model stays nil, so err
// answers every call.
type counterInternalAppsV1Service struct {
	getCalls int
	err      error
	model    *sdk.InternalAppModelV1
	always   bool
}

func (f *counterInternalAppsV1Service) DeleteInternalAppAsync(context.Context, int) (http.Header, error) {
	return nil, nil
}

func (f *counterInternalAppsV1Service) GetInternalAppByIdAsync(context.Context, int) (http.Header, *sdk.InternalAppModelV1, error) {
	f.getCalls++
	if f.always || f.getCalls == 1 {
		return nil, nil, f.err
	}
	return nil, f.model, nil
}

// ambiguous7000Err is the 401/errorCode-7000 shape isAmbiguousNotFoundOrInaccessibleAPIError matches.
var ambiguous7000Err = &sdk.APIError{
	StatusCode: http.StatusUnauthorized,
	ErrorCode:  "7000",
	Message:    "Application not found or user does not have access to it.",
}

// TestRead_FlakyAmbiguous401ThenFound_KeepsStateNoWarning is the
// fail-on-revert wiring test for internal-task: Read's first
// GetInternalAppByIdAsync answers the ambiguous 401/errorCode-7000 shape,
// exactly like a genuinely-deleted (or inaccessible) app would. Without
// notfound.Confirm wired in, Read would drop state (and add the ambiguity
// warning) on that single response. With it wired in, Read waits (zeroed
// here) and re-GETs once; the confirming re-GET succeeds, so the app must
// stay in state, with NO ambiguity warning (the object is fine — see the
// special-case handling described in resource_crud.go's Read), and
// GetInternalAppByIdAsync must have been called exactly twice.
func TestRead_FlakyAmbiguous401ThenFound_KeepsStateNoWarning(t *testing.T) {
	svc := &counterInternalAppsV1Service{
		err:   ambiguous7000Err,
		model: &sdk.InternalAppModelV1{ID: intPtr(42), UUID: notFound401TestUUID},
	}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		client:               &sdk.Client{},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return svc
		},
	}
	req := notFound401TestState(t, 43)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a flaky ambiguous-401 followed by a successful confirming re-GET, got: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky ambiguous-401 followed by a successful confirming re-GET")
	}
	if len(resp.Diagnostics.Warnings()) != 0 {
		t.Fatalf("expected NO ambiguity warning once the confirming re-GET found the app, got: %+v", resp.Diagnostics.Warnings())
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetInternalAppByIdAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestRead_ConfirmedAmbiguous401_RemovesStateWithWarning proves the
// "ambiguous twice" path still exercises notfound.Confirm (not around it):
// both the original and the confirming Get answer the same 401/7000 shape,
// so the app must be dropped from state WITH the ambiguity warning, exactly
// as it would without the Confirm wiring.
func TestRead_ConfirmedAmbiguous401_RemovesStateWithWarning(t *testing.T) {
	svc := &counterInternalAppsV1Service{err: ambiguous7000Err, always: true}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		client:               &sdk.Client{},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return svc
		},
	}
	req := notFound401TestState(t, 42)
	resp := &resource.ReadResponse{State: req.State}

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no hard error dropping a confirmed-ambiguous app, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming Get classify as ambiguous not-found")
	}
	if len(resp.Diagnostics.Warnings()) == 0 {
		t.Fatal("expected the ambiguity warning to still be present when the confirming re-GET also classifies as not-found")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 GetInternalAppByIdAsync calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}
