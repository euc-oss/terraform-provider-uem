package macscript

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const notFoundTestScriptUUID = "0b0d6fc4-838a-e89b-d117-57f62678207d"

// TestRead_404_RemovesFromState is defense-in-depth hardening (internal-task):
// unlike uem_profile and uem_mac_application, the live UEM 26.2 API
// already answers a GET for a deleted mac script with a plain 404 today —
// but nothing in Read wired the existing IsNotFoundAPIError classifier in,
// so a deleted script's GET became a hard "Client Error" instead of
// dropping the resource from state, same symptom as the other two even
// though the wire shape here was never actually broken.
func TestRead_404_RemovesFromState(t *testing.T) {
	t.Parallel()

	svc := &mockScriptService{fetchErr: &sdk.APIError{StatusCode: http.StatusNotFound, Message: "not found"}}
	r := newResourceWithMockService(svc)

	state := emptyResourceState(t)
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}
	resp.Diagnostics.Append(resp.State.SetAttribute(context.Background(), path.Root("id"), notFoundTestScriptUUID)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to seed state id: %v", resp.Diagnostics)
	}
	req.State = resp.State

	r.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a 404 on read, got: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed (Raw.IsNull()) after a 404 response")
	}
}

// TestRead_GenericAPIError_StillErrors keeps a non-404 failure a hard
// error, so the new classifier call doesn't accidentally widen.
func TestRead_GenericAPIError_StillErrors(t *testing.T) {
	t.Parallel()

	svc := &mockScriptService{fetchErr: errors.New("boom")}
	r := newResourceWithMockService(svc)

	state := emptyResourceState(t)
	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}
	resp.Diagnostics.Append(resp.State.SetAttribute(context.Background(), path.Root("id"), notFoundTestScriptUUID)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("failed to seed state id: %v", resp.Diagnostics)
	}
	req.State = resp.State

	r.Read(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a generic fetch error to still surface as a hard error")
	}
}
