package profile

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestMain zeroes notfound.Delay for every test in this package so the
// wiring tests below (and the two-request confirming re-GET they drive
// through Read) run with no wall-clock wait.
func TestMain(m *testing.M) {
	notfound.Delay = 0
	os.Exit(m.Run())
}

// fakeConfirmProfileService is a minimal profileServiceAPI whose Get
// answers a caller-supplied error on its first call and a fixed success
// result on every subsequent call, so tests can drive Read's confirming
// re-GET (internal-task, internal/common/notfound) deterministically.
type fakeConfirmProfileService struct {
	getCalls int
	firstErr error
	result   *sdk.ProfileResult
}

func (f *fakeConfirmProfileService) RegisterEntry(int, string) {}

func (f *fakeConfirmProfileService) Get(_ context.Context, _ int) (*sdk.ProfileResult, error) {
	f.getCalls++
	if f.getCalls == 1 {
		return nil, f.firstErr
	}
	return f.result, nil
}

func (f *fakeConfirmProfileService) Create(context.Context, string, interface{}) (int, error) {
	return 0, nil
}
func (f *fakeConfirmProfileService) Update(context.Context, int, interface{}) error { return nil }
func (f *fakeConfirmProfileService) Delete(context.Context, int) error              { return nil }

// confirmTestProfileStateValues is the field map shared by the wiring tests
// below, mirroring TestProfileResourceRead_InvalidProfile400_RemovesFromState
// in resource_support_factory_test.go.
func confirmTestProfileStateValues() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Flaky Profile"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	}
}

// TestProfileResourceRead_FlakyNotFoundThenFound_KeepsState is the fail-on-
// revert wiring test for internal-task: Read's first Get answers the classic
// "Invalid Profile <id>." not-found shape, exactly like a genuinely-deleted
// profile would. Without notfound.Confirm wired in, Read would drop state
// on that single response. With it wired in, Read waits (zeroed here) and
// re-GETs once; the confirming re-GET succeeds, so the profile must stay in
// state and Get must have been called exactly twice.
func TestProfileResourceRead_FlakyNotFoundThenFound_KeepsState(t *testing.T) {
	svc := &fakeConfirmProfileService{
		firstErr: &sdk.APIError{StatusCode: http.StatusBadRequest, Message: "Invalid Profile 12345."},
		result:   &sdk.ProfileResult{},
	}
	res := &ProfileResource{
		client: &sdk.Client{},
		newProfileService: func(context.Context, *sdk.Client) (profileServiceAPI, error) {
			return svc, nil
		},
	}

	state := createResourceState(t, confirmTestProfileStateValues())
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error for a flaky not-found followed by a successful confirming re-GET, got: %v", msgs)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be KEPT after a flaky not-found followed by a successful confirming re-GET")
	}
	if svc.getCalls != 2 {
		t.Fatalf("expected exactly 2 Get calls (original + confirming re-GET), got %d", svc.getCalls)
	}
}

// TestProfileResourceRead_ConfirmedGone_RemovesState proves the "gone
// twice" path still exercises notfound.Confirm (not around it): both the
// original and the confirming Get answer the same not-found shape, so the
// profile must be dropped from state.
func TestProfileResourceRead_ConfirmedGone_RemovesState(t *testing.T) {
	svc := &alwaysNotFoundProfileService{
		err: &sdk.APIError{StatusCode: http.StatusBadRequest, Message: "Invalid Profile 12345."},
	}
	res := &ProfileResource{
		client: &sdk.Client{},
		newProfileService: func(context.Context, *sdk.Client) (profileServiceAPI, error) {
			return svc, nil
		},
	}

	state := createResourceState(t, confirmTestProfileStateValues())
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error dropping a confirmed-gone profile, got: %v", msgs)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be REMOVED when both the original and confirming Get classify as not-found")
	}
	if svc.calls != 2 {
		t.Fatalf("expected exactly 2 Get calls (original + confirming re-GET), got %d", svc.calls)
	}
}

// alwaysNotFoundProfileService always answers Get with err, counting calls
// so a test can assert Confirm's re-GET actually ran.
type alwaysNotFoundProfileService struct {
	err   error
	calls int
}

func (f *alwaysNotFoundProfileService) RegisterEntry(int, string) {}
func (f *alwaysNotFoundProfileService) Get(context.Context, int) (*sdk.ProfileResult, error) {
	f.calls++
	return nil, f.err
}
func (f *alwaysNotFoundProfileService) Create(context.Context, string, interface{}) (int, error) {
	return 0, nil
}
func (f *alwaysNotFoundProfileService) Update(context.Context, int, interface{}) error { return nil }
func (f *alwaysNotFoundProfileService) Delete(context.Context, int) error              { return nil }
