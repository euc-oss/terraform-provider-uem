package profile

// internal-task #1: proves updateAppleOsXProfile no longer uploads certificate
// blobs to UEM before the Get-based fail-closed checks and the zf1b
// masked-secret guard have run. Before the fix, PrepareCredentialsForPayload
// ran first, so a Get failure or a masked-secret refusal still performed the
// (network-mutating) certificate upload. These tests drive the real
// ProfileResource.Update path with:
//
//   - a fake profileServiceAPI (Get/Update) so Get can be made to fail or
//     return a masked-secret entity without any HTTP traffic, and
//   - a real *sdk.Client pointed at an httptest server that counts POSTs to
//     /api/mdm/profiles/uploadcertificate, since PrepareCredentialsForPayload
//     goes through the SDK client directly (UploadCertificatesTyped ->
//     sdk.ProfilesV1Service.UploadCertificate), not through profileServiceAPI.
//
// (a) and (b) assert zero uploads on the abort paths. (c) proves that a
// normal successful update DOES upload (exactly once) before svc.Update, and
// that the entity handed to svc.Update carries the CertificateID the fake
// upload server returned.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// uploadResponseCertID is the certificate ID every uploadCountingHandler
// answers with.
const uploadResponseCertID = 555

// uploadCountingHandler returns an http.HandlerFunc that answers
// POST /api/mdm/profiles/uploadcertificate with uploadResponseCertID and
// increments *count for every such POST it sees. Any other request fails the
// test — this resource's tests should never hit any other endpoint.
func uploadCountingHandler(t *testing.T, count *int64) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/mdm/profiles/uploadcertificate" {
			t.Errorf("unexpected HTTP call to %s %s; only the certificate upload endpoint should ever be hit in this test", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt64(count, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]int64{"Value": uploadResponseCertID}); err != nil {
			t.Fatalf("failed to encode upload response: %v", err)
		}
	}
}

// newUploadOrderTestResource builds a ProfileResource wired to the given
// fake profileServiceAPI (for Get/Update) and a real SDK client pointed at
// an httptest server driven by handler (for the certificate upload POST).
func newUploadOrderTestResource(t *testing.T, fake *fakeProfileService, handler http.HandlerFunc) *ProfileResource {
	t.Helper()
	c, server := createTestClient(t, handler)
	t.Cleanup(server.Close)
	return &ProfileResource{
		client: c,
		newProfileService: func(ctx context.Context, _ *sdk.Client) (profileServiceAPI, error) {
			return fake, nil
		},
	}
}

// runUploadOrderUpdate drives ProfileResource.Update with the given plan
// values and fake service/upload handler, returning the response for
// assertion. Mirrors runRMWUpdate in resource_update_rmw_test.go, but wires
// a real (counting) upload server instead of one that fails any HTTP call.
func runUploadOrderUpdate(t *testing.T, fake *fakeProfileService, handler http.HandlerFunc, planValues map[string]tftypes.Value) *resource.UpdateResponse {
	t.Helper()
	res := newUploadOrderTestResource(t, fake, handler)
	ctx := context.Background()

	plan := createResourcePlan(t, planValues)
	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(ctx, req, resp)
	return resp
}

// credentialsListVal builds a tftypes.Value for credentials_list from a
// slice of field-value maps, filling any attribute not provided with its
// typed null default. Mirrors the shape of networkListVal in
// testutils_test.go, which has no equivalent helper for credentials_list.
func credentialsListVal(items []map[string]tftypes.Value) tftypes.Value {
	itemType := credentialsListItemType()
	listType := tftypes.List{ElementType: itemType}

	defaults := map[string]tftypes.Value{
		"credential_source":                tftypes.NewValue(tftypes.String, nil),
		"credential_name":                  tftypes.NewValue(tftypes.String, nil),
		"certificate_payload":              tftypes.NewValue(tftypes.String, nil),
		"certificate_password":             tftypes.NewValue(tftypes.String, nil),
		"certificate_id":                   tftypes.NewValue(tftypes.Number, nil),
		"certificate_authority":            tftypes.NewValue(tftypes.Number, nil),
		"certificate_template":             tftypes.NewValue(tftypes.Number, nil),
		"allow_access_to_all_applications": tftypes.NewValue(tftypes.Bool, nil),
		"key_is_extractable":               tftypes.NewValue(tftypes.Bool, nil),
	}

	vals := make([]tftypes.Value, len(items))
	for i, item := range items {
		merged := make(map[string]tftypes.Value, len(defaults))
		for k, v := range defaults {
			merged[k] = v
		}
		for k, v := range item {
			merged[k] = v
		}
		vals[i] = tftypes.NewValue(itemType, merged)
	}
	return tftypes.NewValue(listType, vals)
}

