package macapplication

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

const notFound401TestUUID = "39cc7060-44ce-ec5a-cc98-b537f961203e"

func newNotFound401TestResource(t *testing.T, err error) *macapplicationResource {
	t.Helper()
	readFake := &fakeInternalAppsV1Service{err: err}
	return &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		client:               &sdk.Client{},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return readFake
		},
	}
}

func notFound401TestState(t *testing.T, id int32) resource.ReadRequest {
	t.Helper()
	state := emptyMacApplicationState()
	var prior tf.MacApplicationResourceModel
	if diags := state.Get(context.Background(), &prior); diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	prior.ID = typesInt32(id)
	prior.UUID = typesString(notFound401TestUUID)
	prior = withTypedRecordNulls(prior)
	if diags := state.Set(context.Background(), &prior); diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	return resource.ReadRequest{State: state}
}

// TestRead_401ErrorCode7000_RemovesFromStateWithWarning proves the fix for
// internal-task's uem_mac_application half: UEM 26.2 answers a GET for a
// deleted (or inaccessible) internal app id with HTTP 401, errorCode
// "7000", not a 404. Because this shape is genuinely ambiguous (it could
// mean "gone" or "the current credentials can't see this app" — live
// research confirmed the SAME credentials can 200 on an existing app), the
// resource must be removed from state AND a warning diagnostic must be
// present, never a silent removal and never a hard error.
func TestRead_401ErrorCode7000_RemovesFromStateWithWarning(t *testing.T) {
	t.Parallel()

	apiErr := &sdk.APIError{
		StatusCode: http.StatusUnauthorized,
		ErrorCode:  "7000",
		Message:    "Application not found or user does not have access to it.",
	}
	r := newNotFound401TestResource(t, apiErr)
	req := notFound401TestState(t, 42)

	readResp := &resource.ReadResponse{State: req.State}
	r.Read(context.Background(), req, readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("expected no hard error for 401/errorCode-7000, got: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed (Raw.IsNull()) after a 401/errorCode-7000 response")
	}

	warnings := readResp.Diagnostics.Warnings()
	if len(warnings) == 0 {
		t.Fatal("expected a warning diagnostic about the ambiguous not-found/no-access response")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w.Detail(), "access") || strings.Contains(w.Summary(), "Not Found") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning mentioning access/not-found, got: %+v", warnings)
	}
}

// TestRead_401WithoutErrorCode7000_StillErrors proves the matcher does NOT
// widen to "any 401 is not-found": a genuine auth failure (expired token,
// wrong errorCode, or no errorCode at all) must still surface as a hard
// error with no state removal and no warning-instead-of-error.
func TestRead_401WithoutErrorCode7000_StillErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *sdk.APIError
	}{
		{
			name: "no errorCode",
			err:  &sdk.APIError{StatusCode: http.StatusUnauthorized, Message: "Unauthorized."},
		},
		{
			name: "different errorCode",
			err:  &sdk.APIError{StatusCode: http.StatusUnauthorized, ErrorCode: "9999", Message: "Token expired."},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newNotFound401TestResource(t, tc.err)
			req := notFound401TestState(t, 42)

			readResp := &resource.ReadResponse{State: req.State}
			r.Read(context.Background(), req, readResp)

			if !readResp.Diagnostics.HasError() {
				t.Fatal("expected a genuine 401 (not errorCode 7000) to still surface as a hard error")
			}
			if len(readResp.Diagnostics.Warnings()) != 0 {
				t.Fatalf("expected no warning-instead-of-error, got: %+v", readResp.Diagnostics.Warnings())
			}
			if readResp.State.Raw.IsNull() {
				t.Fatal("expected state NOT to be removed on a genuine (non-7000) 401 error")
			}
		})
	}
}
