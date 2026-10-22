package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// --- Read: UEM's 400 "Invalid Profile" shape (finding: 8rm) ---

// TestProfileResourceRead_InvalidProfile400_RemovesFromState proves the
// fix for internal-task: UEM 26.2 answers a GET for a deleted (or never
// existing) profile with HTTP 400 "Invalid Profile <id>." rather than a
// 404. Before the fix, this fell through to a hard "Client Error" and
// `terraform plan`/`refresh` would error instead of dropping the resource.
func TestProfileResourceRead_InvalidProfile400_RemovesFromState(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "Invalid Profile 12345.",
			"errorCode": 400,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(12345, sdk.PlatformAndroid))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Gone Profile"),
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
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error for the Invalid Profile 400, got: %v", msgs)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed (Raw.IsNull()) after an Invalid Profile 400")
	}
}

// TestProfileResourceRead_DifferentBadRequest_StillErrors proves the
// matcher does NOT widen to "any 400 is not-found": a validation-style 400
// with an unrelated message must still surface as a real error.
func TestProfileResourceRead_DifferentBadRequest_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "Name is a required field.",
			"errorCode": 400,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(12345, sdk.PlatformAndroid))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Some Profile"),
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
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a different 400 (validation error) to still surface as an error, not be classified as not-found")
	}
}

// --- Delete: UEM's 400 "Invalid Profile" shape (finding: 8rm) ---

// TestProfileResourceDelete_InvalidProfile400_NoOp proves Delete of an
// already-gone profile (server answers 400 "Invalid Profile") succeeds as
// a no-op, mirroring the existing 404 handling, so `terraform destroy` on
// an out-of-band-deleted profile is idempotent.
func TestProfileResourceDelete_InvalidProfile400_NoOp(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Invalid Profile 99999.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Already Gone"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error deleting an already-gone (400 Invalid Profile) profile, got: %v", msgs)
	}
}

// TestProfileResourceDelete_DifferentBadRequest_StillErrors proves Delete's
// widened matcher does not swallow an unrelated 400 (e.g. a real
// validation failure on the delete call).
func TestProfileResourceDelete_DifferentBadRequest_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Profile is currently assigned and cannot be deleted.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Assigned Profile"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a different 400 (validation error) on delete to still surface as an error")
	}
}

// --- Delete: UEM's v1 400 "Profile not found..." shape (pin d81aecf63168) ---

// TestProfileResourceDelete_V1ProfileNotFound400_NoOp proves Delete of an
// already-gone profile succeeds as a no-op when the v1 DELETE answers 400
// "Profile not found or User does not have access to the Profile.".
func TestProfileResourceDelete_V1ProfileNotFound400_NoOp(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Profile not found or User does not have access to the Profile.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Already Gone"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error deleting an already-gone (v1 400 Profile not found) profile, got: %v", msgs)
	}
}

// TestProfileResourceDelete_V1ProfileNotFoundNearMiss400_StillErrors
// proves the v1 matcher is an exact full-message match: a 400 that merely
// starts with the v1 not-found message is still an error.
func TestProfileResourceDelete_V1ProfileNotFoundNearMiss400_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Profile not found or User does not have access to the Profile. Profile is locked.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Assigned Profile"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a near-miss 400 (extra text after the v1 not-found message) on delete to still surface as an error")
	}
}

// --- Near-miss regressions for the unanchored-prefix bug (finding 1, 8rm gate round 2) ---
//
// isProfileGoneAPIError used to match on strings.HasPrefix(msg, "Invalid
// Profile") alone. That's an unanchored prefix: it also matches an
// unrelated validation error that merely starts with the same words (e.g.
// "Invalid ProfileScope value.") and it also matches a genuine "gone"
// message for a DIFFERENT profile id than the one actually being
// read/deleted. Both cases below must still be hard errors under the
// fixed, id-pinned exact match.

// TestProfileResourceRead_InvalidProfileScopePrefix400_StillErrors proves
// a 400 that merely starts with "Invalid Profile" (but is really an
// "Invalid ProfileScope" validation error) is NOT classified as not-found.
func TestProfileResourceRead_InvalidProfileScopePrefix400_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "Invalid ProfileScope value.",
			"errorCode": 400,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(12345, sdk.PlatformAndroid))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Some Profile"),
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
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Invalid ProfileScope value.' to surface as a hard error, not be classified as not-found")
	}
}

// TestProfileResourceRead_InvalidProfileWrongID400_StillErrors proves a
// 400 "Invalid Profile <id>." for a DIFFERENT id than the one actually
// being read is NOT classified as not-found — this proves the fix
// actually checks the id, not just the "Invalid Profile" shape.
func TestProfileResourceRead_InvalidProfileWrongID400_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message":   "Invalid Profile 123.",
			"errorCode": 400,
		})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(456, sdk.PlatformAndroid))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("456"),
		"name":                 stringVal("Some Profile"),
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
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Invalid Profile 123.' while reading id 456 to surface as a hard error, not be classified as not-found")
	}
}

// TestProfileResourceDelete_InvalidProfileScopePrefix400_StillErrors
// proves Delete does not swallow a 400 that merely starts with "Invalid
// Profile" (but is really an "Invalid ProfileScope" validation error).
func TestProfileResourceDelete_InvalidProfileScopePrefix400_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Invalid ProfileScope value.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Some Profile"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Invalid ProfileScope value.' on delete to surface as a hard error, not a no-op")
	}
}

// TestProfileResourceDelete_InvalidProfileWrongID400_StillErrors proves
// Delete does not swallow a 400 "Invalid Profile <id>." for a DIFFERENT
// id than the one actually being deleted — this proves the fix actually
// checks the id, not just the "Invalid Profile" shape.
func TestProfileResourceDelete_InvalidProfileWrongID400_StillErrors(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message":   "Invalid Profile 123.",
				"errorCode": 400,
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("456"),
		"name":                 stringVal("Some Profile"),
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
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 'Invalid Profile 123.' while deleting id 456 to surface as a hard error, not a no-op")
	}
}