// planWithUploadCredential returns Update plan values for the macOS
// platform with a network entry referencing an "Upload"-sourced credential
// with no CertificateID yet — the shape that makes
// profilestate.PrepareCredentialsForPayload actually attempt an upload.
func planWithUploadCredential() map[string]tftypes.Value {
	return mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": stringVal("999"),
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"network_interface":      stringVal("WiFi"),
				"service_set_identifier": stringVal("corp-ssid"),
				"security_type":          stringVal("WPA2 Enterprise"),
				"identity_certificate":   stringVal("cert1"),
			},
		}),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source":    stringVal("Upload"),
				"credential_name":      stringVal("cert1"),
				"certificate_payload":  stringVal("base64-cert-blob"),
				"certificate_password": stringVal("cert-pass"),
			},
		}),
	})
}

func TestUpdateAppleOsX_UploadOrder_GetFails_NoUpload(t *testing.T) {
	t.Parallel()

	var uploadCount int64
	fake := &fakeProfileService{getErr: errGetFailed}

	resp := runUploadOrderUpdate(t, fake, uploadCountingHandler(t, &uploadCount), planWithUploadCredential())

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when Get fails")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected svc.Update not to be called when Get fails, got %d calls", fake.updateCalls)
	}
	if got := atomic.LoadInt64(&uploadCount); got != 0 {
		t.Fatalf("expected 0 certificate uploads when Get fails, got %d", got)
	}
}

func TestUpdateAppleOsX_UploadOrder_MaskedGuardRefuses_NoUpload(t *testing.T) {
	t.Parallel()

	var uploadCount int64
	live := liveAppleOsXEntity()
	live.EmailList = []sdk.AppleOsXEmailPayloadEntityV2{{IncomingPassword: profileplatform.UEMMaskedSecret}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}

	resp := runUploadOrderUpdate(t, fake, uploadCountingHandler(t, &uploadCount), planWithUploadCredential())

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error diagnostic when the masked-secret guard refuses")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected svc.Update not to be called when the masked-secret guard refuses, got %d calls", fake.updateCalls)
	}
	if got := atomic.LoadInt64(&uploadCount); got != 0 {
		t.Fatalf("expected 0 certificate uploads when the masked-secret guard refuses, got %d", got)
	}
}

func TestUpdateAppleOsX_UploadOrder_Success_UploadsBeforeUpdate(t *testing.T) {
	t.Parallel()

	var uploadCount int64
	const wantCertID = 555
	live := liveAppleOsXEntity() // no masked unmodeled secrets.
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}

	resp := runUploadOrderUpdate(t, fake, uploadCountingHandler(t, &uploadCount), planWithUploadCredential())

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 svc.Update call, got %d", fake.updateCalls)
	}
	if got := atomic.LoadInt64(&uploadCount); got != 1 {
		t.Fatalf("expected exactly 1 certificate upload, got %d", got)
	}

	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if len(sent.CredentialsList) != 1 {
		t.Fatalf("expected 1 credential in the sent entity, got %d", len(sent.CredentialsList))
	}
	// This is the load-bearing assertion: the entity handed to svc.Update
	// already carries the CertificateID returned by the (single) upload
	// call, which is only possible if the upload ran to completion before
	// BuildAppleOsXCreateEntity/svc.Update — i.e. the upload happened
	// before, not after or interleaved with, the write to UEM.
	if sent.CredentialsList[0].CertificateID == nil || *sent.CredentialsList[0].CertificateID != int(wantCertID) {
		t.Fatalf("expected CredentialsList[0].CertificateID %d from the upload response, got %+v", wantCertID, sent.CredentialsList[0].CertificateID)
	}
}

// A refused secret-wipe update (VPN secret held in UEM, omitted by the plan)
// must stop before the certificate upload: no POST of any kind.
func TestUpdateAppleOsX_UploadOrder_SecretWipeGuardRefuses_NoUpload(t *testing.T) {
	t.Parallel()

	var uploadCount int64
	live := liveAppleOsXEntity()
	live.VpnList = []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "v", ConnectionType: "AirwatchTunnel", Server: "s", Proxy: "None", Password: profileplatform.UEMMaskedSecret}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}

	plan := mergeValues(planWithUploadCredential(), map[string]tftypes.Value{"vpn_list": vpnPlanValues(t, nil)["vpn_list"]})
	resp := runUploadOrderUpdate(t, fake, uploadCountingHandler(t, &uploadCount), plan)

	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), `vpn_list["v"].password`) {
		t.Fatalf("expected the secret-wipe refusal naming the secret, got %v", resp.Diagnostics)
	}
	if got := atomic.LoadInt64(&uploadCount); got != 0 {
		t.Fatalf("expected 0 certificate uploads when the secret-wipe guard refuses, got %d", got)
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected svc.Update not to be called, got %d calls", fake.updateCalls)
	}
}
